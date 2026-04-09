package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	stateLocked    = "LOCKED"
	stateCommitted = "COMMITTED"
	stateRefunded  = "REFUNDED"

	eventLockNew   = "LockCreated"
	eventSettleNew = "SettleCommitted"
	eventRefundNew = "RefundExecuted"

	eventLockCompat   = "Event_Lock"
	eventSettleCompat = "Event_Settle"
	eventRefundCompat = "Event_Refund"
	eventCommitNew    = "CommitExecuted"
	eventCommitCompat = "Event_Commit"

	keyTracePrefix          = "trace:"
	keyTransferPrefix       = "transfer:"
	keyTraceMapPrefix       = "trace2transfer:"
	keyProofPrefix          = "proof:source-lock:"
	keyNoncePrefix          = "nonce:"
	keyVerifierCCPrefix     = "cfg:verifier:cc"
	keyVerifierChannelPrefx = "cfg:verifier:channel"

	defaultVerifierCC = "mockverifiercc"
	proofSignerPrivK  = "59c6995e998f97a5a0044976f7d2cbb7d0c7f8f6ec6cf4de4df2f8f8b7f6d1f7"
	proofMaxAgeMillis = int64(10 * 60 * 1000)

	// Proof metadata for paper-prototype semantics clarity.
	// This explicitly marks Fabric proof as an attested lock snapshot,
	// not a canonical L1 inclusion proof.
	sourceLockProofSchemaVersion  = "fabric-source-lock-proof/v1"
	sourceLockProofSemanticLevel  = "attested_lock_snapshot"
	sourceLockProofSourceMode     = "fabric_chaincode_context"
	sourceLockProofBlockHeightRef = "tx_timestamp_millis_placeholder"
	sourceLockProofEventHashRef   = "deterministic_lock_payload_hash"
)

type GatewayContract struct {
	contractapi.Contract
}

type Trace struct {
	TransferID string `json:"transferId"`
	SessionID  string `json:"sessionId,omitempty"`
	TraceID    string `json:"traceId"`
	State      string `json:"state"`
	SrcChainID string `json:"srcChainId"`
	DstChainID string `json:"dstChainId"`
	Asset      string `json:"asset"`
	Amount     uint64 `json:"amount"`
	Sender     string `json:"sender"`
	Recipient  string `json:"recipient"`
	KeyID      string `json:"keyId"`
	Nonce      uint64 `json:"nonce"`
	ExpireAt   int64  `json:"expireAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

type LockEvent struct {
	TransferID string `json:"transferId"`
	SessionID  string `json:"sessionId,omitempty"`
	TraceID    string `json:"traceId"`
	SrcChainID string `json:"srcChainId"`
	DstChainID string `json:"dstChainId"`
	Asset      string `json:"asset"`
	Amount     uint64 `json:"amount"`
	Sender     string `json:"sender"`
	Recipient  string `json:"recipient"`
	KeyID      string `json:"keyId"`
	Nonce      uint64 `json:"nonce"`
	ExpireAt   int64  `json:"expireAt"`
	Payload    string `json:"payloadHash"`
	Timestamp  int64  `json:"timestamp"`
}

type SettleEvent struct {
	TransferID string `json:"transferId"`
	SessionID  string `json:"sessionId,omitempty"`
	TraceID    string `json:"traceId"`
	SrcChainID string `json:"srcChainId"`
	DstChainID string `json:"dstChainId"`
	Asset      string `json:"asset"`
	Amount     uint64 `json:"amount"`
	Sender     string `json:"sender"`
	Recipient  string `json:"recipient"`
	KeyID      string `json:"keyId"`
	Nonce      uint64 `json:"nonce"`
	ExpireAt   int64  `json:"expireAt"`
	Payload    string `json:"payloadHash"`
	Timestamp  int64  `json:"timestamp"`
}

type RefundEvent struct {
	TransferID string `json:"transferId"`
	TraceID    string `json:"traceId"`
	KeyID      string `json:"keyId"`
	Payload    string `json:"payloadHash"`
	Timestamp  int64  `json:"timestamp"`
}

type CommitEvent struct {
	TransferID      string `json:"transferId"`
	SessionID       string `json:"sessionId,omitempty"`
	TraceID         string `json:"traceId"`
	KeyID           string `json:"keyId"`
	TargetChainTx   string `json:"targetChainTx,omitempty"`
	TargetReceipt   string `json:"targetReceipt,omitempty"`
	TargetChainID   string `json:"targetChainId,omitempty"`
	TargetChainHash string `json:"targetChainHash,omitempty"`
	Payload         string `json:"payloadHash"`
	Timestamp       int64  `json:"timestamp"`
}

type SourceLockProof struct {
	TraceID        string `json:"traceId"`
	TransferID     string `json:"transferId"`
	SessionID      string `json:"sessionId,omitempty"`
	SrcChainID     string `json:"srcChainId"`
	LockState      string `json:"lockState"`
	BlockHeight    uint64 `json:"blockHeight"`
	TxHash         string `json:"txHash"`
	EventHash      string `json:"eventHash"`
	ProofTimestamp int64  `json:"proofTimestamp"`
	Attester       string `json:"attester"`
	Signer         string `json:"signer"`
	ProofDigest    string `json:"proofDigest"`
	ProofSig       string `json:"proofSig"`
	ProofSchema    string `json:"proofSchemaVersion,omitempty"`
	ProofLevel     string `json:"proofSemanticLevel,omitempty"`
	ProofSource    string `json:"proofSourceMode,omitempty"`
	BlockHeightRef string `json:"proofBlockHeightRef,omitempty"`
	EventHashRef   string `json:"proofEventHashRef,omitempty"`
}

func traceKey(traceID string) string {
	return keyTracePrefix + traceID
}

func transferKey(transferID string) string {
	return keyTransferPrefix + transferID
}

func traceToTransferKey(traceID string) string {
	return keyTraceMapPrefix + traceID
}

func sourceLockProofKey(transferID string) string {
	return keyProofPrefix + transferID
}

func nonceKey(keyID string, nonce uint64) string {
	return keyNoncePrefix + keyID + ":" + strconv.FormatUint(nonce, 10)
}

func (g *GatewayContract) SetSigVerifier(ctx contractapi.TransactionContextInterface, verifierCC, verifierChannel string) error {
	if verifierCC == "" {
		return errors.New("verifierCC cannot be empty")
	}
	if err := ctx.GetStub().PutState(keyVerifierCCPrefix, []byte(verifierCC)); err != nil {
		return err
	}
	return ctx.GetStub().PutState(keyVerifierChannelPrefx, []byte(verifierChannel))
}

func (g *GatewayContract) Lock(
	ctx contractapi.TransactionContextInterface,
	traceID, dstChainID, asset string,
	amount uint64,
	sender, recipient, keyID string,
	nonce uint64,
	expireAt int64,
	sessSig string,
) error {
	transferID := buildTransferID("", traceID, dstChainID, asset, amount, sender, recipient, keyID, nonce, expireAt)
	return g.LockV2(ctx, transferID, keyID, traceID, dstChainID, asset, amount, sender, recipient, keyID, nonce, expireAt, sessSig)
}

func (g *GatewayContract) LockV2(
	ctx contractapi.TransactionContextInterface,
	transferID, sessionID, traceID, dstChainID, asset string,
	amount uint64,
	sender, recipient, keyID string,
	nonce uint64,
	expireAt int64,
	sessSig string,
) error {
	if strings.TrimSpace(transferID) == "" {
		return errors.New("transferID cannot be empty")
	}
	if traceID == "" || dstChainID == "" || asset == "" || sender == "" || recipient == "" || keyID == "" || sessSig == "" {
		return errors.New("required parameters cannot be empty")
	}
	if amount == 0 {
		return errors.New("amount must be greater than 0")
	}
	if expireAt <= time.Now().Unix() {
		return errors.New("expireAt must be greater than current time")
	}

	if exists, err := g.transferExists(ctx, transferID); err != nil {
		return err
	} else if exists {
		return errors.New("transferId already exists")
	}
	if oldTransfer, err := g.getTransferIDByTrace(ctx, traceID); err != nil {
		return err
	} else if oldTransfer != "" {
		return errors.New("traceId already mapped")
	}
	if used, err := g.IsNonceUsed(ctx, keyID, nonce); err != nil {
		return err
	} else if used {
		return errors.New("nonce already used")
	}

	srcChainID := ctx.GetStub().GetChannelID()
	payloadHash := buildPayloadHashHex("LOCK", traceID, srcChainID, dstChainID, asset, amount, sender, recipient, keyID, nonce, expireAt)
	ok, err := g.verifySessionSig(ctx, keyID, payloadHash, sessSig)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("verifySessionSig failed")
	}

	now := time.Now().Unix()
	t := Trace{
		TransferID: transferID,
		SessionID:  sessionID,
		TraceID:    traceID,
		State:      stateLocked,
		SrcChainID: srcChainID,
		DstChainID: dstChainID,
		Asset:      asset,
		Amount:     amount,
		Sender:     sender,
		Recipient:  recipient,
		KeyID:      keyID,
		Nonce:      nonce,
		ExpireAt:   expireAt,
		UpdatedAt:  now,
	}
	if err := g.putTrace(ctx, t); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(traceToTransferKey(traceID), []byte(transferID)); err != nil {
		return err
	}
	if err := g.markNonceUsed(ctx, keyID, nonce); err != nil {
		return err
	}

	e := LockEvent{
		TransferID: transferID,
		SessionID:  sessionID,
		TraceID:    traceID,
		SrcChainID: srcChainID,
		DstChainID: dstChainID,
		Asset:      asset,
		Amount:     amount,
		Sender:     sender,
		Recipient:  recipient,
		KeyID:      keyID,
		Nonce:      nonce,
		ExpireAt:   expireAt,
		Payload:    payloadHash,
		Timestamp:  now,
	}
	return g.emitWithCompat(ctx, eventLockNew, eventLockCompat, e)
}

func (g *GatewayContract) Refund(ctx contractapi.TransactionContextInterface, traceID, keyID, _ string) error {
	return g.refundV2(ctx, traceID, keyID)
}

func (g *GatewayContract) RefundV2(ctx contractapi.TransactionContextInterface, traceID, keyID string) error {
	return g.refundV2(ctx, traceID, keyID)
}

func (g *GatewayContract) refundV2(ctx contractapi.TransactionContextInterface, traceID, keyID string) error {
	if traceID == "" || keyID == "" {
		return errors.New("traceID/keyID cannot be empty")
	}
	t, err := g.getTrace(ctx, traceID)
	if err != nil {
		return err
	}
	if t.State != stateLocked {
		return fmt.Errorf("trace %s not in LOCKED state", traceID)
	}
	if t.KeyID != keyID {
		return errors.New("keyID mismatch")
	}
	if time.Now().Unix() <= t.ExpireAt {
		return errors.New("refund only allowed after expireAt")
	}

	payloadHash := buildPayloadHashHex("REFUND", t.TraceID, t.SrcChainID, t.DstChainID, t.Asset, t.Amount, t.Sender, t.Recipient, t.KeyID, t.Nonce, t.ExpireAt)

	now := time.Now().Unix()
	t.State = stateRefunded
	t.UpdatedAt = now
	if err := g.putTrace(ctx, t); err != nil {
		return err
	}

	return g.emitWithCompat(ctx, eventRefundNew, eventRefundCompat, RefundEvent{TransferID: t.TransferID, TraceID: traceID, KeyID: keyID, Payload: payloadHash, Timestamp: now})
}

func (g *GatewayContract) Commit(ctx contractapi.TransactionContextInterface, traceID, keyID, targetChainTx, targetReceipt, targetChainID, targetChainHash string) error {
	return g.commitV2(ctx, traceID, keyID, targetChainTx, targetReceipt, targetChainID, targetChainHash)
}

func (g *GatewayContract) CommitV2(ctx contractapi.TransactionContextInterface, traceID, keyID, targetChainTx, targetReceipt, targetChainID, targetChainHash string) error {
	return g.commitV2(ctx, traceID, keyID, targetChainTx, targetReceipt, targetChainID, targetChainHash)
}

func (g *GatewayContract) commitV2(ctx contractapi.TransactionContextInterface, traceID, keyID, targetChainTx, targetReceipt, targetChainID, targetChainHash string) error {
	if traceID == "" || keyID == "" {
		return errors.New("traceID/keyID cannot be empty")
	}
	if strings.TrimSpace(targetChainTx) == "" || strings.TrimSpace(targetReceipt) == "" {
		return errors.New("targetChainTx/targetReceipt cannot be empty")
	}
	if strings.TrimSpace(targetChainID) == "" || strings.TrimSpace(targetChainHash) == "" {
		return errors.New("targetChainID/targetChainHash cannot be empty")
	}
	t, err := g.getTrace(ctx, traceID)
	if err != nil {
		return err
	}
	if t.State != stateLocked {
		return fmt.Errorf("trace %s not in LOCKED state", traceID)
	}
	if t.KeyID != keyID {
		return errors.New("keyID mismatch")
	}

	now := time.Now().Unix()
	t.State = stateCommitted
	t.UpdatedAt = now
	if err := g.putTrace(ctx, t); err != nil {
		return err
	}

	payloadHash := buildPayloadHashHex("COMMIT", t.TraceID, t.SrcChainID, t.DstChainID, t.Asset, t.Amount, t.Sender, t.Recipient, t.KeyID, t.Nonce, t.ExpireAt)
	return g.emitWithCompat(ctx, eventCommitNew, eventCommitCompat, CommitEvent{
		TransferID:      t.TransferID,
		SessionID:       t.SessionID,
		TraceID:         traceID,
		KeyID:           keyID,
		TargetChainTx:   targetChainTx,
		TargetReceipt:   targetReceipt,
		TargetChainID:   targetChainID,
		TargetChainHash: targetChainHash,
		Payload:         payloadHash,
		Timestamp:       now,
	})
}

func (g *GatewayContract) MintOrUnlock(
	ctx contractapi.TransactionContextInterface,
	traceID, srcChainID, asset string,
	amount uint64,
	sender, recipient, keyID string,
	nonce uint64,
	expireAt int64,
	sessSig string,
) error {
	transferID := buildTransferID("", traceID, srcChainID, asset, amount, sender, recipient, keyID, nonce, expireAt)
	return g.MintOrUnlockV2(ctx, transferID, keyID, traceID, srcChainID, asset, amount, sender, recipient, keyID, nonce, expireAt, sessSig)
}

func (g *GatewayContract) MintOrUnlockV2(
	ctx contractapi.TransactionContextInterface,
	transferID, sessionID, traceID, srcChainID, asset string,
	amount uint64,
	sender, recipient, keyID string,
	nonce uint64,
	expireAt int64,
	sessSig string,
) error {
	if strings.TrimSpace(transferID) == "" {
		return errors.New("transferID cannot be empty")
	}
	if traceID == "" || srcChainID == "" || asset == "" || sender == "" || recipient == "" || keyID == "" || sessSig == "" {
		return errors.New("required parameters cannot be empty")
	}
	if amount == 0 {
		return errors.New("amount must be greater than 0")
	}
	if expireAt <= time.Now().Unix() {
		return errors.New("expireAt must be greater than current time")
	}

	if exists, err := g.transferExists(ctx, transferID); err != nil {
		return err
	} else if exists {
		return errors.New("transferId already exists")
	}
	if oldTransfer, err := g.getTransferIDByTrace(ctx, traceID); err != nil {
		return err
	} else if oldTransfer != "" {
		return errors.New("traceId already mapped")
	}
	if used, err := g.IsNonceUsed(ctx, keyID, nonce); err != nil {
		return err
	} else if used {
		return errors.New("nonce already used")
	}

	dstChainID := ctx.GetStub().GetChannelID()
	payloadHash := buildPayloadHashHex("SETTLE", traceID, srcChainID, dstChainID, asset, amount, sender, recipient, keyID, nonce, expireAt)
	ok, err := g.verifySessionSig(ctx, keyID, payloadHash, sessSig)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("verifySessionSig failed")
	}

	now := time.Now().Unix()
	t := Trace{
		TransferID: transferID,
		SessionID:  sessionID,
		TraceID:    traceID,
		State:      stateCommitted,
		SrcChainID: srcChainID,
		DstChainID: dstChainID,
		Asset:      asset,
		Amount:     amount,
		Sender:     sender,
		Recipient:  recipient,
		KeyID:      keyID,
		Nonce:      nonce,
		ExpireAt:   expireAt,
		UpdatedAt:  now,
	}
	if err := g.putTrace(ctx, t); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(traceToTransferKey(traceID), []byte(transferID)); err != nil {
		return err
	}
	if err := g.markNonceUsed(ctx, keyID, nonce); err != nil {
		return err
	}

	e := SettleEvent{
		TransferID: transferID,
		SessionID:  sessionID,
		TraceID:    traceID,
		SrcChainID: srcChainID,
		DstChainID: dstChainID,
		Asset:      asset,
		Amount:     amount,
		Sender:     sender,
		Recipient:  recipient,
		KeyID:      keyID,
		Nonce:      nonce,
		ExpireAt:   expireAt,
		Payload:    payloadHash,
		Timestamp:  now,
	}
	return g.emitWithCompat(ctx, eventSettleNew, eventSettleCompat, e)
}

func (g *GatewayContract) MintOrUnlockWithProof(
	ctx contractapi.TransactionContextInterface,
	transferID, sessionID, traceID, srcChainID, _ string,
	asset, amount, sender, recipient, keyID string,
	nonce uint64,
	expireAt int64,
	sessSig, _, _, _, _, proofPayload string,
) error {
	if strings.TrimSpace(proofPayload) == "" {
		return errors.New("proofPayload cannot be empty")
	}
	proof, err := decodeSourceLockProofPayload(proofPayload)
	if err != nil {
		return err
	}
	if err := validateSourceLockProof(proof); err != nil {
		return err
	}

	if strings.TrimSpace(traceID) == "" {
		traceID = proof.TraceID
	}
	if strings.TrimSpace(transferID) == "" {
		transferID = proof.TransferID
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = proof.SessionID
	}
	if strings.TrimSpace(srcChainID) == "" {
		srcChainID = proof.SrcChainID
	}

	if !strings.EqualFold(traceID, proof.TraceID) {
		return errors.New("proof traceId mismatch")
	}
	if !strings.EqualFold(transferID, proof.TransferID) {
		return errors.New("proof transferId mismatch")
	}
	if strings.TrimSpace(sessionID) != "" && !strings.EqualFold(sessionID, proof.SessionID) {
		return errors.New("proof sessionId mismatch")
	}
	if !strings.EqualFold(srcChainID, proof.SrcChainID) {
		return errors.New("proof srcChainId mismatch")
	}

	amount = strings.TrimSpace(amount)
	parsedAmount, parseErr := strconv.ParseUint(amount, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid amount: %w", parseErr)
	}

	return g.MintOrUnlockV2(
		ctx,
		transferID,
		sessionID,
		traceID,
		srcChainID,
		asset,
		parsedAmount,
		sender,
		recipient,
		keyID,
		nonce,
		expireAt,
		sessSig,
	)
}

func (g *GatewayContract) GetTrace(ctx contractapi.TransactionContextInterface, traceID string) (string, error) {
	t, err := g.getTrace(ctx, traceID)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (g *GatewayContract) BuildSourceLockProof(ctx contractapi.TransactionContextInterface, traceID, attester, signer string) (string, error) {
	t, err := g.getTrace(ctx, traceID)
	if err != nil {
		return "", err
	}
	if t.State != stateLocked {
		return "", fmt.Errorf("trace %s not in LOCKED state", traceID)
	}

	txID := ctx.GetStub().GetTxID()
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", err
	}
	proofTs := ts.Seconds*1000 + int64(ts.Nanos/1_000_000)

	// Fabric chaincode does not expose canonical block height in this context;
	// use tx timestamp millis as a monotonic placeholder for proof freshness.
	blockHeight := uint64(proofTs)
	eventHash := buildPayloadHashHex("LOCK_PROOF", t.TraceID, t.SrcChainID, t.DstChainID, t.Asset, t.Amount, t.Sender, t.Recipient, t.KeyID, t.Nonce, t.ExpireAt)

	proof := SourceLockProof{
		TraceID:        t.TraceID,
		TransferID:     t.TransferID,
		SessionID:      t.SessionID,
		SrcChainID:     t.SrcChainID,
		LockState:      t.State,
		BlockHeight:    blockHeight,
		TxHash:         txID,
		EventHash:      eventHash,
		ProofTimestamp: proofTs,
		Attester:       attester,
		Signer:         signer,
		ProofSchema:    sourceLockProofSchemaVersion,
		ProofLevel:     sourceLockProofSemanticLevel,
		ProofSource:    sourceLockProofSourceMode,
		BlockHeightRef: sourceLockProofBlockHeightRef,
		EventHashRef:   sourceLockProofEventHashRef,
	}
	if strings.TrimSpace(proof.Signer) == "" || strings.TrimSpace(proof.Signer) == "temp-signer" {
		proof.Signer = mustDefaultProofSignerAddress()
	}
	proof.ProofDigest = buildSourceLockProofDigest(proof)
	proofSig, sigErr := signSourceLockProofDigest(proof.ProofDigest)
	if sigErr != nil {
		return "", sigErr
	}
	proof.ProofSig = proofSig

	raw, err := json.Marshal(proof)
	if err != nil {
		return "", err
	}
	if err := ctx.GetStub().PutState(sourceLockProofKey(t.TransferID), raw); err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildSourceLockProofDigest(p SourceLockProof) string {
	rawHex := normalizeToBytes32Hex(p.TraceID) +
		normalizeToBytes32Hex(p.TransferID) +
		normalizeToBytes32Hex(p.SessionID) +
		normalizeToBytes32Hex(p.SrcChainID) +
		normalizeToBytes32Hex(p.LockState) +
		fmt.Sprintf("%064x", p.BlockHeight) +
		normalizeToBytes32Hex(p.TxHash) +
		normalizeToBytes32Hex(p.EventHash) +
		fmt.Sprintf("%064x", p.ProofTimestamp) +
		normalizeToBytes32Hex(p.Attester) +
		normalizeToBytes32Hex(p.Signer)
	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		return "0x" + strings.Repeat("0", 64)
	}
	h := crypto.Keccak256(raw)
	return "0x" + hex.EncodeToString(h)
}

func decodeSourceLockProofPayload(payloadB64 string) (SourceLockProof, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payloadB64))
	if err != nil {
		return SourceLockProof{}, fmt.Errorf("invalid proof payload base64: %w", err)
	}
	var proof SourceLockProof
	if err := json.Unmarshal(raw, &proof); err != nil {
		return SourceLockProof{}, fmt.Errorf("decoded payload is not SourceLockProof JSON: %w", err)
	}
	return proof, nil
}

func validateSourceLockProof(p SourceLockProof) error {
	if strings.TrimSpace(p.TraceID) == "" {
		return errors.New("proof traceId cannot be empty")
	}
	if strings.TrimSpace(p.TransferID) == "" {
		return errors.New("proof transferId cannot be empty")
	}
	if strings.TrimSpace(p.SessionID) == "" {
		return errors.New("proof sessionId cannot be empty")
	}
	if strings.TrimSpace(p.SrcChainID) == "" {
		return errors.New("proof srcChainId cannot be empty")
	}
	if strings.ToUpper(strings.TrimSpace(p.LockState)) != stateLocked {
		return errors.New("proof lockState must be LOCKED")
	}
	if p.BlockHeight == 0 {
		return errors.New("proof blockHeight invalid")
	}
	if strings.TrimSpace(p.TxHash) == "" || strings.TrimSpace(p.EventHash) == "" {
		return errors.New("proof txHash/eventHash cannot be empty")
	}
	if p.ProofTimestamp <= 0 {
		return errors.New("proof timestamp invalid")
	}
	nowMillis := time.Now().UnixMilli()
	if p.ProofTimestamp > nowMillis {
		return errors.New("proof timestamp future")
	}
	if nowMillis-p.ProofTimestamp > proofMaxAgeMillis {
		return errors.New("proof timestamp expired")
	}
	if strings.TrimSpace(p.Attester) == "" || strings.TrimSpace(p.Signer) == "" {
		return errors.New("proof attester/signer cannot be empty")
	}
	if strings.TrimSpace(p.ProofDigest) == "" || strings.TrimSpace(p.ProofSig) == "" {
		return errors.New("proof digest/signature cannot be empty")
	}
	if strings.TrimSpace(p.ProofSchema) == "" {
		return errors.New("proof schema version cannot be empty")
	}
	if strings.TrimSpace(p.ProofLevel) == "" {
		return errors.New("proof semantic level cannot be empty")
	}
	if strings.TrimSpace(p.ProofSource) == "" {
		return errors.New("proof source mode cannot be empty")
	}
	if strings.TrimSpace(p.BlockHeightRef) == "" || strings.TrimSpace(p.EventHashRef) == "" {
		return errors.New("proof reference mode cannot be empty")
	}
	if p.ProofSchema != sourceLockProofSchemaVersion {
		return errors.New("proof schema version mismatch")
	}
	if p.ProofLevel != sourceLockProofSemanticLevel {
		return errors.New("proof semantic level mismatch")
	}
	if p.ProofSource != sourceLockProofSourceMode {
		return errors.New("proof source mode mismatch")
	}
	if p.BlockHeightRef != sourceLockProofBlockHeightRef || p.EventHashRef != sourceLockProofEventHashRef {
		return errors.New("proof reference mode mismatch")
	}

	expectedDigest := strings.ToLower(buildSourceLockProofDigest(p))
	gotDigest := strings.ToLower(strings.TrimSpace(p.ProofDigest))
	if expectedDigest != gotDigest {
		return errors.New("proof digest mismatch")
	}

	recoveredSigner, err := recoverProofSignerAddress(gotDigest, p.ProofSig)
	if err != nil {
		return err
	}
	if !isSignerMatch(recoveredSigner, p.Signer) {
		return errors.New("proof signer mismatch")
	}

	return nil
}

func recoverProofSignerAddress(digestHex, sigHex string) (string, error) {
	d := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(digestHex), "0x"), "0X")
	digest, err := hex.DecodeString(d)
	if err != nil || len(digest) != 32 {
		return "", errors.New("invalid proof digest")
	}
	s := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(sigHex), "0x"), "0X")
	sig, err := hex.DecodeString(s)
	if err != nil || len(sig) != 65 {
		return "", errors.New("invalid proof signature")
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	if sig[64] > 1 {
		return "", errors.New("invalid proof signature v")
	}
	pub, err := crypto.SigToPub(digest, sig)
	if err != nil {
		return "", errors.New("invalid proof signature recover")
	}
	return strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()), nil
}

func isSignerMatch(recoveredAddr, signer string) bool {
	recovered := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(recoveredAddr)), "0x")
	candidate := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(signer)), "0x")
	if len(candidate) == 40 {
		return recovered == candidate
	}
	if len(candidate) == 64 {
		return recovered == candidate[24:]
	}
	return false
}

func normalizeToBytes32Hex(input string) string {
	v := strings.TrimSpace(input)
	if v == "" {
		return strings.Repeat("0", 64)
	}
	if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
		h := strings.ToLower(v[2:])
		if isHexString(h) {
			if len(h) > 64 {
				h = h[len(h)-64:]
			}
			return strings.Repeat("0", 64-len(h)) + h
		}
	}
	h := crypto.Keccak256([]byte(v))
	return hex.EncodeToString(h)
}

func signSourceLockProofDigest(digestHex string) (string, error) {
	priv, err := crypto.HexToECDSA(proofSignerPrivK)
	if err != nil {
		return "", fmt.Errorf("invalid proof signer private key: %w", err)
	}
	digestHex = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(digestHex), "0x"), "0X")
	raw, err := hex.DecodeString(digestHex)
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("invalid proof digest")
	}
	sig, err := crypto.Sign(raw, priv)
	if err != nil {
		return "", fmt.Errorf("sign proof digest failed: %w", err)
	}
	return "0x" + hex.EncodeToString(sig), nil
}

func mustDefaultProofSignerAddress() string {
	priv, err := crypto.HexToECDSA(proofSignerPrivK)
	if err != nil {
		return "0x0000000000000000000000000000000000000000"
	}
	return strings.ToLower(crypto.PubkeyToAddress(priv.PublicKey).Hex())
}

func isHexString(v string) bool {
	if v == "" {
		return false
	}
	for _, ch := range v {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func (g *GatewayContract) GetSourceLockProof(ctx contractapi.TransactionContextInterface, transferID string) (string, error) {
	raw, err := ctx.GetStub().GetState(sourceLockProofKey(transferID))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("source lock proof not found for transfer %s", transferID)
	}
	return string(raw), nil
}

func (g *GatewayContract) EncodeSourceLockProofPayload(_ contractapi.TransactionContextInterface, proofJSON string) (string, error) {
	if strings.TrimSpace(proofJSON) == "" {
		return "", errors.New("proofJSON cannot be empty")
	}
	var proof SourceLockProof
	if err := json.Unmarshal([]byte(proofJSON), &proof); err != nil {
		return "", fmt.Errorf("invalid proof JSON: %w", err)
	}
	return base64.StdEncoding.EncodeToString([]byte(proofJSON)), nil
}

func (g *GatewayContract) DecodeSourceLockProofPayload(_ contractapi.TransactionContextInterface, payloadB64 string) (string, error) {
	proof, err := decodeSourceLockProofPayload(payloadB64)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(proof)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (g *GatewayContract) IsNonceUsed(ctx contractapi.TransactionContextInterface, keyID string, nonce uint64) (bool, error) {
	raw, err := ctx.GetStub().GetState(nonceKey(keyID, nonce))
	if err != nil {
		return false, err
	}
	return len(raw) > 0, nil
}

func (g *GatewayContract) markNonceUsed(ctx contractapi.TransactionContextInterface, keyID string, nonce uint64) error {
	return ctx.GetStub().PutState(nonceKey(keyID, nonce), []byte{1})
}

func (g *GatewayContract) traceExists(ctx contractapi.TransactionContextInterface, traceID string) (bool, error) {
	raw, err := ctx.GetStub().GetState(traceKey(traceID))
	if err != nil {
		return false, err
	}
	return len(raw) > 0, nil
}

func (g *GatewayContract) getTrace(ctx contractapi.TransactionContextInterface, traceID string) (Trace, error) {
	transferID, err := g.getTransferIDByTrace(ctx, traceID)
	if err != nil {
		return Trace{}, err
	}
	if transferID == "" {
		return Trace{}, fmt.Errorf("trace %s not found", traceID)
	}
	raw, err := ctx.GetStub().GetState(transferKey(transferID))
	if err != nil {
		return Trace{}, err
	}
	if len(raw) == 0 {
		return Trace{}, fmt.Errorf("trace %s not found", traceID)
	}
	var t Trace
	if err := json.Unmarshal(raw, &t); err != nil {
		return Trace{}, err
	}
	return t, nil
}

func (g *GatewayContract) putTrace(ctx contractapi.TransactionContextInterface, t Trace) error {
	if strings.TrimSpace(t.TransferID) == "" {
		return errors.New("transferID missing")
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(transferKey(t.TransferID), raw)
}

func (g *GatewayContract) transferExists(ctx contractapi.TransactionContextInterface, transferID string) (bool, error) {
	raw, err := ctx.GetStub().GetState(transferKey(transferID))
	if err != nil {
		return false, err
	}
	return len(raw) > 0, nil
}

func (g *GatewayContract) getTransferIDByTrace(ctx contractapi.TransactionContextInterface, traceID string) (string, error) {
	raw, err := ctx.GetStub().GetState(traceToTransferKey(traceID))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", nil
	}
	return string(raw), nil
}

func buildTransferID(sessionID, traceID, chainID, asset string, amount uint64, sender, recipient, keyID string, nonce uint64, expireAt int64) string {
	message := strings.Join([]string{
		sessionID,
		traceID,
		chainID,
		asset,
		strconv.FormatUint(amount, 10),
		sender,
		recipient,
		keyID,
		strconv.FormatUint(nonce, 10),
		strconv.FormatInt(expireAt, 10),
	}, "|")
	h := crypto.Keccak256([]byte(message))
	return "0x" + hex.EncodeToString(h)
}

func (g *GatewayContract) verifySessionSig(ctx contractapi.TransactionContextInterface, keyID, payloadHash, sessSig string) (bool, error) {
	verifierCCRaw, err := ctx.GetStub().GetState(keyVerifierCCPrefix)
	if err != nil {
		return false, err
	}
	if len(verifierCCRaw) == 0 {
		verifierCCRaw = []byte(defaultVerifierCC)
	}
	verifierChannelRaw, err := ctx.GetStub().GetState(keyVerifierChannelPrefx)
	if err != nil {
		return false, err
	}
	channel := string(verifierChannelRaw)

	args := [][]byte{[]byte("VerifySessionSig"), []byte(keyID), []byte(payloadHash), []byte(sessSig)}
	resp := ctx.GetStub().InvokeChaincode(string(verifierCCRaw), args, channel)
	if resp.Status != 200 {
		return false, fmt.Errorf("invoke verifier failed: %s", string(resp.Message))
	}

	payload := strings.TrimSpace(string(resp.Payload))
	payload = strings.Trim(payload, "\"")
	return payload == "true", nil
}

func buildPayloadHashHex(action, traceID, srcChainID, dstChainID, asset string, amount uint64, sender, recipient, keyID string, nonce uint64, expireAt int64) string {
	message := strings.Join([]string{
		action,
		traceID,
		srcChainID,
		dstChainID,
		asset,
		strconv.FormatUint(amount, 10),
		sender,
		recipient,
		keyID,
		strconv.FormatUint(nonce, 10),
		strconv.FormatInt(expireAt, 10),
	}, "|")
	h := crypto.Keccak256([]byte(message))
	return "0x" + hex.EncodeToString(h)
}

func (g *GatewayContract) emit(ctx contractapi.TransactionContextInterface, eventName string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return ctx.GetStub().SetEvent(eventName, raw)
}

func (g *GatewayContract) emitWithCompat(ctx contractapi.TransactionContextInterface, newEventName, compatEventName string, payload any) error {
	if err := g.emit(ctx, compatEventName, payload); err != nil {
		return err
	}
	return g.emit(ctx, newEventName, payload)
}
