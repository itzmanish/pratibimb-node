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
		// go func() {
		// 	ticker := time.NewTicker(120 * time.Second)
		// 	for range ticker.C {
		// 		usage, err := worker.GetResourceUsage()
		// 		if err != nil {
		// 			log.Error(err, "pid", worker.Pid(), "mediasoup Worker resource usage")
		// 			continue
		// 		}
		// 		log.Debug("pid", worker.Pid(), "usage", usage, "mediasoup Worker resource usage")
		// 	}
		// }()
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
	log.Debug("CreatePeer req: ", in)
	ruuid, err := uuid.FromString(in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	puuid, err := uuid.FromString(in.BaseInfo.PeerId)
	if err != nil {
		return err
	}
	room, err := getOrCreateRoom(ruuid, in.RoomName, h.coreEvent, h.internalEvent)
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
	peer, err = room.CreatePeer(puuid)
	if err != nil {
		return err
	}
	room.HandlePeer(peer, returning)
	out.Router = peer.GetRouterID()
	return nil
}

func (h *NodeHandler) ClosePeer(ctx context.Context, in *v1.ClosePeerRequest, out *v1.ClosePeerResponse) error {
	log.Debug("ClosePeer req: ", in)
	_, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}

	peer.Close()
	return nil
}

func (h *NodeHandler) GetRouterCapabilities(ctx context.Context, in *v1.GetRouterCapabilitiesRequest, out *v1.GetRouterCapabilitiesResponse) error {
	log.Debug("GetRouterCapabilities req: ", in)
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
	log.Debug("Join req: ", in)
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
	log.Debug("CreateWebRTCTransport req: ", in)
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
	log.Debug("ConnectWebRTCTransport req: ", in)
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
	log.Debug("RestartIce req: ", in)
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

func (h *NodeHandler) ProducerCanConsume(ctx context.Context, in *v1.ProducerCanConsumeRequest, out *v1.ProducerCanConsumeResponse) error {
	log.Debug("ProducerCanConsume req: ", in)
	_, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	var rtpCaps mediasoup.RtpCapabilities
	err = json.Unmarshal(in.RtpCapabilities, &rtpCaps)
	if err != nil {
		return err
	}
	out.CanConsume = peer.CanConsume(in.ProducerId, rtpCaps)
	return nil
}

func (h *NodeHandler) Produce(ctx context.Context, in *v1.ProduceRequest, out *v1.ProduceResponse) error {
	log.Debug("Produce req: ", in)
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
	log.Debug("ProducerAction req: ", in)
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	return room.HandleProducer(peer, in.ProducerId, in.Action)
}

func (h *NodeHandler) Consume(ctx context.Context, in *v1.ConsumeRequest, out *v1.ConsumeResponse) error {
	log.Debug("Consume req: ", in)
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
	var producerAppData internal.H
	err = json.Unmarshal(in.ProducerAppData, &producerAppData)
	if err != nil {
		return err
	}
	res, err := room.CreateConsumer(peer, in.ProducerId, mediasoup.MediaKind(in.ProducerKind), producerAppData)
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
	log.Debug("ConsumerAction req: ", in)
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	out.Score, err = room.HandleConsumer(peer, in.ConsumerId, in.Action)
	if err != nil {
		return err
	}
	return nil
}

func (h *NodeHandler) GetStats(ctx context.Context, in *v1.GetStatsRequest, out *v1.GetStatsResponse) error {
	log.Debug("GetStats req: ", in)
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

// PeerRouterHasProducer verifies that if the router associated with peer have the producer already.
func (h *NodeHandler) PeerRouterHasProducer(ctx context.Context, in *v1.PeerRouterHasProducerRequest, out *v1.PeerRouterHasProducerResponse) error {
	log.Debug("PeerRouterHasProducer() | req: ", in)
	_, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	router := peer.GetRouter()
	if router == nil {
		return internal.ErrNoRouterExists
	}
	var has bool
	for _, producer := range router.Producers() {
		if producer.Id() == in.ProducerId {
			has = true
			break
		}
	}
	out.HasProducer = has
	return nil
}

func getRoom(id uuid.UUID) (*internal.Room, error) {
	value, ok := Rooms.Load(id)
	if !ok {
		return nil, internal.ErrRoomNotExist
	}
	return value.(*internal.Room), nil
}

func getOrCreateRoom(id uuid.UUID, name string, core_publisher, internal_publisher micro.Event) (*internal.Room, error) {
	room, err := getRoom(id)
	if err == nil {
		return room, nil
	}

	room, err = internal.NewRoom(mediasoupWorker, id, name, core_publisher, internal_publisher)
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
