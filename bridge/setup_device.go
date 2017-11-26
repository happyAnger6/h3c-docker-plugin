package bridge

import (
	"github.com/docker/docker/pkg/parsers/kernel"
	"github.com/docker/libnetwork/netutils"
	"fmt"
	"github.com/vishvananda/netlink"
	"github.com/sirupsen/logrus"
)

// SetupDevice create a new bridge interface/
func setupDevice(i *bridgeInterface) error {
	var setMac bool

	// Set the bridgeInterface netlink.Bridge.
	i.Link = &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: i.BridgeName,
		},
	}

	// Only set the bridge's MAC address if the kernel version is > 3.3, as it
	// was not supported before that.
	kv, err := kernel.GetKernelVersion()
	if err != nil {
		logrus.Errorf("Failed to check kernel versions: %v. Will not assign a MAC address to the bridge interface", err)
	} else {
		setMac = kv.Kernel > 3 || (kv.Kernel == 3 && kv.Major >= 3)
	}

	if err = i.nlh.LinkAdd(i.Link); err != nil {
		logrus.Debugf("Failed to create bridge %s via netlink. Trying ioctl", i.BridgeName)
		return ioctlCreateBridge(i.BridgeName, setMac)
	}

	if setMac {
		hwAddr := netutils.GenerateRandomMAC()
		if err = i.nlh.LinkSetHardwareAddr(i.Link, hwAddr); err != nil {
			return fmt.Errorf("failed to set bridge mac-address %s : %s", hwAddr, err.Error())
		}
		logrus.Debugf("Setting bridge mac address to %s", hwAddr)
	}
	return err
}
