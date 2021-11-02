package handler

import (
	"context"
	"encoding/json"
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
	_, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}

	peer.Close()
	return nil
}

func (h *NodeHandler) GetRouterCapabilities(ctx context.Context, in *v1.GetRouterCapabilitiesRequest, out *v1.GetRouterCapabilitiesResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	cap := room.GetRouterRtpCapabilities(peer)
	capBytes, _ := json.Marshal(cap)
	out.RouterCapabilities = capBytes
	return nil
}

func (h *NodeHandler) Join(ctx context.Context, in *v1.JoinPeerRequest, out *v1.JoinPeerResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	var rtpCapabilities mediasoup.RtpCapabilities
	err = json.Unmarshal(in.RtpCapabilities, &rtpCapabilities)
	if err != nil {
		return err
	}
	room.SetRtpCapabilities(peer, &rtpCapabilities)
	return nil
}

func (h *NodeHandler) CreateWebRtcTransport(ctx context.Context, in *v1.CreateWebRtcTransportRequest, out *v1.CreateWebRtcTransportResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	var sctpCapabilities mediasoup.SctpCapabilities
	err = json.Unmarshal(in.SctpCapabilities, &sctpCapabilities)
	if err != nil {
		return err
	}
	res, err := room.CreateWebRtcTransport(peer, internal.CreateWebRtcTransportOption{
		ForceTcp:         in.ForceTcp,
		Producing:        in.Producing,
		Consuming:        in.Consuming,
		SctpCapabilities: &sctpCapabilities,
	})
	if err != nil {
		return err
	}
	out.TransportId = res.TransportId

	out.IceParameters, _ = json.Marshal(res.IceParameters)
	out.IceCandidates, _ = json.Marshal(res.IceCandidates)
	out.DtlsParameters, _ = json.Marshal(res.DtlsParameters)
	out.SctpParameters, _ = json.Marshal(res.SctpParameters)

	return nil
}

func (h *NodeHandler) ConnectWebRtcTransport(ctx context.Context, in *v1.ConnectWebRtcTransportRequest, out *v1.ConnectWebRtcTransportResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	var dtlsParameters mediasoup.DtlsParameters
	err = json.Unmarshal(in.DtlsParameters, &dtlsParameters)
	if err != nil {
		return err
	}
	return room.ConnectWebRtcTransport(peer, internal.ConnectWebRtcTransportOption{
		TransportId:    in.TransportId,
		DtlsParameters: &dtlsParameters,
	})
}

func (h *NodeHandler) RestartIce(ctx context.Context, in *v1.RestartIceRequest, out *v1.RestartIceResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	iceParameters, err := room.RestartICE(peer, internal.RestartICEOption{
		TransportId: in.TransportId,
	})
	if err != nil {
		return err
	}
	out.IceParameters, _ = json.Marshal(*iceParameters)
	return nil
}

func (h *NodeHandler) Produce(ctx context.Context, in *v1.ProduceRequest, out *v1.ProduceResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	var rtpParameters mediasoup.RtpParameters
	err = json.Unmarshal(in.RtpParameters, &rtpParameters)
	if err != nil {
		return err
	}
	var appData internal.H
	err = json.Unmarshal(in.AppData, &appData)
	if err != nil {
		return err
	}
	out.ProducerId, err = room.Produce(peer, internal.ProduceOption{
		TransportId:   in.TransportId,
		Kind:          mediasoup.MediaKind(in.MediaKind),
		RtpParameters: rtpParameters,
		AppData:       appData,
	})
	if err != nil {
		return err
	}
	return nil
}

func (h *NodeHandler) ProducerAction(ctx context.Context, in *v1.ProducerActionRequest, out *v1.ProducerActionResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	return room.HandleProducer(peer, in.ProducerId, in.Action)
}

func (h *NodeHandler) Consume(ctx context.Context, in *v1.ConsumeRequest, out *v1.ConsumeResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	producerPeerID, err := uuid.FromString(in.DestPeerid)
	if err != nil {
		return err
	}
	producerPeer := room.GetPeer(producerPeerID)
	if producerPeer == nil {
		return internal.ErrPeerNotFound(in.DestPeerid)
	}
	producer, ok := producerPeer.GetProducer(in.ProducerId)
	if !ok {
		return internal.ErrProducerNotFound(in.ProducerId)
	}
	res, err := room.CreateConsumer(peer, producerPeer, producer)
	if err != nil {
		return err
	}
	out.ConsumerId = res.ConsumerId
	out.ConsumerType = string(res.ConsumerType)
	out.MediaKind = string(res.MediaKind)
	out.ProducerPaused = res.ProducerPaused
	out.RtpParameters, _ = json.Marshal(res.RtpParameters)
	out.AppData, _ = json.Marshal(res.AppData)

	return nil
}

func (h *NodeHandler) ConsumerAction(ctx context.Context, in *v1.ConsumerActionRequest, out *v1.ConsumerActionResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	out.Data, err = room.HandleConsumer(peer, in.ConsumerId, in.Action)
	if err != nil {
		return err
	}
	return nil
}

func (h *NodeHandler) GetStats(ctx context.Context, in *v1.GetStatsRequest, out *v1.GetStatsResponse) error {
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	out.Stats, err = room.GetStats(peer, in.Id, in.Type)
	if err != nil {
		return err
	}
	return nil
}

func getRoom(id uuid.UUID) (*internal.Room, error) {
	value, ok := Rooms.Load(id)
	if !ok {
		return nil, internal.ErrRoomNotExist
	}
	return value.(*internal.Room), nil
}

func getOrCreateRoom(id uuid.UUID, name string, secret int32, core_publisher, internal_publisher micro.Event) (*internal.Room, error) {
	room, err := getRoom(id)
	if err == nil {
		if !room.ValidSecret(secret) {
			return nil, internal.ErrInvalidSecret
		}
		return room, nil
	}

	room, err = internal.NewRoom(mediasoupWorker, id, name, secret, core_publisher, internal_publisher)
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

func GetPeerAndRoom(peerId, roomId string) (room *internal.Room, peer *internal.Peer, err error) {
	roomUUID, err := uuid.FromString(roomId)
	if err != nil {
		return
	}
	peerUUID, err := uuid.FromString(peerId)
	if err != nil {
		return
	}
	room, err = getRoom(roomUUID)
	if err != nil {
		return
	}
	peer = room.GetPeer(peerUUID)
	if peer == nil {
		err = internal.ErrPeerNotFound(peerId)
		return
	}
	return
}
