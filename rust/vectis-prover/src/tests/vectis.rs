use std::time::Instant;

use ark_ec::{pairing::Pairing, AffineRepr, CurveGroup};
use ark_ff::PrimeField;
use ark_r1cs_std::{alloc::AllocVar, boolean::Boolean, fields::fp::FpVar, ToBitsGadget};
use ark_relations::r1cs::{ConstraintSynthesizer, ConstraintSystemRef, SynthesisError};
use ark_std::{
    rand::{CryptoRng, RngCore},
    vec::Vec,
    Zero,
};

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use super::utils::Average;

use crate::{
    crypto::{
        commitment::{
            pedersen::{Pedersen, PedersenGadget},
            BatchCommitmentGadget, BatchCommitmentScheme,
        },
        protocol::{
            sigma::SigmaProtocol,
            transcript::{sha3::SHA3Base, TranscriptProtocol},
        },
    },
    gro::CCGroth16,
    linker::am_eq::{
        AmEq, CommittingKey as LinkerCommittingKey, Instance, PublicParameters, Witness,
    },
    snark::{CircuitSpecificSetupCCSNARK, CCSNARK},
    solidity::Solidity,
};

#[derive(Clone)]
struct VectisAgeCircuit<C: CurveGroup> {
    // public input
    pub tau: Option<C::ScalarField>,

    // committed witness
    pub aggregation: Option<Vec<C::ScalarField>>,
    pub commitments: Option<Vec<Vec<C::ScalarField>>>,
}

impl<C: CurveGroup> VectisAgeCircuit<C> {
    pub fn new(commitments: Vec<Vec<C::ScalarField>>, tau: C::ScalarField) -> Self {
        let slices: Vec<&[C::ScalarField]> = commitments.iter().map(|cm| &cm[..]).collect();
        let (aggregation, _) = Pedersen::<C>::scalar_aggregate(&slices[..], tau, None);

        Self {
            tau: Some(tau),
            aggregation: Some(aggregation),
            commitments: Some(commitments),
        }
    }

    pub fn mock(batch_size: usize) -> Self {
        Self {
            tau: Some(C::ScalarField::zero()),
            aggregation: Some(vec![C::ScalarField::zero(); 2]),
            commitments: Some(vec![vec![C::ScalarField::zero(); 2]; batch_size]),
        }
    }
}

impl<C: CurveGroup> ConstraintSynthesizer<C::ScalarField> for VectisAgeCircuit<C> {
    fn generate_constraints(
        self,
        cs: ConstraintSystemRef<C::ScalarField>,
    ) -> ark_relations::r1cs::Result<()> {
        let tau = FpVar::new_input(cs.clone(), || {
            self.tau.ok_or_else(|| SynthesisError::AssignmentMissing)
        })?;

        let aggregation = Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || {
            self.aggregation
                .ok_or_else(|| SynthesisError::AssignmentMissing)
        })?;

        let commitments = self
            .commitments
            .ok_or_else(|| SynthesisError::AssignmentMissing)?
            .into_iter()
            .map(|cm| Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || Ok(cm)))
            .collect::<Result<Vec<_>, SynthesisError>>()?;

        PedersenGadget::<C, FpVar<C::ScalarField>>::enforce_equal(
            aggregation,
            commitments.clone(),
            tau,
            None,
        )?;

        // Age Check
        let age_limit_bytes: [u8; 8] = (std::u64::MAX - 1).to_le_bytes();
        let age_limit = C::ScalarField::from_le_bytes_mod_order(&age_limit_bytes);
        let start = commitments.len() >> 9;
        commitments[start..].iter().for_each(|cm| {
            let age = cm[0].to_non_unique_bits_le().unwrap();
            Boolean::enforce_smaller_or_equal_than_le(&age, age_limit.into_bigint()).unwrap();
        });
        Ok(())
    }
}

fn test_commitments<F: PrimeField>(num_commitments: usize, length: usize) -> Vec<Vec<F>> {
    let mut commitments = vec![];
    for i in 0..num_commitments {
        // let value = ((i & 1) + 1) as u64;
        let value = ((i + 1) * (i + 1)) as u64;
        commitments.push(vec![F::from(value); length]);
    }
    commitments
}

// Calculate the time taken to generator, prover and verifier
fn cp_vectis<E: Pairing, R: RngCore + CryptoRng>(batch_size: usize, rng: &mut R) -> (u128, u128)
where
    E::G1Affine: Solidity,
    E::G2Affine: Solidity,
    E::ScalarField: Solidity,
{
    let repeat = 1;
    let mut prover = vec![];
    let mut verifier = vec![];
    for _ in 0..repeat {
        let num_aggregation_variables = 2;
        let num_committed_witness_variables =
            num_aggregation_variables + batch_size * num_aggregation_variables;

        let mock = VectisAgeCircuit::<E::G1>::mock(batch_size);

        let (pk, vk, ck) = CCGroth16::<E>::setup(
            mock,
            num_aggregation_variables,
            num_committed_witness_variables,
            rng,
        )
        .unwrap();

        let g = vec![ck.batch_g1[0].clone()];
        let h = vec![ck.batch_g1[1].clone()];
        let lck = LinkerCommittingKey { g, h };
        let pp = PublicParameters {
            poly_ck: lck.clone(),
            coeff_ck: lck,
        };

        // make random cm (prev, curr)
        let commitments = test_commitments::<E::ScalarField>(batch_size, 2);

        // Generage Proof Dependent Commitment
        let committed_witness = cfg_iter!(commitments)
            .flat_map(|cm| cfg_iter!(cm).cloned())
            .collect::<Vec<_>>();
        let proof_dependent_commitment =
            CCGroth16::<E>::commit(&ck, &committed_witness[..], rng).unwrap();

        // Batch Commitment Module
        let slices = cfg_iter!(commitments).map(|cm| &cm[..]).collect::<Vec<_>>();
        let commitments_g1 = Pedersen::<E::G1>::batch_commit(&pk.vk.ck.batch_g1, &slices[..]);
        let tau =
            Pedersen::<E::G1>::challenge(&[], &commitments_g1, &proof_dependent_commitment.cm);

        // Make circuit
        let circuit = VectisAgeCircuit::<E::G1>::new(commitments, tau);

        // Linker assignments
        let aggregated = circuit.aggregation.clone().unwrap();
        let witness = Witness {
            w: aggregated[..1].to_vec(),
            alpha: aggregated[1..].to_vec(),
        };
        let instance = Instance {
            c_hat: commitments_g1.clone(),
            tau,
        };

        let prv_instant = Instant::now();
        let mut transcript = SHA3Base::new(false);
        let mut dlc_proof =
            CCGroth16::<E>::prove(&pk, circuit.clone(), &proof_dependent_commitment, rng).unwrap();

        let eq_proof = AmEq::<E::G1>::prove(&pp, &instance, &witness, &mut transcript, rng)
            .expect("proof failed");
        prover.push(prv_instant.elapsed().as_micros());
        drop(witness);

        let public_inputs = [tau];

        let vry_instant = Instant::now();
        let mut transcript = SHA3Base::new(false);

        // Aggregate commitments
        let (aggregation_g1, _) = Pedersen::<E::G1>::aggregate(&commitments_g1, tau, None);
        // Update proof dependent commitment
        dlc_proof.d = (dlc_proof.d.into_group() + aggregation_g1).into_affine();

        // In Batch Commitment Circuit, there is no different public inputs
        assert!(
            CCGroth16::<E>::verify(&vk, public_inputs.as_slice(), &dlc_proof).unwrap(),
            "Invalid Proof"
        );

        assert!(
            AmEq::<E::G1>::verify(&pp, &instance, &eq_proof, &mut transcript).unwrap(),
            "eclipse proof failed"
        );

        verifier.push(vry_instant.elapsed().as_micros());
    }

    (prover.average(), verifier.average())
}

pub mod bn254 {
    use crate::tests::{utils::format_time, LOG_MIN};

    use super::*;
    use ark_std::{
        rand::{rngs::StdRng, SeedableRng},
        test_rng,
    };

    type E = ark_bn254::Bn254;
    type R = StdRng;

    #[test]
    fn age_check_commit_and_prove_without_key() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        let batch_size = 1 << *LOG_MIN;

        let (prv, vrf) = cp_vectis::<E, R>(batch_size, &mut rng);
        println!("Prover\t\t: {}", format_time(prv));
        println!("Verifier\t: {}", format_time(vrf));
    }
}
