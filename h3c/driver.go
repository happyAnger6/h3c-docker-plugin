package h3c

import (
	"fmt"
	"net"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	sdk "github.com/docker/go-plugins-helpers/network"
	"github.com/samalba/dockerclient"
	"github.com/vishvananda/netlink"
)

// Driver is the MACVLAN Driver
type Driver struct {
	sdk.Driver
	dockerer
	networks   networkTable
	nameserver string
	sync.Mutex
}

// NewDriver creates a new MACVLAN Driver
func NewDriver(version string, ctx *cli.Context) (*Driver, error) {
	docker, err := dockerclient.NewDockerClient("unix:///var/run/docker.sock", nil)
	if err != nil {
		return nil, fmt.Errorf("could not connect to docker: %s", err)
	}
	// lower bound of v4 MTU is 68-bytes per rfc791
	if ctx.Int("mtu") <= 0 {
		cliMTU = defaultMTU
	} else if ctx.Int("mtu") >= minMTU {
		cliMTU = ctx.Int("mtu")
	} else {
		log.Fatalf("The MTU value passed [ %d ] must be greater than [ %d ] bytes per rfc791", ctx.Int("mtu"), minMTU)
	}
	// Set the default mode to bridge
	if ctx.String("mode") == "" {
		macvlanMode = bridgeMode
	}
	switch ctx.String("mode") {
	case bridgeMode:
		macvlanMode = bridgeMode
		// todo: in other modes if relevant
	}
	d := &Driver{
		networks: networkTable{},
		dockerer: dockerer{
			client: docker,
		},
	}
	return d, nil
}

