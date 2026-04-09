package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

func main() {
	gatewayCC, err := contractapi.NewChaincode(&GatewayContract{})
	if err != nil {
		log.Panicf("create gateway chaincode failed: %v", err)
	}

	if err := gatewayCC.Start(); err != nil {
		log.Panicf("start gateway chaincode failed: %v", err)
	}
}
