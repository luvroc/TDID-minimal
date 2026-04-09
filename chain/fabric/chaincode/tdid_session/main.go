package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

func main() {
	cc, err := contractapi.NewChaincode(
		&SessionKeyRegistryContract{},
		&SigVerifierContract{},
	)
	if err != nil {
		log.Panicf("create session chaincode failed: %v", err)
	}

	if err := cc.Start(); err != nil {
		log.Panicf("start session chaincode failed: %v", err)
	}
}
