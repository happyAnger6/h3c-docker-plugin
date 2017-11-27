package bridge

import (
	"net"
	"github.com/docker/libnetwork/netlabel"
	"fmt"
	"github.com/vishvananda/netlink"
	"github.com/sirupsen/logrus"
	"github.com/docker/libnetwork/netutils"
)

const (
	networkType                = "bridge"
	vethPrefix                 = "veth"
	vethLen                    = 7
	defaultContainerVethPrefix = "eth"
	maxAllocatePortAttempts    = 10
)

// ifaceCreator represents how the bridge interface was created
type ifaceCreator int8

// networkConfiguration for network specific configuration
type networkConfiguration struct {
	ID                   string
	BridgeName           string
	ContainerIfName      string
	EnableIPv6           bool
	EnableIPMasquerade   bool
	EnableICC            bool
	Mtu                  int
	DefaultBindingIP     net.IP
	DefaultBridge        bool
	ContainerIfacePrefix string
	// Internal fields set after ipam data parsing
	AddressIPv4        *net.IPNet
	AddressIPv6        *net.IPNet
	DefaultGatewayIPv4 net.IP
	DefaultGatewayIPv6 net.IP
	dbIndex            uint64
	dbExists           bool
	Internal           bool

	BridgeIfaceCreator ifaceCreator
}

// endpointConfiguration represents the user specified configuration for the sandbox endpoint
type endpointConfiguration struct {
	MacAddress net.HardwareAddr
}

type bridgeEndpoint struct {
	id              string
	nid             string
	srcName         string
	addr            string
	addrv6          string
	macAddress      string
	config          *endpointConfiguration // User specified parameters
}

func parseNetworkOptions(id string, option map[string]interface{}) (*networkConfiguration, error) {
	var (
		config = &networkConfiguration{}
	)

	// Process well-known labels next
	if val, ok := option[netlabel.EnableIPv6]; ok {
		config.EnableIPv6 = val.(bool)
	}

	if val, ok := option[netlabel.Internal]; ok {
		if internal, ok := val.(bool); ok && internal {
			config.Internal = true
		}
	}

	if config.BridgeName == "" && config.DefaultBridge == false {
		config.BridgeName = "br-" + id[:12]
	}

	config.ID = id
	return config, nil
}

func parseEndpointOptions(epOptions map[string]interface{}) (*endpointConfiguration, error) {
	if epOptions == nil {
		return nil, nil
	}

	ec := &endpointConfiguration{}

	if opt, ok := epOptions[netlabel.MacAddress]; ok {
		if mac, ok := opt.(net.HardwareAddr); ok {
			ec.MacAddress = mac
		} else {
			return nil, &ErrInvalidEndpointConfig{}
		}
	}

	return ec, nil
}

func addToBridge(nlh *netlink.Handle, ifaceName, bridgeName string) error {
	link, err := nlh.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("could not find interface %s: %v", ifaceName, err)
	}
	if err = nlh.LinkSetMaster(link,
		&netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}); err != nil {
		logrus.Debugf("Failed to add %s to bridge via netlink.Trying ioctl: %v", ifaceName, err)
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			return fmt.Errorf("could not find network interface %s: %v", ifaceName, err)
		}

		master, err := net.InterfaceByName(bridgeName)
		if err != nil {
			return fmt.Errorf("could not find bridge %s: %v", bridgeName, err)
		}

		return ioctlAddToBridge(iface, master)
	}
	return nil
}

func electMacAddress(epConfig *endpointConfiguration, ip net.IP) net.HardwareAddr {
	if epConfig != nil && epConfig.MacAddress != nil {
		return epConfig.MacAddress
	}
	return netutils.GenerateMACFromIP(ip)
}