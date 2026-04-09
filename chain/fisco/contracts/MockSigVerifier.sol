// SPDX-License-Identifier: MIT
pragma solidity ^0.8.11;

contract MockSigVerifier {
    bool public result = true;

    function setResult(bool v) external {
        result = v;
    }

    function verifySessionSig(bytes32, bytes32, bytes memory) external view returns (bool) {
        return result;
    }
}
