package h3c

import (
	"net"

	log "github.com/sirupsen/logrus"
)

// Generate a mac addr
func makeMac(ip net.IP) string {
	hw := make(net.HardwareAddr, 6)
	hw[0] = 0x7a
	hw[1] = 0x42
	copy(hw[2:], []byte(ip.To4()))
	return hw.String()
}

// Check if a netlink interface exists in the default namespace
func validateHostIface(ifaceStr string) bool {
	_, err := net.InterfaceByName(ifaceStr)
	if err != nil {
		log.Debugf("The requested interface to delete [ %s ] was not found on the host: %s", ifaceStr, err)
		return false
	}
	return true
}