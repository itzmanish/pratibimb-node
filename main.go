package main

import (
	"fmt"
	"os"

	"github.com/itzmanish/pratibimb-node/handler"
	"github.com/itzmanish/pratibimb-node/internal"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
	"github.com/itzmanish/pratibimb-node/subscriber"
	"github.com/itzmanish/pratibimb-node/utils"
	"github.com/jiyeyuran/mediasoup-go"
	"github.com/joho/godotenv"

	"github.com/itzmanish/go-micro/v2"
	log "github.com/itzmanish/go-micro/v2/logger"
)

const (
	serviceName    = "github.itzmanish.service.pratibimb.node.v1"
	serviceVersion = "1.0.0"

	coreServiceName = "github.itzmanish.service.pratibimb.v1"
)

var serviceAddress = utils.GetFreePort()
var publicAddressWithPort = utils.GetPublicAddressWithPort(serviceAddress)

func init() {
	// loads values from .env into the system
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found")
	}
	if err := log.DefaultLogger.Init(log.WithFields(map[string]interface{}{"service": serviceName})); err != nil {
		fmt.Println("Initializing logger with namespace failed!")
	}

	mediasoup.WorkerBin = os.Getenv("MEDIASOUP_WORKER_BIN")
}

func main() {
	service := micro.NewService(
		micro.Name(serviceName),
		micro.Version(serviceVersion),
	)

	cev := micro.NewEvent(coreServiceName, service.Client())
	iev := micro.NewEvent(serviceName, service.Client())

	nodeHandler := handler.NewNodeServiceHandler(internal.DefaultConfig, cev, iev)

	service.Init()

	if err := v1.RegisterNodeServiceHandler(service.Server(), nodeHandler); err != nil {
		log.Fatal(err)
	}

	err := micro.RegisterSubscriber(serviceName, service.Server(), subscriber.NewNodeSubscriber(publicAddressWithPort))
	if err != nil {
		log.Fatal()
	}

	// Run server
	if err := service.Run(); err != nil {
		log.Fatal(err)
	}

}
