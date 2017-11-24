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
)

// Driver is the MACVLAN Driver
type Driver struct {
	sdk.Driver
	dClient *client.Client
	networks networkTable
	nameserver string
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

	// Parse docker network -o opts
	for k, v := range r.Options {
		log.Debugf("Options k:%v v:%v ", k, v)
		if k == "com.docker.sdk.generic" {
			if genericOpts, ok := v.(map[string]interface{}); ok {
				for key, val := range genericOpts {
					log.Debugf("Libnetwork Opts Sent: [ %s ] Value: [ %s ]", key, val)
					// Parse -o host_iface from libnetwork generic opts
					if key == "host_iface" {
						n.ifaceOpt = val.(string)
					}
				}
			}
		}
	}
	d.addNetwork(n)
	return nil
}

// DeleteNetwork deletes a network
func (d *Driver) DeleteNetwork(r *sdk.DeleteNetworkRequest) error {
	log.Debugf("Delete network request: %+v", &r)
	d.deleteNetwork(r.NetworkID)
	return nil
}

// CreateEndpoint creates a new MACVLAN Endpoint
func (d *Driver) CreateEndpoint(r *sdk.CreateEndpointRequest) (*sdk.CreateEndpointResponse, error) {
	endID := r.EndpointID
	log.Debugf("The container subnet for this context is [ %s ]", r.Interface.Address)
	// Request an IP address from libnetwork based on the cidr scope
	// TODO: Add a user defined static ip addr option in Docker v1.10
	containerAddress := r.Interface.Address
	if containerAddress == "" {
		return nil, fmt.Errorf("Unable to obtain an IP address from libnetwork default ipam")
	}
	// generate a mac address for the pending container
	mac := makeMac(net.ParseIP(containerAddress))

	log.Infof("Allocated container IP: [ %s ]", containerAddress)
	// IP addrs comes from libnetwork ipam via user 'docker network' parameters

	res := &sdk.CreateEndpointResponse{
		Interface: &sdk.EndpointInterface{
			Address:    containerAddress,
			MacAddress: mac,
		},
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

	containerLink := r.EndpointID[:5]
	// Check the interface to delete exists to avoid a netlink panic
	if ok := validateHostIface(containerLink); !ok {
		return fmt.Errorf("The requested interface to delete [ %s ] was not found on the host.", containerLink)
	}
	// Get the link handle
	link, err := netlink.LinkByName(containerLink)
	if err != nil {
		return fmt.Errorf("Error looking up link [ %s ] object: [ %v ] error: [ %s ]", link.Attrs().Name, link, err)
	}
	log.Infof("Deleting the unused macvlan link [ %s ] from the removed container", link.Attrs().Name)
	if err := netlink.LinkDel(link); err != nil {
		log.Errorf("unable to delete the Macvlan link [ %s ] on leave: %s", link.Attrs().Name, err)
	}
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