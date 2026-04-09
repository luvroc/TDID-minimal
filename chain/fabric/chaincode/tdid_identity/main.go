package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

func main() {
	cc, err := contractapi.NewChaincode(
		&GovernanceRootContract{},
		&TDIDRegistryContract{},
	)
	if err != nil {
		log.Panicf("create identity chaincode failed: %v", err)
	}

	if err := cc.Start(); err != nil {
		log.Panicf("start identity chaincode failed: %v", err)
	}
}
