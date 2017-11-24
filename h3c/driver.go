package h3c

import (
	"fmt"
	"net"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/codegangsta/cli"
	sdk "github.com/docker/go-plugins-helpers/network"
	"github.com/vishvananda/netlink"
	"github.com/docker/docker/client"
	"github.com/docker/libnetwork/netlabel"
)

const (
	defaultRoute     = "0.0.0.0/0"
	bridgePrefix     = "h3cbr-"
	containerEthName = "eth"

	mtuOption           = "net.h3c.bridge.mtu"
	modeOption          = "net.h3c.bridge.mode"
	bridgeNameOption    = "net.h3c.bridge.name"
	bindInterfaceOption = "net.h3c.bridge.bind_interface"

	modeNAT  = "nat"
	modeFlat = "flat"

	defaultMTU  = 1500
	defaultMode = modeNAT
)

// Driver is the MACVLAN Driver
type Driver struct {
	sdk.Driver
	dClient *client.Client
	networks networkTable
	nameserver string
	nlh *netlink.Handle
	sync.Mutex
}

// NewDriver creates a new h3c Driver
func NewDriver(version string, ctx *cli.Context) (*Driver, error) {
	docker, err := client.NewEnvClient()
	if err != nil {
		return nil, fmt.Errorf("could not connect to docker: %s", err)
	}

	d := &Driver{
		networks: networkTable{},
		dClient: docker,
	}
	return d, nil
}

// GetCapabilities tells libnetwork this driver is local scope
func (d *Driver) GetCapabilities() (*sdk.CapabilitiesResponse, error) {
	scope := &sdk.CapabilitiesResponse{Scope: sdk.LocalScope}
	return scope, nil
}

func truncateID(id string) string {
	return id[:5]
}

func getBridgeName(r *sdk.CreateNetworkRequest) (string, error) {
	bridgeName := bridgePrefix + truncateID(r.NetworkID)
	if r.Options != nil {
		// Parse docker network -o opts
		for k, v := range r.Options {
			if k == netlabel.GenericData {
				if genericOpts, ok := v.(map[string]interface{}); ok {
					for key, val := range genericOpts {
						// Parse -o bridgeNameOption from libnetwork generic opts
						if key == bridgeNameOption {
							bridgeName = val.(string)
						}
					}
				}
			}
		}
	}
	return bridgeName, nil
}

// CreateNetwork creates a new h3c network
// bridge name should in options
func (d *Driver) CreateNetwork(r *sdk.CreateNetworkRequest) error {
	var netCidr *net.IPNet
	var netGw string
	var err error
	log.Debugf("Network Create Called: [ %+v ]", r)
	for _, v4 := range r.IPv4Data {
		netGw = v4.Gateway
		_, netCidr, err = net.ParseCIDR(v4.Pool)
		if err != nil {
			return err
		}
	}

	n := &network{
		id:        r.NetworkID,
		endpoints: endpointTable{},
		cidr:      netCidr,
		gateway:   netGw,
	}

	bName, err := getBridgeName(r)
	log.Debugf("bridgeName:%v", bName)

	// Initialize handle when needed
	d.Lock()
	if d.nlh == nil {
		d.nlh = NlHandle()
	}
	d.Unlock()

	// Create or retrieve the bridge L3 interface
	bridgeIface, err := newInterface(d.nlh, bName)
	if err != nil {
		return err
	}

	n.bridge = bridgeIface
	setupDevice(bridgeIface)
	d.addNetwork(n)
	return nil
}

// DeleteNetwork deletes a network
func (d *Driver) DeleteNetwork(r *sdk.DeleteNetworkRequest) error {
	log.Debugf("Delete network request: %+v", &r)
	n := d.network(r.NetworkID)
	log.Debugf("Delete network name: %v", n.name)

	if err := d.nlh.LinkDel(n.bridge.Link); err != nil {
		log.Warnf("Failed to remove bridge interface %s on network %s delete: %v", n.name, r.NetworkID, err)
	}

	d.deleteNetwork(r.NetworkID)
	return nil
}

// CreateEndpoint creates a new MACVLAN Endpoint
func (d *Driver) CreateEndpoint(r *sdk.CreateEndpointRequest) (*sdk.CreateEndpointResponse, error) {
	endID := r.EndpointID

	res := &sdk.CreateEndpointResponse{
	}

	log.Debugf("Create endpoint response: %+v", res)
	log.Debugf("Create endpoint %s %+v", endID, res)
	return res, nil
}

// DeleteEndpoint deletes a MACVLAN Endpoint
func (d *Driver) DeleteEndpoint(r *sdk.DeleteEndpointRequest) error {
	log.Debugf("Delete endpoint request: %+v", &r)
	//TODO: null check cidr in case driver restarted and doesn't know the network to avoid panic
	log.Debugf("Delete endpoint %s", r.EndpointID)

	return nil
}

// EndpointInfo returns informatoin about a MACVLAN endpoint
func (d *Driver) EndpointInfo(r *sdk.InfoRequest) (*sdk.InfoResponse, error) {
	log.Debugf("Endpoint info request: %+v", &r)
	res := &sdk.InfoResponse{
		Value: make(map[string]string),
	}
	return res, nil
}

// Join creates a MACVLAN interface to be moved to the container netns
func (d *Driver) Join(r *sdk.JoinRequest) (*sdk.JoinResponse, error) {
	log.Debugf("Join request: %+v", &r)

	res := &sdk.JoinResponse{
	}
	log.Debugf("Join response: %+v", res)
	log.Debugf("Join endpoint %s:%s to %s", r.NetworkID, r.EndpointID, r.SandboxKey)
	return res, nil
}

// Leave removes a MACVLAN Endpoint from a container
func (d *Driver) Leave(r *sdk.LeaveRequest) error {
	log.Debugf("Leave request: %+v", &r)
	log.Debugf("Leave %s:%s", r.NetworkID, r.EndpointID)
	return nil
}

// DiscoverNew is not used by local scoped drivers
func (d *Driver) DiscoverNew(r *sdk.DiscoveryNotification) error {
	return nil
}

// DiscoverDelete is not used by local scoped drivers
func (d *Driver) DiscoverDelete(r *sdk.DiscoveryNotification) error {
	return nil
}

// existingNetChecks checks for networks that already exist in libnetwork cache
func (d *Driver) existingNetChecks() {
	// Request all networks on the endpoint without any filters
}
