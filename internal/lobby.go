package internal

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/itzmanish/go-micro/v2/errors"
	"github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/pratibimb-go/internal/role"
	"github.com/jiyeyuran/go-eventemitter"
	uuid "github.com/satori/go.uuid"
)

type Lobby struct {
	EventEmitter
	sync.RWMutex
	logger logger.Logger
	peers  map[uuid.UUID]*Peer
	closed int32
}

func NewLobby() *Lobby {
	return &Lobby{
		EventEmitter: eventemitter.NewEventEmitter(),
		peers:        make(map[uuid.UUID]*Peer),
		logger:       logger.NewLogger(logger.WithFields(map[string]interface{}{"caller": "internal.Lobby"})),
	}
}

func (l *Lobby) Close() {
	if atomic.CompareAndSwapInt32(&l.closed, 0, 1) {
		for _, peer := range l.peers {
			if !peer.Closed() {
				peer.Close()
			}
		}
		l.peers = nil
		l.logger.Log(logger.InfoLevel, "lobby closed")
		l.RemoveAllListeners()
	}
}
func (l *Lobby) Closed() bool {
	return atomic.LoadInt32(&l.closed) > 0
}

func (l *Lobby) CheckEmpty() bool {
	l.logger.Log(logger.InfoLevel, "CheckEmpty")
	return len(l.peers) == 0

}

func (l *Lobby) PeerList() []*Peer {
	l.logger.Log(logger.InfoLevel, "PeerList()")
	var peers []*Peer
	l.RLock()
	for _, peer := range l.peers {
		peers = append(peers, peer)
	}
	l.RUnlock()
	return peers
}

func (l *Lobby) HasPeer(id uuid.UUID) bool {
	for _, peer := range l.peers {
		if uuid.Equal(id, peer.ID) {
			return true
		}
	}
	return false
}

func (l *Lobby) PromoteAllPeers() {
	l.logger.Log(logger.InfoLevel, "PromoteAllPeers()")
	l.RLock()
	for _, peer := range l.peers {
		if !peer.Closed() {
			l.PromotePeer(peer.ID)
		}
	}
	l.RUnlock()

}

func (l *Lobby) PromotePeer(peerId uuid.UUID) {
	l.logger.Logf(logger.InfoLevel, "promotePeer() [Peer: %s]", peerId.String())
	peer, ok := l.peers[peerId]
	if !ok {
		return
	}
	peer.RemoveListener("request", func(request Message, accept func(data interface{}), reject func(err error)) {
		peer.handleTransport()
	})
	peer.RemoveListener("gotRole", func() {
		l.logger.Logf(logger.InfoLevel, "parkPeer() | rolesChange [peer: %s]", peer.GetID())
		l.Emit("peerRolesChanged", peer)
	})
	peer.RemoveListener("displayNameChanged", func() {
		l.logger.Logf(logger.InfoLevel, "parkPeer() | displayNameChange [peer: %s]", peer.GetID())
		l.Emit("changeDisplayName", peer)
	})

	peer.RemoveListener("pictureChanged", func() {
		l.logger.Log(logger.InfoLevel, "parkPeer() | pictureChange [peer: %s]", peer.GetID())
		l.Emit("changePicture", peer)
	})
	peer.RemoveListener("close", func() {
		l.logger.Log(logger.InfoLevel, "Peer close event [peer: %s]", peer.GetID())

		if l.Closed() {
			return
		}

		l.Emit("peerClosed", peer)
		l.Lock()
		delete(l.peers, peer.ID)
		l.Unlock()
		if l.CheckEmpty() {
			l.Emit("lobbyEmpty")
		}

	})
}

func (l *Lobby) ParkPeer(peer *Peer) {
	l.logger.Log(logger.InfoLevel, "ParkPeer() [peer:%s]", peer.ID)
	if l.Closed() {
		return
	}
	peer.On("gotRole", func(newRole role.Role) {
		l.logger.Logf(logger.InfoLevel, "parkPeer() | rolesChange [peer: %s]", peer.GetID())
		l.Emit("peerRolesChanged", peer)
	})
	peer.On("displayNameChanged", func() {
		l.logger.Logf(logger.InfoLevel, "parkPeer() | displayNameChange [peer: %s]", peer.GetID())
		l.Emit("changeDisplayName", peer)
	})

	peer.On("pictureChanged", func() {
		l.logger.Log(logger.InfoLevel, "parkPeer() | pictureChange [peer: %s]", peer.GetID())
		l.Emit("changePicture", peer)
	})

	peer.transport.On("request", func(request Message, accept func(data interface{}), reject func(err error)) {
		l.logger.Logf(logger.ErrorLevel, "Peer 'request' event [method: %s, peer: %s]", request.Method, peer.GetID())
		if err := l.handleSocketRequest(peer, request, accept); err != nil {
			reject(err)
		}
	})

	peer.On("close", func() {
		l.logger.Log(logger.InfoLevel, "Peer close event [peer: %s]", peer.GetID())

		if l.Closed() {
			return
		}

		l.Emit("peerClosed", peer)
		l.Lock()
		delete(l.peers, peer.ID)
		l.Unlock()
		if l.CheckEmpty() {
			l.Emit("lobbyEmpty")
		}

	})

	peer.Notify("enteredLobby", nil)

}

func (l *Lobby) handleSocketRequest(peer *Peer, req Message, cb func(data interface{})) error {
	l.logger.Logf(logger.DebugLevel, "handleSocketRequest [peer: %s], [request: %s]", peer.GetID(), req.Method)

	if l.Closed() {
		return errors.InternalServerError("LOBBY_CLOSED", "lobby is closed.")
	}

	switch req.Method {
	case "changeDisplayName":
		var requestData struct {
			displayName string
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		peer.SetDisplayName(requestData.displayName)

		cb(nil)

	case "changePicture":
		var requestData struct {
			picture string
		}

		if err := json.Unmarshal(req.Data, &requestData); err != nil {
			return errors.BadRequest("BAD_DATA_FORMAT", "Bad Data format", err)
		}

		peer.SetPicture(requestData.picture)

		cb(nil)
	}
	return nil
}
