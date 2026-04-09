// SPDX-License-Identifier: MIT
pragma solidity ^0.8.11;

contract GovernanceRoot {
    address public admin;

    mapping(string => address) private orgSigners;
    mapping(bytes32 => bool) private allowedMeasurements;

    event Event_AdminTransferred(address indexed oldAdmin, address indexed newAdmin);
    event Event_OrgSignerAdded(string indexed orgId, address indexed signerPk);
    event Event_OrgSignerRemoved(string indexed orgId, address indexed oldSignerPk);
    event Event_MeasurementUpdated(bytes32 indexed mrEnclaveHash, bool allowed);

    modifier onlyAdmin() {
        require(msg.sender == admin, "only admin");
        _;
    }

    constructor(address initialAdmin) {
        require(initialAdmin != address(0), "invalid admin");
        admin = initialAdmin;
    }

    function transferAdmin(address newAdmin) external onlyAdmin {
        require(newAdmin != address(0), "invalid admin");
        address old = admin;
        admin = newAdmin;
        emit Event_AdminTransferred(old, newAdmin);
    }

    function addOrgSigner(string memory orgId, address signerPk) external onlyAdmin {
        require(bytes(orgId).length > 0, "orgId empty");
        require(signerPk != address(0), "invalid signer");

        orgSigners[orgId] = signerPk;
        emit Event_OrgSignerAdded(orgId, signerPk);
    }

    function removeOrgSigner(string memory orgId) external onlyAdmin {
        require(bytes(orgId).length > 0, "orgId empty");
        address oldSigner = orgSigners[orgId];
        require(oldSigner != address(0), "signer not set");

        delete orgSigners[orgId];
        emit Event_OrgSignerRemoved(orgId, oldSigner);
    }

    function setMeasurementAllowed(bytes32 mrEnclaveHash, bool allowed) external onlyAdmin {
        require(mrEnclaveHash != bytes32(0), "invalid measurement");
        allowedMeasurements[mrEnclaveHash] = allowed;
        emit Event_MeasurementUpdated(mrEnclaveHash, allowed);
    }

    function isSignerAllowed(string memory orgId, address signerPk) external view returns (bool) {
        return orgSigners[orgId] == signerPk;
    }

    function isMeasurementAllowed(bytes32 mrEnclaveHash) external view returns (bool) {
        return allowedMeasurements[mrEnclaveHash];
    }

    function getOrgSigner(string memory orgId) external view returns (address) {
        return orgSigners[orgId];
    }
}
