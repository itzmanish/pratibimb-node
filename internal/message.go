package internal

import (
	"encoding/json"
	"fmt"

	"github.com/itzmanish/go-micro/v2/errors"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
)

type Message struct {
	OK           bool            `json:"ok,omitempty"`
	Request      bool            `json:"request,omitempty"`
	Response     bool            `json:"response,omitempty"`
	Notification bool            `json:"notification,omitempty"`
	Id           uint32          `json:"id,omitempty"`
	Method       string          `json:"method,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`

	*Error
}

type Error struct {
	Id     string `json:"id,omitempty"`
	Status string `json:"status,omitempty"`
	Code   int    `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func NewError(code int, reason string) *Error {
	return &Error{
		Code:   code,
		Detail: reason,
	}
}

func (e Error) Error() string {
	return fmt.Sprintf("ID: %s, Code: %d, Details: %s", e.Id, e.Code, e.Detail)
}

func (m Message) String() string {
	data, _ := json.Marshal(m)

	return string(data)
}

func (m Message) Marshal() []byte {
	data, _ := json.Marshal(m)

	return data
}

func CreateNotification(method string, data interface{}, peerId, roomId string) *v1.Message {
	raw, _ := json.Marshal(data)

	return &v1.Message{
		Sub:     method,
		Payload: json.RawMessage(raw),
		RoomId:  roomId,
		PeerId:  peerId,
	}
}

func CreateErrorNotification(method string, err error) Message {
	return Message{
		Notification: true,
		Method:       method,
		Error:        parseMicroError(err),
	}
}

func parseMicroError(err error) *Error {
	IErr, ok := err.(Error)
	if !ok {
		merr := errors.FromError(err)
		return &Error{
			Id:     merr.Id,
			Code:   int(merr.Code),
			Status: merr.Status,
			Detail: merr.Detail,
		}
	}
	return &IErr
}
