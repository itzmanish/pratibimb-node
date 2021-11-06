// Code generated by protoc-gen-micro. DO NOT EDIT.
// source: node/v1/node.proto

package v1

import (
	fmt "fmt"
	proto "google.golang.org/protobuf/proto"
	math "math"
)

import (
	context "context"
	api "github.com/itzmanish/go-micro/v2/api"
	client "github.com/itzmanish/go-micro/v2/client"
	server "github.com/itzmanish/go-micro/v2/server"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// Reference imports to suppress errors if they are not otherwise used.
var _ api.Endpoint
var _ context.Context
var _ client.Option
var _ server.Option

// Api Endpoints for NodeService service

func NewNodeServiceEndpoints() []*api.Endpoint {
	return []*api.Endpoint{}
}

// Client API for NodeService service

type NodeService interface {
	Health(ctx context.Context, in *HealthRequest, opts ...client.CallOption) (*HealthResponse, error)
	// Actual rpc methods for creating peers
	CreatePeer(ctx context.Context, in *CreatePeerRequest, opts ...client.CallOption) (*CreatePeerResponse, error)
	ClosePeer(ctx context.Context, in *ClosePeerRequest, opts ...client.CallOption) (*ClosePeerResponse, error)
	GetRouterCapabilities(ctx context.Context, in *GetRouterCapabilitiesRequest, opts ...client.CallOption) (*GetRouterCapabilitiesResponse, error)
	Join(ctx context.Context, in *JoinPeerRequest, opts ...client.CallOption) (*JoinPeerResponse, error)
	CreateWebRtcTransport(ctx context.Context, in *CreateWebRtcTransportRequest, opts ...client.CallOption) (*CreateWebRtcTransportResponse, error)
	ConnectWebRtcTransport(ctx context.Context, in *ConnectWebRtcTransportRequest, opts ...client.CallOption) (*ConnectWebRtcTransportResponse, error)
	RestartIce(ctx context.Context, in *RestartIceRequest, opts ...client.CallOption) (*RestartIceResponse, error)
	ProducerCanConsume(ctx context.Context, in *ProducerCanConsumeRequest, opts ...client.CallOption) (*ProducerCanConsumeResponse, error)
	Produce(ctx context.Context, in *ProduceRequest, opts ...client.CallOption) (*ProduceResponse, error)
	// producer action will have pause/resume/close actions.
	ProducerAction(ctx context.Context, in *ProducerActionRequest, opts ...client.CallOption) (*ProducerActionResponse, error)
	Consume(ctx context.Context, in *ConsumeRequest, opts ...client.CallOption) (*ConsumeResponse, error)
	// consumer action will have pause/resume/close actions.
	ConsumerAction(ctx context.Context, in *ConsumerActionRequest, opts ...client.CallOption) (*ConsumerActionResponse, error)
	// Get stats of transport/consumer/producer
	GetStats(ctx context.Context, in *GetStatsRequest, opts ...client.CallOption) (*GetStatsResponse, error)
	// Check if router has the producer already
	PeerRouterHasProducer(ctx context.Context, in *PeerRouterHasProducerRequest, opts ...client.CallOption) (*PeerRouterHasProducerResponse, error)
	// PipeTransports for one node to another
	CreatePipeTransport(ctx context.Context, in *CreatePipeTransportRequest, opts ...client.CallOption) (*CreatePipeTransportResponse, error)
	ConnectPipeTransport(ctx context.Context, in *ConnectPipeTransportRequest, opts ...client.CallOption) (*ConnectPipeTransportResponse, error)
	ProducePipeTransport(ctx context.Context, in *ProducePipeTransportRequest, opts ...client.CallOption) (*ProducePipeTransportResponse, error)
	ConsumePipeTransport(ctx context.Context, in *ConsumePipeTransportRequest, opts ...client.CallOption) (*ConsumePipeTransportResponse, error)
}

type nodeService struct {
	c    client.Client
	name string
}

func NewNodeService(name string, c client.Client) NodeService {
	return &nodeService{
		c:    c,
		name: name,
	}
}

func (c *nodeService) Health(ctx context.Context, in *HealthRequest, opts ...client.CallOption) (*HealthResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.Health", in)
	out := new(HealthResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) CreatePeer(ctx context.Context, in *CreatePeerRequest, opts ...client.CallOption) (*CreatePeerResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.CreatePeer", in)
	out := new(CreatePeerResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) ClosePeer(ctx context.Context, in *ClosePeerRequest, opts ...client.CallOption) (*ClosePeerResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.ClosePeer", in)
	out := new(ClosePeerResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) GetRouterCapabilities(ctx context.Context, in *GetRouterCapabilitiesRequest, opts ...client.CallOption) (*GetRouterCapabilitiesResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.GetRouterCapabilities", in)
	out := new(GetRouterCapabilitiesResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) Join(ctx context.Context, in *JoinPeerRequest, opts ...client.CallOption) (*JoinPeerResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.Join", in)
	out := new(JoinPeerResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) CreateWebRtcTransport(ctx context.Context, in *CreateWebRtcTransportRequest, opts ...client.CallOption) (*CreateWebRtcTransportResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.CreateWebRtcTransport", in)
	out := new(CreateWebRtcTransportResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) ConnectWebRtcTransport(ctx context.Context, in *ConnectWebRtcTransportRequest, opts ...client.CallOption) (*ConnectWebRtcTransportResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.ConnectWebRtcTransport", in)
	out := new(ConnectWebRtcTransportResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) RestartIce(ctx context.Context, in *RestartIceRequest, opts ...client.CallOption) (*RestartIceResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.RestartIce", in)
	out := new(RestartIceResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) ProducerCanConsume(ctx context.Context, in *ProducerCanConsumeRequest, opts ...client.CallOption) (*ProducerCanConsumeResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.ProducerCanConsume", in)
	out := new(ProducerCanConsumeResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) Produce(ctx context.Context, in *ProduceRequest, opts ...client.CallOption) (*ProduceResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.Produce", in)
	out := new(ProduceResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) ProducerAction(ctx context.Context, in *ProducerActionRequest, opts ...client.CallOption) (*ProducerActionResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.ProducerAction", in)
	out := new(ProducerActionResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) Consume(ctx context.Context, in *ConsumeRequest, opts ...client.CallOption) (*ConsumeResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.Consume", in)
	out := new(ConsumeResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) ConsumerAction(ctx context.Context, in *ConsumerActionRequest, opts ...client.CallOption) (*ConsumerActionResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.ConsumerAction", in)
	out := new(ConsumerActionResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) GetStats(ctx context.Context, in *GetStatsRequest, opts ...client.CallOption) (*GetStatsResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.GetStats", in)
	out := new(GetStatsResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) PeerRouterHasProducer(ctx context.Context, in *PeerRouterHasProducerRequest, opts ...client.CallOption) (*PeerRouterHasProducerResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.PeerRouterHasProducer", in)
	out := new(PeerRouterHasProducerResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) CreatePipeTransport(ctx context.Context, in *CreatePipeTransportRequest, opts ...client.CallOption) (*CreatePipeTransportResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.CreatePipeTransport", in)
	out := new(CreatePipeTransportResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) ConnectPipeTransport(ctx context.Context, in *ConnectPipeTransportRequest, opts ...client.CallOption) (*ConnectPipeTransportResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.ConnectPipeTransport", in)
	out := new(ConnectPipeTransportResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) ProducePipeTransport(ctx context.Context, in *ProducePipeTransportRequest, opts ...client.CallOption) (*ProducePipeTransportResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.ProducePipeTransport", in)
	out := new(ProducePipeTransportResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *nodeService) ConsumePipeTransport(ctx context.Context, in *ConsumePipeTransportRequest, opts ...client.CallOption) (*ConsumePipeTransportResponse, error) {
	req := c.c.NewRequest(c.name, "NodeService.ConsumePipeTransport", in)
	out := new(ConsumePipeTransportResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for NodeService service

type NodeServiceHandler interface {
	Health(context.Context, *HealthRequest, *HealthResponse) error
	// Actual rpc methods for creating peers
	CreatePeer(context.Context, *CreatePeerRequest, *CreatePeerResponse) error
	ClosePeer(context.Context, *ClosePeerRequest, *ClosePeerResponse) error
	GetRouterCapabilities(context.Context, *GetRouterCapabilitiesRequest, *GetRouterCapabilitiesResponse) error
	Join(context.Context, *JoinPeerRequest, *JoinPeerResponse) error
	CreateWebRtcTransport(context.Context, *CreateWebRtcTransportRequest, *CreateWebRtcTransportResponse) error
	ConnectWebRtcTransport(context.Context, *ConnectWebRtcTransportRequest, *ConnectWebRtcTransportResponse) error
	RestartIce(context.Context, *RestartIceRequest, *RestartIceResponse) error
	ProducerCanConsume(context.Context, *ProducerCanConsumeRequest, *ProducerCanConsumeResponse) error
	Produce(context.Context, *ProduceRequest, *ProduceResponse) error
	// producer action will have pause/resume/close actions.
	ProducerAction(context.Context, *ProducerActionRequest, *ProducerActionResponse) error
	Consume(context.Context, *ConsumeRequest, *ConsumeResponse) error
	// consumer action will have pause/resume/close actions.
	ConsumerAction(context.Context, *ConsumerActionRequest, *ConsumerActionResponse) error
	// Get stats of transport/consumer/producer
	GetStats(context.Context, *GetStatsRequest, *GetStatsResponse) error
	// Check if router has the producer already
	PeerRouterHasProducer(context.Context, *PeerRouterHasProducerRequest, *PeerRouterHasProducerResponse) error
	// PipeTransports for one node to another
	CreatePipeTransport(context.Context, *CreatePipeTransportRequest, *CreatePipeTransportResponse) error
	ConnectPipeTransport(context.Context, *ConnectPipeTransportRequest, *ConnectPipeTransportResponse) error
	ProducePipeTransport(context.Context, *ProducePipeTransportRequest, *ProducePipeTransportResponse) error
	ConsumePipeTransport(context.Context, *ConsumePipeTransportRequest, *ConsumePipeTransportResponse) error
}

func RegisterNodeServiceHandler(s server.Server, hdlr NodeServiceHandler, opts ...server.HandlerOption) error {
	type nodeService interface {
		Health(ctx context.Context, in *HealthRequest, out *HealthResponse) error
		CreatePeer(ctx context.Context, in *CreatePeerRequest, out *CreatePeerResponse) error
		ClosePeer(ctx context.Context, in *ClosePeerRequest, out *ClosePeerResponse) error
		GetRouterCapabilities(ctx context.Context, in *GetRouterCapabilitiesRequest, out *GetRouterCapabilitiesResponse) error
		Join(ctx context.Context, in *JoinPeerRequest, out *JoinPeerResponse) error
		CreateWebRtcTransport(ctx context.Context, in *CreateWebRtcTransportRequest, out *CreateWebRtcTransportResponse) error
		ConnectWebRtcTransport(ctx context.Context, in *ConnectWebRtcTransportRequest, out *ConnectWebRtcTransportResponse) error
		RestartIce(ctx context.Context, in *RestartIceRequest, out *RestartIceResponse) error
		ProducerCanConsume(ctx context.Context, in *ProducerCanConsumeRequest, out *ProducerCanConsumeResponse) error
		Produce(ctx context.Context, in *ProduceRequest, out *ProduceResponse) error
		ProducerAction(ctx context.Context, in *ProducerActionRequest, out *ProducerActionResponse) error
		Consume(ctx context.Context, in *ConsumeRequest, out *ConsumeResponse) error
		ConsumerAction(ctx context.Context, in *ConsumerActionRequest, out *ConsumerActionResponse) error
		GetStats(ctx context.Context, in *GetStatsRequest, out *GetStatsResponse) error
		PeerRouterHasProducer(ctx context.Context, in *PeerRouterHasProducerRequest, out *PeerRouterHasProducerResponse) error
		CreatePipeTransport(ctx context.Context, in *CreatePipeTransportRequest, out *CreatePipeTransportResponse) error
		ConnectPipeTransport(ctx context.Context, in *ConnectPipeTransportRequest, out *ConnectPipeTransportResponse) error
		ProducePipeTransport(ctx context.Context, in *ProducePipeTransportRequest, out *ProducePipeTransportResponse) error
		ConsumePipeTransport(ctx context.Context, in *ConsumePipeTransportRequest, out *ConsumePipeTransportResponse) error
	}
	type NodeService struct {
		nodeService
	}
	h := &nodeServiceHandler{hdlr}
	return s.Handle(s.NewHandler(&NodeService{h}, opts...))
}

type nodeServiceHandler struct {
	NodeServiceHandler
}

func (h *nodeServiceHandler) Health(ctx context.Context, in *HealthRequest, out *HealthResponse) error {
	return h.NodeServiceHandler.Health(ctx, in, out)
}

func (h *nodeServiceHandler) CreatePeer(ctx context.Context, in *CreatePeerRequest, out *CreatePeerResponse) error {
	return h.NodeServiceHandler.CreatePeer(ctx, in, out)
}

func (h *nodeServiceHandler) ClosePeer(ctx context.Context, in *ClosePeerRequest, out *ClosePeerResponse) error {
	return h.NodeServiceHandler.ClosePeer(ctx, in, out)
}

func (h *nodeServiceHandler) GetRouterCapabilities(ctx context.Context, in *GetRouterCapabilitiesRequest, out *GetRouterCapabilitiesResponse) error {
	return h.NodeServiceHandler.GetRouterCapabilities(ctx, in, out)
}

func (h *nodeServiceHandler) Join(ctx context.Context, in *JoinPeerRequest, out *JoinPeerResponse) error {
	return h.NodeServiceHandler.Join(ctx, in, out)
}

func (h *nodeServiceHandler) CreateWebRtcTransport(ctx context.Context, in *CreateWebRtcTransportRequest, out *CreateWebRtcTransportResponse) error {
	return h.NodeServiceHandler.CreateWebRtcTransport(ctx, in, out)
}

func (h *nodeServiceHandler) ConnectWebRtcTransport(ctx context.Context, in *ConnectWebRtcTransportRequest, out *ConnectWebRtcTransportResponse) error {
	return h.NodeServiceHandler.ConnectWebRtcTransport(ctx, in, out)
}

func (h *nodeServiceHandler) RestartIce(ctx context.Context, in *RestartIceRequest, out *RestartIceResponse) error {
	return h.NodeServiceHandler.RestartIce(ctx, in, out)
}

func (h *nodeServiceHandler) ProducerCanConsume(ctx context.Context, in *ProducerCanConsumeRequest, out *ProducerCanConsumeResponse) error {
	return h.NodeServiceHandler.ProducerCanConsume(ctx, in, out)
}

func (h *nodeServiceHandler) Produce(ctx context.Context, in *ProduceRequest, out *ProduceResponse) error {
	return h.NodeServiceHandler.Produce(ctx, in, out)
}

func (h *nodeServiceHandler) ProducerAction(ctx context.Context, in *ProducerActionRequest, out *ProducerActionResponse) error {
	return h.NodeServiceHandler.ProducerAction(ctx, in, out)
}

func (h *nodeServiceHandler) Consume(ctx context.Context, in *ConsumeRequest, out *ConsumeResponse) error {
	return h.NodeServiceHandler.Consume(ctx, in, out)
}

func (h *nodeServiceHandler) ConsumerAction(ctx context.Context, in *ConsumerActionRequest, out *ConsumerActionResponse) error {
	return h.NodeServiceHandler.ConsumerAction(ctx, in, out)
}

func (h *nodeServiceHandler) GetStats(ctx context.Context, in *GetStatsRequest, out *GetStatsResponse) error {
	return h.NodeServiceHandler.GetStats(ctx, in, out)
}

func (h *nodeServiceHandler) PeerRouterHasProducer(ctx context.Context, in *PeerRouterHasProducerRequest, out *PeerRouterHasProducerResponse) error {
	return h.NodeServiceHandler.PeerRouterHasProducer(ctx, in, out)
}

func (h *nodeServiceHandler) CreatePipeTransport(ctx context.Context, in *CreatePipeTransportRequest, out *CreatePipeTransportResponse) error {
	return h.NodeServiceHandler.CreatePipeTransport(ctx, in, out)
}

func (h *nodeServiceHandler) ConnectPipeTransport(ctx context.Context, in *ConnectPipeTransportRequest, out *ConnectPipeTransportResponse) error {
	return h.NodeServiceHandler.ConnectPipeTransport(ctx, in, out)
}

func (h *nodeServiceHandler) ProducePipeTransport(ctx context.Context, in *ProducePipeTransportRequest, out *ProducePipeTransportResponse) error {
	return h.NodeServiceHandler.ProducePipeTransport(ctx, in, out)
}

func (h *nodeServiceHandler) ConsumePipeTransport(ctx context.Context, in *ConsumePipeTransportRequest, out *ConsumePipeTransportResponse) error {
	return h.NodeServiceHandler.ConsumePipeTransport(ctx, in, out)
}
