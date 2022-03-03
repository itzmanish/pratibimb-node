package handler

import (
	"context"
	"encoding/json"

	log "github.com/itzmanish/go-micro/v2/logger"
	"github.com/itzmanish/pratibimb-node/internal"
	v1 "github.com/itzmanish/pratibimb-node/proto/gen/node/v1"
	"github.com/itzmanish/pratibimb-node/utils"
	"github.com/jiyeyuran/mediasoup-go"
)

func (h *NodeHandler) CreatePipeTransport(ctx context.Context, in *v1.CreatePipeTransportRequest, out *v1.CreatePipeTransportResponse) error {
	log.Debug("CreatePipeTransport() | req: ", in)
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}

	res, err := room.CreatePipeTransport(peer, mediasoup.PipeTransportOptions{
		ListenIp: mediasoup.TransportListenIp{
			Ip: utils.GetSelfIP(),
			// commenting out announced ip for now.
			// AnnouncedIp: internal.GetOutboundIP(),
		},
		EnableSctp: true,
		EnableRtx:  true,
		EnableSrtp: true,
	})
	if err != nil {
		return err
	}
	tuple, err := json.Marshal(res.Tuple)
	if err != nil {
		return err
	}
	srtpParameters, err := json.Marshal(res.SrtpParameters)
	if err != nil {
		return err
	}
	out.TransportId = res.TransportID
	out.SrtpParameters = srtpParameters
	out.Tuple = tuple
	return nil
}

func (h *NodeHandler) ConnectPipeTransport(ctx context.Context, in *v1.ConnectPipeTransportRequest, out *v1.ConnectPipeTransportResponse) error {
	log.Debug("ConnectPipeTransport() | req: ", in)
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	var transportTuple mediasoup.TransportTuple
	var srtpParameters mediasoup.SrtpParameters
	err = json.Unmarshal(in.SrtpParameters, &srtpParameters)
	if err != nil {
		return err
	}
	err = json.Unmarshal(in.Tuple, &transportTuple)
	if err != nil {
		return err
	}
	return room.ConnectPipeTransport(peer, internal.ConnectPipeTransportOptions{
		TransportID:    in.TransportId,
		Tuple:          transportTuple,
		SrtpParameters: &srtpParameters,
	})
}

func (h *NodeHandler) ProducePipeTransport(ctx context.Context, in *v1.ProducePipeTransportRequest, out *v1.ProducePipeTransportResponse) error {
	log.Debug("ProducePipeTransport() | req: ", in)
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	var rtpParameters mediasoup.RtpParameters
	var appData internal.H

	err = json.Unmarshal(in.RtpParameters, &rtpParameters)
	if err != nil {
		return err
	}

	err = json.Unmarshal(in.AppData, &appData)
	if err != nil {
		return err
	}

	err = room.ProducePipeTransport(peer, internal.ProducePipeTransportPayload{
		ProducerID:     in.ProducerId,
		Kind:           mediasoup.MediaKind(in.MediaKind),
		Paused:         in.Paused,
		TransportID:    in.TransportId,
		PipeConsumerID: in.PipeConsumerId,
		RtpParameters:  rtpParameters,
		AppData:        appData,
	})
	if err != nil {
		return err
	}
	// TODO: optimization is required here later
	// room.PipeProducerToJoinedPeersRouter(peer, in.ProducerId)
	return nil
}

func (h *NodeHandler) ConsumePipeTransport(ctx context.Context, in *v1.ConsumePipeTransportRequest, out *v1.ConsumePipeTransportResponse) error {
	log.Debug("ConsumePipeTransport() | req: ", in)
	room, peer, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	res, err := room.ConsumePipeTransport(peer, in.ProducerId, in.TransportId)
	if err != nil {
		return err
	}
	rtpParameters, err := json.Marshal(res.RtpParameters)
	if err != nil {
		return err
	}
	out.RtpParameters = rtpParameters
	out.MediaKind = string(res.Kind)
	out.Paused = res.Paused
	out.PipeConsumerId = res.PipeConsumerID
	out.ProducerId = res.ProducerID
	out.TransportId = res.TransportID
	return nil
}

func (h *NodeHandler) PipeProducerAction(ctx context.Context, in *v1.PipeProducerActionRequest, out *v1.PipeProducerActionResponse) error {
	log.Debug("PipeProducerAction() | req: ", in)
	room, _, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	return room.HandlePipeProducer(in.ProducerId, in.Action)
}

func (h *NodeHandler) ClosePipeTransport(ctx context.Context, in *v1.ClosePipeTransportRequest, out *v1.ClosePipeTransportResponse) error {
	log.Debug("ClosePipeTransport() | req: ", in)
	room, _, err := GetPeerAndRoom(in.BaseInfo.PeerId, in.BaseInfo.RoomId)
	if err != nil {
		return err
	}
	return room.ClosePipeTransport(in.TransportId)
}
