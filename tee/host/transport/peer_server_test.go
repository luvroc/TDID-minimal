package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	sharedtypes "tdid-final/shared/types"
)

type fakeExecutionHandler struct {
	calls int
}

func (h *fakeExecutionHandler) HandlePeerExecution(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) (sharedtypes.CrossChainExecutionResponse, error) {
	_ = ctx
	h.calls++
	return sharedtypes.CrossChainExecutionResponse{TraceID: req.TraceID, Accepted: true}, nil
}

func TestValidateCrossChainRequest(t *testing.T) {
	req := sharedtypes.CrossChainExecutionRequest{
		TraceID:        "0xtrace",
		Asset:          "asset",
		Amount:         "1",
		Sender:         "alice",
		Recipient:      "bob",
		SrcChainID:     "fisco",
		DstChainID:     "fabric",
		SrcPayloadHash: "0x11",
		SrcSessSig:     "0x22",
	}
	if err := validateCrossChainRequest(req); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}
	req.TraceID = ""
	if err := validateCrossChainRequest(req); err == nil {
		t.Fatalf("expected validation error when traceId empty")
	}
}

func TestExecuteUsesTraceIdIdempotency(t *testing.T) {
	h := &fakeExecutionHandler{}
	s := &PeerServer{handler: h, auth: NewPeerAuth(nil), seenByTraceID: map[string]sharedtypes.CrossChainExecutionResponse{}}

	req := &ExecuteRequest{Request: sharedtypes.CrossChainExecutionRequest{
		TraceID:        "0xtrace",
		Asset:          "asset",
		Amount:         "1",
		Sender:         "alice",
		Recipient:      "bob",
		SrcChainID:     "fisco",
		DstChainID:     "fabric",
		SrcPayloadHash: "0x11",
		SrcSessSig:     "0x22",
	}}
	resp1, err := s.execute(context.Background(), req)
	if err != nil {
		t.Fatalf("execute #1 failed: %v", err)
	}
	resp2, err := s.execute(context.Background(), req)
	if err != nil {
		t.Fatalf("execute #2 failed: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("expected handler called once due to idempotency, got %d", h.calls)
	}
	if !resp1.Response.Accepted || !resp2.Response.Accepted {
		t.Fatalf("expected accepted responses")
	}
}

func TestPeerAuthAllowList(t *testing.T) {
	auth := NewPeerAuth([]string{"tee-b-node"})
	if err := auth.VerifyCommonName("tee-b-node"); err != nil {
		t.Fatalf("expected allowlisted CN to pass, got %v", err)
	}
	if err := auth.VerifyCommonName("unknown"); err == nil {
		t.Fatalf("expected unknown CN to be rejected")
	}
}

func TestUnaryAuthInterceptorRejectsUnknownCertCN(t *testing.T) {
	s := &PeerServer{auth: NewPeerAuth([]string{"tee-b-node"})}
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tlsStateWithCN("unknown-node")},
	})
	_, err := s.unaryAuthInterceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	if err == nil {
		t.Fatalf("expected unknown CN to be rejected")
	}
}

func TestUnaryAuthInterceptorAllowsAllowlistedCertCN(t *testing.T) {
	s := &PeerServer{auth: NewPeerAuth([]string{"tee-b-node"})}
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tlsStateWithCN("tee-b-node")},
	})
	called := false
	_, err := s.unaryAuthInterceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		called = true
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("expected allowlisted CN to pass, got %v", err)
	}
	if !called {
		t.Fatalf("expected handler to be called")
	}
}

func tlsStateWithCN(cn string) tls.ConnectionState {
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: cn}}
	return tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
}
