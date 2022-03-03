package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/itzmanish/go-micro/v2"
	"github.com/itzmanish/go-micro/v2/errors"
	log "github.com/itzmanish/go-micro/v2/logger"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
	"github.com/itzmanish/pratibimb-node/utils"
	"github.com/jiyeyuran/go-eventemitter"
	"github.com/jiyeyuran/mediasoup-go"
	uuid "github.com/satori/go.uuid"
)

type Room struct {
	EventEmitter
	locker             sync.RWMutex
	NodeID             string
	ID                 uuid.UUID
	RoomName           string
	AudioLevelObserver mediasoup.IRtpObserver
	// Routers sis a map of router id to Router
	Routers map[string]*mediasoup.Router
	// mapPipeTransportQueue is a map of transport id to pipeTransport for unconnected pipeTransport
	mapPipeTransportQueue map[string]*mediasoup.PipeTransport
	// mapPipeTransports is a map of transport id to pipeTransport for connected pipeTransport (map[string]*mediasoup.PipeTransport)
	mapPipeTransports sync.Map
	// mapPipeProducers is a map of pipeProducer id to Producer (map[string]*mediasoup.Producer)
	mapPipeProducers sync.Map
	// mapPipeConsumers is a map of pipeConsumer id to Consumer (map[string]*mediasoup.Consumer)
	mapPipeConsumers sync.Map
	// peers is a map of peer uuid to Peer
	peers              map[uuid.UUID]*Peer
	logger             log.Logger
	closed             int32
	locked             bool
	core_publisher     micro.Event
	internal_publisher micro.Event
}

func NewRoom(mediasoupWorker []*mediasoup.Worker, roomID uuid.UUID, roomName string, core_publisher, internal_publisher micro.Event) (*Room, error) {

	log.Infof("create() [RoomId: %s]", roomID)

	routers := map[string]*mediasoup.Router{}
	for _, worker := range mediasoupWorker {
		router, err := worker.CreateRouter(
			DefaultConfig.Mediasoup.RouterOptions,
		)
		if err != nil {
			return nil, err
		}
		routers[router.Id()] = router
	}
	log.Debugf("Routers available: %v, Count: %d", routers, len(routers))

	room := &Room{
		EventEmitter:          eventemitter.NewEventEmitter(),
		ID:                    roomID,
		RoomName:              roomName,
		Routers:               routers,
		logger:                log.NewLogger(log.WithFields(map[string]interface{}{"caller": "Room"})),
		peers:                 make(map[uuid.UUID]*Peer),
		core_publisher:        core_publisher,
		internal_publisher:    internal_publisher,
		mapPipeTransportQueue: make(map[string]*mediasoup.PipeTransport),
	}

	return room, nil
}

func (r *Room) String() string {
	return "[Room: " + r.ID.String() + "RoomName: " + r.RoomName + "]"
}

func (r *Room) IsLocked() bool {
	return r.locked
}

func (r *Room) Close() {
	if r.Closed() {
		return
	}
	for _, peer := range r.Peers() {
		peer.Close()
	}
	r.locker.Lock()
	r.peers = make(map[uuid.UUID]*Peer)
	r.locker.Unlock()
	r.locker.RLock()
	for _, router := range r.Routers {
		router.Close()
	}
	r.locker.RUnlock()

	r.mapPipeProducers.Range(func(key, value interface{}) bool {
		producer := value.(*mediasoup.Producer)
		producer.Close()
		return true
	})

	r.mapPipeConsumers.Range(func(key, value interface{}) bool {
		consumer := value.(*mediasoup.Consumer)
		consumer.Close()
		return true
	})

	r.mapPipeTransports.Range(func(key, value interface{}) bool {
		transport := value.(*mediasoup.PipeTransport)
		transport.Close()
		return true
	})

	if atomic.CompareAndSwapInt32(&r.closed, 0, 1) {
		r.logger.Log(log.InfoLevel, "room closed")
	}
	r.SafeEmit("close")
}

func (r *Room) Closed() bool {
	return atomic.LoadInt32(&r.closed) > 0
}

func (r *Room) Peers() (peers []*Peer) {
	r.locker.RLock()
	defer r.locker.RUnlock()

	for _, peer := range r.peers {
		peers = append(peers, peer)
	}

	return
}

func (r *Room) CreatePeer(peerId uuid.UUID) (peer *Peer, err error) {

	r.logger.Log(log.InfoLevel, "createPeer()", "peerId", peerId)
	peer = r.GetPeer(peerId)
	if peer != nil {
		err = errors.Conflict("PEER_EXISTS", `there is already a Peer with same peerId [peerId:"%s"]`, peerId)
		delete(r.peers, peerId)
		return
	}

	peer = NewPeer(peerId, r.ID, r.RoomName, r.core_publisher)

	if r.peers == nil {
		r.peers = make(map[uuid.UUID]*Peer)
	}

	peer.On("close", func() {
		r.locker.Lock()
		defer r.locker.Unlock()
		delete(r.peers, peerId)
		peer.RemoveAllListeners()
	})

	r.locker.Lock()
	r.peers[peerId] = peer
	r.locker.Unlock()
	return
}

func (r *Room) HasPeer(peerId uuid.UUID) bool {
	r.locker.Lock()
	defer r.locker.Unlock()

	_, ok := r.peers[peerId]

	return ok
}

func (r *Room) GetPeer(peerId uuid.UUID) *Peer {
	r.locker.RLock()
	defer r.locker.RUnlock()
	p, ok := r.peers[peerId]
	if !ok {
		return nil
	}
	return p
}

// TODO: check if I need this
func (r *Room) HandlePeer(peer *Peer, returning bool) {
	r.logger.Logf(log.InfoLevel, "[peer:%v, returning:%v]", peer.GetID(), returning)

	// Returning user
	if returning {
		r.peerJoining(peer, true)
	} else {
		r.peerJoining(peer, false)
	}
}

func (r *Room) handleAudioLevelObserver() {
	r.AudioLevelObserver.On("volumes", func(volumes []mediasoup.AudioLevelObserverVolume) {
		r.logger.Logf(log.DebugLevel, "volume changed: %v", volumes)
		producer := volumes[0].Producer
		volume := volumes[0].Volume
		// todo: fix this
		r.notification(nil, "notification/activeSpeaker", H{"peerId": producer.AppData().(H)["peerId"],
			"volume": volume}, true, false)

	})
	r.AudioLevelObserver.On("silence", func() {
		r.notification(nil, "notification/activeSpeaker", H{"peerId": nil}, true, false)
	})
}

func (r *Room) LogStatus() {
	for _, router := range r.Routers {
		dump, err := router.Dump()
		if err != nil {
			r.logger.Logf(log.ErrorLevel, "LogStatus error: %v", err)
			return
		}
		r.logger.Logf(log.DebugLevel, "RoomID: %s Peers Length: %d, transports: %v", r.ID.String(), len(r.peers), dump.TransportIds)
	}

}

func (r *Room) GetID() string {
	return r.ID.String()
}

func (r *Room) CheckEmpty() bool {
	return len(r.peers) == 0
}

func (r *Room) peerJoining(peer *Peer, returning bool) {

	peer.router = r.GetLeastLoadedRouter(peer)
	if r.AudioLevelObserver == nil {
		audioLevelObserverOption := &mediasoup.AudioLevelObserverOptions{
			MaxEntries: 1,
			Threshold:  -80,
			Interval:   800,
		}
		// Create a mediasoup AudioLevelObserver on first router
		audioLevelObserver, err := peer.router.CreateAudioLevelObserver(
			func(o *mediasoup.AudioLevelObserverOptions) {
				o.MaxEntries = audioLevelObserverOption.MaxEntries
				o.Threshold = audioLevelObserverOption.Threshold
				o.Interval = audioLevelObserverOption.Interval
			},
		)
		if err == nil {
			r.AudioLevelObserver = audioLevelObserver
			r.handleAudioLevelObserver()
		} else {
			r.logger.Logf(log.ErrorLevel, "NewRoom(): CreateAudioLevelObserver | err: %v", err)
		}
	}
	r.handlePeer(peer)
}

func (r *Room) handlePeer(peer *Peer) {
	r.logger.Logf(log.DebugLevel, "handlePeer() [peer: %s]", peer.ID.String())

	peer.On("close", func() {
		r.handlePeerClose(peer)
	})

	// Peer left before we were done joining
	if peer.Closed() {
		r.handlePeerClose(peer)
	}

}

func (r *Room) handlePeerClose(peer *Peer) {
	r.logger.Logf(log.DebugLevel, "handlePeerClose() [peer: %s]", peer.ID.String())
	if r.Closed() {
		return
	}

	// If the Peer was joined, notify all Peers.
	if peer.joined {
		r.notification(peer, "notification/peerClosed",
			H{"peerId": peer.GetID()}, true, false)
	}

	var filteredPeers = make(map[uuid.UUID]*Peer)
	r.locker.Lock()
	for _, p := range r.peers {
		if !uuid.Equal(p.ID, peer.ID) {
			filteredPeers[p.ID] = p
		}
	}
	r.peers = filteredPeers
	r.locker.Unlock()

	if len(r.Peers()) == 0 {
		r.Close()
	}
}

func (r *Room) GetRouterRtpCapabilities(peer *Peer) mediasoup.RtpCapabilities {
	return peer.router.RtpCapabilities()
}

func (r *Room) SetRtpCapabilities(peer *Peer, rtpCapabilities *mediasoup.RtpCapabilities) {
	peer.SetJoined(true)
	peer.SetRtpCapabilities(rtpCapabilities)
}

func (r *Room) PipeActiveProducersToPeerRouter(peer *Peer) {
	for _, joinedPeer := range r.getJoinedPeers(peer) {
		for _, producer := range joinedPeer.GetProducers() {
			var has bool
			for _, rprod := range peer.router.Producers() {
				if rprod.Id() == producer.Id() {
					has = true
				}
			}
			if !has {
				r.logger.Logf(log.DebugLevel, "Piping producer: %v to router: %v", producer.Id(), peer.router.Id())
				_, err := joinedPeer.router.PipeToRouter(mediasoup.PipeToRouterOptions{
					ProducerId: producer.Id(),
					Router:     peer.router,
				})
				if err != nil {
					r.logger.Logf(log.WarnLevel, "PipeActiveProducersToPeerRouter() | err: %v", err)
				}
			}
		}
	}
}

func (r *Room) PipeProducerToJoinedPeersRouter(peer *Peer, producerId string) {
	for _, joinedPeer := range r.getJoinedPeers(peer) {
		var has bool
		for _, rprod := range joinedPeer.router.Producers() {
			if rprod.Id() == producerId {
				has = true
			}
		}
		if !has {
			r.logger.Logf(log.DebugLevel, "Piping producer: %v to router: %v", producerId, joinedPeer.router.Id())
			_, err := peer.router.PipeToRouter(mediasoup.PipeToRouterOptions{
				ProducerId: producerId,
				Router:     joinedPeer.router,
			})
			if err != nil {
				r.logger.Logf(log.WarnLevel, "PipeProducerToJoinedPeersRouter() | err: %v", err)
			}

		}

	}
}

func (r *Room) CreateWebRtcTransport(peer *Peer, opt CreateWebRtcTransportOption) (*CreateWebrtcTransportResponse, error) {
	webRtcTransportOptions := mediasoup.WebRtcTransportOptions{}
	err := utils.Clone(&webRtcTransportOptions, DefaultConfig.Mediasoup.WebRtcTransportOptions)
	if err != nil {
		return nil, errors.InternalServerError("CLONING_ERROR", "Unable to clone transport options.")
	}

	webRtcTransportOptions.EnableSctp = opt.SctpCapabilities != nil

	if opt.SctpCapabilities != nil {
		webRtcTransportOptions.NumSctpStreams = opt.SctpCapabilities.NumStreams
	}

	webRtcTransportOptions.AppData = &TransportData{
		Producing: opt.Producing,
		Consuming: opt.Consuming,
	}

	if opt.ForceTcp {
		webRtcTransportOptions.EnableUdp = utils.NewBool(false)
		webRtcTransportOptions.EnableTcp = true
	} else {
		webRtcTransportOptions.PreferUdp = true
	}

	transport, err := peer.router.CreateWebRtcTransport(webRtcTransportOptions)
	if err != nil {
		return nil, err
	}

	transport.On("dtlsstatechange", func(dtlsState mediasoup.DtlsState) {
		if dtlsState == "failed" || dtlsState == "closed" {
			r.logger.Log(log.WarnLevel, fmt.Sprintf("WebRtcTransport 'dtlsstatechange' event [dtlsState: %s]", dtlsState))
		}
	})

	// NOTE: For testing.
	// transport.EnableTraceEvent("probation", "bwe")
	// if err = transport.EnableTraceEvent("bwe"); err != nil {
	// 	return err
	// }

	// transport.On("trace", func(trace mediasoup.TransportTraceEventData) {
	// 	r.logger.Debug().
	// 		Str("transportId", transport.Id()).
	// 		Str("trace.type", string(trace.Type)).
	// 		Interface("trace", trace).
	// 		Msg(`"transport "trace" event`)

	// 	if trace.Type == "bwe" && trace.Direction == "out" {
	// 		peer.Notify("downlinkBwe", trace.Info)
	// 	}
	// })

	// Store the WebRtcTransport into the protoo Peer data Object.
	peer.AddTransport(transport)

	maxIncomingBitrate := DefaultConfig.Mediasoup.WebRtcTransportOptions.MaxIncomingBitrate

	if maxIncomingBitrate > 0 {
		if err := transport.SetMaxIncomingBitrate(maxIncomingBitrate); err != nil {
			return nil, err
		}
	}
	return &CreateWebrtcTransportResponse{
		TransportId:    transport.Id(),
		IceParameters:  transport.IceParameters(),
		IceCandidates:  transport.IceCandidates(),
		DtlsParameters: transport.DtlsParameters(),
		SctpParameters: transport.SctpParameters(),
	}, nil

}
func (r *Room) ConnectWebRtcTransport(peer *Peer, opt ConnectWebRtcTransportOption) error {
	transport, ok := peer.GetTransport(opt.TransportId)
	if !ok {
		return ErrTransportNotFound(opt.TransportId)

	}
	return transport.Connect(mediasoup.TransportConnectOptions{
		DtlsParameters: opt.DtlsParameters,
	})
}

func (r *Room) RestartICE(peer *Peer, opt RestartICEOption) (*mediasoup.IceParameters, error) {
	// Ensure the Peer is joined.
	if !peer.GetJoined() {
		return nil, ErrPeerNotJoined
	}
	transport, ok := peer.GetTransport(opt.TransportId)
	if !ok {
		return nil, ErrTransportNotFound(opt.TransportId)
	}
	iceParameters, err := transport.RestartIce()
	if err != nil {
		return nil, err
	}
	return &iceParameters, nil
}

func (r *Room) Produce(peer *Peer, opt ProduceOption) (string, error) {
	// Ensure the Peer is joined.
	if !peer.GetJoined() {
		return "", ErrPeerNotJoined
	}
	transport, ok := peer.GetTransport(opt.TransportId)
	if !ok {
		return "", ErrTransportNotFound(opt.TransportId)
	}
	// // Add peerId into appData to later get the associated Peer during
	// // the "loudest" event of the audioLevelObserver.
	appData := opt.AppData
	if appData == nil {
		appData = H{}
	}

	if !peer.GetJoined() {
		return "", errors.NotFound("PEER_NOT_JOINED", "Peer not joined")
	}

	appData["peerId"] = peer.GetID()

	producer, err := transport.Produce(mediasoup.ProducerOptions{
		Kind:          opt.Kind,
		RtpParameters: opt.RtpParameters,
		AppData:       appData,
		// KeyFrameRequestDelay: 5000,
	})
	if err != nil {
		return "", err
	}

	// Store the Producer into the protoo Peer data Object.
	peer.AddProducer(producer)

	r.PipeProducerToJoinedPeersRouter(peer, producer.Id())

	producer.On("score", func(score []mediasoup.ProducerScore) {
		db := H{
			"producerId": producer.Id(),
			"score":      score,
		}
		peer.Notify("notification/producerScore", db)
	})
	producer.On("videoorientationchange", func(videoOrientation mediasoup.ProducerVideoOrientation) {
		r.logger.Log(log.DebugLevel, "producerId", producer.Id(), "videoOrientation", videoOrientation, "producer 'videoorientationchange' event")
	})

	// // Add into the audioLevelObserver.
	if producer.Kind() == mediasoup.MediaKind_Audio {
		r.AudioLevelObserver.AddProducer(producer.Id())
	}
	return producer.Id(), nil
}

func (r *Room) HandleProducer(peer *Peer, producerId string, action v1.Action) error {
	// Ensure the Peer is joined.
	if !peer.GetJoined() {
		return ErrPeerNotJoined
	}
	producer, ok := peer.GetProducer(producerId)
	if !ok {
		return ErrProducerNotFound(producerId)
	}
	switch action {
	case v1.Action_CLOSE:
		producer.Close()
		peer.RemoveProducer(producer.Id())

	case v1.Action_PAUSE:
		return producer.Pause()

	case v1.Action_RESUME:
		return producer.Resume()

	default:
		return ErrActionNotDefined
	}
	return nil
}

func (r *Room) HandleConsumer(peer *Peer, consumerId, producerId string, action v1.Action) ([]byte, error) {
	// Ensure the Peer is joined.
	if !peer.GetJoined() {
		return nil, ErrPeerNotJoined
	}
	if len(consumerId) == 0 {
		peer.data.RLock()
		for _, consumer := range peer.GetConsumers() {
			if consumer.ProducerId() == producerId {
				consumerId = consumer.Id()
			}
		}
		peer.data.RUnlock()
		if consumerId == "" {
			return nil, ErrConsumerNotFoundForProducer(producerId)
		}
	}
	consumer, ok := peer.GetConsumer(consumerId)
	if !ok {
		return nil, ErrConsumerNotFound(consumerId)
	}
	var data []byte
	switch action {
	case v1.Action_CLOSE:
		consumer.Close()
		peer.RemoveConsumer(consumer.Id())
		peer.Notify("notification/consumerClosed", H{
			"consumerId": consumer.Id(),
		})
	case v1.Action_PAUSE:
		err := consumer.Pause()
		if err != nil {
			return nil, err
		}
	case v1.Action_RESUME:
		err := consumer.Resume()
		if err != nil {
			return nil, err
		}
		data, _ = json.Marshal(consumer.Score())
	default:
		return nil, ErrActionNotDefined
	}
	return data, nil
}

func (r *Room) HandlePipeProducer(producerId string, action v1.Action) error {
	value, ok := r.mapPipeProducers.Load(producerId)
	if !ok {
		return ErrPipeProducerNotFound(producerId)
	}
	pipeProducer := value.(*mediasoup.Producer)
	switch action {
	case v1.Action_CLOSE:
		pipeProducer.Close()
		r.mapPipeProducers.Delete(producerId)

	case v1.Action_PAUSE:
		return pipeProducer.Pause()

	case v1.Action_RESUME:
		return pipeProducer.Resume()

	default:
		return ErrActionNotDefined
	}
	return nil
}

func (r *Room) ClosePipeConsumer(consumerId string) error {
	value, ok := r.mapPipeConsumers.Load(consumerId)
	if !ok {
		return ErrPipeConsumerNotFound(consumerId)
	}
	pipeConsumer := value.(*mediasoup.Consumer)
	err := pipeConsumer.Close()
	if err != nil {
		r.logger.Logf(log.WarnLevel, "pipeConsumer close failed. | Err: %v", err)
	}
	r.mapPipeConsumers.Delete(consumerId)
	return nil
}

func (r *Room) GetStats(peer *Peer, id string, statsType v1.StatsType) ([]byte, error) {
	// Ensure the Peer is joined.
	if !peer.GetJoined() {
		return nil, ErrPeerNotJoined
	}
	var statsData []byte
	switch statsType {
	case v1.StatsType_TRANSPORT:
		transport, ok := peer.GetTransport(id)
		if !ok {
			return nil, ErrTransportNotFound(id)
		}
		stats, err := transport.GetStats()
		if err != nil {
			return nil, err
		}
		statsData, _ = json.Marshal(stats)
	case v1.StatsType_PRODUCER:
		producer, ok := peer.GetProducer(id)
		if !ok {
			return nil, ErrProducerNotFound(id)
		}
		stats, err := producer.GetStats()
		if err != nil {
			return nil, err
		}
		statsData, _ = json.Marshal(stats)
	case v1.StatsType_CONSUMER:
		consumer, ok := peer.GetConsumer(id)
		if !ok {
			return nil, ErrConsumerNotFound(id)
		}
		stats, err := consumer.GetStats()
		if err != nil {
			return nil, err
		}
		statsData, _ = json.Marshal(stats)
	default:
		return nil, ErrStatsTypeNotDefined
	}
	return statsData, nil
}

// Creates a mediasoup Consumer for the given mediasoup Producer.
func (r *Room) CreateConsumer(consumerPeer *Peer, producerID string, producerMediaKind mediasoup.MediaKind, appData interface{}) (*CreateConsumerResponse, error) {
	r.logger.Logf(log.DebugLevel, "createConsumer() [consumerPeer: %s, producerID: %s]",
		consumerPeer.GetID(),
		producerID,
	)

	// Must take the Transport the remote Peer is using for consuming.
	transport := consumerPeer.GetConsumerTransport()
	// This should not happen.
	if transport == nil {
		r.logger.Log(log.WarnLevel, "createConsumer() | Transport for consuming not found")
		return nil, ErrConsumingTransportNotFound
	}

	consumer, err := transport.Consume(mediasoup.ConsumerOptions{
		ProducerId:      producerID,
		RtpCapabilities: *consumerPeer.GetRtpCapabilities(),
		Paused:          producerMediaKind == mediasoup.MediaKind_Video,
		AppData:         appData,
	})

	if err != nil {
		r.logger.Logf(log.ErrorLevel, "createConsumer() | transport.consume() Error: %v", err)
		return nil, err
	}

	if producerMediaKind == mediasoup.MediaKind_Audio {
		consumer.SetPriority(255)
	}

	consumerPeer.AddConsumer(consumer)

	// Set Consumer events.
	consumer.On("transportclose", func() {
		consumer.Close()
		consumerPeer.RemoveConsumer(consumer.Id())
	})
	consumer.On("producerclose", func() {
		consumer.Close()
		consumerPeer.RemoveConsumer(consumer.Id())

		consumerPeer.Notify("notification/consumerClosed", H{
			"consumerId": consumer.Id(),
		})
	})
	consumer.On("producerpause", func() {
		consumerPeer.Notify("notification/consumerPaused", H{
			"consumerId": consumer.Id(),
		})
	})

	consumer.On("producerresume", func() {
		consumerPeer.Notify("notification/consumerResumed", H{
			"consumerId": consumer.Id(),
		})
	})

	consumer.On("score", func(score mediasoup.ConsumerScore) {
		consumerPeer.Notify("notification/consumerScore", H{
			"consumerId": consumer.Id(),
			"score":      score,
		})
	})
	consumer.On("layerschange", func(layers mediasoup.ConsumerLayers) {
		notifyData := H{
			"consumerId": consumer.Id(),
		}
		notifyData["spatialLayer"] = layers.SpatialLayer
		notifyData["temporalLayer"] = layers.TemporalLayer

		consumerPeer.Notify("notification/consumerLayersChanged", notifyData)
	})

	// NOTE: For testing.
	// consumer.EnableTraceEvent("rtp", "keyframe", "nack", "pli", "fir");
	// consumer.EnableTraceEvent("pli", "fir");
	// consumer.EnableTraceEvent("keyframe");

	// consumer.On("trace", func(trace mediasoup.ConsumerTraceEventData) {
	// 	r.logger.Debug().
	// 		Str("consumerId", consumer.Id()).
	// 		Str("trace.type", string(trace.Type)).
	// 		Interface("trace", trace).
	// 		Msg(`consumer "trace" event`)
	// })

	return &CreateConsumerResponse{
		ConsumerId:     consumer.Id(),
		MediaKind:      consumer.Kind(),
		RtpParameters:  consumer.RtpParameters(),
		ConsumerType:   consumer.Type(),
		ProducerPaused: consumer.ProducerPaused(),
		AppData:        consumer.AppData(),
	}, nil
}

// getJoinedPeers returns joined peers
func (r *Room) getJoinedPeers(excludePeer *Peer) []*Peer {
	var peers []*Peer
	r.locker.RLock()
	for _, p := range r.peers {
		if p.GetJoined() && !MatchPeer(p, excludePeer) {
			peers = append(peers, p)
		}
	}
	r.locker.RUnlock()
	return peers
}

func (r *Room) notification(peer *Peer, message string, data interface{}, broadcast bool, includeSender bool) {
	if broadcast {
		if includeSender {
			for _, joinedPeer := range r.getJoinedPeers(nil) {
				joinedPeer.Notify(message, data)
			}
		} else {
			for _, joinedPeer := range r.getJoinedPeers(peer) {
				joinedPeer.Notify(message, data)
			}
		}
	} else {
		if peer == nil {
			r.logger.Log(log.ErrorLevel, "Please be sure to pass peer")
			return
		}
		peer.Notify(message, data)
	}
}

func (r *Room) GetLeastLoadedRouter(excludePeer ...*Peer) *mediasoup.Router {
	router, _ := getNextRouter(r.Peers(), r.Routers, excludePeer...)
	// r.pipeProducersToRouter(router, excludePeer...)
	return router
}

func (r *Room) pipeProducersToRouter(router *mediasoup.Router, excludePeer ...*Peer) {
	peersToPipe := []*Peer{}

	for _, peer := range PeersWithoutMatchedPeers(r.Peers(), excludePeer...) {
		if peer.GetRouterID() != router.Id() {
			peersToPipe = append(peersToPipe, peer)
		}
	}

	for _, peer := range peersToPipe {
		srcRouter := r.Routers[peer.GetRouterID()]
		for producerId := range peer.GetProducers() {
			routerHasProducer := false
			for _, prod := range router.Producers() {
				if prod.Id() == producerId {
					routerHasProducer = true
				}
			}
			if routerHasProducer {
				continue
			}
			srcRouter.PipeToRouter(mediasoup.PipeToRouterOptions{
				ProducerId: producerId,
				Router:     router,
			})
		}
	}
}

func (r *Room) getRoutersToPipeTo(originRouterId string) []*mediasoup.Router {
	routers := []*mediasoup.Router{}
	for _, peer := range r.Peers() {
		if peer.GetRouterID() != originRouterId {
			routers = append(routers, peer.router)
		}
	}
	return routers
}

func (r *Room) CreatePipeTransport(peer *Peer, opts mediasoup.PipeTransportOptions) (ConnectPipeTransportOptions, error) {
	srcRouter := peer.router
	localPipeTransport, err := srcRouter.CreatePipeTransport(opts)
	if err != nil {
		return ConnectPipeTransportOptions{}, err
	}
	r.locker.Lock()
	r.mapPipeTransportQueue[localPipeTransport.Id()] = localPipeTransport
	r.locker.Unlock()

	return ConnectPipeTransportOptions{
		TransportID:    localPipeTransport.Id(),
		Tuple:          localPipeTransport.Tuple(),
		SrtpParameters: localPipeTransport.SrtpParameters(),
	}, nil
}

func (r *Room) ConnectPipeTransport(peer *Peer, opts ConnectPipeTransportOptions) error {
	r.locker.RLock()
	transport, ok := r.mapPipeTransportQueue[opts.TransportID]
	r.locker.RUnlock()
	if !ok {
		return ErrPipeTransportNotFound(opts.TransportID)
	}
	err := transport.Connect(mediasoup.TransportConnectOptions{
		Ip:             opts.Tuple.LocalIp,
		Port:           opts.Tuple.LocalPort,
		SrtpParameters: opts.SrtpParameters,
	})
	if err != nil {
		r.logger.Log(log.ErrorLevel, err)
		return err
	}
	transport.Observer().On("close", func() {
		err := r.core_publisher.Publish(context.TODO(), CreateNotification("internal/pipeTransport.close", H{"transport_id": transport.Id(), "router_id": peer.GetRouterID()}, peer.GetID(), r.RoomName))
		if err != nil {
			r.logger.Log(log.ErrorLevel, err)
		}
	})
	r.mapPipeTransports.Store(transport.Id(), transport)
	r.locker.Lock()
	delete(r.mapPipeTransportQueue, opts.TransportID)
	r.locker.Unlock()
	return nil
}

func (r *Room) ConsumePipeTransport(peer *Peer, producerId, transportId string) (result ProducePipeTransportPayload, err error) {
	r.logger.Logf(log.DebugLevel, "consumePipeTransport() | [TransportID: %s, ProducerID: %s]", transportId, producerId)
	var pipeConsumer *mediasoup.Consumer

	defer func() {
		if err != nil {
			r.logger.Logf(log.ErrorLevel, "pipeToRouter() | error creating pipe Consumer/Producer pair:%s", err)

			if pipeConsumer != nil {
				pipeConsumer.Close()
			}
		}
	}()

	value, ok := r.mapPipeTransports.Load(transportId)
	if !ok {
		return
	}
	transport := value.(*mediasoup.PipeTransport)
	pipeConsumer, err = transport.Consume(mediasoup.ConsumerOptions{
		ProducerId: producerId,
	})
	if err != nil {
		return
	}

	// Ensure that the producer has not been closed in the meanwhile.
	// if producer.Closed() {
	// 	err = mediasoup.NewInvalidStateError("original Producer closed")
	// 	return
	// }

	// Pipe events from the pipe Consumer to the pipe Producer.
	pipeConsumer.Observer().On("close", func() {
		// close pipeProducer on remote
		r.internal_publisher.Publish(context.TODO(), CreateNotification("pipeProducer.close", H{"producerId": producerId}, peer.GetID(), r.GetID()))

	})
	pipeConsumer.Observer().On("pause", func() {
		// pause pipeProducer on remote
		r.internal_publisher.Publish(context.TODO(), CreateNotification("pipeProducer.pause", H{"producerId": producerId}, peer.GetID(), r.GetID()))

	})
	pipeConsumer.Observer().On("resume", func() {
		// resume pipeProducer on remote
		r.internal_publisher.Publish(context.TODO(), CreateNotification("pipeProducer.resume", H{"producerId": producerId}, peer.GetID(), r.GetID()))

	})

	r.mapPipeConsumers.Store(pipeConsumer.Id(), pipeConsumer)

	result = ProducePipeTransportPayload{
		ProducerID:     producerId,
		Kind:           pipeConsumer.Kind(),
		RtpParameters:  pipeConsumer.RtpParameters(),
		Paused:         pipeConsumer.ProducerPaused(),
		TransportID:    transportId,
		PipeConsumerID: pipeConsumer.Id(),
	}

	return
}

func (r *Room) ProducePipeTransport(peer *Peer, opts ProducePipeTransportPayload) error {
	value, ok := r.mapPipeTransports.Load(opts.TransportID)
	if !ok {
		return ErrPipeTransportNotFound(opts.TransportID)
	}
	transport := value.(*mediasoup.PipeTransport)
	pipeProducer, err := transport.Produce(mediasoup.ProducerOptions{
		Id:            opts.ProducerID,
		Kind:          opts.Kind,
		RtpParameters: opts.RtpParameters,
		Paused:        opts.Paused,
		AppData:       opts.AppData,
	})
	if err != nil {
		r.logger.Log(log.ErrorLevel, err)
		return err
	}
	pipeProducer.Observer().On("close", func() {
		r.internal_publisher.Publish(context.TODO(), CreateNotification("pipeConsumer.close", H{"pipeConsumerId": opts.PipeConsumerID}, peer.GetID(), r.GetID()))
		r.mapPipeProducers.Delete(pipeProducer.Id())
	})

	r.mapPipeProducers.Store(pipeProducer.Id(), pipeProducer)

	return nil
}

func (r *Room) ClosePipeTransport(transportId string) error {
	value, ok := r.mapPipeTransports.Load(transportId)
	if !ok {
		return ErrPipeTransportNotFound(transportId)
	}
	pipeTransport := value.(*mediasoup.PipeTransport)
	pipeTransport.Close()
	return nil
}

func getNextRouter(peers []*Peer, routers map[string]*mediasoup.Router, excludePeer ...*Peer) (*mediasoup.Router, error) {
	if len(routers) == 0 {
		return nil, ErrNoRouterExists
	}
	finalPeers := PeersWithoutMatchedPeers(peers, excludePeer...)

	if len(finalPeers) == 0 {
		for _, router := range routers {
			return router, nil
		}
	}
	routerLoad := map[string]int{}
	leastLoadedRouterId := ""
	leastLoadedRouterLoad := 100
	for id := range routers {
		leastLoadedRouterId = id
		routerLoad[id] = 0
	}
	for _, peer := range finalPeers {
		if _, ok := routerLoad[peer.GetRouterID()]; ok {
			routerLoad[peer.GetRouterID()] += 1
		}
		routerLoad[peer.GetRouterID()] = 1
	}
	for routerId, load := range routerLoad {
		if load < leastLoadedRouterLoad {
			leastLoadedRouterLoad = load
			leastLoadedRouterId = routerId
		}
	}
	return routers[leastLoadedRouterId], nil
}

// MatchPeer match two peers and return true if both are same else false.
func MatchPeer(peer1, peer2 *Peer) bool {
	if (peer1 != nil || peer2 != nil) && (uuid.Equal(peer1.ID, peer2.ID)) {
		return true
	}
	return false
}

// PeersWithouthMatchedPeers returns peers which are not present in excluded peer slice.
func PeersWithoutMatchedPeers(peers []*Peer, excludePeer ...*Peer) []*Peer {
	out := []*Peer{}
	for _, peer := range peers {
		for _, exPeer := range excludePeer {
			if !MatchPeer(peer, exPeer) {
				out = append(out, peer)
			}
		}
	}
	return out
}
