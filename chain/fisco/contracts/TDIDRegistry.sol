// SPDX-License-Identifier: MIT
pragma solidity ^0.8.11;

interface IGovernanceRoot {
    function admin() external view returns (address);
    function isSignerAllowed(string memory orgId, address signerPk) external view returns (bool);
    function isMeasurementAllowed(bytes32 mrEnclaveHash) external view returns (bool);
    function getOrgSigner(string memory orgId) external view returns (address);
}

contract TDIDRegistry {
    enum NodeStatus {
        NONE,
        ACTIVE,
        REVOKED
    }

    struct NodeInfo {
        bytes pkNode;
        bytes32 mrEnclaveHash;
        bytes32 quoteHash;
        string orgId;
        NodeStatus status;
        uint256 validTo;
    }

    IGovernanceRoot public governanceRoot;
    mapping(bytes32 => NodeInfo) private nodes;

    event Event_NodeRegistered(
        bytes32 indexed didNode,
        string orgId,
        bytes32 mrEnclaveHash,
        bytes32 quoteHash,
        uint256 validTo
    );
    event Event_NodeRevoked(bytes32 indexed didNode, string reason);

    modifier onlyGovernanceAdmin() {
        require(msg.sender == governanceRoot.admin(), "only governance admin");
        _;
    }

    constructor(address governanceRootAddress) {
        require(governanceRootAddress != address(0), "invalid governance root");
        governanceRoot = IGovernanceRoot(governanceRootAddress);
    }

    function registerNode(
        bytes32 didNode,
        bytes memory pkNode,
        bytes32 mrEnclaveHash,
        bytes32 quoteHash,
        string memory orgId,
        bytes memory orgSig,
        uint256 validTo
    ) external {
        require(didNode != bytes32(0), "didNode empty");
        require(pkNode.length > 0, "pkNode empty");
        require(bytes(orgId).length > 0, "orgId empty");
        require(validTo > block.timestamp, "validTo expired");
        require(governanceRoot.isMeasurementAllowed(mrEnclaveHash), "measurement not allowed");

        bytes32 expectDID = keccak256(pkNode);
        require(expectDID == didNode, "didNode mismatch");

        NodeInfo storage oldNode = nodes[didNode];
        require(oldNode.status != NodeStatus.ACTIVE, "active node already exists");

        bytes32 msgHash = _buildRegisterHash(didNode, pkNode, mrEnclaveHash, quoteHash, orgId, validTo);
        address recovered = _recoverSigner(msgHash, orgSig);
        require(governanceRoot.isSignerAllowed(orgId, recovered), "invalid org signature");

        nodes[didNode] = NodeInfo({
            pkNode: pkNode,
            mrEnclaveHash: mrEnclaveHash,
            quoteHash: quoteHash,
            orgId: orgId,
            status: NodeStatus.ACTIVE,
            validTo: validTo
        });

        emit Event_NodeRegistered(didNode, orgId, mrEnclaveHash, quoteHash, validTo);
    }

    function revokeNode(bytes32 didNode) external onlyGovernanceAdmin {
        NodeInfo storage node = nodes[didNode];
        require(node.status == NodeStatus.ACTIVE, "node not active");

        node.status = NodeStatus.REVOKED;
        emit Event_NodeRevoked(didNode, "manual revoke");
    }

    function getNode(bytes32 didNode) external view returns (
        bytes memory pkNode,
        bytes32 mrEnclaveHash,
        bytes32 quoteHash,
        string memory orgId,
        string memory status,
        uint256 validTo
    ) {
        NodeInfo storage n = nodes[didNode];
        string memory textStatus = "NONE";
        if (n.status == NodeStatus.ACTIVE) {
            textStatus = "ACTIVE";
        } else if (n.status == NodeStatus.REVOKED) {
            textStatus = "REVOKED";
        }
        return (n.pkNode, n.mrEnclaveHash, n.quoteHash, n.orgId, textStatus, n.validTo);
    }

    function isNodeActive(bytes32 didNode) external view returns (bool) {
        NodeInfo storage n = nodes[didNode];
        return n.status == NodeStatus.ACTIVE && n.validTo >= block.timestamp;
    }

    function _buildRegisterHash(
        bytes32 didNode,
        bytes memory pkNode,
        bytes32 mrEnclaveHash,
        bytes32 quoteHash,
        string memory orgId,
        uint256 validTo
    ) internal view returns (bytes32) {
        return keccak256(
            abi.encodePacked(
                didNode,
                keccak256(pkNode),
                mrEnclaveHash,
                quoteHash,
                orgId,
                validTo,
                block.chainid,
                address(this)
            )
        );
    }

    function _recoverSigner(bytes32 msgHash, bytes memory sig) internal pure returns (address) {
        require(sig.length == 65, "invalid sig length");
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
        require(v == 27 || v == 28, "invalid sig v");

        return ecrecover(msgHash, v, r, s);
    }
}
