package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/itzmanish/pratibimb-node/handler"
	"github.com/itzmanish/pratibimb-node/internal"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
	"github.com/itzmanish/pratibimb-node/utils"
	"github.com/jiyeyuran/mediasoup-go"
	"github.com/joho/godotenv"

	"github.com/itzmanish/go-micro/v2"
	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/go-micro/v2/registry"
	"github.com/itzmanish/go-micro/v2/server"
	httpServer "github.com/itzmanish/go-micro/v2/server/http"
)

const (
	wsServiceName    = "github.itzmanish.service.pratibimb.node.ws.v1"
	wsServiceVersion = "1.0.0"
	serviceName      = "github.itzmanish.service.pratibimb.node.v1"
	serviceVersion   = "1.0.0"
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

	wsService := httpServer.NewServer(server.Name(wsServiceName),
		server.Version(wsServiceVersion),
		server.Address(serviceAddress),
	)

	if os.Getenv("ENV") != "production" {
		publicAddressWithPort = utils.GetFreePortWithHost(serviceAddress)
	}

	mux := http.NewServeMux()

	log.Debugf("webrtc announce ip: %v", internal.DefaultConfig.Mediasoup.WebRtcTransportOptions.ListenIps)

	// register call handler
	wshandler := handler.NewWsHandler(internal.DefaultConfig)

	mux.HandleFunc("/", handler.IndexHandler)
	// router.Handle("/v1/room", middleware.AuthWrapper()(handler.CreateRoom))
	mux.Handle("/v1/ws", wshandler)

	nodeHandler := handler.NewNodeService()

	service.Init(
		micro.Metadata(map[string]string{"ws_address": publicAddressWithPort}),
		micro.AfterStart(func() error {
			log.Debug("After start executing")
			srvs, err := registry.GetService(serviceName)
			if err != nil {
				return err
			}

			for _, srv := range srvs {
				for _, node := range srv.Nodes {
					if strings.Split(node.Address, ":")[1] == strings.Split(service.Server().Options().Address, ":")[3] {
						nodeHandler.Init(node.Id, publicAddressWithPort)
					}
				}
			}
			return nil
		}))

	if err := v1.RegisterNodeServiceHandler(service.Server(), nodeHandler); err != nil {
		log.Fatal(err)
	}

	wsServerHandler := wsService.NewHandler(mux)
	wsService.Handle(wsServerHandler)

	// Start API
	if err := wsService.Start(); err != nil {
		log.Fatal(err)
	}

	// Run server
	if err := service.Run(); err != nil {
		log.Fatal(err)
	}

	// Stop API
	if err := wsService.Stop(); err != nil {
		log.Fatal(err)
	}
}
