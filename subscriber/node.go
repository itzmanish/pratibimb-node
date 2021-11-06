package subscriber

import (
	"context"
	"encoding/json"

	"github.com/itzmanish/go-micro/v2/errors"
	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/pratibimb-node/handler"
	"github.com/itzmanish/pratibimb-node/internal"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
)

func Handler(ctx context.Context, msg *v1.Message) error {
	log.Info("node event recieved: ", msg)
	roomInterface, ok := handler.Rooms.Load(msg.RoomId)
	if !ok {
		return nil
	}
	room, ok := roomInterface.(*internal.Room)
	if !ok {
		return errors.InternalServerError("ROOM_TYPE_MISMATCH", "room has different type, this should not happen")
	}

	var payload internal.H
	err := json.Unmarshal(msg.Payload, &payload)
	if err != nil {
		log.Warnf("unmarshal payload failed, err: %v", err)
		return err
	}

	switch msg.Sub {
	case "pipeProducer.close":
		err = room.HandlePipeProducer(payload["producerId"].(string), v1.Action_CLOSE)
		if err != nil && err != internal.ErrPipeProducerNotFound(payload["producer_id"].(string)) {
			log.Warnf("pipeProducer.close | err: %v", err)
			return err
		}
	case "pipeProducer.pause":
		err = room.HandlePipeProducer(payload["producerId"].(string), v1.Action_PAUSE)
		if err != nil && err != internal.ErrPipeProducerNotFound(payload["producerId"].(string)) {
			log.Warnf("pipeProducer.pause | err: %v", err)
			return err
		}
	case "pipeProducer.resume":
		err = room.HandlePipeProducer(payload["producerId"].(string), v1.Action_RESUME)
		if err != nil && err != internal.ErrPipeProducerNotFound(payload["producerId"].(string)) {
			log.Warnf("pipeProducer.resume | err: %v", err)
			return err
		}
	case "pipeConsumer.close":
		err = room.ClosePipeConsumer(payload["pipeConsumerId"].(string))
		if err != nil && err != internal.ErrPipeConsumerNotFound(payload["pipeConsumerId"].(string)) {
			log.Warnf("pipeConsumer.close | err: %v", err)
			return err
		}
	}

	return nil
}
