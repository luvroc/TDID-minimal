package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	keyGovAdminPrefix         = "gov:admin"
	keyOrgSignerPrefix        = "gov:orgSigner:"
	keyMeasurementAllowPrefix = "gov:measurement:"

	eventOrgSignerAdded    = "Event_OrgSignerAdded"
	eventOrgSignerRemoved  = "Event_OrgSignerRemoved"
	eventMeasurementUpdate = "Event_MeasurementUpdated"
	eventAdminBootstrapped = "Event_AdminBootstrapped"
)

type GovernanceRootContract struct {
	contractapi.Contract
}

type OrgSignerEvent struct {
	OrgID    string `json:"orgId"`
	SignerPK string `json:"signerPk"`
}

type MeasurementEvent struct {
	MrEnclaveHash string `json:"mrEnclaveHash"`
	Allowed       bool   `json:"allowed"`
}

type AdminEvent struct {
	AdminClientID string `json:"adminClientId"`
}

func orgSignerKey(orgID string) string {
	return keyOrgSignerPrefix + orgID
}

func measurementKey(hash string) string {
	return keyMeasurementAllowPrefix + hash
}

func (g *GovernanceRootContract) BootstrapAdmin(ctx contractapi.TransactionContextInterface, adminClientID string) error {
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
	return emitJSONEvent(ctx, eventAdminBootstrapped, AdminEvent{AdminClientID: adminClientID})
}

func (g *GovernanceRootContract) AddOrgSigner(ctx contractapi.TransactionContextInterface, orgID, signerPK string) error {
	if orgID == "" || signerPK == "" {
		return errors.New("orgID/signerPK cannot be empty")
	}
	if err := requireGovernanceAdmin(ctx); err != nil {
		return err
	}

	if err := ctx.GetStub().PutState(orgSignerKey(orgID), []byte(signerPK)); err != nil {
		return err
	}

	return emitJSONEvent(ctx, eventOrgSignerAdded, OrgSignerEvent{OrgID: orgID, SignerPK: signerPK})
}

func (g *GovernanceRootContract) RemoveOrgSigner(ctx contractapi.TransactionContextInterface, orgID string) error {
	if orgID == "" {
		return errors.New("orgID cannot be empty")
	}
	if err := requireGovernanceAdmin(ctx); err != nil {
		return err
	}

	signer, err := g.GetOrgSigner(ctx, orgID)
	if err != nil {
		return err
	}
	if signer == "" {
		return fmt.Errorf("org signer for %s not found", orgID)
	}

	if err := ctx.GetStub().DelState(orgSignerKey(orgID)); err != nil {
		return err
	}

	return emitJSONEvent(ctx, eventOrgSignerRemoved, OrgSignerEvent{OrgID: orgID, SignerPK: signer})
}

func (g *GovernanceRootContract) SetMeasurementAllowed(ctx contractapi.TransactionContextInterface, mrEnclaveHash string, allowed bool) error {
	if mrEnclaveHash == "" {
		return errors.New("mrEnclaveHash cannot be empty")
	}
	if err := requireGovernanceAdmin(ctx); err != nil {
		return err
	}

	raw, err := json.Marshal(allowed)
	if err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(measurementKey(mrEnclaveHash), raw); err != nil {
		return err
	}

	return emitJSONEvent(ctx, eventMeasurementUpdate, MeasurementEvent{MrEnclaveHash: mrEnclaveHash, Allowed: allowed})
}

func (g *GovernanceRootContract) IsSignerAllowed(ctx contractapi.TransactionContextInterface, orgID, signerPK string) (bool, error) {
	current, err := g.GetOrgSigner(ctx, orgID)
	if err != nil {
		return false, err
	}
	return current == signerPK && current != "", nil
}

func (g *GovernanceRootContract) IsMeasurementAllowed(ctx contractapi.TransactionContextInterface, mrEnclaveHash string) (bool, error) {
	raw, err := ctx.GetStub().GetState(measurementKey(mrEnclaveHash))
	if err != nil {
		return false, err
	}
	if len(raw) == 0 {
		return false, nil
	}
	var allowed bool
	if err := json.Unmarshal(raw, &allowed); err != nil {
		return false, err
	}
	return allowed, nil
}

func (g *GovernanceRootContract) GetOrgSigner(ctx contractapi.TransactionContextInterface, orgID string) (string, error) {
	raw, err := ctx.GetStub().GetState(orgSignerKey(orgID))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", nil
	}
	return string(raw), nil
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
