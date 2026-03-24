// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Bn254} from "./Bn254.sol";

interface ICCGroth16BatchVerifier {
    // G2 coordinates follow the bn254 precompile convention:
    // x = [c1, c0], y = [c1, c0]
    struct BatchTransitionProof {
        Bn254.G1Point a;
        Bn254.G2Point b;
        Bn254.G1Point c;
        Bn254.G1Point d;
    }

    struct BatchTransitionArtifact {
        uint16 laneId;
        uint64 batchId;
        uint32 txCount;
        Bn254.G1Point prevStateCommitment;
        Bn254.G1Point nextStateCommitment;
        BatchTransitionProof proof;
    }

    function verifyBatchTransition(BatchTransitionArtifact calldata artifact)
        external
        view
        returns (bool);
}
