package internal

import (
	"fmt"

	"github.com/itzmanish/go-micro/v2/errors"
)

var (
	ErrInvalidSecret = errors.Unauthorized("ROOM_UNAUTHORIZED", "Invalid secret for given room")
	ErrRoomNotExist  = errors.NotFound("ROOM_NOT_EXIST", "Room does not exist")
	ErrMethodUnknown = errors.BadRequest("UNKNOWN_METHOD", "Method is unknown!")
	ErrPeerNotJoined = errors.NotFound("PEER_NOT_JOINED", "Peer not joined")
	ErrPeerNotFound  = func(id string) error {
		return errors.NotFound("PEER_NOT_FOUND", fmt.Sprintf(`Peer with id "%s" not found`, id))
	}
	ErrPeerNotAuthorized = errors.Unauthorized("PEER_UNAUTHORIZED", "Peer not authorized")
	ErrTransportNotFound = func(id string) error {
		return errors.NotFound("TRANSPORT_NOT_FOUND", fmt.Sprintf(`transport with id "%s" not found`, id))
	}
	ErrPipeTransportNotFound = func(id string) error {
		return errors.NotFound("PIPE_TRANSPORT_NOT_FOUND", fmt.Sprintf(`pipe transport with id "%s" not found`, id))
	}
	ErrPipeProducerNotFound = func(id string) error {
		return errors.NotFound("PIPE_PRODUCER_NOT_FOUND", fmt.Sprintf(`pipe producer with id "%s" not found`, id))
	}
	ErrPipeConsumerNotFound = func(id string) error {
		return errors.NotFound("PIPE_CONSUMER_NOT_FOUND", fmt.Sprintf(`pipe consumer with id "%s" not found`, id))
	}
	ErrProducerNotFound = func(id string) error {
		return errors.NotFound("PRODUCER_NOT_FOUND", fmt.Sprintf(`Producer with id "%s" not found`, id))
	}
	ErrConsumerNotFound = func(id string) error {
		return errors.NotFound("CONSUMER_NOT_FOUND", fmt.Sprintf(`Consumer with id "%s" not found`, id))
	}
	ErrConsumingTransportNotFound   = errors.NotFound("TRANSPORT_NOT_FOUND", "Consumer transport not found")
	ErrNoRouterExists               = errors.InternalServerError("ROUTER_DOES_NOT_EXIST", "no router available")
	ErrActionNotDefined             = errors.BadRequest("ACTION_NOT_DEFINED", "Action not defined")
	ErrStatsTypeNotDefined          = errors.BadRequest("STATS_TYPE_NOT_DEFINED", "Stats type not defined")
	ErrUnableToConsume              = errors.InternalServerError("CANT_CONSUME", "can't consume the given producer")
	ErrConsumerOrProducerIDRequired = errors.BadRequest("VALIDATION_ERROR", "one of consumer or producer id is required")
	ErrConsumerNotFoundForProducer  = func(pid string) error {
		return errors.NotFound("CONSUMER_NOT_FOUND", fmt.Sprintf("consumer not found for producer(%s)", pid))
	}
)
