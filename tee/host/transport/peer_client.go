package transport

import (
	"context"
	"crypto/tls"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding"

	sharedtypes "tdid-final/shared/types"
)

type PeerClient struct {
	conn       *grpc.ClientConn
	maxRetries int
	backoff    time.Duration
}

func NewPeerClient(addr string, tlsConfig *tls.Config, maxRetries int, backoff time.Duration) (*PeerClient, error) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if backoff <= 0 {
		backoff = 150 * time.Millisecond
	}
	encoding.RegisterCodec(jsonCodec{})
	opts := []grpc.DialOption{grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{}))}
	if tlsConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, err
	}
	return &PeerClient{conn: conn, maxRetries: maxRetries, backoff: backoff}, nil
}

func (c *PeerClient) SendExecution(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) (sharedtypes.CrossChainExecutionResponse, error) {
	var lastErr error
	attempts := c.maxRetries + 1
	for i := 0; i < attempts; i++ {
		out, err := c.sendOnce(ctx, req)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if i < attempts-1 {
			select {
			case <-ctx.Done():
				return sharedtypes.CrossChainExecutionResponse{}, ctx.Err()
			case <-time.After(c.backoff):
			}
		}
	}
	return sharedtypes.CrossChainExecutionResponse{}, lastErr
}

func (c *PeerClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *PeerClient) sendOnce(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) (sharedtypes.CrossChainExecutionResponse, error) {
	if c == nil || c.conn == nil {
		return sharedtypes.CrossChainExecutionResponse{}, grpc.ErrClientConnClosing
	}
	in := &ExecuteRequest{Request: req}
	out := &ExecuteResponse{}
	if err := c.conn.Invoke(ctx, peerExecuteFullRoute, in, out); err != nil {
		return sharedtypes.CrossChainExecutionResponse{}, err
	}
	return out.Response, nil
}
