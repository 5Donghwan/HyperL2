// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Bn254} from "./Bn254.sol";
import {ICCGroth16BatchVerifier} from "./ICCGroth16BatchVerifier.sol";

contract SumPreservingBatchVerifier {
    uint32 public constant BATCH_TX_COUNT = 20_000;
    uint8 public constant CERT_BATCH_COUNT = 10;

    ICCGroth16BatchVerifier public immutable verifier;
    mapping(uint16 => Bn254.G1Point) public laneHead;
    uint256 public totalVerifiedTx;

    event LaneHeadInitialized(uint16 indexed laneId, uint256 x, uint256 y);
    event BatchVerified(uint16 indexed laneId, uint64 indexed batchId, uint32 txCount);

    constructor(ICCGroth16BatchVerifier verifier_) {
        verifier = verifier_;
    }

    function initializeLaneHead(
        uint16 laneId,
        Bn254.G1Point calldata initialHead
    ) external {
        require(!_isSet(laneHead[laneId]), "lane already initialized");
        laneHead[laneId] = initialHead;
        emit LaneHeadInitialized(laneId, initialHead.x, initialHead.y);
    }

    function verifyTenBatches(
        ICCGroth16BatchVerifier.BatchTransitionArtifact[10] calldata artifacts
    ) external {
        uint256 verifiedTx = 0;

        for (uint256 i = 0; i < CERT_BATCH_COUNT; ++i) {
            ICCGroth16BatchVerifier.BatchTransitionArtifact calldata artifact = artifacts[i];
            require(artifact.txCount == BATCH_TX_COUNT, "unexpected tx count");

            Bn254.G1Point memory currentHead = laneHead[artifact.laneId];
            require(_isSet(currentHead), "lane not initialized");
            require(_samePoint(currentHead, artifact.prevStateCommitment), "prev head mismatch");
            require(verifier.verifyBatchTransition(artifact), "invalid batch proof");

            laneHead[artifact.laneId] = artifact.nextStateCommitment;
            verifiedTx += artifact.txCount;
            emit BatchVerified(artifact.laneId, artifact.batchId, artifact.txCount);
        }

        totalVerifiedTx += verifiedTx;
    }

    function _samePoint(
        Bn254.G1Point memory lhs,
        Bn254.G1Point memory rhs
    ) private pure returns (bool) {
        return lhs.x == rhs.x && lhs.y == rhs.y;
    }

    function _isSet(Bn254.G1Point memory point) private pure returns (bool) {
        return point.x != 0 || point.y != 0;
    }
}
