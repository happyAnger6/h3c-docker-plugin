package h3c

import (
	"net"

	"github.com/vishvananda/netlink"
	"fmt"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultBridgeName is the default name for the bridge interface managed
	// by the driver when unspecified by the caller.
	DefaultBridgeName = "docker0"
)
// Interface models the bridge network device.
type bridgeInterface struct {
	BridgeName  string
	Link        netlink.Link
	bridgeIPv4  *net.IPNet
	bridgeIPv6  *net.IPNet
	gatewayIPv4 net.IP
	gatewayIPv6 net.IP
	nlh         *netlink.Handle
}

// newInterface creates a new bridge interface structure. It attempts to find
// an already existing device identified by the configuration BridgeName field,
// or the default bridge name when unspecified, but doesn't attempt to create
// one when missing
func newInterface(nlh *netlink.Handle, bridgeName string) (*bridgeInterface, error) {
	var err error
	i := &bridgeInterface{
		BridgeName:bridgeName,
		nlh: nlh}

	// Initialize the bridge name to the default if unspecified.
	if bridgeName == "" {
		i.BridgeName = DefaultBridgeName
	}

	// Attempt to find an existing bridge named with the specified name.
	i.Link, err = nlh.LinkByName(i.BridgeName)
	if err != nil {
		logrus.Debugf("Did not find any interface with name %s: %v", i.BridgeName, err)
	} else if _, ok := i.Link.(*netlink.Bridge); !ok {
		return nil, fmt.Errorf("existing interface %s is not a bridge", i.Link.Attrs().Name)
	}
	return i, nil
}
