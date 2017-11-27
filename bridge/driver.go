package bridge

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
	"github.com/docker/libnetwork/types"
	"github.com/docker/libnetwork/netutils"
)

const (
	defaultRoute     = "0.0.0.0/0"
	defaultChannelName   = "ibc"
	bridgePrefix     = "h3cbr-"
	containerEthName = "eth"

	mtuOption           = "net.bridge.bridge.mtu"
	modeOption          = "net.bridge.bridge.mode"
	bridgeNameOption    = "net.bridge.bridge.name"
	bindInterfaceOption = "net.bridge.bridge.bind_interface"

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

// NewDriver creates a new bridge Driver
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

func (d *Driver) getContainerIfName(r *sdk.CreateEndpointRequest) (string, error) {
	// Generate a name for what will be the sandbox side pipe interface
	containerIfName, err := netutils.GenerateIfaceName(d.nlh, vethPrefix, vethLen)
	if err != nil {
		return defaultChannelName, err
	}

	if r.Options != nil {
		// Parse docker network -o opts
		for k, v := range r.Options {
			if k == ChannelType {
				containerIfName = v.(string)
			}
		}
	}
	return containerIfName, nil
}

// CreateNetwork creates a new bridge network
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

	// Parse and validate the config. It should not be conflict with existing networks' config
	config, err := parseNetworkOptions(r.NetworkID, r.Options)
	if err != nil {
		return err
	}

	n := &network{
		id:        r.NetworkID,
		config:    config,
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
	if n == nil {
		return nil
	}

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
	netID := r.NetworkID
	eInfo := r.Interface

	// Get the network handler and make sure it exists
	d.Lock()
	network, ok := d.networks[r.NetworkID]
	d.Unlock()

	if !ok {
		return nil, types.NotFoundErrorf("network %s does not exist", netID)
	}

	// Try to convert the options to endpoint configuration
	epConfig, err := parseEndpointOptions(r.Options)
	if err != nil {
		return nil, err
	}

	// Create and add the endpoint
	network.Lock()
	endpoint := &bridgeEndpoint{id: endID, nid: netID, config: epConfig}
	network.endpoints[endID] = endpoint
	network.Unlock()

	// On failure make sure to remove the endpoint
	defer func() {
		if err != nil {
			network.Lock()
			delete(network.endpoints, endID)
			network.Unlock()
		}
	}()

	// Generate a name for what will be the host side pipe interface
	hostIfName, err := netutils.GenerateIfaceName(d.nlh, vethPrefix, vethLen)
	if err != nil {
		return nil, err
	}

	// Generate a name for what will be the sandbox side pipe interface
	containerIfName, err := d.getContainerIfName(r)
	if err != nil {
		return nil, err
	}
	log.Debugf("netID:%v endpointId:%V containerIfname: %v", netID, endID, containerIfName)

	// Generate and add the interface pipe host <-> sandbox
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostIfName, TxQLen: 0},
		PeerName:  containerIfName}
	if err = d.nlh.LinkAdd(veth); err != nil {
		return nil, types.InternalErrorf("failed to add the host (%s) <=> sandbox (%s) pair interfaces: %v", hostIfName, containerIfName, err)
	}

	// Get the host side pipe interface handler
	host, err := d.nlh.LinkByName(hostIfName)
	if err != nil {
		return nil, types.InternalErrorf("failed to find host side interface %s: %v", hostIfName, err)
	}
	defer func() {
		if err != nil {
			d.nlh.LinkDel(host)
		}
	}()

	// Get the sandbox side pipe interface handler
	sbox, err := d.nlh.LinkByName(containerIfName)
	if err != nil {
		return nil, types.InternalErrorf("failed to find sandbox side interface %s: %v", containerIfName, err)
	}
	defer func() {
		if err != nil {
			d.nlh.LinkDel(sbox)
		}
	}()

	network.Lock()
	config := network.config
	network.Unlock()

	// Add bridge inherited attributes to pipe interfaces
	if config.Mtu != 0 {
		err = d.nlh.LinkSetMTU(host, config.Mtu)
		if err != nil {
			return nil, types.InternalErrorf("failed to set MTU on host interface %s: %v", hostIfName, err)
		}
		err = d.nlh.LinkSetMTU(sbox, config.Mtu)
		if err != nil {
			return nil, types.InternalErrorf("failed to set MTU on sandbox interface %s: %v", containerIfName, err)
		}
	}

	// Attach host side pipe interface into the bridge
	if err = addToBridge(d.nlh, hostIfName, config.BridgeName); err != nil {
		return nil, fmt.Errorf("adding interface %s to bridge %s failed: %v", hostIfName, config.BridgeName, err)
	}

	// Store the sandbox side pipe interface parameters
	endpoint.srcName = containerIfName
	endpoint.macAddress = eInfo.MacAddress
	endpoint.addr = eInfo.Address
	endpoint.addrv6 = eInfo.AddressIPv6

	// Up the host interface after finishing all netlink configuration
	if err = d.nlh.LinkSetUp(host); err != nil {
		return nil, fmt.Errorf("could not set link up for host interface %s: %v", hostIfName, err)
	}

	res := &sdk.CreateEndpointResponse{
		Interface: &sdk.EndpointInterface{
			Address:    endpoint.addr,
			MacAddress: endpoint.macAddress,
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
	log.Debugf("Join endpoint %s:%s to %s options:%v", r.NetworkID, r.EndpointID, r.SandboxKey, r.Options)
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
