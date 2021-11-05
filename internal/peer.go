package internal

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/itzmanish/go-micro/v2"
	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/jiyeyuran/go-eventemitter"
	"github.com/jiyeyuran/mediasoup-go"
	uuid "github.com/satori/go.uuid"
)

type Peer struct {
	EventEmitter
	locker          sync.Mutex
	ID              uuid.UUID `json:"id"`
	RoomID          uuid.UUID `json:"room_id"`
	RoomName        string
	data            *PeerData
	publisher       micro.Event
	joined          bool
	closed          int32
	closeCh         chan int32
	logger          log.Logger
	router          *mediasoup.Router
	rtpCapabilities *mediasoup.RtpCapabilities
	JoinedAt        time.Time
	LeftAt          time.Time
}

func NewPeer(id, roomID uuid.UUID, roomName string, publisher micro.Event) *Peer {
	peer := &Peer{
		EventEmitter: eventemitter.NewEventEmitter(),
		logger:       log.NewLogger(log.WithFields(map[string]interface{}{"caller": "internal.Room"})),
		ID:           id,
		RoomID:       roomID,
		RoomName:     roomName,
		closeCh:      make(chan int32),
		data: &PeerData{
			Transports:    make(map[string]mediasoup.ITransport),
			Producers:     make(map[string]*mediasoup.Producer),
			Consumers:     make(map[string]*mediasoup.Consumer),
			DataProducers: make(map[string]*mediasoup.DataProducer),
			DataConsumers: make(map[string]*mediasoup.DataConsumer),
		},
		publisher: publisher,
	}

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
	for _, consumer := range p.GetConsumers() {
		consumer.Close()
	}
	for _, producer := range p.GetProducers() {
		producer.Close()
	}
	consumerTransport := p.GetConsumerTransport()
	if consumerTransport != nil {
		consumerTransport.Close()
	}
	for _, transports := range p.GetTransports() {
		transports.Close()
	}

	p.SafeEmit("close")
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

func (p *Peer) GetRouterID() string {
	return p.router.Id()
}

func (p *Peer) GetRtpCapabilities() *mediasoup.RtpCapabilities {
	return p.rtpCapabilities
}
func (p *Peer) SetRtpCapabilities(capabilities *mediasoup.RtpCapabilities) {
	p.rtpCapabilities = capabilities
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

func (peer *Peer) Notify(method string, data interface{}) error {
	notification := CreateNotification(method, data, peer.GetID(), peer.RoomName)
	return peer.publisher.Publish(context.TODO(), notification)
}

func (peer *Peer) CanConsume(producerID string, rtpCaps mediasoup.RtpCapabilities) bool {
	return peer.router.CanConsume(producerID, rtpCaps)
}
