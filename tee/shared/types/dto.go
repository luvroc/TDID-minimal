package types

type ChainKind string

const (
	ChainFabric ChainKind = "fabric"
	ChainFISCO  ChainKind = "fisco"
)

type ActionType string

const (
	ActionLock         ActionType = "LOCK"
	ActionMintOrUnlock ActionType = "MINT_OR_UNLOCK"
	ActionRefundV2     ActionType = "REFUND_V2"
)

type SignLockRequest struct {
	Chain      ChainKind
	SessionID  string
	TransferID string
	TraceID    string
	SrcChainID string
	DstChainID string
	Asset      string
	Amount     string
	Sender     string
	Recipient  string
	KeyID      string
	Nonce      uint64
	ExpireAt   int64
}

type SignMintOrUnlockRequest struct {
	Chain      ChainKind
	SessionID  string
	TransferID string
	TraceID    string
	SrcChainID string
	DstChainID string
	Asset      string
	Amount     string
	Sender     string
	Recipient  string
	KeyID      string
	Nonce      uint64
	ExpireAt   int64
}

type SignRefundV2Request struct {
	Chain      ChainKind
	SessionID  string
	TransferID string
	TraceID    string
	KeyID      string
	Nonce      uint64
	ExpireAt   int64
}

type BindSessionRequest struct {
	Chain        ChainKind
	ChainID      string
	ContractAddr string
	ExpireAt     int64
	RatchetSeed  []byte
}

type BindSessionResponse struct {
	SessionID string
	KeyID     string
	PublicKey []byte
	ExpireAt  int64
	ChainID   string
	BindHash  []byte
	BindSig   []byte
}

type CurrentSessionResponse struct {
	SessionID    string
	KeyID        string
	PublicKey    []byte
	ExpireAt     int64
	ChainID      string
	ContractAddr string
}

type NodeIdentity struct {
	PublicKey []byte
	Address   string
}

type SignedPayload struct {
	SessionID   string
	TransferID  string
	PayloadHash []byte
	SessSig     []byte
	KeyID       string
	Nonce       uint64
	ExpireAt    int64
}

type BuildReceiptRequest struct {
	TransferID  string
	TraceID     string
	TxHash      string
	ChainID     string
	Amount      string
	Recipient   string
	PayloadHash string
	FinalState  string
	SrcChainID  string
	DstChainID  string
}

type BuildReceiptResponse struct {
	TransferID     string
	TraceID        string
	ReceiptHash    []byte
	ReceiptHashHex string
}

type CrossChainExecutionRequest struct {
	SessionID       string
	TransferID      string
	RequestDigest   string
	TraceID         string
	Asset           string
	Amount          string
	Sender          string
	Recipient       string
	SrcChainID      string
	DstChainID      string
	KeyID           string
	Nonce           uint64
	ExpireAt        int64
	SrcLockTx       string
	SrcReceipt      string
	SrcPayloadHash  string
	SrcSessSig      string
	SourceLockProof string
	Timestamp       int64
}

type CrossChainExecutionResponse struct {
	TransferID      string
	TraceID         string
	Accepted        bool
	Reason          string
	TargetChainTx   string
	TargetReceipt   string
	TargetChainID   string
	TargetChainHash string
}

type TargetExecutionEvidenceRequest struct {
	TraceID          string
	TransferID       string
	DstChainID       string
	TargetTraceID    string
	TargetTransferID string
	TargetChainTx    string
	TargetReceipt    string
	TargetChainID    string
	TargetChainHash  string
}
