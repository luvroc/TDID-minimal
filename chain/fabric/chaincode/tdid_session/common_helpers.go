package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const keyGovAdminPrefix = "gov:admin"

type AdminEvent struct {
	AdminClientID string `json:"adminClientId"`
}

func (s *SessionKeyRegistryContract) BootstrapAdmin(ctx contractapi.TransactionContextInterface, adminClientID string) error {
	if adminClientID == "" {
		return errors.New("adminClientID cannot be empty")
	}
	exists, err := ctx.GetStub().GetState(keyGovAdminPrefix)
	if err != nil {
		return err
	}
	if len(exists) != 0 {
		return errors.New("admin already bootstrapped")
	}

	invokerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return err
	}
	if invokerID != adminClientID {
		return errors.New("invoker must equal adminClientID during bootstrap")
	}

	if err := ctx.GetStub().PutState(keyGovAdminPrefix, []byte(adminClientID)); err != nil {
		return err
	}
	return emitJSONEvent(ctx, "Event_AdminBootstrapped", AdminEvent{AdminClientID: adminClientID})
}

func requireGovernanceAdmin(ctx contractapi.TransactionContextInterface) error {
	adminRaw, err := ctx.GetStub().GetState(keyGovAdminPrefix)
	if err != nil {
		return err
	}
	if len(adminRaw) == 0 {
		return errors.New("governance admin not bootstrapped")
	}

	invokerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return err
	}
	if invokerID != string(adminRaw) {
		return errors.New("only governance admin can call this function")
	}
	return nil
}

func emitJSONEvent(ctx contractapi.TransactionContextInterface, eventName string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return ctx.GetStub().SetEvent(eventName, raw)
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
