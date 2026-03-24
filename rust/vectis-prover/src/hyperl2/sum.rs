use core::fmt;

use ark_ec::{pairing::Pairing, AffineRepr, CurveGroup};
use ark_ff::{PrimeField, Zero};
use ark_r1cs_std::{alloc::AllocVar, eq::EqGadget, fields::fp::FpVar};
use ark_relations::r1cs::{ConstraintSynthesizer, ConstraintSystemRef, SynthesisError};
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize, SerializationError};
use ark_std::{rand::{CryptoRng, RngCore}, vec::Vec};

use crate::{
    crypto::commitment::{
        pedersen::{Pedersen, PedersenGadget},
        BatchCommitmentGadget, BatchCommitmentScheme, CommitmentScheme,
    },
    gro::{CCGroth16, CommittingKey, Proof, ProvingKey, VerifyingKey},
    snark::{CCSNARK, CircuitSpecificSetupCCSNARK},
};

use super::challenge::compute_batch_challenge;

#[derive(Debug)]
pub enum HyperL2Error {
    InvalidInput(String),
    Synthesis(SynthesisError),
    Serialization(SerializationError),
}

impl fmt::Display for HyperL2Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InvalidInput(message) => write!(f, "invalid input: {message}"),
            Self::Synthesis(err) => write!(f, "synthesis error: {err}"),
            Self::Serialization(err) => write!(f, "serialization error: {err}"),
        }
    }
}

impl std::error::Error for HyperL2Error {}

impl From<SynthesisError> for HyperL2Error {
    fn from(value: SynthesisError) -> Self {
        Self::Synthesis(value)
    }
}

impl From<SerializationError> for HyperL2Error {
    fn from(value: SerializationError) -> Self {
        Self::Serialization(value)
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct BatchMetadata {
    pub lane_id: u16,
    pub batch_id: u64,
    pub tx_count: u32,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct StateTransitionBatchInput<F: PrimeField> {
    pub lane_id: u16,
    pub batch_id: u64,
    pub tx_count: u32,
    pub pre_state_values: Vec<F>,
    pub post_state_values: Vec<F>,
    pub pre_state_blind: F,
    pub post_state_blind: F,
}

pub type ProverBatchInput<F> = StateTransitionBatchInput<F>;

impl<F: PrimeField> StateTransitionBatchInput<F> {
    pub fn batch_size(&self) -> Result<usize, HyperL2Error> {
        usize::try_from(self.tx_count)
            .map_err(|_| HyperL2Error::InvalidInput("tx_count does not fit usize".to_string()))
    }

    pub fn metadata(&self) -> BatchMetadata {
        BatchMetadata {
            lane_id: self.lane_id,
            batch_id: self.batch_id,
            tx_count: self.tx_count,
        }
    }

    pub fn validate(&self) -> Result<(), HyperL2Error> {
        let batch_size = self.batch_size()?;
        if batch_size == 0 {
            return Err(HyperL2Error::InvalidInput(
                "batch_size must be greater than zero".to_string(),
            ));
        }
        if self.pre_state_values.len() != batch_size {
            return Err(HyperL2Error::InvalidInput(format!(
                "pre_state_values length {} does not match tx_count {}",
                self.pre_state_values.len(),
                batch_size
            )));
        }
        if self.post_state_values.len() != batch_size {
            return Err(HyperL2Error::InvalidInput(format!(
                "post_state_values length {} does not match tx_count {}",
                self.post_state_values.len(),
                batch_size
            )));
        }
        Ok(())
    }

    pub fn pre_state_commitment_vector(&self) -> Vec<F> {
        let mut values = self.pre_state_values.clone();
        values.push(self.pre_state_blind);
        values
    }

    pub fn post_state_commitment_vector(&self) -> Vec<F> {
        let mut values = self.post_state_values.clone();
        values.push(self.post_state_blind);
        values
    }
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct BatchProofArtifact<E: Pairing> {
    pub lane_id: u16,
    pub batch_id: u64,
    pub tx_count: u32,
    pub prev_state_commitment: E::G1Affine,
    pub next_state_commitment: E::G1Affine,
    pub proof: Proof<E>,
}

impl<E: Pairing> BatchProofArtifact<E> {
    pub fn metadata(&self) -> BatchMetadata {
        BatchMetadata {
            lane_id: self.lane_id,
            batch_id: self.batch_id,
            tx_count: self.tx_count,
        }
    }
}

#[derive(Clone, Debug)]
pub struct SumPreservingBatchCircuit<C: CurveGroup> {
    pub batch_size: usize,
    pub tau: Option<C::ScalarField>,
    pub aggregation: Option<Vec<C::ScalarField>>,
    pub pre_state_vec: Option<Vec<C::ScalarField>>,
    pub post_state_vec: Option<Vec<C::ScalarField>>,
}

impl<C: CurveGroup> SumPreservingBatchCircuit<C> {
    fn width(batch_size: usize) -> usize {
        batch_size + 1
    }

    pub fn new(
        batch_size: usize,
        tau: C::ScalarField,
        aggregation: Vec<C::ScalarField>,
        pre_state_vec: Vec<C::ScalarField>,
        post_state_vec: Vec<C::ScalarField>,
    ) -> Result<Self, HyperL2Error> {
        let width = Self::width(batch_size);
        if aggregation.len() != width {
            return Err(HyperL2Error::InvalidInput(format!(
                "aggregation length {} does not match expected width {}",
                aggregation.len(),
                width
            )));
        }
        if pre_state_vec.len() != width {
            return Err(HyperL2Error::InvalidInput(format!(
                "pre-state vector length {} does not match expected width {}",
                pre_state_vec.len(),
                width
            )));
        }
        if post_state_vec.len() != width {
            return Err(HyperL2Error::InvalidInput(format!(
                "post-state vector length {} does not match expected width {}",
                post_state_vec.len(),
                width
            )));
        }
        Ok(Self {
            batch_size,
            tau: Some(tau),
            aggregation: Some(aggregation),
            pre_state_vec: Some(pre_state_vec),
            post_state_vec: Some(post_state_vec),
        })
    }

    pub fn mock(batch_size: usize) -> Self {
        let width = Self::width(batch_size);
        Self {
            batch_size,
            tau: Some(C::ScalarField::zero()),
            aggregation: Some(vec![C::ScalarField::zero(); width]),
            pre_state_vec: Some(vec![C::ScalarField::zero(); width]),
            post_state_vec: Some(vec![C::ScalarField::zero(); width]),
        }
    }
}

impl<C: CurveGroup> ConstraintSynthesizer<C::ScalarField> for SumPreservingBatchCircuit<C> {
    fn generate_constraints(
        self,
        cs: ConstraintSystemRef<C::ScalarField>,
    ) -> Result<(), SynthesisError> {
        let SumPreservingBatchCircuit {
            batch_size,
            tau,
            aggregation,
            pre_state_vec,
            post_state_vec,
        } = self;

        let tau = FpVar::new_input(cs.clone(), || tau.ok_or(SynthesisError::AssignmentMissing))?;

        let aggregation = Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || {
            aggregation.ok_or(SynthesisError::AssignmentMissing)
        })?;

        let pre_state_vec = Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || {
            pre_state_vec.ok_or(SynthesisError::AssignmentMissing)
        })?;

        let post_state_vec = Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || {
            post_state_vec.ok_or(SynthesisError::AssignmentMissing)
        })?;

        PedersenGadget::<C, FpVar<C::ScalarField>>::enforce_equal(
            aggregation,
            vec![pre_state_vec.clone(), post_state_vec.clone()],
            tau,
            None,
        )?;

        let sum_input = pre_state_vec[..batch_size]
            .iter()
            .fold(FpVar::Constant(C::ScalarField::zero()), |acc, value| {
                acc + value.clone()
            });
        let sum_output = post_state_vec[..batch_size]
            .iter()
            .fold(FpVar::Constant(C::ScalarField::zero()), |acc, value| {
                acc + value.clone()
            });
        sum_input.enforce_equal(&sum_output)?;
        Ok(())
    }
}

pub fn setup_sum_preserving_circuit<E, R>(
    batch_size: usize,
    rng: &mut R,
) -> Result<(ProvingKey<E>, VerifyingKey<E>, CommittingKey<E>), HyperL2Error>
where
    E: Pairing,
    R: RngCore + CryptoRng,
    <E::G1Affine as AffineRepr>::BaseField: PrimeField,
{
    if batch_size == 0 {
        return Err(HyperL2Error::InvalidInput(
            "batch_size must be greater than zero".to_string(),
        ));
    }

    let width = batch_size + 1;
    let num_aggregation_variables = width;
    let num_committed_witness_variables = width * 3;
    let mock = SumPreservingBatchCircuit::<E::G1>::mock(batch_size);
    Ok(CCGroth16::<E>::setup(
        mock,
        num_aggregation_variables,
        num_committed_witness_variables,
        rng,
    )?)
}

pub fn prove_batch<E, R>(
    pk: &ProvingKey<E>,
    ck: &CommittingKey<E>,
    input: StateTransitionBatchInput<E::ScalarField>,
    rng: &mut R,
) -> Result<BatchProofArtifact<E>, HyperL2Error>
where
    E: Pairing,
    R: RngCore + CryptoRng,
    <E::G1Affine as AffineRepr>::BaseField: PrimeField,
{
    input.validate()?;
    let batch_size = input.batch_size()?;
    let pre_state_vec = input.pre_state_commitment_vector();
    let post_state_vec = input.post_state_commitment_vector();
    let width = batch_size + 1;

    if ck.batch_g1.len() != width {
        return Err(HyperL2Error::InvalidInput(format!(
            "commitment key width {} does not match batch width {}",
            ck.batch_g1.len(),
            width
        )));
    }
    if ck.proof_dependent_g1.len() != width * 2 {
        return Err(HyperL2Error::InvalidInput(format!(
            "proof-dependent key width {} does not match expected {}",
            ck.proof_dependent_g1.len(),
            width * 2
        )));
    }

    let committed_witness = [&pre_state_vec[..], &post_state_vec[..]].concat();
    let proof_dependent_commitment = CCGroth16::<E>::commit(ck, &committed_witness, rng)?;

    let prev_state_commitment = Pedersen::<E::G1>::commit(&ck.batch_g1, &pre_state_vec);
    let next_state_commitment = Pedersen::<E::G1>::commit(&ck.batch_g1, &post_state_vec);

    let metadata = input.metadata();
    let tau = compute_batch_challenge::<E>(
        &metadata,
        &prev_state_commitment,
        &next_state_commitment,
        &proof_dependent_commitment.cm,
    )?;

    let slices = [&pre_state_vec[..], &post_state_vec[..]];
    let (aggregation, _) = Pedersen::<E::G1>::scalar_aggregate(&slices, tau, None);
    let circuit = SumPreservingBatchCircuit::<E::G1>::new(
        batch_size,
        tau,
        aggregation,
        pre_state_vec,
        post_state_vec,
    )?;
    let proof = CCGroth16::<E>::prove(pk, circuit, &proof_dependent_commitment, rng)?;

    Ok(BatchProofArtifact {
        lane_id: metadata.lane_id,
        batch_id: metadata.batch_id,
        tx_count: metadata.tx_count,
        prev_state_commitment,
        next_state_commitment,
        proof,
    })
}

pub fn prepare_verifier_proof<E: Pairing>(
    artifact: &BatchProofArtifact<E>,
) -> Result<(E::ScalarField, Proof<E>), HyperL2Error>
where
    <E::G1Affine as AffineRepr>::BaseField: PrimeField,
{
    let tau = compute_batch_challenge::<E>(
        &artifact.metadata(),
        &artifact.prev_state_commitment,
        &artifact.next_state_commitment,
        &artifact.proof.d,
    )?;
    let commitments = [
        artifact.prev_state_commitment.clone(),
        artifact.next_state_commitment.clone(),
    ];
    let (aggregation, _) = Pedersen::<E::G1>::aggregate(&commitments, tau, None);
    let mut verifier_proof = artifact.proof.clone();
    verifier_proof.d = (verifier_proof.d.into_group() + aggregation).into_affine();
    Ok((tau, verifier_proof))
}

pub fn verify_batch<E: Pairing>(
    vk: &VerifyingKey<E>,
    artifact: &BatchProofArtifact<E>,
) -> Result<bool, HyperL2Error>
where
    <E::G1Affine as AffineRepr>::BaseField: PrimeField,
{
    let (tau, proof) = prepare_verifier_proof(artifact)?;
    Ok(CCGroth16::<E>::verify(vk, &[tau], &proof)?)
}

pub fn verify_batch_against_head<E: Pairing>(
    vk: &VerifyingKey<E>,
    expected_prev: &E::G1Affine,
    artifact: &BatchProofArtifact<E>,
) -> Result<bool, HyperL2Error>
where
    <E::G1Affine as AffineRepr>::BaseField: PrimeField,
{
    if &artifact.prev_state_commitment != expected_prev {
        return Ok(false);
    }
    verify_batch(vk, artifact)
}

#[cfg(test)]
mod tests {
    use ark_bn254::{Bn254, Fr};
    use ark_std::rand::{rngs::StdRng, SeedableRng};

    use crate::hyperl2::state::demo_state_transition_batch;

    use super::*;

    #[test]
    fn sum_preserving_batch_round_trip() {
        let mut rng = StdRng::seed_from_u64(7);
        let (pk, vk, ck) = setup_sum_preserving_circuit::<Bn254, _>(8, &mut rng).unwrap();
        let (_, _, input) = demo_state_transition_batch::<Fr>(0, 1, 8).unwrap();
        let artifact = prove_batch::<Bn254, _>(&pk, &ck, input, &mut rng).unwrap();
        assert!(verify_batch(&vk, &artifact).unwrap());
        assert!(verify_batch_against_head(&vk, &artifact.prev_state_commitment, &artifact).unwrap());
    }

    #[test]
    fn tampered_metadata_fails_verification() {
        let mut rng = StdRng::seed_from_u64(13);
        let (pk, vk, ck) = setup_sum_preserving_circuit::<Bn254, _>(8, &mut rng).unwrap();
        let (_, _, input) = demo_state_transition_batch::<Fr>(2, 9, 8).unwrap();
        let artifact = prove_batch::<Bn254, _>(&pk, &ck, input, &mut rng).unwrap();
        let mut tampered = artifact.clone();
        tampered.batch_id += 1;
        assert!(!verify_batch(&vk, &tampered).unwrap());
    }
}
