package internal

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/itzmanish/go-micro/v2/errors"
	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/pratibimb-go/internal/role"
	"github.com/jiyeyuran/go-eventemitter"
	"github.com/jiyeyuran/mediasoup-go"
	uuid "github.com/satori/go.uuid"
)

type sentInfo struct {
	id     uint32
	method string
	respCh chan PeerResponse
}

type PeerResponse struct {
	data json.RawMessage
	err  error
}

func (r PeerResponse) Unmarshal(v interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(r.data) == 0 {
		return nil
	}
	return json.Unmarshal([]byte(r.data), v)
}

func (r PeerResponse) Data() []byte {
	return []byte(r.data)
}

func (r PeerResponse) Err() error {
	return r.err
}

type Peer struct {
	EventEmitter
	locker            sync.Mutex
	transport         Transport
	ID                uuid.UUID `json:"id"`
	RoomID            uuid.UUID `json:"room_id"`
	Name              string    `json:"name"`
	Email             string    `json:"email"`
	ProfilePictureURL string    `json:"profile_picture"`
	raisedHand        bool
	data              *PeerData
	joined            bool
	closed            int32
	closeCh           chan int32
	sents             map[uint32]sentInfo
	roles             []role.Role
	logger            log.Logger
	router            *mediasoup.Router
	rtpCapabilities   *mediasoup.RtpCapabilities
	raisedHandTime    time.Time
	JoinedAt          time.Time
	LeftAt            time.Time
}

func NewPeer(id, roomID uuid.UUID, conn Transport) *Peer {
	peer := &Peer{
		EventEmitter: eventemitter.NewEventEmitter(),
		logger:       log.NewLogger(log.WithFields(map[string]interface{}{"caller": "internal.Room"})),
		ID:           id,
		RoomID:       roomID,
		transport:    conn,
		sents:        make(map[uint32]sentInfo),
		closeCh:      make(chan int32),
		roles:        []role.Role{role.NORMAL},
		data: &PeerData{
			Transports:    make(map[string]mediasoup.ITransport),
			Producers:     make(map[string]*mediasoup.Producer),
			Consumers:     make(map[string]*mediasoup.Consumer),
			DataProducers: make(map[string]*mediasoup.DataProducer),
			DataConsumers: make(map[string]*mediasoup.DataConsumer),
		},
	}

	peer.handleTransport()
	return peer
}

func (p *Peer) Close() {
	if p.Closed() {
		return
	}

	if atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		p.logger.Log(log.InfoLevel, "peer closed")
		close(p.closeCh)
	}

	p.transport.Close()

	p.SafeEmit("close")
}

func (p *Peer) handlePeer() {

	if p.transport != nil {
		p.transport.On("disconnect", func() {
			if p.Closed() {
				return
			}
			p.logger.Logf(log.DebugLevel, "disconnect event [id:%s]", p.GetID())
			p.Close()
		})
	}
}

func (p *Peer) GetID() string {
	return p.ID.String()
}
func (p *Peer) SetID(id uuid.UUID) {
	p.ID = id
}
func (p *Peer) GetRoomID() string {
	return p.RoomID.String()
}
func (p *Peer) SetRoomID(id uuid.UUID) {
	p.RoomID = id
}
func (p *Peer) GetAuthID() {}
func (p *Peer) SetAuthID() {}
func (p *Peer) GetSocket() Transport {
	return p.transport
}
func (p *Peer) SetSocket(t Transport) {
	p.transport = t
}

func (p *Peer) Closed() bool {
	p.locker.Lock()
	defer p.locker.Unlock()

	return atomic.LoadInt32(&p.closed) > 0
}
func (p *Peer) GetJoined() bool {
	return p.joined
}
func (p *Peer) SetJoined(joined bool) {
	if joined {
		p.JoinedAt = time.Now()
	}
	p.joined = joined
}
func (p *Peer) GetJoinedTimestamp() string {
	return p.JoinedAt.Format(time.RFC3339)
}
func (p *Peer) GetInLobby()                {}
func (p *Peer) SetInLobby()                {}
func (p *Peer) GetAuthenticated()          {}
func (p *Peer) SetAuthenticated()          {}
func (p *Peer) GetAuthenticatedTimestamp() {}
func (p *Peer) GetRoles() []role.Role {
	return p.roles
}
func (p *Peer) GetDisplayName() string {
	return p.Name
}
func (p *Peer) SetDisplayName(name string) {
	p.Name = name
}
func (p *Peer) GetPicture() string {
	return p.ProfilePictureURL
}
func (p *Peer) SetPicture(pic string) {
	p.ProfilePictureURL = pic
}
func (p *Peer) GetEmail() string { return p.Email }
func (p *Peer) SetEmail(email string) {
	p.Email = email
}
func (p *Peer) GetRouterID() string {
	return p.router.Id()
}

func (p *Peer) GetRtpCapabilities() *mediasoup.RtpCapabilities {
	return p.rtpCapabilities
}
func (p *Peer) SetRtpCapabilities(capabilities *mediasoup.RtpCapabilities) {
	p.rtpCapabilities = capabilities
}
func (p *Peer) GetRaisedHand() bool {
	return p.raisedHand
}
func (p *Peer) SetRaisedHand(value bool) {
	if value {
		p.raisedHandTime = time.Now()
	}
	p.raisedHand = value
}
func (p *Peer) GetRaisedHandTimestamp() string {
	return p.raisedHandTime.Format(time.RFC3339)
}
func (p *Peer) GetTransports() map[string]mediasoup.ITransport {
	return p.data.Transports
}
func (p *Peer) GetProducers() map[string]*mediasoup.Producer {
	return p.data.Producers
}
func (p *Peer) GetConsumers() map[string]*mediasoup.Consumer {
	return p.data.Consumers
}

func (p *Peer) AddRole(role role.Role) {
	var exists bool
	for _, r := range p.roles {
		if r == role {
			exists = true
		}
	}

	if !exists {
		p.roles = append(p.roles, role)
	}

	p.logger.Logf(log.InfoLevel, "AddRole() | [newRole:%v]", role)
	p.Emit("gotRole", role)
}

func (p *Peer) RemoveRole(oldRole role.Role) {
	var newRole []role.Role
	for _, role := range p.roles {
		if oldRole != role {
			newRole = append(newRole, role)
		}
	}
	p.locker.Lock()
	p.roles = newRole
	p.locker.Unlock()
	p.logger.Logf(log.InfoLevel, "RemoveRole() | [oldRole:%v]", oldRole)
	p.Emit("lostRole", oldRole)

}

func (p *Peer) HasRole(role role.Role) bool {
	for _, r := range p.roles {
		if role == r {
			return true
		}
	}
	return false
}

func (p *Peer) AddTransport(transport *mediasoup.WebRtcTransport) {
	p.data.Lock()
	p.data.Transports[transport.Id()] = transport
	p.data.Unlock()
}

func (p *Peer) GetTransport(id string) (*mediasoup.WebRtcTransport, bool) {
	p.data.RLock()
	defer p.data.RUnlock()
	transport, ok := p.data.Transports[id]
	return transport.(*mediasoup.WebRtcTransport), ok
}

func (p *Peer) GetConsumerTransport() mediasoup.ITransport {
	var transport mediasoup.ITransport

	for _, t := range p.data.Transports {
		if data, ok := t.AppData().(*TransportData); ok && data.Consuming {
			transport = t
			break
		}
	}
	return transport
}

func (p *Peer) RemoveTransport(id string) {
	p.data.Lock()
	defer p.data.Unlock()

	delete(p.data.Transports, id)
}

func (p *Peer) AddProducer(producer *mediasoup.Producer) {
	p.data.Lock()
	p.data.Producers[producer.Id()] = producer
	p.data.Unlock()
}

func (p *Peer) GetProducer(id string) (producer *mediasoup.Producer, ok bool) {
	p.data.RLock()
	defer p.data.RUnlock()
	producer, ok = p.data.Producers[id]
	return
}

func (p *Peer) RemoveProducer(id string) {
	p.data.Lock()
	delete(p.data.Producers, id)
	p.data.Unlock()
}

func (p *Peer) AddConsumer(consumer *mediasoup.Consumer) {
	p.data.Lock()
	p.data.Consumers[consumer.Id()] = consumer
	p.data.Unlock()
}

func (p *Peer) GetConsumer(id string) (consumer *mediasoup.Consumer, ok bool) {
	p.data.RLock()
	defer p.data.RUnlock()
	consumer, ok = p.data.Consumers[id]
	return
}

func (p *Peer) RemoveConsumer(id string) {
	p.data.Lock()
	defer p.data.Unlock()
	delete(p.data.Consumers, id)

}

func (p *Peer) GetPeerInfo() *Peer {
	return p
}

func (peer *Peer) Request(method string, data interface{}) (rsp PeerResponse) {
	request := CreateRequest(method, data)

	sent := sentInfo{
		id:     request.Id,
		method: method,
		respCh: make(chan PeerResponse),
	}

	peer.locker.Lock()

	size := len(peer.sents)
	peer.sents[sent.id] = sent

	peer.locker.Unlock()

	defer func() {
		peer.locker.Lock()

		delete(peer.sents, sent.id)

		peer.locker.Unlock()
	}()

	if err := peer.transport.Send(request.Marshal()); err != nil {
		rsp.err = err
		return
	}

	timeout := 2000 * (15 + (0.1 * float64(size)))
	timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
	defer timer.Stop()

	select {
	case rsp = <-sent.respCh:
	case <-timer.C:
		rsp.err = errors.Timeout("TIMEOUT", "request timeout")
	case <-peer.closeCh:
		rsp.err = errors.InternalServerError("PEER_CLOSED", "peer closed")
	}

	return
}

func (peer *Peer) Notify(method string, data interface{}) error {
	notification := CreateNotification(method, data)

	return peer.transport.Send(notification.Marshal())
}

func (peer *Peer) handleTransport() {
	if peer.transport.Closed() {
		peer.Close()
		return
	}

	peer.transport.On("close", func() {
		peer.Close()
	})

	peer.transport.On("message", func(message Message) {
		if message.Request {
			peer.handleRequest(message)
		} else if message.Response {
			peer.handleResponse(message)
		} else if message.Notification {
			peer.handleNotification(message)
		}
	})
}

func (peer *Peer) handleRequest(request Message) {
	peer.SafeEmit("request", request, func(data interface{}) {
		response := CreateSuccessResponse(request, data)
		peer.transport.Send(response.Marshal())
	}, func(err error) {
		var anErr *Error
		e1, ok := err.(*Error)
		if ok {
			anErr = e1
		} else if e2, ok := err.(Error); ok {
			anErr = &e2
		} else {
			anErr = NewError(500, err.Error())
		}
		response := CreateErrorResponse(request, anErr)
		peer.transport.Send(response.Marshal())
	})
}

func (peer *Peer) handleResponse(response Message) {
	peer.locker.Lock()

	sent, ok := peer.sents[response.Id]

	if !ok {
		peer.locker.Unlock()
		err := errors.BadRequest("BAD_REQUEST", "bad response")
		peer.logger.Log(log.ErrorLevel, err, "received response does not match any sent request", "id", response.Id)
		return
	}

	delete(peer.sents, response.Id)

	peer.locker.Unlock()

	if response.OK {
		sent.respCh <- PeerResponse{
			data: response.Data,
		}
	} else {
		sent.respCh <- PeerResponse{
			err: response.Error,
		}
	}
}

func (peer *Peer) handleNotification(notification Message) {
	peer.SafeEmit("notification", notification)
}
