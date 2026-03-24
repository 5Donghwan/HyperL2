use crate::snark::{CircuitSpecificSetupCCSNARK, CCSNARK};
/// Reduce an R1CS instance to a *Quadratic Arithmetic Program* instance.
pub use ark_groth16::r1cs_to_qap;

/// Data structures used by the prover, verifier, and generator.
pub mod data_structures;

/// Generate public parameters for the Groth16 zkSNARK construction.
pub mod generator;

/// Create proofs for the Groth16 zkSNARK construction.
pub mod prover;

/// Verify proofs for the Groth16 zkSNARK construction.
pub mod verifier;

pub use self::data_structures::*;
pub use self::verifier::*;

use ark_ec::pairing::Pairing;
use ark_groth16::r1cs_to_qap::{LibsnarkReduction, R1CSToQAP};
use ark_relations::r1cs::{ConstraintSynthesizer, SynthesisError};
use ark_std::{marker::PhantomData, rand::RngCore};

pub struct CCGroth16<E: Pairing, QAP: R1CSToQAP = LibsnarkReduction> {
    _p: PhantomData<(E, QAP)>,
}

impl<E: Pairing, QAP: R1CSToQAP> CCSNARK<E> for CCGroth16<E, QAP> {
    type CommittingKey = CommittingKey<E>;
    type ProvingKey = ProvingKey<E>;
    type VerifyingKey = VerifyingKey<E>;
    type Commitment = Commitment<E>;
    type Proof = Proof<E>;
    type ProcessedVerifyingKey = PreparedVerifyingKey<E>;
    type Error = SynthesisError;

    fn circuit_specific_setup<C: ConstraintSynthesizer<E::ScalarField>, R: RngCore>(
        circuit: C,
        num_aggregation_variables: usize,
        num_committed_witness_variables: usize,
        rng: &mut R,
    ) -> Result<(Self::ProvingKey, Self::VerifyingKey, Self::CommittingKey), Self::Error> {
        let pk = Self::generate_random_parameters_with_reduction(
            circuit,
            num_aggregation_variables,
            num_committed_witness_variables,
            rng,
        )?;
        let vk = pk.vk.clone();
        let ck = vk.ck.clone();

        Ok((pk, vk, ck))
    }

    fn commit<R: RngCore>(
        circuit_ck: &Self::CommittingKey,
        committed_witness: &[E::ScalarField],
        rng: &mut R,
    ) -> Result<Self::Commitment, Self::Error> {
        Self::batch_commit_with_challenge(circuit_ck, committed_witness, rng)
    }

    fn prove<C: ConstraintSynthesizer<E::ScalarField>, R: RngCore>(
        pk: &Self::ProvingKey,
        circuit: C,
        commitment: &Self::Commitment,
        rng: &mut R,
    ) -> Result<Self::Proof, Self::Error> {
        Self::create_random_proof_with_reduction(circuit, pk, commitment, rng)
    }

    fn process_vk(
        circuit_vk: &Self::VerifyingKey,
    ) -> Result<Self::ProcessedVerifyingKey, Self::Error> {
        Ok(prepare_verifying_key(circuit_vk))
    }

    fn verify_with_processed_vk(
        circuit_pvk: &Self::ProcessedVerifyingKey,
        public_input: &[E::ScalarField],
        proof: &Self::Proof,
    ) -> Result<bool, Self::Error> {
        Ok(Self::verify_proof(circuit_pvk, proof, public_input)?)
    }
}

impl<E: Pairing, QAP: R1CSToQAP> CircuitSpecificSetupCCSNARK<E> for CCGroth16<E, QAP> {}
