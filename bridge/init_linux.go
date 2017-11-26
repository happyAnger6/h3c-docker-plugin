package bridge

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

var (
	initNs   netns.NsHandle
	initNl   *netlink.Handle
	initOnce sync.Once
	// NetlinkSocketsTimeout represents the default timeout duration for the sockets
	NetlinkSocketsTimeout = 3 * time.Second
)

// Init initializes a new network namespace
func Init() {
	var err error
	initNs, err = netns.Get()
	if err != nil {
		logrus.Errorf("could not get initial namespace: %v", err)
	}
	initNl, err = netlink.NewHandle()
	if err != nil {
		logrus.Errorf("could not create netlink handle on initial namespace: %v", err)
	}
	err = initNl.SetSocketTimeout(NetlinkSocketsTimeout)
	if err != nil {
		logrus.Warnf("Failed to set the timeout on the default netlink handle sockets: %v", err)
	}
}

// NlHandle returns the netlink handler
func NlHandle() *netlink.Handle {
	initOnce.Do(Init)
	return initNl
}