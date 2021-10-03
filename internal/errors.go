package internal

import (
	"fmt"

	"github.com/itzmanish/go-micro/v2/errors"
)

var (
	ErrInvalidSecret = errors.Unauthorized("ROOM_UNAUTHORIZED", "Invalid secret for given room")
	ErrMethodUnknown = errors.BadRequest("UNKNOWN_METHOD", "Method is unknown!")
	ErrPeerNotJoined = errors.NotFound("PEER_NOT_JOINED", "Peer not joined")
	ErrPeerNotFound  = func(id string) error {
		return errors.NotFound("PEER_NOT_FOUND", fmt.Sprintf(`Peer with id "%s" not found`, id))
	}
	ErrPeerNotAuthorized = errors.Unauthorized("PEER_UNAUTHORIZED", "Peer not authorized")
	ErrTransportNotFound = func(id string) error {
		return errors.NotFound("TRANSPORT_NOT_FOUND", fmt.Sprintf(`transport with id "%s" not found`, id))
	}
	ErrProducerNotFound = func(id string) error {
		return errors.NotFound("PRODUCER_NOT_FOUND", fmt.Sprintf(`Producer with id "%s" not found`, id))
	}
	ErrConsumerNotFound = func(id string) error {
		return errors.NotFound("CONSUMER_NOT_FOUND", fmt.Sprintf(`Consumer with id "%s" not found`, id))
	}
)
