# syntax=docker/dockerfile:1

##
## Build
##
FROM golang:1.16-buster AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-w' -o pratibimb

##
## Deploy
##
FROM debian

WORKDIR /

COPY --from=build /app/pratibimb .
COPY mediasoup ./mediasoup
COPY .env .env

ENTRYPOINT ["/pratibimb"]
