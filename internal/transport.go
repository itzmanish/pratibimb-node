package internal

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/itzmanish/go-micro/v2/errors"
	"github.com/itzmanish/go-micro/v2/logger"
	"github.com/jiyeyuran/go-eventemitter"
)

type Transport interface {
	EventEmitter
	fmt.Stringer
	Send(data []byte) error
	Close()
	CloseWithMessage(message []byte)
	Closed() bool
	Run() error
}

type WebsocketTransport struct {
	EventEmitter
	logger logger.Logger
	locker sync.Mutex
	conn   *websocket.Conn
	closed bool
}

func NewWebsocketTransport(conn *websocket.Conn) Transport {
	t := &WebsocketTransport{
		EventEmitter: eventemitter.NewEventEmitter(),
		logger:       logger.NewLogger(logger.WithFields(map[string]interface{}{"caller": "WebSocketTransport"})),
		conn:         conn,
	}

	return t
}

func (t *WebsocketTransport) Send(message []byte) error {
	t.locker.Lock()
	defer t.locker.Unlock()

	if t.closed {
		return errors.InternalServerError("TRANSPORT_CLOSED", "transport closed")
	}

	return t.conn.WriteMessage(websocket.TextMessage, message)
}

func (t *WebsocketTransport) Close() {
	t.locker.Lock()
	defer t.locker.Unlock()

	if t.closed {
		return
	}

	t.logger.Log(logger.ErrorLevel, "close()", "conn", t.String())

	t.closed = true
	t.conn.Close()
	t.SafeEmit("close")
}

func (t *WebsocketTransport) CloseWithMessage(message []byte) {
	t.locker.Lock()
	defer t.locker.Unlock()

	if t.closed {
		return
	}

	t.logger.Log(logger.ErrorLevel, "closeWithMessage()", "conn", t.String())

	t.closed = true
	err := t.Send(message)
	if err != nil {
		t.logger.Log(logger.ErrorLevel, "closeWithMessage()", err)
	}
	t.conn.Close()
	t.SafeEmit("close")
}

func (t *WebsocketTransport) Closed() bool {
	t.locker.Lock()
	defer t.locker.Unlock()

	return t.closed
}

func (t *WebsocketTransport) String() string {
	return t.conn.RemoteAddr().String()
}

func (t *WebsocketTransport) Run() error {
	for {
		messageType, data, err := t.conn.ReadMessage()

		if err != nil {
			t.Close()
			return err
		}

		if messageType == websocket.BinaryMessage {
			t.logger.Log(logger.InfoLevel, "warning of ignoring received binary message", "conn", t.String())
			continue
		}

		if t.ListenerCount("message") == 0 {
			err := errors.InternalServerError("NO_LISTENER", `no listeners for "message" event`)
			t.logger.Log(logger.ErrorLevel, err, `ignoring received message`, "conn", t.String())
			continue
		}

		message := Message{}

		if err := json.Unmarshal(data, &message); err != nil {
			t.logger.Log(logger.ErrorLevel, err, `json unmarshal`, "conn", t.String())
			continue
		}

		t.SafeEmit("message", message)
	}
}
