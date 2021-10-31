package handler

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/itzmanish/go-micro/v2"
	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/pratibimb-node/internal"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
	"github.com/jiyeyuran/mediasoup-go"
	uuid "github.com/satori/go.uuid"
)

type NodeHandler struct {
	sync.RWMutex
	NodeID        string
	config        internal.Config
	coreEvent     micro.Event
	internalEvent micro.Event
}

var Rooms sync.Map
var mediasoupWorker []*mediasoup.Worker

func NewNodeServiceHandler(config internal.Config, ev, iev micro.Event) *NodeHandler {
	workers := []*mediasoup.Worker{}
	for i := 0; i < config.Mediasoup.NumWorkers/4; i++ {
		worker, err := mediasoup.NewWorker(
			mediasoup.WithLogLevel(config.Mediasoup.WorkerSettings.LogLevel),
			mediasoup.WithLogTags(config.Mediasoup.WorkerSettings.LogTags),
			mediasoup.WithRtcMinPort(config.Mediasoup.WorkerSettings.RtcMinPort),
			mediasoup.WithRtcMaxPort(config.Mediasoup.WorkerSettings.RtcMaxPort),
		)
		if err != nil {
			panic(err)
		}
		worker.On("died", func(err error) {
			log.Error("[Error: %v] exiting in 2 second ...", err)
			time.AfterFunc(2*time.Second, func() {
				os.Exit(1)
			})
		})
		go func() {
			ticker := time.NewTicker(120 * time.Second)
			for range ticker.C {
				usage, err := worker.GetResourceUsage()
				if err != nil {
					log.Error(err, "pid", worker.Pid(), "mediasoup Worker resource usage")
					continue
				}
				log.Debug("pid", worker.Pid(), "usage", usage, "mediasoup Worker resource usage")
			}
		}()
		workers = append(workers, worker)
	}

	mediasoupWorker = workers
	return &NodeHandler{
		config:        config,
		coreEvent:     ev,
		internalEvent: iev,
	}
}

func (service *NodeHandler) Health(ctx context.Context, in *v1.HealthRequest, out *v1.HealthResponse) error {
	log.Debug("Health request called")
	out.Status = "ok"
	out.Message = "service is up."
	return nil
}

func (h *NodeHandler) CreatePeer(ctx context.Context, in *v1.CreatePeerRequest, out *v1.CreatePeerResponse) error {
	ruuid, err := uuid.FromString(in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	puuid, err := uuid.FromString(in.BaseInfo.PeerId)
	if err != nil {
		return err
	}
	room, err := getOrCreateRoom(ruuid, in.RoomName, in.Secret, h.coreEvent, h.internalEvent)
	if err != nil {
		return err
	}

	peer := room.GetPeer(puuid)
	var returning bool
	if peer != nil {
		if room.HasPeer(puuid) {
			peer.Close()
			peer.RemoveAllListeners()
			returning = true
		}
	}
	_, err = room.CreatePeer(puuid)
	if err != nil {
		return err
	}
	room.HandlePeer(peer, returning)
	return nil
}

func (h *NodeHandler) ClosePeer(ctx context.Context, in *v1.ClosePeerRequest, out *v1.ClosePeerResponse) error {
	return nil
}

func (h *NodeHandler) GetRouterCapabilities(ctx context.Context, in *v1.GetRouterCapabilitiesRequest, out *v1.GetRouterCapabilitiesResponse) error {
	return nil
}

func (h *NodeHandler) Join(ctx context.Context, in *v1.JoinPeerRequest, out *v1.JoinPeerResponse) error {
	return nil
}

func (h *NodeHandler) CreateWebRtcTransport(ctx context.Context, in *v1.CreateWebRtcTransportRequest, out *v1.CreateWebRtcTransportResponse) error {
	return nil
}

func (h *NodeHandler) ConnectWebRtcTransport(ctx context.Context, in *v1.ConnectWebRtcTransportRequest, out *v1.ConnectWebRtcTransportResponse) error {
	return nil
}

func (h *NodeHandler) RestartIce(ctx context.Context, in *v1.RestartIceRequest, out *v1.RestartIceResponse) error {
	return nil
}

func (h *NodeHandler) Produce(ctx context.Context, in *v1.ProduceRequest, out *v1.ProduceResponse) error {
	return nil
}

func (h *NodeHandler) ProducerAction(ctx context.Context, in *v1.ProducerActionRequest, out *v1.ProducerActionResponse) error {
	return nil
}

func (h *NodeHandler) Consume(ctx context.Context, in *v1.ConsumeRequest, out *v1.ConsumeResponse) error {
	return nil
}

func (h *NodeHandler) ConsumerAction(ctx context.Context, in *v1.ConsumerActionRequest, out *v1.ConsumerActionResponse) error {
	return nil
}

func (h *NodeHandler) GetStats(ctx context.Context, in *v1.GetStatsRequest, out *v1.GetStatsResponse) error {
	return nil
}

func getOrCreateRoom(id uuid.UUID, name string, secret int32, core_publisher, internal_publisher micro.Event) (*internal.Room, error) {
	value, ok := Rooms.Load(id)
	if ok {
		return value.(*internal.Room), nil
	}
	room, err := internal.NewRoom(mediasoupWorker, id, name, secret, core_publisher, internal_publisher)
	if err != nil {
		return nil, err
	}
	room.On("close", func() {
		Rooms.Delete(room.ID)
		room.RemoveAllListeners()
	})
	Rooms.Store(room.ID, room)
	return room, nil
}
