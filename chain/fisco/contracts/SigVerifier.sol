// SPDX-License-Identifier: MIT
pragma solidity ^0.8.11;

interface ISessionKeyRegistry {
    function verifyBindSig(bytes32 didNode, bytes memory pkSess, uint256 expireAt, bytes memory bindSig) external view returns (bool);
    function verifySessionSig(bytes32 keyId, bytes32 payloadHash, bytes memory sessSig) external view returns (bool);
}

contract SigVerifier {
    ISessionKeyRegistry public sessionRegistry;

    constructor(address sessionRegistryAddress) {
        require(sessionRegistryAddress != address(0), "invalid registry");
        sessionRegistry = ISessionKeyRegistry(sessionRegistryAddress);
    }

    function verifyBindSig(
        bytes32 didNode,
        bytes memory pkSess,
        uint256 expireAt,
        bytes memory bindSig
    ) external view returns (bool) {
        return sessionRegistry.verifyBindSig(didNode, pkSess, expireAt, bindSig);
    }

    function verifySessionSig(
        bytes32 keyId,
        bytes32 payloadHash,
        bytes memory sessSig
    ) external view returns (bool) {
        return sessionRegistry.verifySessionSig(keyId, payloadHash, sessSig);
    }
}
