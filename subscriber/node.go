package subscriber

import (
	"context"
	"strings"

	"github.com/itzmanish/go-micro/v2/errors"
	"github.com/itzmanish/go-micro/v2/util/log"
	"github.com/itzmanish/pratibimb-node/handler"
	"github.com/itzmanish/pratibimb-node/internal"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
)

type nodeSubscriber struct {
	nodeID string
}

func NewNodeSubscriber(nodeId string) *nodeSubscriber {
	return &nodeSubscriber{
		nodeID: nodeId,
	}
}

func (ns *nodeSubscriber) Handler(ctx context.Context, msg *v1.Message) error {
	log.Info("event recieved: ", msg)
	if strings.Contains(msg.Sub, "scale/") {
		roomInterface, ok := handler.Rooms.Load(msg.RoomId)
		if !ok {
			return nil
		}
		room, ok := roomInterface.(*internal.Room)
		if !ok {
			return errors.InternalServerError("ROOM_TYPE_MISMATCH", "room has different type, this should not happen")
		}
		// emit createPipeTransport event for room with peer_id
		// if msg.NodeId != ns.nodeID {
		room.Emit("scale", msg)
		// }
	}

	// sentInterface, ok := event.Sents.Load(msg.Cid)
	// if ok {
	// 	// check if this message is sent by this server itself
	// 	if msg.NodeId != ns.nodeID && msg.IsResponse {
	// 		event.Sents.Delete(msg.Cid)
	// 		sent := sentInterface.(*event.SentInfo)
	// 		if msg.Error != "" {
	// 			sent.RespCh <- event.Response{
	// 				Err: errors.New("RESPONSE_ERROR", msg.Error, 500),
	// 			}
	// 		} else {
	// 			sent.RespCh <- event.Response{
	// 				Data: msg.Data,
	// 			}
	// 		}
	// 	}
	// }

	return nil
}
