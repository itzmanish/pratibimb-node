package handler

import (
	"context"

	"github.com/itzmanish/go-micro/v2/errors"
	"github.com/itzmanish/pratibimb-node/internal"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
)

type NodeService struct {
	ID         string
	wsEndpoint string
}

func NewNodeService(ep string) *NodeService {
	return &NodeService{
		wsEndpoint: ep,
	}
}

func (service *NodeService) Init(id string) {
	service.ID = id
}

func (service *NodeService) CreateNodeRoom(ctx context.Context, in *v1.CreateNodeRoomRequest, out *v1.CreateNodeRoomResponse) error {
	if len(in.Id) == 0 {
		return errors.BadRequest("MISSING_ROOM_ID", "Room id is missing.")
	}
	_, ok := rooms.Load(in.Id)
	if !ok {
		room, err := internal.NewRoom(mediasoupWorker, in.Id, in.Secret)
		if err != nil {
			return errors.InternalServerError("ROOM_CREATE_FAILED", "room creation failed with err: %v", err)
		}

		rooms.Store(in.Id, room)

		room.On("close", func() {
			rooms.Delete(in.Id)
			room.RemoveAllListeners()
		})
	}

	out.Message = "Room created."
	out.Status = "success"
	out.NodeId = service.ID
	out.WsEndpoint = service.wsEndpoint
	return nil
}
