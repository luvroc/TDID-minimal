package main

import "github.com/hyperledger/fabric-contract-api-go/v2/contractapi"

type SigVerifierContract struct {
	contractapi.Contract
}

func (s *SigVerifierContract) VerifyBindSig(
	ctx contractapi.TransactionContextInterface,
	didNode string,
	pkSess string,
	expireAt int64,
	bindSig string,
) (bool, error) {
	return (&SessionKeyRegistryContract{}).VerifyBindSig(ctx, didNode, pkSess, expireAt, bindSig)
}

func (s *SigVerifierContract) VerifySessionSig(
	ctx contractapi.TransactionContextInterface,
	keyID string,
	payloadHash string,
	sessSig string,
) (bool, error) {
	return (&SessionKeyRegistryContract{}).VerifySessionSig(ctx, keyID, payloadHash, sessSig)
}
