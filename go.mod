module github.com/itzmanish/pratibimb-go

go 1.16

replace (
	github.com/coreos/etcd => github.com/ozonru/etcd v3.3.20-grpc1.27-origmodule+incompatible
	google.golang.org/grpc => google.golang.org/grpc v1.27.0
)

require (
	github.com/golang/protobuf v1.5.2
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/consul/api v1.11.0
	github.com/itzmanish/go-micro-plugins/registry/consul/v2 v2.10.0
	github.com/itzmanish/go-micro/v2 v2.10.1
	github.com/jiyeyuran/go-eventemitter v1.4.0
	github.com/jiyeyuran/mediasoup-go v1.8.1
	github.com/jkawamoto/structpbconv v0.0.0-20191225002205-7d1a3174c262
	github.com/joho/godotenv v1.4.0
	github.com/satori/go.uuid v1.2.0
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
)
