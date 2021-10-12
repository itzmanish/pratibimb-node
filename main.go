package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/itzmanish/pratibimb-go/handler"
	"github.com/itzmanish/pratibimb-go/internal"
	v1 "github.com/itzmanish/pratibimb-go/proto/gen/node/v1"
	"github.com/jiyeyuran/mediasoup-go"
	"github.com/joho/godotenv"

	"github.com/itzmanish/go-micro/v2"
	"github.com/itzmanish/go-micro/v2/api/server"
	httpapi "github.com/itzmanish/go-micro/v2/api/server/http"
	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/go-micro/v2/registry"
)

const (
	serviceName    = "com.itzmanish.pratibimb.node.v1"
	serviceVersion = "1.0.0"
	serviceAddress = ":0"
)

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

	router := mux.NewRouter()

	api := httpapi.NewServer(serviceAddress)

	log.Debugf("webrtc announce ip: %v", internal.DefaultConfig.Mediasoup.WebRtcTransportOptions.ListenIps)

	// register call handler
	wshandler := handler.NewWsHandler(internal.DefaultConfig, service.Options().Client)

	router.HandleFunc("/", handler.IndexHandler)
	// router.Handle("/v1/room", middleware.AuthWrapper()(handler.CreateRoom))
	router.Handle("/v1/ws", wshandler)

	nodeHandler := handler.NewNodeService("")

	api.Init(server.EnableCORS(true))

	service.Init(micro.AfterStart(func() error {
		log.Debug("After start executing")
		srvs, err := registry.GetService(serviceName)
		if err != nil {
			return err
		}

		for _, srv := range srvs {
			for _, node := range srv.Nodes {
				if strings.Split(node.Address, ":")[1] == strings.Split(service.Server().Options().Address, ":")[3] {
					nodeHandler.Init(node.Id)
				}
			}
		}
		return nil
	}))

	if err := v1.RegisterNodeServiceHandler(service.Server(), nodeHandler); err != nil {
		log.Fatal(err)
	}

	api.Handle("/", router)

	// Start API
	if err := api.Start(); err != nil {
		log.Fatal(err)
	}

	// Run server
	if err := service.Run(); err != nil {
		log.Fatal(err)
	}

	// Stop API
	if err := api.Stop(); err != nil {
		log.Fatal(err)
	}
}
