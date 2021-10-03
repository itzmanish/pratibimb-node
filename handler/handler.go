package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/itzmanish/go-micro/v2/client"
	log "github.com/itzmanish/go-micro/v2/logger"
	accountpb "github.com/itzmanish/pratibimb-go/account/proto/account/v1"
	"github.com/itzmanish/pratibimb-go/pratibimb/internal"
	"github.com/jiyeyuran/mediasoup-go"
	uuid "github.com/satori/go.uuid"
)

type ws struct {
	sync.RWMutex
	log.Logger
	config                 internal.Config
	rooms                  sync.Map
	mediasoupWorker        *mediasoup.Worker
	nextMediasoupWorkerIdx int
	accountService         accountpb.AccountService
}

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Subprotocols:    []string{"protoo"},
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type indexHandler struct{}

func NewIndexHandler() *indexHandler {
	return &indexHandler{}
}

func NewWsHandler(config internal.Config, c client.Client) *ws {
	logger := log.NewLogger(log.WithFields(map[string]interface{}{"caller": "WS Handler"}))

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
		for {
			select {
			case <-ticker.C:
				usage, err := worker.GetResourceUsage()
				if err != nil {
					log.Error(err, "pid", worker.Pid(), "mediasoup Worker resource usage")
					continue
				}
				log.Info("pid", worker.Pid(), "usage", usage, "mediasoup Worker resource usage")
			}
		}
	}()

	return &ws{
		Logger:          logger,
		config:          config,
		mediasoupWorker: worker,
		accountService:  accountpb.NewAccountService("com.itzmanish.pratibimb.service.v1.account", c),
	}
}

func (*indexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// decode the incoming request as json
	// var request map[string]interface{}
	// if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
	// 	http.Error(w, err.Error(), 500)
	// 	return
	// }

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
		log.Info("connection request without roomId")
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
		log.Info("upgrade:", err)
		return
	}
	defer conn.Close()

	transport := internal.NewWebsocketTransport(conn)
	log.Infof("connection request [roomId: %s, peerId: %s]", roomId, peerId)

	peerID, err := uuid.FromString(peerId)
	if err != nil {
		h.Logger.Log(log.ErrorLevel, err)
		return
	}

	room, err := h.getOrCreateRoom(r, roomId, int32(secret))
	if err != nil {
		h.Logger.Log(log.ErrorLevel, err, "getOrCreateRoom")
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

	if err := transport.Run(); err != nil {
		h.Logger.Log(log.ErrorLevel, err, "transport.run")
		transport.Close()
	}
}

func (h *ws) getOrCreateRoom(r *http.Request, roomId string, secret int32) (room *internal.Room, err error) {
	val, ok := h.rooms.Load(roomId)
	if ok {
		room := val.(*internal.Room)
		if room.ValidSecret(secret) {
			return room, nil
		}
		return nil, internal.ErrInvalidSecret
	} else {
		room, err = internal.NewRoom(h.mediasoupWorker, roomId, secret)
		if err != nil {
			return nil, err
		}
	}

	h.rooms.Store(roomId, room)

	room.On("close", func() {
		h.rooms.Delete(roomId)
		room.RemoveAllListeners()
	})

	return
}
