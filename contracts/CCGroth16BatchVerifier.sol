// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Bn254} from "./Bn254.sol";
import {ICCGroth16BatchVerifier} from "./ICCGroth16BatchVerifier.sol";

contract CCGroth16BatchVerifier is ICCGroth16BatchVerifier {
    bytes internal constant HYPERL2_SUM_BATCH_DOMAIN = "HyperL2.SumPreservingBatch.v1";
    uint256 internal constant SNARK_SCALAR_FIELD =
        21888242871839275222246405745257275088548364400416034343698204186575808495617;

    struct VerifyingKey {
        Bn254.G1Point alphaG1;
        Bn254.G2Point betaG2;
        Bn254.G2Point gammaG2;
        Bn254.G2Point deltaG2;
        Bn254.G1Point[2] gammaAbcG1;
    }

    address public immutable admin;
    bool public verifyingKeyInitialized;
    VerifyingKey private verifyingKey;

    event VerifyingKeyInitialized();

    constructor() {
        admin = msg.sender;
    }

    function initializeVerifyingKey(VerifyingKey calldata vk) external {
        require(msg.sender == admin, "not admin");
        require(!verifyingKeyInitialized, "vk already initialized");
        verifyingKey = vk;
        verifyingKeyInitialized = true;
        emit VerifyingKeyInitialized();
    }

    function getVerifyingKey() external view returns (VerifyingKey memory) {
        return verifyingKey;
    }

    function verifyBatchTransition(
        BatchTransitionArtifact calldata artifact
    ) external view override returns (bool) {
        require(verifyingKeyInitialized, "vk not initialized");

        // This reconstructs the exact verifier path used off-chain:
        // tau <- transcript(D1, D2, proof.d, metadata)
        // Agg <- tau * D1 + tau^2 * D2
        // proof.d' <- proof.d + Agg
        // prepared_inputs <- gamma_abc[0] + gamma_abc[1] * tau
        // pairing check over 4 pairs
        uint256 tau = _computeTau(artifact);
        Bn254.G1Point memory aggregation = _computeAggregation(
            artifact.prevStateCommitment,
            artifact.nextStateCommitment,
            tau
        );
        Bn254.G1Point memory preparedProofD = Bn254.g1Add(artifact.proof.d, aggregation);
        Bn254.G1Point memory preparedInputs = _prepareInputs(tau);
        Bn254.G1Point memory dWithInputs = Bn254.g1Add(preparedProofD, preparedInputs);

        return Bn254.pairingProd4(
            artifact.proof.a,
            artifact.proof.b,
            Bn254.negate(dWithInputs),
            verifyingKey.gammaG2,
            Bn254.negate(artifact.proof.c),
            verifyingKey.deltaG2,
            Bn254.negate(verifyingKey.alphaG1),
            verifyingKey.betaG2
        );
    }

    function _computeTau(
        BatchTransitionArtifact calldata artifact
    ) private pure returns (uint256) {
        return uint256(
            keccak256(
                abi.encodePacked(
                    HYPERL2_SUM_BATCH_DOMAIN,
                    artifact.laneId,
                    artifact.batchId,
                    artifact.txCount,
                    artifact.prevStateCommitment.x,
                    artifact.prevStateCommitment.y,
                    artifact.nextStateCommitment.x,
                    artifact.nextStateCommitment.y,
                    artifact.proof.d.x,
                    artifact.proof.d.y
                )
            )
        ) % SNARK_SCALAR_FIELD;
    }

    function _prepareInputs(uint256 tau) private view returns (Bn254.G1Point memory) {
        Bn254.G1Point memory vkX = verifyingKey.gammaAbcG1[0];
        Bn254.G1Point memory tauTerm = Bn254.g1Mul(verifyingKey.gammaAbcG1[1], tau);
        return Bn254.g1Add(vkX, tauTerm);
    }

    function _computeAggregation(
        Bn254.G1Point calldata prevStateCommitment,
        Bn254.G1Point calldata nextStateCommitment,
        uint256 tau
    ) private view returns (Bn254.G1Point memory) {
        uint256 tauSquared = mulmod(tau, tau, SNARK_SCALAR_FIELD);
        Bn254.G1Point memory prevTerm = Bn254.g1Mul(prevStateCommitment, tau);
        Bn254.G1Point memory nextTerm = Bn254.g1Mul(nextStateCommitment, tauSquared);
        return Bn254.g1Add(prevTerm, nextTerm);
    }
}
