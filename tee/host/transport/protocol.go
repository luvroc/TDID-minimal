package transport

import (
	"encoding/json"

	sharedtypes "tdid-final/shared/types"
)

const (
	peerServiceName      = "tdid.transport.v1.PeerService"
	peerExecuteMethod    = "Execute"
	peerExecuteFullRoute = "/" + peerServiceName + "/" + peerExecuteMethod
)

type ExecuteRequest struct {
	Request sharedtypes.CrossChainExecutionRequest `json:"request"`
}

type ExecuteResponse struct {
	Response sharedtypes.CrossChainExecutionResponse `json:"response"`
}

type jsonCodec struct{}

func (jsonCodec) Name() string {
	return "json"
}

func (jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
