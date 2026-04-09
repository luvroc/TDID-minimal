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
	keyNodePrefix = "tdid:node:"

	statusActive  = "ACTIVE"
	statusRevoked = "REVOKED"

	eventNodeRegistered = "Event_NodeRegistered"
	eventNodeRevoked    = "Event_NodeRevoked"
)

type TDIDRegistryContract struct {
	contractapi.Contract
}

type NodeInfo struct {
	DIDNode       string `json:"didNode"`
	PKNode        string `json:"pkNode"`
	MrEnclaveHash string `json:"mrEnclaveHash"`
	QuoteHash     string `json:"quoteHash"`
	OrgID         string `json:"orgId"`
	Status        string `json:"status"`
	ValidTo       int64  `json:"validTo"`
}

type NodeRevokeEvent struct {
	DIDNode string `json:"didNode"`
	Reason  string `json:"reason"`
}

func nodeKey(didNode string) string {
	return keyNodePrefix + didNode
}

func (t *TDIDRegistryContract) RegisterNode(
	ctx contractapi.TransactionContextInterface,
	didNode string,
	pkNode string,
	mrEnclaveHash string,
	quoteHash string,
	orgID string,
	orgSig string,
	validTo int64,
) error {
	if didNode == "" || pkNode == "" || mrEnclaveHash == "" || quoteHash == "" || orgID == "" || orgSig == "" {
		return errors.New("input cannot be empty")
	}
	if validTo <= time.Now().Unix() {
		return errors.New("validTo must be greater than current time")
	}

	measurementAllowed, err := (&GovernanceRootContract{}).IsMeasurementAllowed(ctx, mrEnclaveHash)
	if err != nil {
		return err
	}
	if !measurementAllowed {
		return errors.New("measurement is not allowed")
	}

	expectDID, err := deriveDIDFromPK(pkNode)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expectDID, normalizeHex(didNode)) {
		return errors.New("didNode mismatch with hash(pkNode)")
	}

	oldNode, err := t.GetNode(ctx, didNode)
	if err != nil {
		return err
	}
	if oldNode != nil && oldNode.Status == statusActive {
		return errors.New("active node already exists")
	}

	signerPK, err := (&GovernanceRootContract{}).GetOrgSigner(ctx, orgID)
	if err != nil {
		return err
	}
	if signerPK == "" {
		return errors.New("org signer not found")
	}

	msgHash := buildRegisterHashForFabric(normalizeHex(didNode), normalizeHex(pkNode), normalizeHex(mrEnclaveHash), normalizeHex(quoteHash), orgID, validTo)
	validSig, err := verifySecp256k1Sig(signerPK, msgHash, orgSig)
	if err != nil {
		return err
	}
	if !validSig {
		return errors.New("org signature verify failed")
	}

	node := NodeInfo{
		DIDNode:       normalizeHex(didNode),
		PKNode:        normalizeHex(pkNode),
		MrEnclaveHash: normalizeHex(mrEnclaveHash),
		QuoteHash:     normalizeHex(quoteHash),
		OrgID:         orgID,
		Status:        statusActive,
		ValidTo:       validTo,
	}
	if err := putNode(ctx, node); err != nil {
		return err
	}

	return emitJSONEvent(ctx, eventNodeRegistered, node)
}

func (t *TDIDRegistryContract) RevokeNode(ctx contractapi.TransactionContextInterface, didNode string) error {
	if didNode == "" {
		return errors.New("didNode cannot be empty")
	}
	if err := requireGovernanceAdmin(ctx); err != nil {
		return err
	}

	node, err := t.GetNode(ctx, didNode)
	if err != nil {
		return err
	}
	if node == nil || node.Status != statusActive {
		return errors.New("node is not active")
	}

	node.Status = statusRevoked
	if err := putNode(ctx, *node); err != nil {
		return err
	}

	return emitJSONEvent(ctx, eventNodeRevoked, NodeRevokeEvent{DIDNode: normalizeHex(didNode), Reason: "manual revoke"})
}

func (t *TDIDRegistryContract) GetNode(ctx contractapi.TransactionContextInterface, didNode string) (*NodeInfo, error) {
	raw, err := ctx.GetStub().GetState(nodeKey(normalizeHex(didNode)))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var node NodeInfo
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func (t *TDIDRegistryContract) IsNodeActive(ctx contractapi.TransactionContextInterface, didNode string) (bool, error) {
	node, err := t.GetNode(ctx, didNode)
	if err != nil {
		return false, err
	}
	if node == nil {
		return false, nil
	}
	return node.Status == statusActive && node.ValidTo >= time.Now().Unix(), nil
}

func putNode(ctx contractapi.TransactionContextInterface, node NodeInfo) error {
	raw, err := json.Marshal(node)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(nodeKey(node.DIDNode), raw)
}

func deriveDIDFromPK(pkNodeHex string) (string, error) {
	pkRaw, err := decodeHex(pkNodeHex)
	if err != nil {
		return "", err
	}
	h := crypto.Keccak256Hash(pkRaw)
	return "0x" + hex.EncodeToString(h.Bytes()), nil
}

func buildRegisterHashForFabric(didNode, pkNode, mrEnclaveHash, quoteHash, orgID string, validTo int64) []byte {
	payload := didNode + "|" + pkNode + "|" + mrEnclaveHash + "|" + quoteHash + "|" + orgID + "|" + strconv.FormatInt(validTo, 10) + "|TDIDRegistry"
	return crypto.Keccak256([]byte(payload))
}

func verifySecp256k1Sig(pubKeyHex string, msgHash []byte, sigHex string) (bool, error) {
	pubRaw, err := decodeHex(pubKeyHex)
	if err != nil {
		return false, err
	}
	sigRaw, err := decodeHex(sigHex)
	if err != nil {
		return false, err
	}

	if len(sigRaw) != 65 {
		return false, fmt.Errorf("signature length must be 65, got %d", len(sigRaw))
	}

	return crypto.VerifySignature(pubRaw, msgHash, sigRaw[:64]), nil
}

func normalizeHex(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	if strings.HasPrefix(v, "0x") {
		return v
	}
	return "0x" + v
}

func decodeHex(value string) ([]byte, error) {
	v := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if len(v)%2 != 0 {
		v = "0" + v
	}
	raw, err := hex.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}
	return raw, nil
}
