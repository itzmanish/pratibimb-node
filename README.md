# PratibimbGo Service

This is the PratibimbGo service

Generated with

```
micro new --namespace=com.itzmanish.pratibimb --type=web pratibimb-go
```

## Getting Started

- [Configuration](#configuration)
- [Dependencies](#dependencies)
- [Usage](#usage)

## Configuration

- FQDN: com.itzmanish.pratibimb.web.pratibimb-go
- Type: web
- Alias: pratibimb-go

## Dependencies

Micro services depend on service discovery. The default is multicast DNS, a zeroconf system.

In the event you need a resilient multi-host setup we recommend etcd.

```
# install etcd
brew install etcd

# run etcd
etcd
```

## Usage

A Makefile is included for convenience

Build the binary

```
make build
```

Run the service

```
./pratibimb-go-web
```

Build a docker image

```
make docker
```
