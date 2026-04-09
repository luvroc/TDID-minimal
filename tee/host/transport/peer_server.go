package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/peer"

	sharedtypes "tdid-final/shared/types"
)

type ExecutionHandler interface {
	HandlePeerExecution(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) (sharedtypes.CrossChainExecutionResponse, error)
}

type PeerServer struct {
	grpcServer *grpc.Server
	listener   net.Listener
	handler    ExecutionHandler
	auth       *PeerAuth

	mu            sync.Mutex
	seenByTraceID map[string]sharedtypes.CrossChainExecutionResponse
}

func NewPeerServer(bindAddr string, tlsConfig *tls.Config, allowList []string, handler ExecutionHandler) (*PeerServer, error) {
	if handler == nil {
		return nil, fmt.Errorf("execution handler is required")
	}
	lis, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}
	encoding.RegisterCodec(jsonCodec{})

	s := &PeerServer{
		listener:      lis,
		handler:       handler,
		auth:          NewPeerAuth(allowList),
		seenByTraceID: make(map[string]sharedtypes.CrossChainExecutionResponse),
	}
	opts := []grpc.ServerOption{grpc.ForceServerCodec(jsonCodec{}), grpc.UnaryInterceptor(s.unaryAuthInterceptor)}
	if tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	s.grpcServer = grpc.NewServer(opts...)
	s.grpcServer.RegisterService(&grpc.ServiceDesc{
		ServiceName: peerServiceName,
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: peerExecuteMethod,
				Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
					in := new(ExecuteRequest)
					if err := dec(in); err != nil {
						return nil, err
					}
					h := func(ctx context.Context, req any) (any, error) {
						return s.execute(ctx, req.(*ExecuteRequest))
					}
					if interceptor == nil {
						return h(ctx, in)
					}
					return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: peerExecuteFullRoute}, h)
				},
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "peer.json",
	}, s)

	return s, nil
}

func (s *PeerServer) Start() error {
	if s == nil || s.grpcServer == nil || s.listener == nil {
		return fmt.Errorf("peer server is not initialized")
	}
	return s.grpcServer.Serve(s.listener)
}

func (s *PeerServer) Stop() {
	if s == nil || s.grpcServer == nil {
		return
	}
	s.grpcServer.GracefulStop()
}

func (s *PeerServer) execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if err := validateCrossChainRequest(req.Request); err != nil {
		return nil, err
	}

	s.mu.Lock()
	if cached, ok := s.seenByTraceID[req.Request.TraceID]; ok {
		s.mu.Unlock()
		return &ExecuteResponse{Response: cached}, nil
	}
	s.mu.Unlock()

	resp, err := s.handler.HandlePeerExecution(ctx, req.Request)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.seenByTraceID[req.Request.TraceID] = resp
	s.mu.Unlock()

	return &ExecuteResponse{Response: resp}, nil
}

func (s *PeerServer) unaryAuthInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("peer info missing")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if ok && len(tlsInfo.State.PeerCertificates) > 0 {
		if err := s.auth.VerifyCommonName(tlsInfo.State.PeerCertificates[0].Subject.CommonName); err != nil {
			return nil, err
		}
	}
	return handler(ctx, req)
}

func validateCrossChainRequest(req sharedtypes.CrossChainExecutionRequest) error {
	if req.TraceID == "" {
		return fmt.Errorf("traceId is required")
	}
	if req.SrcChainID == "" || req.DstChainID == "" {
		return fmt.Errorf("srcChainId and dstChainId are required")
	}
	if req.Asset == "" || req.Amount == "" || req.Sender == "" || req.Recipient == "" {
		return fmt.Errorf("asset, amount, sender, recipient are required")
	}
	if req.SrcPayloadHash == "" || req.SrcSessSig == "" {
		return fmt.Errorf("srcPayloadHash and srcSessSig are required")
	}
	return nil
}
