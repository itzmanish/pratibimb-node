package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/itzmanish/go-micro/v2"
	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/pratibimb-node/internal"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
	"github.com/jiyeyuran/mediasoup-go"
	uuid "github.com/satori/go.uuid"
)

type RoomUpdateRequest struct {
	RoomID   string `json:"room_id"`
	PeerName string `json:"peer_name"`
	PeerID   string `json:"peer_id"`
}

type ws struct {
	sync.RWMutex
	log.Logger
	config internal.Config
	event  micro.Event
}

var rooms sync.Map
var mediasoupWorker []*mediasoup.Worker

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Subprotocols:    []string{"protoo"},
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func NewWsHandler(config internal.Config, event micro.Event) *ws {
	logger := log.NewLogger(log.WithFields(map[string]interface{}{"caller": "WS Handler"}))

	workers := []*mediasoup.Worker{}
	for i := 0; i < internal.DefaultConfig.Mediasoup.NumWorkers/4; i++ {
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

	return &ws{
		Logger: logger,
		config: config,
		event:  event,
	}
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {

	// we want to augment the response
	response := map[string]interface{}{
		"version": "1.0.0",
	}
	w.Header().Add("Content-Type", "application/json")

	// encode and write the response as json
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

func (h *ws) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	peerId := query.Get("peerId")
	if len(peerId) == 0 {
		http.Error(w, "Peer ID is missing.", http.StatusUnauthorized)
		return
	}

	roomId := query.Get("roomId")
	if roomId == "" {
		log.Debug("connection request without roomId")
		http.Error(w, "RoomID and ", http.StatusUnauthorized)
		return
	}

	secret, err := strconv.Atoi(query.Get("secret"))
	if err != nil {
		log.Infof("error on converting secret string to int32, : %v", err)
		http.Error(w, "secret is not valid number", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	// upgrader.CheckOrigin = func(r *http.Request) bool { log.Info(r.URL.String()); return true }
	if err != nil {
		log.Error("upgrade:", err)
		return
	}
	defer conn.Close()

	transport := internal.NewWebsocketTransport(conn)
	log.Debugf("connection request [roomId: %s, peerId: %s]", roomId, peerId)

	peerID, err := uuid.FromString(peerId)
	if err != nil {
		h.Logger.Log(log.ErrorLevel, err)
		return
	}

	room, err := h.getRoom(r, roomId, int32(secret))
	if err != nil {
		h.Logger.Log(log.ErrorLevel, err, "getRoom")
		msg := internal.CreateErrorNotification("ERROR_BEFORE_CLOSING", err)
		transport.Send(msg.Marshal())
		return
	}
	peer := room.GetPeer(peerID)
	var returning bool
	if peer != nil {
		if room.VerifyPeer(peerID) {
			peer.Close()
			peer.RemoveAllListeners()
			returning = true
		}
	}

	peer, err = room.CreatePeer(peerID, room.ID, transport)
	if err != nil {
		h.Logger.Log(log.ErrorLevel, err)
		msg := internal.CreateErrorNotification("PEER_NOT_CREATED", err)
		transport.Send(msg.Marshal())
		return
	}

	room.HandlePeer(peer, returning)
	data := &RoomUpdateRequest{
		RoomID:   roomId,
		PeerName: peer.Name,
		PeerID:   peerId,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Error(err)
	}
	err = h.event.Publish(context.TODO(), &v1.Message{Cmd: "add_peer", Data: jsonData})
	if err != nil {
		log.Error(err)
	}
	if err := transport.Run(); err != nil {
		h.Logger.Log(log.ErrorLevel, err, "transport.run")
		transport.Close()
	}
	err = h.event.Publish(context.TODO(), &v1.Message{Cmd: "remove_peer", Data: jsonData})
	if err != nil {
		log.Error(err)
	}
}

func (h *ws) getRoom(r *http.Request, roomId string, secret int32) (*internal.Room, error) {
	val, ok := rooms.Load(roomId)
	if !ok {
		return nil, internal.ErrRoomNotExist
	}
	room := val.(*internal.Room)
	if !room.ValidSecret(secret) {
		return nil, internal.ErrInvalidSecret
	}
	return room, nil
}
