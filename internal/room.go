package internal

import (
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
	Routers            map[string]*mediasoup.Router
	peers              map[uuid.UUID]*Peer
	lastN              []uuid.UUID
	logger             log.Logger
	closed             int32
	locked             bool
	accessCode         int32
	core_publisher     micro.Event
	internal_publisher micro.Event
}

func NewRoom(mediasoupWorker []*mediasoup.Worker, roomID uuid.UUID, roomName string, accessCode int32, core_publisher, internal_publisher micro.Event) (*Room, error) {

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

	audioLevelObserverOption := &mediasoup.AudioLevelObserverOptions{
		MaxEntries: 1,
		Threshold:  -80,
		Interval:   800,
	}

	log.Debugf("Routers available: %v, Count: %d", routers, len(routers))

	firstRouter, err := getNextRouter(nil, routers)
	if err != nil {
		return nil, err
	}

	// Create a mediasoup AudioLevelObserver on first router
	audioLevelObserver, err := firstRouter.CreateAudioLevelObserver(
		func(o *mediasoup.AudioLevelObserverOptions) {
			o.MaxEntries = audioLevelObserverOption.MaxEntries
			o.Threshold = audioLevelObserverOption.Threshold
			o.Interval = audioLevelObserverOption.Interval
		},
	)
	if err != nil {
		return nil, errors.InternalServerError("NewRoom(): CreateAudioLevelObserver", err.Error())
	}

	room := &Room{
		ID:                 roomID,
		RoomName:           roomName,
		Routers:            routers,
		AudioLevelObserver: audioLevelObserver,
		logger:             log.NewLogger(log.WithFields(map[string]interface{}{"caller": "Room"})),
		peers:              make(map[uuid.UUID]*Peer),
		EventEmitter:       eventemitter.NewEventEmitter(),
		lastN:              make([]uuid.UUID, 0),
		accessCode:         accessCode,
		core_publisher:     core_publisher,
		internal_publisher: internal_publisher,
	}

	room.handleAudioLevelObserver()

	// room.On("scale", func(msg *v1.Message) {
	// 	room.handleRoomScaling(msg)
	// })

	// room.On("pipeTransportConnected", func(transportId string) {
	// 	room.handlePipeTransportConnect(transportId)
	// })

	return room, nil
}

func (r *Room) String() string {
	return "[Room: " + r.ID.String() + "RoomName: " + r.RoomName + "]"
}

func (r *Room) IsLocked() bool {
	return r.locked
}

func (r *Room) Close() {
	r.locker.Lock()
	defer r.locker.Unlock()

	if r.Closed() {
		return
	}
	for _, peer := range r.peers {
		peer.Close()
	}

	r.peers = make(map[uuid.UUID]*Peer)

	if atomic.CompareAndSwapInt32(&r.closed, 0, 1) {
		r.logger.Log(log.InfoLevel, "room closed")
	}
	for _, router := range r.Routers {
		router.Close()
	}
	r.SafeEmit("close")
}

func (r *Room) Closed() bool {
	return atomic.LoadInt32(&r.closed) > 0
}

func (r *Room) Peers() (peers []*Peer) {
	r.locker.Lock()
	defer r.locker.Unlock()

	for _, peer := range r.peers {
		peers = append(peers, peer)
	}

	return
}

func (r *Room) ValidSecret(secret int32) bool {
	return r.accessCode == secret
}

func (r *Room) CreatePeer(peerId uuid.UUID) (peer *Peer, err error) {
	r.locker.Lock()
	defer r.locker.Unlock()

	r.logger.Log(log.InfoLevel, "createPeer()", "peerId", peerId)

	if _, ok := r.peers[peerId]; ok {
		err = errors.Conflict("PEER_EXISTS", `there is already a Peer with same peerId [peerId:"%s"]`, peerId)
		delete(r.peers, peerId)
		return
	}

	peer = NewPeer(peerId, r.ID, r.core_publisher)

	if r.peers == nil {
		r.peers = make(map[uuid.UUID]*Peer)
	}
	r.peers[peerId] = peer

	peer.On("close", func() {
		r.locker.Lock()
		defer r.locker.Unlock()
		delete(r.peers, peerId)
		peer.RemoveAllListeners()
	})

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
	} else if DefaultConfig.MaxUserPerRoom <= len(r.peers) {
		r.handleOverRoomLimit(peer)
	} else {
		r.peerJoining(peer, false)
	}
}

func (r *Room) handleOverRoomLimit(peer *Peer) {
	r.notification(peer, "overRoomLimit", nil, false, false)
}

func (r *Room) handleAudioLevelObserver() {
	r.AudioLevelObserver.On("volumes", func(volumes []mediasoup.AudioLevelObserverVolume) {
		producer := volumes[0].Producer
		volume := volumes[0].Volume
		// todo: fix this
		r.notification(nil, "activeSpeaker", H{"peerId": producer.AppData().(H)["peerId"],
			"volume": volume}, true, false)

	})
	r.AudioLevelObserver.On("silence", func() {
		r.notification(nil, "activeSpeaker", H{"peerId": nil}, true, false)
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

	r.locker.Lock()
	r.peers[peer.ID] = peer
	r.locker.Unlock()

	peer.router = r.GetLeastLoadedRouter(peer)
	r.handlePeer(peer)
	if returning {
		r.notification(peer, "roomBack", nil, false, false)
	} else {
		r.notification(peer, "roomReady", DefaultConfig.TurnConfig, false, false)
	}

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
		r.notification(peer, "peerClosed",
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

}

func (r *Room) GetRouterRtpCapabilities(peer *Peer) mediasoup.RtpCapabilities {
	return peer.router.RtpCapabilities()
}

func (r *Room) SetRtpCapabilities(peer *Peer, rtpCapabilities *mediasoup.RtpCapabilities) {
	peer.SetJoined(true)
	peer.SetRtpCapabilities(rtpCapabilities)
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
	// transport.On("sctpstatechange", func(sctpState mediasoup.SctpState) {
	// 	r.logger.Log(log.DebugLevel,fmt.Sprintf("sctpState: %v WebRtcTransport: %v Event",sctpState,sctpstatechange))
	// })
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
	pipeRouters := r.getRoutersToPipeTo(peer.GetRouterID())

	for routerId, destinationRouter := range r.Routers {
		has := false
		for _, rr := range pipeRouters {
			if rr.Id() == routerId {
				has = true
			}
		}
		if has {
			peer.router.PipeToRouter(mediasoup.PipeToRouterOptions{
				ProducerId: producer.Id(),
				Router:     destinationRouter,
			})
		}
	}

	// Store the Producer into the protoo Peer data Object.
	peer.AddProducer(producer)

	// r.mapPipeTransports.Range(func(key, value interface{}) bool {
	// 	v, ok := value.(PipeTransportPair)
	// 	if !ok {
	// 		return true
	// 	}
	// 	r.handlePipeTransportConnect(v.localPipeTransport.Id())
	// 	return true
	// })

	producer.On("score", func(score []mediasoup.ProducerScore) {
		db := H{
			"producerId": producer.Id(),
			"score":      score,
		}
		peer.Notify("producerScore", db)
	})
	producer.On("videoorientationchange", func(videoOrientation mediasoup.ProducerVideoOrientation) {
		r.logger.Log(log.DebugLevel, "producerId", producer.Id(), "videoOrientation", videoOrientation, "producer 'videoorientationchange' event")
	})

	// NOTE: For testing.
	// producer.EnableTraceEvent("rtp", "keyframe", "nack", "pli", "fir");
	// producer.EnableTraceEvent("pli", "fir");
	// producer.EnableTraceEvent("keyframe");

	// producer.On("trace", func(trace mediasoup.ProducerTraceEventData) {
	// 	r.logger.Debug().
	// 		Str("producerId", producer.Id()).
	// 		Str("trace.type", string(trace.Type)).
	// 		Interface("trace", trace).
	// 		Msg(`producer "trace" event`)
	// })getConsumerStats

	// TODO: should be handled by core
	// // Optimization: Create a server-side Consumer for each Peer.
	// for _, otherPeer := range r.getJoinedPeers(peer) {
	// 	r.createConsumer(otherPeer, peer, producer)
	// }

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
func (r *Room) HandleConsumer(peer *Peer, consumerId string, action v1.Action) ([]byte, error) {
	// Ensure the Peer is joined.
	if !peer.GetJoined() {
		return nil, ErrPeerNotJoined
	}
	consumer, ok := peer.GetConsumer(consumerId)
	if !ok {
		return nil, ErrConsumerNotFound(consumerId)
	}
	var data []byte
	switch action {
	case v1.Action_CLOSE:
		consumer.Close()
		peer.RemoveProducer(consumer.Id())
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
func (r *Room) CreateConsumer(consumerPeer, producerPeer *Peer, producer *mediasoup.Producer) (*CreateConsumerResponse, error) {
	r.logger.Logf(log.DebugLevel, "createConsumer() [consumerPeer:%s, producerPeer:%s, producer:%s]",
		consumerPeer.GetID(),
		producerPeer.GetID(),
		producer.Id(),
	)

	// Optimization:
	// - Create the server-side Consumer in paused mode.
	// - Tell its Peer about it and wait for its response.
	// - Upon receipt of the response, resume the server-side Consumer.
	// - If video, this will mean a single key frame requested by the
	//   server-side Consumer (when resuming it).
	// - If audio (or video), it will avoid that RTP packets are received by the
	//   remote endpoint *before* the Consumer is locally created in the endpoint
	//   (and before the local SDP O/A procedure ends). If that happens (RTP
	//   packets are received before the SDP O/A is done) the PeerConnection may
	//   fail to associate the RTP stream.

	// consumerPeerData := consumerPeer.data

	// NOTE: Don"t create the Consumer if the remote Peer cannot consume it.
	// TODO: check this on core level
	if consumerPeer.GetRtpCapabilities() == nil ||
		!producerPeer.router.CanConsume(producer.Id(), *consumerPeer.GetRtpCapabilities()) {
		return nil, ErrUnableToConsume
	}

	// Must take the Transport the remote Peer is using for consuming.
	transport := consumerPeer.GetConsumerTransport()
	// This should not happen.
	if transport == nil {
		r.logger.Log(log.WarnLevel, "createConsumer() | Transport for consuming not found")
		return nil, ErrConsumingTransportNotFound
	}

	consumer, err := transport.Consume(mediasoup.ConsumerOptions{
		ProducerId:      producer.Id(),
		RtpCapabilities: *consumerPeer.GetRtpCapabilities(),
		Paused:          producer.Kind() == mediasoup.MediaKind_Video,
		AppData:         producer.AppData(),
	})

	if err != nil {
		r.logger.Logf(log.ErrorLevel, "createConsumer() | transport.consume() Error: %v", err)
		return nil, err
	}

	if producer.Kind() == mediasoup.MediaKind_Audio {
		consumer.SetPriority(255)
	}

	consumerPeer.AddConsumer(consumer)

	// Set Consumer events.
	consumer.On("transportclose", func() {
		consumerPeer.RemoveConsumer(consumer.Id())
	})
	consumer.On("producerclose", func() {

		consumerPeer.RemoveConsumer(consumer.Id())

		consumerPeer.Notify("consumerClosed", H{
			"consumerId": consumer.Id(),
		})
	})
	consumer.On("producerpause", func() {
		consumerPeer.Notify("consumerPaused", H{
			"consumerId": consumer.Id(),
		})
	})
	consumer.On("producerresume", func() {

		consumerPeer.Notify("consumerResumed", H{
			"consumerId": consumer.Id(),
		})
	})
	consumer.On("score", func(score mediasoup.ConsumerScore) {

		consumerPeer.Notify("consumerScore", H{
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

		consumerPeer.Notify("consumerLayersChanged", notifyData)
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
	r.pipeProducersToRouter(router, excludePeer...)
	// r.handleRemoteRouterPipeTransport(router)
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

// func (r *Room) handleRemoteRouterPipeTransport(router *mediasoup.Router) {
// 	nodes := r.getRemoteRoutersToPipeTo()
// 	for _, node := range nodes {
// 		if node.ID != r.NodeID && len(node.Routers) > 0 {
// 			for _, routerid := range node.Routers {
// 				cid := uuid.NewV4()
// 				err := r.event.Publish(context.TODO(), event.CreateEventRequest(r.RoomName, r.NodeID, "scale/createPipeTransport", H{"router_id": routerid, "cid": cid.String()}))
// 				if err != nil {
// 					r.logger.Log(log.ErrorLevel, err)
// 					break
// 				}
// 				payload, err := r.CreatePipeTransport(router, mediasoup.PipeTransportOptions{
// 					ListenIp:   DefaultConfig.Mediasoup.WebRtcTransportOptions.ListenIps[0],
// 					EnableSrtp: true,
// 					EnableRtx:  true,
// 					AppData: H{
// 						"cid": cid.String(),
// 					},
// 				})
// 				if err != nil {
// 					r.logger.Log(log.ErrorLevel, err)
// 					break
// 				}
// 				msg := event.CreateEventRequest(r.RoomName, r.NodeID, "scale/connectPipeTransport", payload)
// 				err = r.event.Publish(context.TODO(), msg)
// 				if err != nil {
// 					r.logger.Log(log.ErrorLevel, err)
// 					r.mapPipeTransportQueue.Delete(payload.TransportID)
// 				}
// 			}

// 		}
// 	}
// }

// func (r *Room) getRemoteRoutersToPipeTo() (nodes Nodes) {
// 	// TODO
// 	record, err := r.store.Read(r.RoomName)
// 	if err != nil {
// 		if err == store.ErrNotFound {
// 			return
// 		}
// 		log.Error(err)
// 		return
// 	}
// 	var room StoreRoom
// 	if len(record) == 0 {
// 		return
// 	}
// 	err = json.Unmarshal(record[0].Value, &room)
// 	if err != nil {
// 		log.Error(err)
// 		return
// 	}
// 	nodes = room.Nodes
// 	return
// }

// // TODO:
// func (r *Room) handleRoomScaling(msg *v1.Message) error {
// 	if msg.Error != "" {
// 		return errors.New("EVENT_ERROR", msg.Error, 500)
// 	}
// 	if msg.NodeId == r.NodeID {
// 		return nil
// 	}
// 	switch msg.Sub {
// 	case "scale/createPipeTransport":
// 		var payload struct {
// 			RouterID string `json:"router_id"`
// 			Cid      string `json:"cid"`
// 		}
// 		if err := json.Unmarshal(msg.Data, &payload); err != nil {
// 			return err
// 		}
// 		srcRouter, ok := r.Routers[payload.RouterID]
// 		if !ok {
// 			return errors.NotFound("ROUTER_NOT_FOUND", "router not found")
// 		}
// 		data, err := r.CreatePipeTransport(srcRouter, mediasoup.PipeTransportOptions{
// 			ListenIp:   DefaultConfig.Mediasoup.WebRtcTransportOptions.ListenIps[0],
// 			EnableSrtp: true,
// 			EnableRtx:  true,
// 			AppData: H{
// 				"cid": payload.Cid,
// 			},
// 		})
// 		if err != nil {
// 			return err
// 		}
// 		return r.event.Publish(context.TODO(), event.CreateEventRequest(r.RoomName, r.NodeID, "scale/connectPipeTransport", data))

// 	case "scale/connectPipeTransport":
// 		var payload ConnectPipeRouterPayload
// 		if err := json.Unmarshal(msg.Data, &payload); err != nil {
// 			return err
// 		}
// 		r.ConnectPipeTransport(payload)
// 	case "scale/pipeTransportClose":
// 	case "scale/producePipeTransport":
// 		var payload ProducePipeTransportPayload
// 		if err := json.Unmarshal(msg.Data, &payload); err != nil {
// 			return err
// 		}
// 		r.producePipeTransport(payload)

// 	}
// 	return nil
// }

// func (r *Room) CreatePipeTransport(srcRouter *mediasoup.Router, opts mediasoup.PipeTransportOptions) (ConnectPipeRouterPayload, error) {
// 	localPipeTransport, err := srcRouter.CreatePipeTransport(opts)
// 	if err != nil {
// 		return ConnectPipeRouterPayload{}, err
// 	}
// 	r.mapPipeTransportQueue.Store(localPipeTransport.Id(), localPipeTransport)
// 	var appData H
// 	if opts.AppData != nil {
// 		appData = opts.AppData.(H)
// 	}
// 	return ConnectPipeRouterPayload{
// 		TransportID:    localPipeTransport.Id(),
// 		Cid:            appData["cid"].(string),
// 		Tuple:          localPipeTransport.Tuple(),
// 		SrtpParameters: localPipeTransport.SrtpParameters(),
// 	}, nil
// }

// func (r *Room) ConnectPipeTransport(data ConnectPipeRouterPayload) {
// 	var transport *mediasoup.PipeTransport
// 	r.mapPipeTransportQueue.Range(func(key, value interface{}) bool {
// 		tt, ok := value.(*mediasoup.PipeTransport)
// 		if ok {
// 			log.Info(tt, tt.AppData())
// 			log.Infof("type: %T", tt.AppData())
// 			appData, _ := tt.AppData().(H)
// 			id := appData["cid"].(string)
// 			if id == data.Cid {
// 				transport = tt
// 				return false
// 			}
// 		}
// 		return true
// 	})
// 	if transport == nil {
// 		return
// 	}
// 	err := transport.Connect(mediasoup.TransportConnectOptions{
// 		Ip:             data.Tuple.LocalIp,
// 		Port:           data.Tuple.LocalPort,
// 		SrtpParameters: data.SrtpParameters,
// 	})
// 	if err != nil {
// 		r.logger.Log(log.ErrorLevel, err)
// 		r.mapPipeTransportQueue.Delete(transport.Id())
// 		return
// 	}
// 	transport.Observer().On("close", func() {
// 		err := r.event.Publish(context.TODO(), event.CreateEventRequest(r.RoomName, r.NodeID, "scale/pipeTransportClose", H{"transport_id": transport.Id()}))
// 		if err != nil {
// 			r.logger.Log(log.ErrorLevel, err)
// 		}
// 	})
// 	r.mapPipeTransports.Store(transport.Id(), PipeTransportPair{localPipeTransport: transport, remotePipeTransportID: data.TransportID})
// 	r.mapPipeTransportQueue.Delete(transport.Id())
// 	r.Emit("pipeTransportConnected", transport.Id())
// }

// func (r *Room) handlePipeTransportConnect(transportId string) {
// 	for _, peer := range r.Peers() {
// 		for _, producer := range peer.GetProducers() {
// 			r.pipeProducerToRemoteRouter(producer, transportId)
// 		}
// 	}
// }

// func (r *Room) pipeProducerToRemoteRouter(producer *mediasoup.Producer, transportId string) (result *mediasoup.PipeToRouterResult, err error) {

// 	var pipeConsumer *mediasoup.Consumer

// 	defer func() {
// 		if err != nil {
// 			r.logger.Logf(log.ErrorLevel, "pipeToRouter() | error creating pipe Consumer/Producer pair:%s", err)

// 			if pipeConsumer != nil {
// 				pipeConsumer.Close()
// 			}
// 		}
// 	}()

// 	transport, ok := r.mapPipeTransports.Load(transportId)
// 	if !ok {
// 		return
// 	}
// 	transportPair := transport.(PipeTransportPair)
// 	localPipeTransport := transportPair.localPipeTransport

// 	pipeConsumer, err = localPipeTransport.Consume(mediasoup.ConsumerOptions{
// 		ProducerId: producer.Id(),
// 	})
// 	if err != nil {
// 		return
// 	}
// 	// Tell remote producer to consume
// 	msg := event.CreateEventRequest(r.RoomName, r.NodeID, "scale/producePipeTransport", ProducePipeTransportPayload{
// 		ProducerID:     producer.Id(),
// 		Kind:           pipeConsumer.Kind(),
// 		RtpParameters:  pipeConsumer.RtpParameters(),
// 		Paused:         pipeConsumer.ProducerPaused(),
// 		AppData:        producer.AppData(),
// 		TransportID:    transportPair.remotePipeTransportID,
// 		PipeConsumerID: pipeConsumer.Id(),
// 	})

// 	err = r.event.Publish(context.TODO(), msg)
// 	if err != nil {
// 		return
// 	}

// 	// Ensure that the producer has not been closed in the meanwhile.
// 	if producer.Closed() {
// 		err = mediasoup.NewInvalidStateError("original Producer closed")
// 		return
// 	}

// 	// Pipe events from the pipe Consumer to the pipe Producer.
// 	pipeConsumer.Observer().On("close", func() {
// 		// close pipeProducer on remote
// 		r.event.Publish(context.TODO(), event.CreateEventRequest(r.RoomName, r.NodeID, "scale/pipeProducer.close", producer.Id()))

// 	})
// 	pipeConsumer.Observer().On("pause", func() {
// 		// pause pipeProducer on remote
// 		r.event.Publish(context.TODO(), event.CreateEventRequest(r.RoomName, r.NodeID, "scale/pipeProducer.pause", producer.Id()))

// 	})
// 	pipeConsumer.Observer().On("resume", func() {
// 		// resume pipeProducer on remote
// 		r.event.Publish(context.TODO(), event.CreateEventRequest(r.RoomName, r.NodeID, "scale/pipeProducer.resume", producer.Id()))

// 	})

// 	// fire pipeconsumer close if pipeProducer gets close in remote router
// 	// Pipe events from the pipe Producer to the pipe Consumer.
// 	// pipeProducer.Observer().On("close", func() { pipeConsumer.Close() })
// 	r.mapPipeConsumers.Store(pipeConsumer.Id(), pipeConsumer)
// 	result = &mediasoup.PipeToRouterResult{
// 		PipeConsumer: pipeConsumer,
// 	}

// 	return
// }

// func (r *Room) producePipeTransport(payload ProducePipeTransportPayload) {
// 	var transport *mediasoup.PipeTransport
// 	iTransport, ok := r.mapPipeTransports.Load(payload.TransportID)
// 	if !ok {
// 		return
// 	}
// 	transport = iTransport.(*mediasoup.PipeTransport)
// 	if transport == nil {
// 		return
// 	}
// 	pipeProducer, err := transport.Produce(mediasoup.ProducerOptions{
// 		Id:            payload.ProducerID,
// 		Kind:          payload.Kind,
// 		RtpParameters: payload.RtpParameters,
// 		Paused:        payload.Paused,
// 		AppData:       payload.AppData,
// 	})
// 	if err != nil {
// 		r.logger.Log(log.ErrorLevel, err)
// 		return
// 	}
// 	pipeProducer.Observer().On("close", func() {
// 		r.event.Publish(context.TODO(), event.CreateEventRequest(r.RoomName, r.NodeID, "scale/pipeProducer.close", H{"pipeConsumerId": payload.PipeConsumerID}))
// 		r.mapPipeProducers.Delete(pipeProducer.Id())
// 	})
// 	r.mapPipeProducers.Store(pipeProducer.Id(), pipeProducer)
// }

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
