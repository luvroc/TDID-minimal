package relay

import (
	"context"

	"tdid-final/host/chain"
	sharedtypes "tdid-final/shared/types"
)

type EnclaveClient interface {
	GetNodeIdentity(ctx context.Context) (sharedtypes.NodeIdentity, error)
	BindSession(ctx context.Context, req sharedtypes.BindSessionRequest) (sharedtypes.BindSessionResponse, error)
	CurrentSession(ctx context.Context) (*sharedtypes.CurrentSessionResponse, error)
	SignLock(ctx context.Context, req sharedtypes.SignLockRequest) (sharedtypes.SignedPayload, error)
	SignMintOrUnlock(ctx context.Context, req sharedtypes.SignMintOrUnlockRequest) (sharedtypes.SignedPayload, error)
	SignRefundV2(ctx context.Context, req sharedtypes.SignRefundV2Request) (sharedtypes.SignedPayload, error)
	VerifyPeerCrossMessage(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) error
	VerifyTargetExecutionEvidence(ctx context.Context, req sharedtypes.TargetExecutionEvidenceRequest) error
	BuildReceipt(ctx context.Context, req sharedtypes.BuildReceiptRequest) (sharedtypes.BuildReceiptResponse, error)
}

type SourceChainClient interface {
	InvokeSourceLock(ctx context.Context, req chain.SourceLockRequest) (chain.ChainWriteResult, error)
	BuildSourceLockProof(ctx context.Context, req chain.SourceLockProofRequest) (string, error)
	EncodeSourceLockProofPayload(ctx context.Context, req chain.SourceLockProofPayloadRequest) (string, error)
	InvokeSourceCommit(ctx context.Context, req chain.SourceCommitRequest) (chain.ChainWriteResult, error)
}

type TargetChainClient interface {
	InvokeTargetExecute(ctx context.Context, req chain.TargetExecuteRequest) (chain.ChainWriteResult, error)
}

type AuditClient interface {
	SubmitSourceReceipt(ctx context.Context, req chain.AuditReceiptRequest) (chain.ChainWriteResult, error)
	SubmitTargetReceipt(ctx context.Context, req chain.AuditReceiptRequest) (chain.ChainWriteResult, error)
	MatchTransfer(ctx context.Context, req chain.AuditMatchRequest) (chain.ChainWriteResult, error)
}

type RegistrationClient interface {
	RegisterNode(ctx context.Context, payload map[string]any) (chain.ChainWriteResult, error)
	RegisterSession(ctx context.Context, payload map[string]any) (chain.ChainWriteResult, error)
}

type PeerSender interface {
	SendExecution(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) (sharedtypes.CrossChainExecutionResponse, error)
}
