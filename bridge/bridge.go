package bridge

import (
	"net"
	"github.com/docker/libnetwork/netlabel"
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
	addr            *net.IPNet
	addrv6          *net.IPNet
	macAddress      net.HardwareAddr
	config          *endpointConfiguration // User specified parameters
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