package relay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"tdid-final/host/chain"
	"tdid-final/shared/errors"
	sharedtypes "tdid-final/shared/types"
)

type IntentRequest struct {
	TraceID      string
	SrcChainID   string
	DstChainID   string
	ContractAddr string
	Asset        string
	Amount       string
	Sender       string
	Recipient    string
	ExpireAt     int64
	RatchetSeed  []byte
}

type IntentResult struct {
	Node         sharedtypes.NodeIdentity        `json:"node"`
	Binding      sharedtypes.BindSessionResponse `json:"binding"`
	TransferID   string                          `json:"transferId"`
	RegisterInfo chain.ChainWriteResult          `json:"registerInfo"`
}

type IntentOrchestrator struct {
	enclave  EnclaveClient
	registry RegistrationClient
}

func NewIntentOrchestrator(enclave EnclaveClient, registry RegistrationClient) *IntentOrchestrator {
	return &IntentOrchestrator{enclave: enclave, registry: registry}
}

func (o *IntentOrchestrator) HandleIntent(ctx context.Context, req IntentRequest) (IntentResult, error) {
	if o == nil || o.enclave == nil {
		return IntentResult{}, errors.New(errors.CodeInternal, "intent orchestrator dependencies are not ready", nil)
	}
	if strings.TrimSpace(req.TraceID) == "" || strings.TrimSpace(req.SrcChainID) == "" || strings.TrimSpace(req.DstChainID) == "" {
		return IntentResult{}, errors.New(errors.CodeInvalidInput, "traceId/srcChainId/dstChainId are required", nil)
	}
	node, err := o.enclave.GetNodeIdentity(ctx)
	if err != nil {
		return IntentResult{}, err
	}
	if noSessionBaselineEnabled() {
		sessionID := "nosess:" + strings.TrimSpace(req.TraceID)
		binding := sharedtypes.BindSessionResponse{
			SessionID: sessionID,
			KeyID:     "0xnosession",
			ExpireAt:  req.ExpireAt,
			ChainID:   req.SrcChainID,
		}
		transferID := deriveIntentTransferID(binding.SessionID, req.TraceID, req.SrcChainID, req.DstChainID, req.Asset, req.Amount, req.Sender, req.Recipient)
		regInfo := chain.ChainWriteResult{
			SessionID:  binding.SessionID,
			TransferID: transferID,
			TraceID:    req.TraceID,
		}
		return IntentResult{Node: node, Binding: binding, TransferID: transferID, RegisterInfo: regInfo}, nil
	}
	binding, err := o.enclave.BindSession(ctx, sharedtypes.BindSessionRequest{Chain: chainFromID(req.SrcChainID), ChainID: req.SrcChainID, ContractAddr: req.ContractAddr, ExpireAt: req.ExpireAt, RatchetSeed: req.RatchetSeed})
	if err != nil {
		return IntentResult{}, err
	}
	transferID := deriveIntentTransferID(binding.SessionID, req.TraceID, req.SrcChainID, req.DstChainID, req.Asset, req.Amount, req.Sender, req.Recipient)
	regInfo := chain.ChainWriteResult{SessionID: binding.SessionID, TransferID: transferID, TraceID: req.TraceID}
	if noIDBindingBaselineEnabled() {
		return IntentResult{Node: node, Binding: binding, TransferID: transferID, RegisterInfo: regInfo}, nil
	}
	if o.registry != nil {
		regInfo, err = o.registry.RegisterSession(ctx, map[string]any{"sessionId": binding.SessionID, "transferId": transferID, "traceId": req.TraceID, "keyId": binding.KeyID, "chainId": binding.ChainID, "expireAt": binding.ExpireAt, "nodeAddress": node.Address})
		if err != nil {
			return IntentResult{}, err
		}
		if regInfo.SessionID == "" {
			regInfo.SessionID = binding.SessionID
		}
		if regInfo.TransferID == "" {
			regInfo.TransferID = transferID
		}
		if regInfo.TraceID == "" {
			regInfo.TraceID = req.TraceID
		}
	}
	return IntentResult{Node: node, Binding: binding, TransferID: transferID, RegisterInfo: regInfo}, nil
}

func deriveIntentTransferID(sessionID string, traceID string, srcChainID string, dstChainID string, asset string, amount string, sender string, recipient string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{sessionID, traceID, srcChainID, dstChainID, asset, amount, sender, recipient}, "|")))
	return "0x" + hex.EncodeToString(h[:])
}

func noSessionBaselineEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TDID_T3_NO_SESSION_BASELINE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func noIDBindingBaselineEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TDID_T6_NO_ID_BINDING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
