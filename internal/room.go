package internal

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/itzmanish/go-micro/v2/errors"
	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/pratibimb-node/internal/permission"
	"github.com/itzmanish/pratibimb-node/internal/role"
	"github.com/jiyeyuran/go-eventemitter"
	"github.com/jiyeyuran/mediasoup-go"
	uuid "github.com/satori/go.uuid"
)

type Room struct {
	EventEmitter
	locker               sync.RWMutex
	ID                   uuid.UUID
	RoomName             string
	AudioLevelObserver   mediasoup.IRtpObserver
	Routers              map[string]*mediasoup.Router
	logger               log.Logger
	peers                map[uuid.UUID]*Peer
	chatHistory          []map[string]interface{}
	fileHistory          []string
	lastN                []uuid.UUID
	closed               int32
	locked               bool
	accessCode           int32
	selfDestructTimeout  time.Duration
	currentActiveSpeaker *Peer
}

var wg sync.WaitGroup

func NewRoom(mediasoupWorker []*mediasoup.Worker, roomID string, accessCode int32) (*Room, error) {

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
		ID:                  uuid.NewV4(),
		RoomName:            roomID,
		Routers:             routers,
		AudioLevelObserver:  audioLevelObserver,
		logger:              log.NewLogger(log.WithFields(map[string]interface{}{"caller": "Room"})),
		peers:               make(map[uuid.UUID]*Peer),
		EventEmitter:        eventemitter.NewEventEmitter(),
		selfDestructTimeout: 1 * time.Minute,
		chatHistory:         make([]map[string]interface{}, 0),
		fileHistory:         make([]string, 0),
		lastN:               make([]uuid.UUID, 0),
		accessCode:          accessCode,
	}

	room.handleAudioLevelObserver()

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

func (r *Room) CreatePeer(peerId, roomID uuid.UUID, transport Transport) (peer *Peer, err error) {
	r.locker.Lock()
	defer r.locker.Unlock()

	r.logger.Log(log.InfoLevel, "createPeer()", "peerId", peerId, "transport", transport.String())

	if _, ok := r.peers[peerId]; ok {
		transport.Close()
		err = errors.Conflict("PEER_EXISTS", `there is already a Peer with same peerId [peerId:"%s"]`, peerId)
		delete(r.peers, peerId)
		return
	}

	peer = NewPeer(peerId, roomID, transport)
	peer.Name = "Guest"

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

func (r *Room) VerifyPeer(id uuid.UUID) bool {
	p := r.GetPeer(id)
	return p != nil
}

func (r *Room) HandlePeer(peer *Peer, returning bool) {
	r.logger.Logf(log.InfoLevel, "[peer:%v, roles:%v, returning:%v]", peer.GetID(), peer.GetRoles(), returning)

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

func (r *Room) selfDestructCountdown() {
	r.logger.Log(log.DebugLevel, "selfDestructCountdown() started")
	wg.Add(1)
	time.AfterFunc(r.selfDestructTimeout, func() {
		if r.Closed() {
			wg.Done()
			return
		}
		if r.CheckEmpty() {
			r.logger.Logf(log.InfoLevel, "Room deserted for some time, closing the room [roomId: %s]", r.GetID())
			r.Close()
			wg.Done()
		} else {
			r.logger.Log(log.DebugLevel, "SelfDestructCountdown() aborted; room is not empty!")
			wg.Done()
		}
	})
	wg.Wait()
}

func (r *Room) CheckEmpty() bool {
	return len(r.peers) == 0
}

func (r *Room) peerJoining(peer *Peer, returning bool) {
	for _, v := range r.lastN {
		if !uuid.Equal(v, peer.ID) {
			r.lastN = append(r.lastN, peer.ID)
		}
	}
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
	peer.On("displayNameChanged", func(oldDisplayName string) {
		if !peer.joined {
			return
		}
		r.notification(peer, "changeDisplayName",
			[]byte(fmt.Sprintf("{peerId: %s,displayName: %s,oldDisplayName: %s}", peer.ID.String(), peer.Name, oldDisplayName)),
			true, false)
	})
	peer.On("pictureChanged", func() {
		if !peer.joined {
			return
		}
		r.notification(peer, "changePicture",
			H{"peerId": nil, "picture": peer.ProfilePictureURL}, true, false)
	})

	peer.On("gotRole", func(newRole role.Role) {
		// TODO gotRole
		if !peer.GetJoined() {
			return
		}
		r.notification(peer, "gotRole",
			H{"peerId": nil, "role": newRole}, true, true)
	})

	peer.On("lostRole", func(oldRole role.Role) {
		if !peer.joined {
			return
		}
		r.notification(peer, "lostRole",
			H{"peerId": peer.GetID(), "role": oldRole}, true, true)
	})

	peer.On("request", func(request Message, accept func(data interface{}), reject func(err error)) {
		r.logger.Log(log.DebugLevel, fmt.Sprintf("Peer 'request' event [method:%s, peerId: %s]", request.Method, peer.GetID()))
		err := r.handleTransportRequest(peer, request, accept)
		if err != nil {
			reject(err)
		}
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
	r.locker.Lock()
	// Remove from lastN
	var filteredLastN []uuid.UUID
	for _, p := range r.lastN {
		if !uuid.Equal(peer.ID, p) {
			filteredLastN = append(filteredLastN, p)
		}
	}
	r.lastN = filteredLastN
	r.locker.Unlock()

	var filteredPeers = make(map[uuid.UUID]*Peer)
	r.locker.Lock()
	for _, p := range r.peers {
		if !uuid.Equal(p.ID, peer.ID) {
			filteredPeers[p.ID] = p
		}
	}
	r.peers = filteredPeers
	r.locker.Unlock()

	// If this is the last Peer in the room and
	// lobby is empty, close the room after a while.
	if r.CheckEmpty() {
		r.selfDestructCountdown()
	}
}

func (r *Room) handleTransportRequest(peer *Peer, req Message, accept func(data interface{})) error {
	router := peer.router
	r.logger.Logf(log.InfoLevel, "request recieved with method: %v", req.Method)
	switch req.Method {
	case "getRouterRtpCapabilities":
		accept(router.RtpCapabilities())

	case "join":
		if peer.GetJoined() {
			return errors.Conflict("ALREADY_JOINED", "Peer already joined")
		}
		requestData := PeerData{}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format: %v", err)
		}

		peer.SetRtpCapabilities(requestData.RtpCapabilities)
		peer.SetDisplayName(requestData.DisplayName)

		joinedPeers := r.getJoinedPeers(peer)

		peerInfos := []*PeerInfo{}

		for _, joinedPeer := range joinedPeers {
			peerInfos = append(peerInfos, &PeerInfo{
				Id:          joinedPeer.GetID(),
				DisplayName: joinedPeer.Name,
				Device:      joinedPeer.data.Device,
			})
		}

		db := H{
			"peers":                peerInfos,
			"roles":                peer.GetRoles(),
			"tracker":              DefaultConfig.FileTracker,
			"roomPermission":       permission.RoomPermissions,
			"userRoles":            []role.Role{role.ADMIN, role.AUTHENTICATED, role.MODERATOR, role.NORMAL, role.PRESENTER},
			"allowWhenRoleMissing": permission.AllowWhenRoleMissing,
			"chatHistory":          r.chatHistory,
			"fileHistory":          r.fileHistory,
			"lastNHistory":         r.lastN,
			"locked":               r.locked,
			"accessCode":           r.accessCode,
		}

		accept(db)

		peer.SetJoined(true)

		for _, joinedPeer := range joinedPeers {

			// Create Consumers for existing Producers.
			for _, producer := range joinedPeer.data.Producers {
				r.createConsumer(peer, joinedPeer, producer)
			}

			// No need of creating data consumer for now.
			// Create DataConsumers for existing DataProducers.
			// for _, dataProducer := range data.DataProducers {
			// 	r.createDataConsumer(peer, joinedPeer, dataProducer)
			// }
		}

		db = H{
			"id":          peer.GetID(),
			"displayName": peer.GetDisplayName(),
			"picture":     peer.data.Picture,
			"roles":       peer.GetRoles(),
		}

		r.notification(peer, "newPeer", db, true, false)

	case "createWebRtcTransport":
		{
			// NOTE: Don't require that the Peer is joined here, so the client can
			// initiate mediasoup Transports and be ready when he later joins.

			var requestData struct {
				ForceTcp         bool
				Producing        bool
				Consuming        bool
				SctpCapabilities *mediasoup.SctpCapabilities
			}
			if err := json.Unmarshal(req.Data, &requestData); err != nil {
				return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format: %v", err)
			}

			webRtcTransportOptions := mediasoup.WebRtcTransportOptions{}
			err := Clone(&webRtcTransportOptions, DefaultConfig.Mediasoup.WebRtcTransportOptions)
			if err != nil {
				return errors.InternalServerError("CLONING_ERROR", "Unable to clone transport options.")
			}

			webRtcTransportOptions.EnableSctp = requestData.SctpCapabilities != nil

			if requestData.SctpCapabilities != nil {
				webRtcTransportOptions.NumSctpStreams = requestData.SctpCapabilities.NumStreams
			}

			webRtcTransportOptions.AppData = &TransportData{
				Producing: requestData.Producing,
				Consuming: requestData.Consuming,
			}

			if requestData.ForceTcp {
				webRtcTransportOptions.EnableUdp = NewBool(false)
				webRtcTransportOptions.EnableTcp = true
			} else {
				webRtcTransportOptions.PreferUdp = true
			}

			transport, err := router.CreateWebRtcTransport(webRtcTransportOptions)
			if err != nil {
				return err
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
			db := H{
				"id":             transport.Id(),
				"iceParameters":  transport.IceParameters(),
				"iceCandidates":  transport.IceCandidates(),
				"dtlsParameters": transport.DtlsParameters(),
				"sctpParameters": transport.SctpParameters(),
			}

			accept(db)

			maxIncomingBitrate := DefaultConfig.Mediasoup.WebRtcTransportOptions.MaxIncomingBitrate

			if maxIncomingBitrate > 0 {
				if err := transport.SetMaxIncomingBitrate(maxIncomingBitrate); err != nil {
					return err
				}
			}
		}

	case "connectWebRtcTransport":
		var requestData struct {
			TransportId    string                    `json:"transportId,omitempty"`
			DtlsParameters *mediasoup.DtlsParameters `json:"dtlsParameters,omitempty"`
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		transport, ok := peer.GetTransport(requestData.TransportId)
		if !ok {
			return ErrTransportNotFound(requestData.TransportId)

		}
		if err := transport.Connect(mediasoup.TransportConnectOptions{
			DtlsParameters: requestData.DtlsParameters,
		}); err != nil {
			return err
		}
		accept(nil)

	case "restartIce":
		var requestData struct {
			TransportId string `json:"transportId,omitempty"`
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		transport, ok := peer.GetTransport(requestData.TransportId)
		if !ok {
			return ErrTransportNotFound(requestData.TransportId)
		}
		iceParameters, err := transport.RestartIce()
		if err != nil {
			return err
		}
		accept(iceParameters)

	case "produce":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {

			return ErrPeerNotJoined
		}
		var requestData struct {
			TransportId   string                  `json:"transportId,omitempty"`
			Kind          mediasoup.MediaKind     `json:"kind,omitempty"`
			RtpParameters mediasoup.RtpParameters `json:"rtpParameters,omitempty"`
			AppData       H                       `json:"appData,omitempty"`
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		transport, ok := peer.GetTransport(requestData.TransportId)
		if !ok {
			return ErrTransportNotFound(requestData.TransportId)
		}
		// // Add peerId into appData to later get the associated Peer during
		// // the "loudest" event of the audioLevelObserver.
		appData := requestData.AppData
		if appData == nil {
			appData = H{}
		}
		if source, ok := appData["source"]; ok {
			if source == "screen" && !r.hasPermission(peer, permission.SHARE_SCREEN) {
				return errors.Unauthorized("PEER_UNAUTHORIZED", "Peer not authorized")
			}
			if source == "extravideo" && !r.hasPermission(peer, permission.EXTRA_VIDEO) {
				return errors.Unauthorized("PEER_UNAUTHORIZED", "Peer not authorized")
			}
		}
		if !peer.GetJoined() {
			return errors.NotFound("PEER_NOT_JOINED", "Peer not joined")
		}

		appData["peerId"] = peer.GetID()

		producer, err := transport.Produce(mediasoup.ProducerOptions{
			Kind:          requestData.Kind,
			RtpParameters: requestData.RtpParameters,
			AppData:       appData,
			// KeyFrameRequestDelay: 5000,
		})
		if err != nil {
			return err
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
				router.PipeToRouter(mediasoup.PipeToRouterOptions{
					ProducerId: producer.Id(),
					Router:     destinationRouter,
				})
			}
		}
		// Store the Producer into the protoo Peer data Object.
		peer.AddProducer(producer)

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

		accept(H{"id": producer.Id()})

		// Optimization: Create a server-side Consumer for each Peer.
		for _, otherPeer := range r.getJoinedPeers(peer) {
			r.createConsumer(otherPeer, peer, producer)
		}

		// // Add into the audioLevelObserver.
		if producer.Kind() == mediasoup.MediaKind_Audio {
			r.AudioLevelObserver.AddProducer(producer.Id())
		}

	case "closeProducer":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			ProducerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		producer, ok := peer.GetProducer(requestData.ProducerId)
		if !ok {
			return ErrProducerNotFound(requestData.ProducerId)
		}
		producer.Close()
		peer.RemoveProducer(producer.Id())

		accept(nil)

	case "pauseProducer":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			ProducerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		producer, ok := peer.GetProducer(requestData.ProducerId)
		if !ok {
			return ErrProducerNotFound(requestData.ProducerId)
		}
		if err := producer.Pause(); err != nil {
			return err
		}

		accept(nil)

	case "resumeProducer":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			ProducerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		producer, ok := peer.GetProducer(requestData.ProducerId)
		if !ok {
			return ErrProducerNotFound(requestData.ProducerId)
		}
		if err := producer.Pause(); err != nil {
			return err
		}

		accept(nil)

	case "pauseConsumer":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			ConsumerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		consumer, ok := peer.GetConsumer(requestData.ConsumerId)
		if !ok {
			return ErrConsumerNotFound(requestData.ConsumerId)
		}
		if err := consumer.Pause(); err != nil {
			return err
		}

		accept(nil)

	case "resumeConsumer":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			ConsumerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		consumer, ok := peer.GetConsumer(requestData.ConsumerId)
		if !ok {
			return ErrConsumerNotFound(requestData.ConsumerId)
		}
		if err := consumer.Resume(); err != nil {
			return err
		}

		accept(nil)

	case "setConsumerPreferredLayers":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			mediasoup.ConsumerLayers
			ConsumerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format")
		}
		consumer, ok := peer.GetConsumer(requestData.ConsumerId)
		if !ok {
			return ErrConsumerNotFound(requestData.ConsumerId)
		}
		if err := consumer.SetPreferredLayers(requestData.ConsumerLayers); err != nil {
			return err
		}

		accept(nil)

	case "setConsumerPriority":
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			ConsumerId string
			Priority   uint32
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		consumer, ok := peer.GetConsumer(requestData.ConsumerId)
		if !ok {
			return ErrConsumerNotFound(requestData.ConsumerId)
		}
		if err := consumer.SetPriority(requestData.Priority); err != nil {
			return err
		}

		accept(nil)

	case "requestConsumerKeyFrame":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			ConsumerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		consumer, ok := peer.GetConsumer(requestData.ConsumerId)
		if !ok {
			return ErrConsumerNotFound(requestData.ConsumerId)
		}
		if err := consumer.RequestKeyFrame(); err != nil {
			return err
		}

		accept(nil)

	// case "produceData":
	// 	// Ensure the Peer is joined.
	// 	if !peer.GetJoined() {
	// 		return ErrPeerNotJoined
	// 	}
	// 	var requestData struct {
	// 		TransportId          string                          `json:"transportId,omitempty"`
	// 		SctpStreamParameters *mediasoup.SctpStreamParameters `json:"sctpStreamParameters,omitempty"`
	// 		Label                string                          `json:"label,omitempty"`
	// 		Protocol             string                          `json:"protocol,omitempty"`
	// 		AppData              H                               `json:"appData,omitempty"`
	// 	}
	// 	if err := json.Unmarshal(req.Data, &requestData); err != nil {
	// 		return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
	// 	}
	// 	transport, ok := peer.GetTransport(requestData.TransportId)
	// 	if !ok {
	// 		return ErrTransportNotFound(requestData.TransportId)
	// 	}
	// 	dataProducer, err := transport.ProduceData(mediasoup.DataProducerOptions{
	// 		SctpStreamParameters: requestData.SctpStreamParameters,
	// 		Label:                requestData.Label,
	// 		Protocol:             requestData.Protocol,
	// 		AppData:              requestData.AppData,
	// 	})
	// 	if err != nil {
	// 		return err
	// 	}
	// 	peer.prod[dataProducer.Id()] = dataProducer

	// 	accept(H{"id": dataProducer.Id()})

	// 	switch dataProducer.Label() {
	// 	case "chat":
	// 		// Create a server-side DataConsumer for each Peer.
	// 		for _, otherPeer := range r.getJoinedPeers(peer) {
	// 			r.createDataConsumer(otherPeer, peer, dataProducer)
	// 		}

	// 	case "bot":
	// 		// Pass it to the bot.
	// 		r.bot.HandlePeerDataProducer(dataProducer.Id(), peer)
	// 	}

	case "changeDisplayName":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			DisplayName string `json:"displayName,omitempty"`
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		oldDisplayName := peer.GetDisplayName()
		peer.SetDisplayName(requestData.DisplayName)

		db := H{
			"peerId":         peer.GetID(),
			"displayName":    requestData.DisplayName,
			"oldDisplayName": oldDisplayName,
		}
		// Notify to others
		r.notification(peer, "peerDisplayNameChanged", db, true, false)

		accept(nil)

	case "getTransportStats":
		// Ensure the Peer is joined.
		if !peer.GetJoined() {
			return ErrPeerNotJoined
		}
		var requestData struct {
			TransportId string `json:"transportId,omitempty"`
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		transport, ok := peer.GetTransport(requestData.TransportId)
		if !ok {
			return ErrTransportNotFound(requestData.TransportId)
		}
		stats, err := transport.GetStats()
		if err != nil {
			return err
		}

		accept(stats)

	case "getProducerStats":
		var requestData struct {
			ProducerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		producer, ok := peer.GetProducer(requestData.ProducerId)
		if !ok {
			return ErrProducerNotFound(requestData.ProducerId)
		}
		stats, err := producer.GetStats()
		if err != nil {
			return err
		}

		accept(stats)

	case "getConsumerStats":
		var requestData struct {
			ConsumerId string
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		consumer, ok := peer.GetConsumer(requestData.ConsumerId)
		if !ok {
			return ErrConsumerNotFound(requestData.ConsumerId)
		}
		stats, err := consumer.GetStats()
		if err != nil {
			return err
		}

		accept(stats)

	// case "getDataProducerStats":
	// 	var requestData struct {
	// 		DataProducerId string
	// 	}
	// 	if err = PbToStruct(request.Data, &requestData); err != nil {
	// 		return
	// 	}
	// 	dataProducer, ok := peerData.DataProducers[requestData.DataProducerId]
	// 	if !ok {
	// 		err = fmt.Errorf(`dataProducer with id "%s" not found`, requestData.DataProducerId)
	// 		return
	// 	}
	// 	stats, err := dataProducer.GetStats()
	// 	if err != nil {
	// 		return err
	// 	}

	// 	accept(stats)

	// case "getDataConsumerStats":
	// 	var requestData struct {
	// 		DataConsumerId string
	// 	}
	// 	if err = PbToStruct(request.Data, &requestData); err != nil {
	// 		return
	// 	}
	// 	dataConsumer, ok := peerData.DataConsumers[requestData.DataConsumerId]
	// 	if !ok {
	// 		err = fmt.Errorf(`dataConsumer with id "%s" not found`, requestData.DataConsumerId)
	// 		return
	// 	}
	// 	stats, err := dataConsumer.GetStats()
	// 	if err != nil {
	// 		return err
	// 	}

	// 	accept(stats)

	case "chatMessage":
		// TODO chatMessage
		if !r.hasPermission(peer, permission.SEND_CHAT) {
			return ErrPeerNotAuthorized
		}
		var requestData struct {
			ChatMessage map[string]interface{} `json:"chatMessage"`
		}
		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}
		r.chatHistory = append(r.chatHistory, requestData.ChatMessage)

		// Notify to others
		r.notification(peer, "chatMessage", H{
			"peerId":      peer.GetID(),
			"chatMessage": requestData.ChatMessage,
		}, true, false)

		accept(nil)

	case "moderator:clearChat":
		if !r.hasPermission(peer, permission.MODERATE_CHAT) {
			return ErrPeerNotAuthorized
		}

		r.chatHistory = make([]map[string]interface{}, 0)

		// Notify to others
		r.notification(peer, "moderator:clearChat", nil, true, false)

		accept(nil)

	case "lockRoom":
		if !r.hasPermission(peer, permission.CHANGE_ROOM_LOCK) {
			return ErrPeerNotAuthorized
		}

		r.locked = true

		db := H{
			"peerId": peer.GetID(),
		}
		// Notify to others
		r.notification(peer, "lockRoom", db, true, false)

		accept(nil)

	case "unlockRoom":
		if !r.hasPermission(peer, permission.CHANGE_ROOM_LOCK) {
			return ErrPeerNotAuthorized
		}
		r.locked = false

		db := H{
			"peerId": peer.GetID(),
		}
		// Notify to others
		r.notification(peer, "unlockRoom", db, true, false)

		accept(nil)

	case "setAccessCode":
		var requestData struct {
			accessCode int32
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		r.accessCode = requestData.accessCode

		db := H{
			"peerId":     peer.GetID(),
			"accessCode": requestData.accessCode,
		}
		// Notify to others
		r.notification(peer, "setAccessCode", db, true, false)

		accept(nil)

	case "sendFile":
		if !r.hasPermission(peer, permission.SHARE_FILE) {
			return ErrPeerNotAuthorized
		}

		var requestData struct {
			magnetUri string
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		db := H{
			"peerId":    peer.GetID(),
			"magnetUri": requestData.magnetUri,
		}
		// Notify to others
		r.notification(peer, "sendFile", db, true, false)

		accept(nil)

	case "moderator:clearFileSharing":
		//TODO clearFileSharing
		if !r.hasPermission(peer, permission.MODERATE_FILES) {
			return ErrPeerNotAuthorized
		}
		r.fileHistory = []string{}

		// Notify to others
		r.notification(peer, "moderator:clearFileSharing", nil, true, false)

		accept(nil)

	case "raiseHand":
		var requestData struct {
			raisedHand bool
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		peer.SetRaisedHand(requestData.raisedHand)

		db := H{
			"peerId":              peer.GetID(),
			"raisedHand":          requestData.raisedHand,
			"raisedHandTimestamp": peer.GetRaisedHandTimestamp(),
		}

		// Notify to others
		r.notification(peer, "raiseHand", db, true, false)

		accept(nil)

	case "moderator:mute":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		var requestData struct {
			peerId string
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		id, err := uuid.FromString(requestData.peerId)

		if err != nil {
			return err
		}

		mutePeer := r.GetPeer(id)

		if mutePeer == nil {
			return ErrPeerNotFound(requestData.peerId)
		}

		r.notification(mutePeer, "moderator:mutePeer", nil, false, false)

		accept(nil)

	case "moderator:muteAll":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		// Notify to others
		r.notification(peer, "moderator:muteAll", nil, true, false)

		accept(nil)

	case "moderator:stopVideo":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		var requestData struct {
			peerId string
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		id, err := uuid.FromString(requestData.peerId)

		if err != nil {
			return err
		}

		stopVideoPeer := r.GetPeer(id)

		if stopVideoPeer == nil {
			return ErrPeerNotFound(requestData.peerId)
		}

		r.notification(stopVideoPeer, "moderator:stopVideo", nil, false, false)

		accept(nil)

	case "moderator:stopAllVideo":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		// Notify to others
		r.notification(peer, "moderator:stopAllVideo", nil, true, false)

		accept(nil)

	case "moderator:stopAllScreenSharing":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		// Notify to others
		r.notification(peer, "moderator:stopAllScreenSharing", nil, true, false)

		accept(nil)

	case "moderator:stopScreenSharing":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		var requestData struct {
			peerId string
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		id, err := uuid.FromString(requestData.peerId)

		if err != nil {
			return err
		}

		stopVideoPeer := r.GetPeer(id)

		if stopVideoPeer == nil {
			return ErrPeerNotFound(requestData.peerId)
		}

		r.notification(peer, "moderator:stopScreenSharing", nil, false, false)

		accept(nil)

	case "moderator:closeMeeting":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		// Notify to others
		r.notification(peer, "moderator:kick", nil, true, false)

		r.Close()

		accept(nil)

	case "moderator:kickPeer":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		var requestData struct {
			peerId string
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		id, err := uuid.FromString(requestData.peerId)

		if err != nil {
			return err
		}

		kickPeer := r.GetPeer(id)

		if kickPeer == nil {
			return ErrPeerNotFound(requestData.peerId)
		}

		r.notification(peer, "moderator:kickPeer", nil, false, false)

		kickPeer.Close()

		accept(nil)

	case "moderator:lowerHand":
		if !r.hasPermission(peer, permission.MODERATE_ROOM) {
			return ErrPeerNotAuthorized
		}

		var requestData struct {
			peerId string
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		id, err := uuid.FromString(requestData.peerId)

		if err != nil {
			return err
		}

		lowerPeer := r.GetPeer(id)

		if lowerPeer == nil {
			return ErrPeerNotFound(requestData.peerId)
		}

		r.notification(peer, "moderator:lowerHand", nil, false, false)

		accept(nil)

	case "applyNetworkThrottle":
		//TODO: throttle.start

	case "resetNetworkThrottle":
		//TODO: throttle.stop

	default:
		r.logger.Logf(log.ErrorLevel, "unknown request.method %s", req.Method)
		return ErrMethodUnknown
	}
	return nil
}

// Creates a mediasoup Consumer for the given mediasoup Producer.
func (r *Room) createConsumer(consumerPeer, producerPeer *Peer, producer *mediasoup.Producer) {
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
	if consumerPeer.GetRtpCapabilities() == nil ||
		!producerPeer.router.CanConsume(producer.Id(), *consumerPeer.GetRtpCapabilities()) {
		return
	}

	// Must take the Transport the remote Peer is using for consuming.
	transport := consumerPeer.GetConsumerTransport()
	// This should not happen.
	if transport == nil {
		r.logger.Log(log.WarnLevel, "createConsumer() | Transport for consuming not found")
		return
	}

	consumer, err := transport.Consume(mediasoup.ConsumerOptions{
		ProducerId:      producer.Id(),
		RtpCapabilities: *consumerPeer.GetRtpCapabilities(),
		Paused:          producer.Kind() == mediasoup.MediaKind_Video,
		AppData:         producer.AppData(),
	})
	if err != nil {
		r.logger.Logf(log.ErrorLevel, "createConsumer() | transport.consume() Error: %v", err)
		return
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

	go func() {
		// Send a request to the remote Peer with Consumer parameters.
		rsp := consumerPeer.Request("newConsumer", H{
			"peerId":         producerPeer.GetID(),
			"producerId":     producer.Id(),
			"id":             consumer.Id(),
			"kind":           consumer.Kind(),
			"rtpParameters":  consumer.RtpParameters(),
			"type":           consumer.Type(),
			"appData":        consumer.AppData(),
			"producerPaused": consumer.ProducerPaused(),
		})
		if rsp.Err() != nil {
			r.logger.Logf(log.WarnLevel, "createConsumer() | failed: %v", rsp.Err)
			return
		}

		// Now that we got the positive response from the remote endpoint, resume
		// the Consumer so the remote endpoint will receive the a first RTP packet
		// of this new stream once its PeerConnection is already ready to process
		// and associate it.
		if err = consumer.Resume(); err != nil {
			r.logger.Logf(log.WarnLevel, "createConsumer() | failed: %v", err)
			return
		}
		consumerPeer.Notify("consumerScore", H{
			"consumerId": consumer.Id(),
			"score":      consumer.Score(),
		})
	}()

}

func (r *Room) hasPermission(peer *Peer, perm permission.Permission) bool {

	for _, role := range peer.roles {
		if r, ok := permission.RoomPermissions[perm]; ok {
			for _, rr := range r {
				if rr == role {
					return true
				}
			}
		}
	}
	for _, p := range permission.AllowWhenRoleMissing {
		if p == perm && len(r.getPeersWithPermission(perm, nil, false)) == 0 {
			return true
		}
	}
	return false
}

func (r *Room) hasAccess(peer *Peer, access permission.Access) bool {
	for _, r := range peer.roles {
		if rr, ok := permission.RoomAccess[access]; ok {
			for _, role := range rr {
				if role == r {
					return true
				}
			}
		}
	}
	return false
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

func (r *Room) getAllowedPeers(perm permission.Permission, excludePeer *Peer, joined bool) []*Peer {
	//joined =true
	peers := r.getPeersWithPermission(perm, excludePeer, joined)
	if len(peers) > 0 {
		return peers
	}
	for _, perms := range permission.AllowWhenRoleMissing {
		if perm == perms {
			return r.Peers()
		}
	}
	return peers
}

func (r *Room) getPeersWithPermission(perm permission.Permission, excludePeer *Peer, joined bool) []*Peer {
	//joined =true
	var peers []*Peer
	r.locker.RLock()
	for _, p := range r.peers {
		if p.joined == joined && MatchPeer(p, excludePeer) && r.hasPermission(p, perm) {
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
