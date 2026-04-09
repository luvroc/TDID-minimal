package main

import (
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
	keySessionPrefix = "sess:key:"
	keyIdentityCC    = "cfg:identity:cc"

	defaultIdentityCC = "tdid-identity-cc"

	sessionStatusActive  = "ACTIVE"
	sessionStatusRevoked = "REVOKED"

	eventSessionBound   = "Event_SessionBound"
	eventSessionRevoked = "Event_SessionRevoked"
)

type SessionKeyRegistryContract struct {
	contractapi.Contract
}

type SessionInfo struct {
	KeyID      string `json:"keyId"`
	OwnerDID   string `json:"ownerDID"`
	PKSess     string `json:"pkSess"`
	SessSigner string `json:"sessSigner"`
	ExpireAt   int64  `json:"expireAt"`
	Status     string `json:"status"`
}

type SessionRevokeEvent struct {
	KeyID  string `json:"keyId"`
	Reason string `json:"reason"`
}

func sessionKey(keyID string) string {
	return keySessionPrefix + normalizeHex(keyID)
}

type identityNodeInfo struct {
	PKNode string `json:"pkNode"`
}

func (s *SessionKeyRegistryContract) SetIdentityRegistryCC(ctx contractapi.TransactionContextInterface, ccName string) error {
	if strings.TrimSpace(ccName) == "" {
		return errors.New("ccName cannot be empty")
	}
	if err := requireGovernanceAdmin(ctx); err != nil {
		return err
	}
	return ctx.GetStub().PutState(keyIdentityCC, []byte(ccName))
}

func (s *SessionKeyRegistryContract) getIdentityRegistryCC(ctx contractapi.TransactionContextInterface) (string, error) {
	raw, err := ctx.GetStub().GetState(keyIdentityCC)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return defaultIdentityCC, nil
	}
	return strings.TrimSpace(string(raw)), nil
}

func (s *SessionKeyRegistryContract) invokeIdentity(ctx contractapi.TransactionContextInterface, args ...string) ([]byte, error) {
	ccName, err := s.getIdentityRegistryCC(ctx)
	if err != nil {
		return nil, err
	}
	payloadArgs := make([][]byte, 0, len(args))
	for _, a := range args {
		payloadArgs = append(payloadArgs, []byte(a))
	}
	resp := ctx.GetStub().InvokeChaincode(ccName, payloadArgs, "")
	if resp.Status != 200 {
		return nil, fmt.Errorf("invoke identity chaincode failed: %s", string(resp.Message))
	}
	return resp.Payload, nil
}

func (s *SessionKeyRegistryContract) isNodeActiveViaIdentity(ctx contractapi.TransactionContextInterface, didNode string) (bool, error) {
	payload, err := s.invokeIdentity(ctx, "IsNodeActive", didNode)
	if err != nil {
		return false, err
	}
	v := strings.TrimSpace(string(payload))
	v = strings.Trim(v, "\"")
	return v == "true", nil
}

func (s *SessionKeyRegistryContract) getNodePKViaIdentity(ctx contractapi.TransactionContextInterface, didNode string) (string, error) {
	payload, err := s.invokeIdentity(ctx, "GetNode", didNode)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(payload))
	if v == "" || v == "null" {
		return "", nil
	}
	var node identityNodeInfo
	if err := json.Unmarshal(payload, &node); err != nil {
		return "", fmt.Errorf("parse identity node payload failed: %w", err)
	}
	return node.PKNode, nil
}

func (s *SessionKeyRegistryContract) BindSession(
	ctx contractapi.TransactionContextInterface,
	didNode string,
	pkSess string,
	expireAt int64,
	bindSig string,
) (string, error) {
	if didNode == "" || pkSess == "" || bindSig == "" {
		return "", errors.New("didNode/pkSess/bindSig cannot be empty")
	}
	if expireAt <= time.Now().Unix() {
		return "", errors.New("expireAt must be greater than current time")
	}

	active, err := s.isNodeActiveViaIdentity(ctx, didNode)
	if err != nil {
		return "", err
	}
	if !active {
		return "", errors.New("didNode is not active")
	}

	validBindSig, err := s.VerifyBindSig(ctx, didNode, pkSess, expireAt, bindSig)
	if err != nil {
		return "", err
	}
	if !validBindSig {
		return "", errors.New("verify bindSig failed")
	}

	keyID, err := deriveKeyIDFromPK(pkSess)
	if err != nil {
		return "", err
	}
	old, err := s.GetSession(ctx, keyID)
	if err != nil {
		return "", err
	}
	if old != nil && old.Status == sessionStatusActive {
		return "", errors.New("session already active")
	}

	sessSigner, err := pubKeyToAddressHex(pkSess)
	if err != nil {
		return "", err
	}

	info := SessionInfo{
		KeyID:      keyID,
		OwnerDID:   normalizeHex(didNode),
		PKSess:     normalizeHex(pkSess),
		SessSigner: sessSigner,
		ExpireAt:   expireAt,
		Status:     sessionStatusActive,
	}

	if err := putSession(ctx, info); err != nil {
		return "", err
	}
	if err := emitJSONEvent(ctx, eventSessionBound, info); err != nil {
		return "", err
	}

	return keyID, nil
}

func (s *SessionKeyRegistryContract) RevokeSession(ctx contractapi.TransactionContextInterface, keyID string) error {
	if keyID == "" {
		return errors.New("keyID cannot be empty")
	}
	if err := requireGovernanceAdmin(ctx); err != nil {
		return err
	}

	info, err := s.GetSession(ctx, keyID)
	if err != nil {
		return err
	}
	if info == nil || info.Status != sessionStatusActive {
		return errors.New("session is not active")
	}

	info.Status = sessionStatusRevoked
	if err := putSession(ctx, *info); err != nil {
		return err
	}

	return emitJSONEvent(ctx, eventSessionRevoked, SessionRevokeEvent{KeyID: normalizeHex(keyID), Reason: "manual revoke"})
}

func (s *SessionKeyRegistryContract) GetSession(ctx contractapi.TransactionContextInterface, keyID string) (*SessionInfo, error) {
	raw, err := ctx.GetStub().GetState(sessionKey(keyID))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var info SessionInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (s *SessionKeyRegistryContract) IsSessionActive(ctx contractapi.TransactionContextInterface, keyID string) (bool, error) {
	info, err := s.GetSession(ctx, keyID)
	if err != nil {
		return false, err
	}
	if info == nil {
		return false, nil
	}
	return info.Status == sessionStatusActive && info.ExpireAt >= time.Now().Unix(), nil
}

func (s *SessionKeyRegistryContract) VerifyBindSig(
	ctx contractapi.TransactionContextInterface,
	didNode string,
	pkSess string,
	expireAt int64,
	bindSig string,
) (bool, error) {
	active, err := s.isNodeActiveViaIdentity(ctx, didNode)
	if err != nil {
		return false, err
	}
	if !active || expireAt <= time.Now().Unix() {
		return false, nil
	}

	nodePK, err := s.getNodePKViaIdentity(ctx, didNode)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(nodePK) == "" {
		return false, nil
	}

	msgHash := buildBindHashForFabric(normalizeHex(pkSess), expireAt, ctx.GetStub().GetChannelID(), "SessionKeyRegistry")
	return verifySecp256k1Sig(nodePK, msgHash, bindSig)
}

func (s *SessionKeyRegistryContract) VerifySessionSig(
	ctx contractapi.TransactionContextInterface,
	keyID string,
	payloadHash string,
	sessSig string,
) (bool, error) {
	active, err := s.IsSessionActive(ctx, keyID)
	if err != nil {
		return false, err
	}
	if !active {
		return false, nil
	}

	info, err := s.GetSession(ctx, keyID)
	if err != nil {
		return false, err
	}
	if info == nil {
		return false, nil
	}

	hashRaw, err := decodeHex(payloadHash)
	if err != nil {
		return false, err
	}
	if len(hashRaw) != 32 {
		return false, fmt.Errorf("payloadHash must be 32 bytes, got %d", len(hashRaw))
	}

	return verifySecp256k1Sig(info.PKSess, hashRaw, sessSig)
}

func putSession(ctx contractapi.TransactionContextInterface, info SessionInfo) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(sessionKey(info.KeyID), raw)
}

func deriveKeyIDFromPK(pkSessHex string) (string, error) {
	raw, err := decodeHex(pkSessHex)
	if err != nil {
		return "", err
	}
	h := crypto.Keccak256Hash(raw)
	return "0x" + hex.EncodeToString(h.Bytes()), nil
}

func buildBindHashForFabric(pkSess string, expireAt int64, chainID string, contractName string) []byte {
	payload := pkSess + "|" + strconv.FormatInt(expireAt, 10) + "|" + chainID + "|" + contractName
	return crypto.Keccak256([]byte(payload))
}

func pubKeyToAddressHex(pubKeyHex string) (string, error) {
	raw, err := decodeHex(pubKeyHex)
	if err != nil {
		return "", err
	}
	if len(raw) == 65 && raw[0] == 0x04 {
		h := crypto.Keccak256(raw[1:])
		return "0x" + hex.EncodeToString(h[12:]), nil
	}
	if len(raw) == 64 {
		h := crypto.Keccak256(raw)
		return "0x" + hex.EncodeToString(h[12:]), nil
	}
	return "", errors.New("pubkey must be 64/65 bytes")
}
