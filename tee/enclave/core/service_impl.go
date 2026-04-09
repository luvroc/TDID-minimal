package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	sharederr "tdid-final/shared/errors"
	sharedtypes "tdid-final/shared/types"
	"tdid-final/tee"
)

type Config struct {
	StatePath     string
	SealKey       []byte
	NodeID        string
	Role          string
	PeerAllowList []string
}

type service struct {
	store      tee.StateStore
	keyManager tee.KeyManager
	sessionMgr tee.SessionManager
	hasher     tee.Hasher
	nodeID     string
	role       string
	peerAllow  []string
}

const (
	peerMessageFutureTolerance = 2 * time.Minute
	peerMessageMaxAge          = 10 * time.Minute
	peerReplayNonceWindowMax   = 2048
)

func NewService(cfg Config) (Service, error) {
	role := normalizeRole(cfg.Role)
	statePath := strings.TrimSpace(cfg.StatePath)
	if statePath == "" {
		statePath = defaultStatePathByRole(role)
	}
	if len(cfg.SealKey) < 16 {
		return nil, sharederr.New(sharederr.CodeInvalidInput, "seal key must be at least 16 bytes", nil)
	}
	store, err := tee.NewFileSealedStateStore(statePath, cfg.SealKey)
	if err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(cfg.NodeID)
	if nodeID == "" {
		nodeID = "tee-" + role
	}
	return NewServiceWithStoreAndMeta(store, nodeID, role, cfg.PeerAllowList), nil
}

func NewServiceWithStore(store tee.StateStore) Service {
	return NewServiceWithStoreAndMeta(store, "tee-source", "source", nil)
}

func NewServiceWithStoreAndMeta(store tee.StateStore, nodeID string, role string, peerAllow []string) Service {
	hasher := tee.NewKeccakHasher()
	keyManager := tee.NewKeyManager(store, tee.NewSecp256k1Signer())
	sessionMgr := tee.NewSessionManager(store, keyManager, hasher)
	return &service{
		store:      store,
		keyManager: keyManager,
		sessionMgr: sessionMgr,
		hasher:     hasher,
		nodeID:     strings.TrimSpace(nodeID),
		role:       normalizeRole(role),
		peerAllow:  normalizeAllowList(peerAllow),
	}
}

func (s *service) InitNode(ctx context.Context) error {
	if _, err := s.keyManager.InitOrLoadNodeIdentity(ctx); err != nil {
		return err
	}
	st, err := s.store.Load(ctx)
	if err != nil {
		return err
	}
	if st == nil {
		st = tee.NewEmptySealedState()
	}
	st.NodeID = s.nodeID
	st.Role = s.role
	st.PeerAllowList = append([]string(nil), s.peerAllow...)
	if st.UsedNonceByKey == nil {
		st.UsedNonceByKey = map[string]map[uint64]bool{}
	}
	if st.UsedNonceWindow == nil {
		st.UsedNonceWindow = st.UsedNonceByKey
	}
	return s.store.Save(ctx, st)
}

func (s *service) GetNodeIdentity(ctx context.Context) (sharedtypes.NodeIdentity, error) {
	identity, err := s.keyManager.InitOrLoadNodeIdentity(ctx)
	if err != nil {
		return sharedtypes.NodeIdentity{}, err
	}
	return sharedtypes.NodeIdentity{PublicKey: identity.PublicKey, Address: identity.Address}, nil
}

func (s *service) BindSession(ctx context.Context, req sharedtypes.BindSessionRequest) (sharedtypes.BindSessionResponse, error) {
	bindResult, err := s.sessionMgr.BindSession(ctx, tee.BindRequest{
		Chain:        toTEEChain(req.Chain, req.ChainID),
		ChainID:      req.ChainID,
		ContractAddr: req.ContractAddr,
		ExpireAt:     req.ExpireAt,
		RatchetSeed:  append([]byte(nil), req.RatchetSeed...),
		RequestTime:  time.Now().UTC(),
	})
	if err != nil {
		return sharedtypes.BindSessionResponse{}, err
	}
	return sharedtypes.BindSessionResponse{
		SessionID: bindResult.Session.SessionID,
		KeyID:     bindResult.Session.KeyID,
		PublicKey: bindResult.Session.PublicKey,
		ExpireAt:  bindResult.Session.ExpireAt,
		ChainID:   bindResult.Session.ChainID,
		BindHash:  bindResult.BindHash,
		BindSig:   bindResult.BindSig,
	}, nil
}

func (s *service) CurrentSession(ctx context.Context) (*sharedtypes.CurrentSessionResponse, error) {
	session, err := s.sessionMgr.CurrentSession(ctx)
	if err != nil {
		if err == tee.ErrSessionMissing {
			return nil, nil
		}
		return nil, err
	}
	if session.SessionID == "" && len(session.PublicKey) > 0 {
		session.SessionID = tee.DeriveSessionID(session.PublicKey, session.ChainID, session.ContractAddr, session.ExpireAt)
	}
	return &sharedtypes.CurrentSessionResponse{
		SessionID:    session.SessionID,
		KeyID:        session.KeyID,
		PublicKey:    session.PublicKey,
		ExpireAt:     session.ExpireAt,
		ChainID:      session.ChainID,
		ContractAddr: session.ContractAddr,
	}, nil
}

func (s *service) SignLock(ctx context.Context, req sharedtypes.SignLockRequest) (sharedtypes.SignedPayload, error) {
	return s.signPayload(ctx, tee.ActionLock, req.Chain, req.SessionID, req.TransferID, req.TraceID, req.SrcChainID, req.DstChainID, req.Asset, req.Amount, req.Sender, req.Recipient, req.KeyID, req.Nonce, req.ExpireAt)
}

func (s *service) SignMintOrUnlock(ctx context.Context, req sharedtypes.SignMintOrUnlockRequest) (sharedtypes.SignedPayload, error) {
	return s.signPayload(ctx, tee.ActionMint, req.Chain, req.SessionID, req.TransferID, req.TraceID, req.SrcChainID, req.DstChainID, req.Asset, req.Amount, req.Sender, req.Recipient, req.KeyID, req.Nonce, req.ExpireAt)
}

func (s *service) SignRefundV2(ctx context.Context, req sharedtypes.SignRefundV2Request) (sharedtypes.SignedPayload, error) {
	return s.signPayload(ctx, tee.ActionRefund, req.Chain, req.SessionID, req.TransferID, req.TraceID, "", "", "", "0", "", "", req.KeyID, req.Nonce, req.ExpireAt)
}

func (s *service) BuildReceipt(ctx context.Context, req sharedtypes.BuildReceiptRequest) (sharedtypes.BuildReceiptResponse, error) {
	_ = ctx
	result, err := tee.BuildReceiptResult(s.hasher, tee.ReceiptRequest{
		TransferID:  req.TransferID,
		TraceID:     req.TraceID,
		TxHash:      req.TxHash,
		ChainID:     req.ChainID,
		Amount:      req.Amount,
		Recipient:   req.Recipient,
		PayloadHash: req.PayloadHash,
		FinalState:  req.FinalState,
		SrcChainID:  req.SrcChainID,
		DstChainID:  req.DstChainID,
	})
	if err != nil {
		return sharedtypes.BuildReceiptResponse{}, err
	}
	return sharedtypes.BuildReceiptResponse{
		TransferID:     result.TransferID,
		TraceID:        result.TraceID,
		ReceiptHash:    result.ReceiptHash,
		ReceiptHashHex: result.ReceiptHashHex,
	}, nil
}

func (s *service) VerifyPeerCrossMessage(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) error {
	_ = ctx
	if strings.TrimSpace(req.TraceID) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "traceId is required", nil)
	}
	if strings.TrimSpace(req.SrcChainID) == "" || strings.TrimSpace(req.DstChainID) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "srcChainId and dstChainId are required", nil)
	}
	if strings.TrimSpace(req.Asset) == "" || strings.TrimSpace(req.Amount) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "asset and amount are required", nil)
	}
	if strings.TrimSpace(req.Sender) == "" || strings.TrimSpace(req.Recipient) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "sender and recipient are required", nil)
	}
	if strings.TrimSpace(req.TransferID) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "transferId is required", nil)
	}
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.KeyID) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "sessionId and keyId are required", nil)
	}
	if strings.TrimSpace(req.SourceLockProof) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "sourceLockProof is required", nil)
	}
	if strings.TrimSpace(req.SrcPayloadHash) == "" || strings.TrimSpace(req.SrcSessSig) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "srcPayloadHash and srcSessSig are required", nil)
	}
	if strings.TrimSpace(req.RequestDigest) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "requestDigest is required", nil)
	}
	if req.Timestamp <= 0 {
		return sharederr.New(sharederr.CodeInvalidInput, "timestamp is required", nil)
	}
	now := time.Now().UnixMicro()
	if req.Timestamp > now+peerMessageFutureTolerance.Microseconds() {
		return sharederr.New(sharederr.CodePeerAuthFailed, "peer message timestamp is from the future", nil)
	}
	if now-req.Timestamp > peerMessageMaxAge.Microseconds() {
		return sharederr.New(sharederr.CodePeerAuthFailed, "peer message timestamp expired", nil)
	}

	expectedDigest := derivePeerRequestDigest(req.TransferID, req.TraceID, req.SrcPayloadHash)
	if !strings.EqualFold(strings.TrimSpace(req.RequestDigest), expectedDigest) {
		return sharederr.New(sharederr.CodePeerAuthFailed, "requestDigest mismatch", nil)
	}

	payloadHash, err := decodeHexStrict(strings.TrimSpace(req.SrcPayloadHash))
	if err != nil || len(payloadHash) != 32 {
		return sharederr.New(sharederr.CodePeerAuthFailed, "srcPayloadHash must be a 32-byte hex string", err)
	}
	recoveredAddr, err := recoverSignerAddress(payloadHash, strings.TrimSpace(req.SrcSessSig))
	if err != nil {
		return sharederr.New(sharederr.CodePeerAuthFailed, "srcSessSig verification failed", err)
	}
	if err := verifyPeerAllowList(s.peerAllow, recoveredAddr, req.KeyID, req.SessionID); err != nil {
		return sharederr.New(sharederr.CodePeerAuthFailed, err.Error(), nil)
	}
	if err := verifyPeerCapabilityConstraints(s.peerAllow, req); err != nil {
		return sharederr.New(sharederr.CodePeerAuthFailed, err.Error(), nil)
	}
	if err := s.verifyPeerSessionBinding(ctx, req); err != nil {
		return sharederr.New(sharederr.CodePeerAuthFailed, err.Error(), nil)
	}
	if err := s.verifyAndRememberPeerNonce(ctx, req.SessionID, req.KeyID, req.Nonce); err != nil {
		return sharederr.New(sharederr.CodePeerAuthFailed, err.Error(), nil)
	}

	return nil
}

func (s *service) Health(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (s *service) VerifyTargetExecutionEvidence(ctx context.Context, req sharedtypes.TargetExecutionEvidenceRequest) error {
	_ = ctx
	if strings.TrimSpace(req.TraceID) == "" || strings.TrimSpace(req.TransferID) == "" || strings.TrimSpace(req.DstChainID) == "" {
		return sharederr.New(sharederr.CodeInvalidInput, "traceId/transferId/dstChainId are required", nil)
	}
	if strings.TrimSpace(req.TargetTraceID) != "" && !strings.EqualFold(strings.TrimSpace(req.TargetTraceID), strings.TrimSpace(req.TraceID)) {
		return sharederr.New(sharederr.CodePeerAuthFailed, "peer response trace mismatch", nil)
	}
	if strings.TrimSpace(req.TargetTransferID) != "" && !strings.EqualFold(strings.TrimSpace(req.TargetTransferID), strings.TrimSpace(req.TransferID)) {
		return sharederr.New(sharederr.CodePeerAuthFailed, "peer response transfer mismatch", nil)
	}
	if strings.TrimSpace(req.TargetChainTx) == "" || strings.TrimSpace(req.TargetReceipt) == "" {
		return sharederr.New(sharederr.CodePeerAuthFailed, "peer response missing target commit evidence", nil)
	}
	if strings.TrimSpace(req.TargetChainID) == "" || !strings.EqualFold(strings.TrimSpace(req.TargetChainID), strings.TrimSpace(req.DstChainID)) {
		return sharederr.New(sharederr.CodePeerAuthFailed, "peer response target chain id mismatch", nil)
	}
	if strings.TrimSpace(req.TargetChainHash) == "" {
		return sharederr.New(sharederr.CodePeerAuthFailed, "peer response missing target chain hash", nil)
	}
	return nil
}

func (s *service) signPayload(ctx context.Context, action tee.ActionType, chain sharedtypes.ChainKind, sessionID string, transferID string, traceID string, srcChainID string, dstChainID string, asset string, amount string, sender string, recipient string, keyID string, nonce uint64, expireAt int64) (sharedtypes.SignedPayload, error) {
	signed, err := s.sessionMgr.SignPayload(ctx, tee.PayloadRequest{SessionID: sessionID, TransferID: transferID, Chain: toTEEChain(chain, srcChainID+"|"+dstChainID), Action: action, TraceID: traceID, SrcChainID: srcChainID, DstChainID: dstChainID, Asset: asset, Amount: amount, Sender: sender, Recipient: recipient, KeyID: keyID, Nonce: nonce, ExpireAt: expireAt})
	if err != nil {
		return sharedtypes.SignedPayload{}, err
	}
	return sharedtypes.SignedPayload{SessionID: signed.SessionID, TransferID: signed.TransferID, PayloadHash: signed.PayloadHash, SessSig: signed.SessSig, KeyID: signed.KeyID, Nonce: signed.Nonce, ExpireAt: signed.ExpireAt}, nil
}

func toTEEChain(chain sharedtypes.ChainKind, chainHint string) tee.ChainKind {
	switch chain {
	case sharedtypes.ChainFISCO:
		return tee.ChainFISCO
	case sharedtypes.ChainFabric:
		return tee.ChainFabric
	}
	if strings.Contains(strings.ToLower(chainHint), "fisco") {
		return tee.ChainFISCO
	}
	return tee.ChainFabric
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "target", "tee-b":
		return "target"
	default:
		return "source"
	}
}

func normalizeAllowList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func defaultStatePathByRole(role string) string {
	switch normalizeRole(role) {
	case "target":
		return "/var/lib/tee-b/state.sealed"
	default:
		return "/var/lib/tee-a/state.sealed"
	}
}

func (s *service) String() string {
	return fmt.Sprintf("service(nodeID=%s role=%s)", s.nodeID, s.role)
}

func derivePeerRequestDigest(transferID string, traceID string, payloadHash string) string {
	raw := strings.TrimSpace(transferID) + "|" + strings.TrimSpace(traceID) + "|" + strings.TrimSpace(payloadHash)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("sha256:%x", sum[:])
}

func decodeHexStrict(value string) ([]byte, error) {
	v := strings.TrimSpace(value)
	v = strings.TrimPrefix(v, "0x")
	v = strings.TrimPrefix(v, "0X")
	if len(v)%2 != 0 {
		return nil, fmt.Errorf("invalid hex length")
	}
	out, err := hex.DecodeString(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func recoverSignerAddress(payloadHash []byte, sigHex string) (string, error) {
	sig65, err := decodeHexStrict(sigHex)
	if err != nil {
		return "", err
	}
	recoverySig, err := tee.SignatureToRecoveryID(sig65)
	if err != nil {
		return "", err
	}
	pub, err := crypto.SigToPub(payloadHash, recoverySig)
	if err != nil {
		return "", err
	}
	return strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()), nil
}

func verifyPeerAllowList(allowList []string, recoveredAddr string, keyID string, sessionID string) error {
	if noIDBindingBaselineEnabled() {
		return nil
	}
	if len(allowList) == 0 {
		return nil
	}
	keyID = strings.ToLower(strings.TrimSpace(keyID))
	sessionID = strings.TrimSpace(sessionID)
	recoveredAddr = strings.ToLower(strings.TrimSpace(recoveredAddr))

	strictRules := 0
	for _, raw := range allowList {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		lower := strings.ToLower(item)
		switch {
		case strings.HasPrefix(lower, "addr:"):
			strictRules++
			if recoveredAddr == strings.TrimSpace(strings.TrimPrefix(lower, "addr:")) {
				return nil
			}
		case strings.HasPrefix(lower, "key:"):
			strictRules++
			if keyID == strings.TrimSpace(strings.TrimPrefix(lower, "key:")) {
				return nil
			}
		case strings.HasPrefix(lower, "session:"):
			strictRules++
			if sessionID == strings.TrimSpace(strings.TrimPrefix(lower, "session:")) {
				return nil
			}
		}
	}
	if strictRules == 0 {
		// Backward-compatible mode: legacy allowlist values (e.g. node names) are
		// still enforced by mTLS CN on peer server interceptor.
		return nil
	}
	return fmt.Errorf("peer identity is not in allowlist")
}

func noIDBindingBaselineEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TDID_T6_NO_ID_BINDING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func verifyPeerCapabilityConstraints(allowList []string, req sharedtypes.CrossChainExecutionRequest) error {
	if len(allowList) == 0 {
		return nil
	}
	sourceLockProof := req.SourceLockProof
	proofRaw := strings.TrimSpace(sourceLockProof)
	proofText := strings.ToLower(proofRaw)
	proofPayload, payloadOK := decodeProofPayloadStruct(proofRaw)
	requestTsMillis := req.Timestamp / 1000
	requestSender := strings.ToLower(strings.TrimSpace(req.Sender))
	requestAsset := strings.ToLower(strings.TrimSpace(req.Asset))
	requestDstChain := strings.ToLower(strings.TrimSpace(req.DstChainID))
	requestAction := "mint_or_unlock"
	requiredTokens := make([]string, 0, 2)
	for _, raw := range allowList {
		item := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case strings.HasPrefix(item, "vc:subject:"):
			expected := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(item, "vc:subject:")))
			if expected == "" {
				continue
			}
			if requestSender == "" || requestSender != expected {
				return fmt.Errorf("vc subject mismatch")
			}
		case strings.HasPrefix(item, "vc:action:"):
			expected := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(item, "vc:action:")))
			if expected == "" {
				continue
			}
			if expected != requestAction && !(expected == "mint" && requestAction == "mint_or_unlock") {
				return fmt.Errorf("vc action mismatch")
			}
		case strings.HasPrefix(item, "vc:resource:"):
			expected := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(item, "vc:resource:")))
			if expected == "" {
				continue
			}
			resourceAsset := requestAsset
			resourcePair := requestAsset + "@" + requestDstChain
			if expected != resourceAsset && expected != resourcePair {
				return fmt.Errorf("vc resource mismatch")
			}
		case strings.HasPrefix(item, "vc:notbefore:") || strings.HasPrefix(item, "vc:nbf:"):
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(item, "vc:notbefore:"), "vc:nbf:"))
			if value == "" {
				continue
			}
			nbf, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("vc notbefore invalid")
			}
			if requestTsMillis <= 0 || requestTsMillis < nbf {
				return fmt.Errorf("vc notbefore violation")
			}
		case strings.HasPrefix(item, "vc:expireat:") || strings.HasPrefix(item, "vc:exp:"):
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(item, "vc:expireat:"), "vc:exp:"))
			if value == "" {
				continue
			}
			exp, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("vc expireat invalid")
			}
			if requestTsMillis <= 0 || requestTsMillis > exp {
				return fmt.Errorf("vc expireat violation")
			}
		case strings.HasPrefix(item, "vc:issuer:"):
			expected := strings.TrimSpace(strings.TrimPrefix(item, "vc:issuer:"))
			if expected == "" {
				continue
			}
			if !payloadOK {
				return fmt.Errorf("sourceLockProof payload is required for vc:issuer check")
			}
			expectedB32 := canonicalBytes32Token(expected)
			if expectedB32 == "" || (proofPayload.Attester != expectedB32 && proofPayload.Signer != expectedB32) {
				return fmt.Errorf("vc issuer mismatch")
			}
		case strings.HasPrefix(item, "vc:attester:"):
			expected := strings.TrimSpace(strings.TrimPrefix(item, "vc:attester:"))
			if expected == "" {
				continue
			}
			if !payloadOK {
				return fmt.Errorf("sourceLockProof payload is required for vc:attester check")
			}
			expectedB32 := canonicalBytes32Token(expected)
			if expectedB32 == "" || proofPayload.Attester != expectedB32 {
				return fmt.Errorf("vc attester mismatch")
			}
		case strings.HasPrefix(item, "vc:signer:"):
			expected := strings.TrimSpace(strings.TrimPrefix(item, "vc:signer:"))
			if expected == "" {
				continue
			}
			if !payloadOK {
				return fmt.Errorf("sourceLockProof payload is required for vc:signer check")
			}
			expectedB32 := canonicalBytes32Token(expected)
			if expectedB32 == "" || proofPayload.Signer != expectedB32 {
				return fmt.Errorf("vc signer mismatch")
			}
		case item == "cap:proofsig":
			if !payloadOK {
				return fmt.Errorf("sourceLockProof payload is required for cap:proofsig")
			}
			if proofPayload.SigR == "" || proofPayload.SigS == "" || proofPayload.SigV == "" {
				return fmt.Errorf("proof signature parts are missing")
			}
		case item == "cap:proofdigest":
			if !payloadOK {
				return fmt.Errorf("sourceLockProof payload is required for cap:proofdigest")
			}
			if proofPayload.ProofDigest == "" {
				return fmt.Errorf("proof digest is missing")
			}
		case strings.HasPrefix(item, "cap:"):
			v := strings.TrimSpace(strings.TrimPrefix(item, "cap:"))
			if v != "" {
				requiredTokens = append(requiredTokens, v)
			}
		case strings.HasPrefix(item, "vc:"):
			v := strings.TrimSpace(strings.TrimPrefix(item, "vc:"))
			if v != "" {
				requiredTokens = append(requiredTokens, v)
			}
		}
	}
	if len(requiredTokens) == 0 {
		return nil
	}
	if proofText == "" {
		return fmt.Errorf("sourceLockProof is required for capability constraints")
	}
	for _, token := range requiredTokens {
		if !strings.Contains(proofText, token) {
			return fmt.Errorf("missing capability token: %s", token)
		}
	}
	return nil
}

type proofPayloadFields struct {
	Attester    string
	Signer      string
	ProofDigest string
	SigR        string
	SigS        string
	SigV        string
}

func decodeProofPayloadStruct(proof string) (proofPayloadFields, bool) {
	raw := strings.TrimSpace(proof)
	raw = strings.TrimPrefix(raw, "0x")
	raw = strings.TrimPrefix(raw, "0X")
	// layout from encode_source_lock_proof.py: 15 words (32 bytes each)
	const hexLen = 15 * 64
	if len(raw) != hexLen {
		return proofPayloadFields{}, false
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return proofPayloadFields{}, false
	}
	word := func(i int) string {
		start := i * 64
		end := start + 64
		return "0x" + strings.ToLower(raw[start:end])
	}
	return proofPayloadFields{
		Attester:    word(9),
		Signer:      word(10),
		ProofDigest: word(11),
		SigR:        word(12),
		SigS:        word(13),
		SigV:        word(14),
	}, true
}

func canonicalBytes32Token(token string) string {
	v := strings.TrimSpace(strings.ToLower(token))
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "0x") {
		h := strings.TrimPrefix(v, "0x")
		if len(h) == 40 {
			return "0x" + strings.Repeat("0", 24) + h
		}
		if len(h) <= 64 {
			if _, err := hex.DecodeString(h); err == nil {
				return "0x" + strings.Repeat("0", 64-len(h)) + h
			}
		}
		return ""
	}
	sum := crypto.Keccak256([]byte(v))
	return fmt.Sprintf("0x%x", sum)
}

func (s *service) verifyPeerSessionBinding(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) error {
	current, err := s.sessionMgr.CurrentSession(ctx)
	if err != nil {
		if err == tee.ErrSessionMissing {
			// Backward compatibility: old deployment may not bind target session yet.
			return nil
		}
		return fmt.Errorf("load current session failed: %w", err)
	}
	if current == nil {
		return nil
	}
	currentSessionID := strings.TrimSpace(current.SessionID)
	currentKeyID := strings.TrimSpace(current.KeyID)
	reqSessionID := strings.TrimSpace(req.SessionID)
	reqKeyID := strings.TrimSpace(req.KeyID)

	if current.ExpireAt > 0 && time.Now().UnixMilli() > current.ExpireAt {
		return fmt.Errorf("current session expired")
	}
	if currentSessionID != "" && !strings.EqualFold(currentSessionID, reqSessionID) {
		return fmt.Errorf("sessionId mismatch with current session")
	}
	if currentKeyID != "" && !strings.EqualFold(currentKeyID, reqKeyID) {
		return fmt.Errorf("keyId mismatch with current session")
	}
	return nil
}

func (s *service) verifyAndRememberPeerNonce(ctx context.Context, sessionID string, keyID string, nonce uint64) error {
	st, err := s.store.Load(ctx)
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}
	if st == nil {
		st = tee.NewEmptySealedState()
	}
	if st.UsedNonceByKey == nil {
		st.UsedNonceByKey = map[string]map[uint64]bool{}
	}
	if st.UsedNonceWindow == nil {
		st.UsedNonceWindow = st.UsedNonceByKey
	}

	bucketID := strings.ToLower(strings.TrimSpace(sessionID) + "|" + strings.TrimSpace(keyID))
	if bucketID == "|" {
		bucketID = "_peer_default_"
	}
	used := st.UsedNonceByKey[bucketID]
	if used == nil {
		used = map[uint64]bool{}
		st.UsedNonceByKey[bucketID] = used
	}
	if used[nonce] {
		return fmt.Errorf("replayed nonce")
	}

	used[nonce] = true
	if len(used) > peerReplayNonceWindowMax {
		trimmed := make(map[uint64]bool, peerReplayNonceWindowMax)
		trimmed[nonce] = true
		st.UsedNonceByKey[bucketID] = trimmed
	}
	st.UsedNonceWindow = st.UsedNonceByKey
	if err := s.store.Save(ctx, st); err != nil {
		return fmt.Errorf("save replay window failed: %w", err)
	}
	return nil
}
