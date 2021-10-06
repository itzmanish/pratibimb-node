package main

import (
	"fmt"
	"os"

	"github.com/itzmanish/pratibimb-go/handler"
	"github.com/itzmanish/pratibimb-go/internal"
	"github.com/itzmanish/pratibimb-go/utils"
	"github.com/jiyeyuran/mediasoup-go"
	"github.com/joho/godotenv"

	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/go-micro/v2/registry"
	"github.com/itzmanish/go-micro/v2/web"
)

const (
	serviceName    = "com.itzmanish.pratibimb.v1"
	serviceVersion = "1.0.0"
	serviceAddress = "0.0.0.0:8443"
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
	// create new web service
	service := web.NewService(
		web.Name(serviceName),
		web.Version(serviceVersion),
		web.Address(serviceAddress),
		web.Registry(registry.DefaultRegistry),
	)
	var opts []web.Option
	if os.Getenv("SECURE_TLS") == "true" {
		opts = append(opts, web.Secure(os.Getenv("SECURE_TLS") == "true"),
			web.TLSConfig(utils.LoadTLSCredentials(os.Getenv("CERT_PATH"), os.Getenv("PRIVATE_KEY_PATH"))))

	}

	// initialise service
	if err := service.Init(opts...); err != nil {
		log.Fatal(err)
	}

	log.Debugf("webrtc announce ip: %v", internal.DefaultConfig.Mediasoup.WebRtcTransportOptions.ListenIps)

	// register call handler
	wshandler := handler.NewWsHandler(internal.DefaultConfig, service.Options().Service.Client())
	service.Handle("/", handler.NewIndexHandler())
	service.Handle("/v1/ws", wshandler)

	// run service
	if err := service.Run(); err != nil {
		log.Fatal(err)
	}
}
