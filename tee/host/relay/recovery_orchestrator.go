package relay

import (
	"context"
	"strings"

	"tdid-final/shared/errors"
	sharedtypes "tdid-final/shared/types"
)

type RecoveryRequest struct {
	SessionID  string
	TransferID string
	TraceID    string
	Chain      sharedtypes.ChainKind
	KeyID      string
	Nonce      uint64
	ExpireAt   int64
}

type RecoveryOrchestrator struct {
	enclave EnclaveClient
}

func NewRecoveryOrchestrator(enclave EnclaveClient) *RecoveryOrchestrator {
	return &RecoveryOrchestrator{enclave: enclave}
}

func (o *RecoveryOrchestrator) HandleRecovery(ctx context.Context, req RecoveryRequest) (sharedtypes.SignedPayload, error) {
	if o == nil || o.enclave == nil {
		return sharedtypes.SignedPayload{}, errors.New(errors.CodeInternal, "recovery orchestrator dependencies are not ready", nil)
	}
	if strings.TrimSpace(req.TransferID) == "" || strings.TrimSpace(req.TraceID) == "" || strings.TrimSpace(req.KeyID) == "" {
		return sharedtypes.SignedPayload{}, errors.New(errors.CodeInvalidInput, "transferId/traceId/keyId are required", nil)
	}
	return o.enclave.SignRefundV2(ctx, sharedtypes.SignRefundV2Request{Chain: req.Chain, SessionID: req.SessionID, TransferID: req.TransferID, TraceID: req.TraceID, KeyID: req.KeyID, Nonce: req.Nonce, ExpireAt: req.ExpireAt})
}
