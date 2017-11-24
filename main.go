package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/go-plugins-helpers/network"
	"h3c-docker-plugin/h3c"
)

const (
	version = "0.0.1"
	pluginName = "h3c-docker-network"
)

func main() {

	var flagDebug = cli.BoolFlag{
		Name:  "debug, d",
		Usage: "enable debugging",
	}
	app := cli.NewApp()
	app.Name = "h3c-docker-network"
	app.Usage = "h3c Docker Networking"
	app.Version = version
	app.Flags = []cli.Flag{
		flagDebug,
	}
	app.Action = Run
	app.Run(os.Args)
}

// Run initializes the driver
func Run(ctx *cli.Context) {
	if ctx.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	}
	log.SetLevel(log.DebugLevel)

	d, err := h3c.NewDriver(version, ctx)
	if err != nil {
		panic(err)
	}
	h := network.NewHandler(d)
	h.ServeUnix(pluginName, 0)
}
