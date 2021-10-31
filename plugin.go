package main

import (
	"os"

	"github.com/hashicorp/consul/api"
	"github.com/itzmanish/go-micro-plugins/registry/consul/v2"
	"github.com/itzmanish/go-micro-plugins/store/redis/v2"
	"github.com/itzmanish/go-micro/v2/config/cmd"
	"github.com/itzmanish/go-micro/v2/registry"
)

func init() {
	cmd.DefaultRegistries["consul"] = func(o ...registry.Option) registry.Registry {
		return consul.NewRegistry(consul.Config(
			&api.Config{
				Address:    os.Getenv("CONSUL_HOST"),
				Datacenter: os.Getenv("CONSUL_DATACENTER"),
				Token:      os.Getenv("CONSUL_TOKEN"),
			},
		))
	}
	cmd.DefaultStores["redis"] = redis.NewStore

}
