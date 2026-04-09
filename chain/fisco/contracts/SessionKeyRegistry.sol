// SPDX-License-Identifier: MIT
pragma solidity ^0.8.11;

interface ITDIDRegistry {
    function isNodeActive(bytes32 didNode) external view returns (bool);
    function getNode(bytes32 didNode) external view returns (
        bytes memory pkNode,
        bytes32 mrEnclaveHash,
        bytes32 quoteHash,
        string memory orgId,
        string memory status,
        uint256 validTo
    );
}

contract SessionKeyRegistry {
    enum SessionStatus {
        NONE,
        ACTIVE,
        REVOKED
    }

    struct SessionInfo {
        bytes32 ownerDID;
        bytes pkSess;
        address sessSigner;
        uint256 expireAt;
        SessionStatus status;
    }

    ITDIDRegistry public tdidRegistry;
    mapping(bytes32 => SessionInfo) private sessions;

    event Event_SessionBound(bytes32 indexed keyId, bytes32 indexed ownerDID, uint256 expireAt, address sessSigner);
    event Event_SessionRevoked(bytes32 indexed keyId, string reason);

    constructor(address tdidRegistryAddress) {
        require(tdidRegistryAddress != address(0), "invalid TDID registry");
        tdidRegistry = ITDIDRegistry(tdidRegistryAddress);
    }

    function bindSession(
        bytes32 didNode,
        bytes memory pkSess,
        uint256 expireAt,
        bytes memory bindSig
    ) external returns (bytes32 keyId) {
        require(didNode != bytes32(0), "didNode empty");
        require(pkSess.length > 0, "pkSess empty");
        require(expireAt > block.timestamp, "expireAt invalid");
        require(tdidRegistry.isNodeActive(didNode), "didNode not active");
        require(verifyBindSig(didNode, pkSess, expireAt, bindSig), "invalid bindSig");

        keyId = keccak256(pkSess);
        SessionInfo storage old = sessions[keyId];
        require(old.status != SessionStatus.ACTIVE, "session already active");

        address sessSigner = _pubKeyToAddress(pkSess);
        sessions[keyId] = SessionInfo({
            ownerDID: didNode,
            pkSess: pkSess,
            sessSigner: sessSigner,
            expireAt: expireAt,
            status: SessionStatus.ACTIVE
        });

        emit Event_SessionBound(keyId, didNode, expireAt, sessSigner);
    }

    function revokeSession(bytes32 keyId) external {
        SessionInfo storage s = sessions[keyId];
        require(s.status == SessionStatus.ACTIVE, "session not active");
        require(s.ownerDID != bytes32(0), "session missing owner");

        s.status = SessionStatus.REVOKED;
        emit Event_SessionRevoked(keyId, "manual revoke");
    }

    function getSession(bytes32 keyId) external view returns (
        bytes32 ownerDID,
        bytes memory pkSess,
        uint256 expireAt,
        string memory status
    ) {
        SessionInfo storage s = sessions[keyId];
        string memory st = "NONE";
        if (s.status == SessionStatus.ACTIVE) {
            st = "ACTIVE";
        } else if (s.status == SessionStatus.REVOKED) {
            st = "REVOKED";
        }
        return (s.ownerDID, s.pkSess, s.expireAt, st);
    }

    function isSessionActive(bytes32 keyId) public view returns (bool) {
        SessionInfo storage s = sessions[keyId];
        return s.status == SessionStatus.ACTIVE && s.expireAt >= block.timestamp;
    }

    function verifyBindSig(
        bytes32 didNode,
        bytes memory pkSess,
        uint256 expireAt,
        bytes memory bindSig
    ) public view returns (bool) {
        if (!tdidRegistry.isNodeActive(didNode) || expireAt <= block.timestamp) {
            return false;
        }

        (bytes memory pkNode,,,,,) = tdidRegistry.getNode(didNode);
        if (pkNode.length == 0) {
            return false;
        }

        bytes32 msgHash = _buildBindHash(pkSess, expireAt);
        address recovered = _recoverSigner(msgHash, bindSig);
        return recovered != address(0) && recovered == _pubKeyToAddress(pkNode);
    }

    function verifySessionSig(bytes32 keyId, bytes32 payloadHash, bytes memory sessSig) public view returns (bool) {
        SessionInfo storage s = sessions[keyId];
        if (s.status != SessionStatus.ACTIVE || s.expireAt < block.timestamp) {
            return false;
        }

        address recovered = _recoverSigner(payloadHash, sessSig);
        return recovered != address(0) && recovered == s.sessSigner;
    }

    function _buildBindHash(bytes memory pkSess, uint256 expireAt) internal view returns (bytes32) {
        return keccak256(abi.encodePacked(pkSess, expireAt, block.chainid, address(this)));
    }

    function _pubKeyToAddress(bytes memory pubKey) internal pure returns (address) {
        if (pubKey.length == 65 && pubKey[0] == 0x04) {
            return address(uint160(uint256(keccak256(slice(pubKey, 1, 64)))));
        }
        if (pubKey.length == 64) {
            return address(uint160(uint256(keccak256(pubKey))));
        }
        return address(0);
    }

    function _recoverSigner(bytes32 msgHash, bytes memory sig) internal pure returns (address) {
        if (sig.length != 65) {
            return address(0);
        }
        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := mload(add(sig, 32))
            s := mload(add(sig, 64))
            v := byte(0, mload(add(sig, 96)))
        }
        if (v < 27) {
            v += 27;
        }
        if (v != 27 && v != 28) {
            return address(0);
        }
        return ecrecover(msgHash, v, r, s);
    }

    function slice(bytes memory data, uint256 start, uint256 len) internal pure returns (bytes memory) {
        require(data.length >= start + len, "slice out of range");
        bytes memory out = new bytes(len);
        for (uint256 i = 0; i < len; i++) {
            out[i] = data[start + i];
        }
        return out;
    }
}
