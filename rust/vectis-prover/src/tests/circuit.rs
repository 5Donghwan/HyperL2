use std::time::Instant;

use ark_ec::{pairing::Pairing, AffineRepr, CurveGroup};
use ark_ff::PrimeField;
use ark_r1cs_std::{
    alloc::AllocVar, boolean::Boolean, eq::EqGadget, fields::fp::FpVar, ToBitsGadget,
};
use ark_relations::r1cs::{ConstraintSynthesizer, ConstraintSystemRef, SynthesisError};
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};
use ark_std::{
    rand::{CryptoRng, RngCore},
    vec::Vec,
    One, Zero,
};

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use super::utils::{Average, Transpose};

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
    gro::{CCGroth16, Commitment, CommittingKey, Proof, ProvingKey, VerifyingKey},
    linker::am_eq::{
        AmEq, CommittingKey as LinkerCommittingKey, Instance, PublicParameters, Witness,
    },
    snark::{CircuitSpecificSetupCCSNARK, CCSNARK},
    solidity::Solidity,
};

#[derive(Clone)]
struct BatchCommitmentCircuit<C: CurveGroup> {
    // public input
    pub tau: Option<C::ScalarField>,

    // committed witness
    pub aggregation: Option<Vec<C::ScalarField>>,
    pub commitments: Option<Vec<Vec<C::ScalarField>>>,
}

impl<C: CurveGroup> BatchCommitmentCircuit<C> {
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

impl<C: CurveGroup> ConstraintSynthesizer<C::ScalarField> for BatchCommitmentCircuit<C> {
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

#[derive(Clone)]
struct ZKSTCircuit<C: CurveGroup> {
    // public input
    pub tau: Option<C::ScalarField>,

    // committed witness
    pub aggregation: Option<Vec<C::ScalarField>>,
    pub flag: Option<bool>,
    pub permuted: Option<Vec<C::ScalarField>>,
    pub current_commitments: Option<Vec<Vec<C::ScalarField>>>,
    pub delta_commitments: Option<Vec<Vec<C::ScalarField>>>,
}

impl<C: CurveGroup> ZKSTCircuit<C> {
    pub fn new(
        tau: C::ScalarField,
        flag: bool,
        permuted: Vec<C::ScalarField>,
        current_commitments: Vec<Vec<C::ScalarField>>,
        delta_commitments: Vec<Vec<C::ScalarField>>,
    ) -> Self {
        let public_inputs = vec![vec![C::ScalarField::from(flag)], permuted.clone()]
            .concat()
            .transpose();
        let slices: Vec<&[C::ScalarField]> = public_inputs.iter().map(|x| &x[..]).collect();
        let (x_aggregation, initial) = Pedersen::<C>::scalar_aggregate(&slices, tau, None);
        drop(public_inputs);

        let commitments = [&current_commitments[..], &delta_commitments[..]].concat();
        let slices: Vec<&[C::ScalarField]> = commitments.iter().map(|cm| &cm[..]).collect();
        let (mut aggregation, _) = Pedersen::<C>::scalar_aggregate(&slices, tau, Some(initial));
        aggregation[0] += x_aggregation[0];

        Self {
            tau: Some(tau),
            aggregation: Some(aggregation),
            flag: Some(flag),
            permuted: Some(permuted),
            current_commitments: Some(current_commitments),
            delta_commitments: Some(delta_commitments),
        }
    }

    pub fn mock(batch_size: usize) -> Self {
        Self {
            tau: Some(C::ScalarField::zero()),
            aggregation: Some(vec![C::ScalarField::zero(); 2]),
            flag: Some(false),
            permuted: Some(vec![C::ScalarField::zero(); batch_size]),
            current_commitments: Some(vec![vec![C::ScalarField::zero(); 2]; batch_size]),
            delta_commitments: Some(vec![vec![C::ScalarField::zero(); 2]; batch_size]),
        }
    }
}

impl<C: CurveGroup> ConstraintSynthesizer<C::ScalarField> for ZKSTCircuit<C> {
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

        let flag = Boolean::new_witness(cs.clone(), || {
            self.flag.ok_or_else(|| SynthesisError::AssignmentMissing)
        })?;

        let permuted = Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || {
            self.permuted
                .ok_or_else(|| SynthesisError::AssignmentMissing)
        })?;

        let current_commitments = self
            .current_commitments
            .ok_or_else(|| SynthesisError::AssignmentMissing)?
            .into_iter()
            .map(|cm| Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || Ok(cm)))
            .collect::<Result<Vec<_>, SynthesisError>>()?;

        let delta_commitments = self
            .delta_commitments
            .ok_or_else(|| SynthesisError::AssignmentMissing)?
            .into_iter()
            .map(|cm| Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || Ok(cm)))
            .collect::<Result<Vec<_>, SynthesisError>>()?;

        // permtation check
        let permutation = permuted
            .iter()
            .fold(FpVar::Constant(C::ScalarField::zero()), |acc, msg| {
                acc * (&tau + msg)
            });

        let _permutation = delta_commitments
            .iter()
            .fold(FpVar::Constant(C::ScalarField::zero()), |acc, msg| {
                acc * (&tau - &msg[0])
            });

        _permutation
            .conditional_enforce_equal(&permutation, &flag)
            .expect("Permutation Check");

        // aggregation check
        let flag = flag
            .select(
                &FpVar::Constant(C::ScalarField::one()),
                &FpVar::Constant(C::ScalarField::zero()),
            )
            .unwrap();

        let public_inputs = vec![vec![flag], permuted]
            .concat()
            .transpose()
            .iter()
            .map(|x| vec![x[0].clone(), FpVar::Constant(C::ScalarField::zero())])
            .collect();
        let commitments = [public_inputs, current_commitments, delta_commitments]
            .concat::<Vec<FpVar<C::ScalarField>>>();
        PedersenGadget::<C, FpVar<C::ScalarField>>::enforce_equal(
            aggregation,
            commitments,
            tau,
            None,
        )
        .expect("Aggregation Check");

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

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
struct VkWithoutCk<E: Pairing> {
    /// The `alpha * G`, where `G` is the generator of `E::G1`.
    pub alpha_g1: E::G1Affine,
    /// The `alpha * H`, where `H` is the generator of `E::G2`.
    pub beta_g2: E::G2Affine,
    /// The `gamma * H`, where `H` is the generator of `E::G2`.
    pub gamma_g2: E::G2Affine,
    /// The `delta * H`, where `H` is the generator of `E::G2`.
    pub delta_g2: E::G2Affine,

    /// The `gamma^{-1} * (beta * a_i + alpha * b_i + c_i) * H`, where `H` is
    /// the generator of `E::G1`
    pub gamma_abc_g1: Vec<E::G1Affine>,
}

impl<E: Pairing> From<VerifyingKey<E>> for VkWithoutCk<E> {
    fn from(vk: VerifyingKey<E>) -> Self {
        VkWithoutCk {
            alpha_g1: vk.alpha_g1,
            beta_g2: vk.beta_g2,
            gamma_g2: vk.gamma_g2,
            delta_g2: vk.delta_g2,
            gamma_abc_g1: vk.gamma_abc_g1,
        }
    }
}

// Calculate the time taken to generator, prover and verifier
fn process_batch_commitment_circuit<E: Pairing, R: RngCore + CryptoRng>(
    repeat: usize,
    batch_size: usize,
    rng: &mut R,
) -> (u128, u128, u128, u128)
where
    E::G1Affine: Solidity,
    E::G2Affine: Solidity,
{
    let mut generator = vec![];
    let mut prover = vec![];
    let mut aggregation = vec![];
    let mut verifier = vec![];
    for _ in 0..repeat {
        let num_aggregation_variables = 2;
        let num_committed_witness_variables =
            num_aggregation_variables + batch_size * num_aggregation_variables;

        let mock = BatchCommitmentCircuit::<E::G1>::mock(batch_size);

        let gen_instant = Instant::now();
        let (pk, vk, ck) = CCGroth16::<E>::setup(
            mock,
            num_aggregation_variables,
            num_committed_witness_variables,
            rng,
        )
        .unwrap();
        generator.push(gen_instant.elapsed().as_micros());

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
        let circuit = BatchCommitmentCircuit::<E::G1>::new(commitments, tau);

        let prv_instant = Instant::now();
        let mut proof =
            CCGroth16::<E>::prove(&pk, circuit.clone(), &proof_dependent_commitment, rng).unwrap();
        prover.push(prv_instant.elapsed().as_micros());

        if repeat == 1 {
            println!("const cm = {:?}", commitments_g1.to_solidity());
            println!("const proof = {:?}", proof.to_solidity());
            println!("const vk = {:?}", vk.to_solidity());
            println!("\nconst batch{} = {{ cm, proof, vk }}", batch_size);
            println!("\nexport default batch{}", batch_size);
        }

        let public_inputs = [tau];

        let vry_instant = Instant::now();
        let agg_instant = Instant::now();
        // Aggregate commitments
        let (aggregation_g1, _) = Pedersen::<E::G1>::aggregate(&commitments_g1, tau, None);
        // Update proof dependent commitment
        proof.d = (proof.d.into_group() + aggregation_g1).into_affine();
        aggregation.push(agg_instant.elapsed().as_micros());

        // In Batch Commitment Circuit, there is no different public inputs
        assert!(
            CCGroth16::<E>::verify(&vk, public_inputs.as_slice(), &proof).unwrap(),
            "Invalid Proof"
        );

        verifier.push(vry_instant.elapsed().as_micros());
    }

    (
        generator.average(),
        prover.average(),
        aggregation.average(),
        verifier.average(),
    )
}

// Calculate the time taken to generator, prover and verifier
fn process_batch_commitment_circuit_link<E: Pairing, R: RngCore + CryptoRng>(
    repeat: usize,
    batch_size: usize,
    rng: &mut R,
) -> (u128, u128, u128, u128)
where
    E::G1Affine: Solidity,
    E::G2Affine: Solidity,
    E::ScalarField: Solidity,
{
    let mut generator = vec![];
    let mut prover = vec![];
    let mut aggregation = vec![];
    let mut verifier = vec![];
    for _ in 0..repeat {
        let num_aggregation_variables = 2;
        let num_committed_witness_variables =
            num_aggregation_variables + batch_size * num_aggregation_variables;

        let mock = BatchCommitmentCircuit::<E::G1>::mock(batch_size);

        let gen_instant = Instant::now();
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
        generator.push(gen_instant.elapsed().as_micros());

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
        let circuit = BatchCommitmentCircuit::<E::G1>::new(commitments, tau);

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

        if repeat == 1 {
            println!("const cm = {:?}", commitments_g1.to_solidity());
            println!("const dlc_proof = {:?}", dlc_proof.to_solidity());
            println!("const eq_proof = {:?}", eq_proof.to_solidity());
            println!("const vk = {:?}", vk.to_solidity());
            println!("const pp = {:?}", pp.to_solidity());
            println!(
                "\nconst batch{} = {{ cm, dlc_proof, eq_proof, vk, pp }}",
                batch_size
            );
            println!("\nexport default batch{}", batch_size);
        }

        let public_inputs = [tau];

        let vry_instant = Instant::now();
        let mut transcript = SHA3Base::new(false);

        let agg_instant = Instant::now();
        // Aggregate commitments
        let (aggregation_g1, _) = Pedersen::<E::G1>::aggregate(&commitments_g1, tau, None);
        // Update proof dependent commitment
        dlc_proof.d = (dlc_proof.d.into_group() + aggregation_g1).into_affine();
        aggregation.push(agg_instant.elapsed().as_micros());

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

    (
        generator.average(),
        prover.average(),
        aggregation.average(),
        verifier.average(),
    )
}

fn zkst_circuit_setup<E: Pairing, R: RngCore + CryptoRng>(
    batch_size: usize,
    rng: &mut R,
) -> (ProvingKey<E>, VerifyingKey<E>, CommittingKey<E>) {
    let num_aggregation_variables = 2;
    let num_committed_witness_variables = num_aggregation_variables + batch_size * 3 + 1;

    let mock = ZKSTCircuit::<E::G1>::mock(batch_size);

    CCGroth16::<E>::setup(
        mock,
        num_aggregation_variables,
        num_committed_witness_variables,
        rng,
    )
    .unwrap()
}

fn zkst_circuit_commit<E: Pairing, R: RngCore + CryptoRng>(
    ck: &CommittingKey<E>,
    public_inputs: &Vec<E::ScalarField>,
    commitments: &Vec<Vec<E::ScalarField>>,
    rng: &mut R,
) -> (Vec<E::G1Affine>, Commitment<E>, E::ScalarField) {
    let batch_size = commitments.len() >> 1;

    // Generage Proof Dependent Commitment
    let committed_witness = cfg_iter!(commitments)
        .flat_map(|cm| cfg_iter!(cm).cloned())
        .collect::<Vec<_>>();

    let committed = [&public_inputs[..], &committed_witness].concat();
    let proof_dependent_commitment = CCGroth16::<E>::commit(&ck, &committed[..], rng).unwrap();

    // Batch Commitment Module
    let slices = cfg_iter!(commitments).map(|cm| &cm[..]).collect::<Vec<_>>();
    let commitments_g1 = Pedersen::<E::G1>::batch_commit(&ck.batch_g1, &slices);
    let tau = Pedersen::<E::G1>::challenge(
        public_inputs,
        &commitments_g1[batch_size..],
        &proof_dependent_commitment.cm,
    );
    (commitments_g1, proof_dependent_commitment, tau)
}

fn zkst_circuit_prove_and_verify<E: Pairing, R: RngCore + CryptoRng>(
    pk: &ProvingKey<E>,
    vk: &VerifyingKey<E>,
    circuit: ZKSTCircuit<E::G1>,
    commitments: &Vec<E::G1Affine>,
    proof_dependent_commitment: &Commitment<E>,
    rng: &mut R,
) -> Proof<E> {
    let proof =
        CCGroth16::<E>::prove(&pk, circuit.clone(), &proof_dependent_commitment, rng).unwrap();

    let tau = circuit.tau.unwrap();
    let flag = E::ScalarField::from(circuit.flag.unwrap());
    let public_inputs = [vec![tau, flag], circuit.permuted.unwrap().clone()].concat();
    let (public_inputs, committed) = public_inputs.split_at(1);

    // Aggregate commitments
    let committed = committed.to_vec().transpose();
    let slices = cfg_iter!(committed).map(|x| &x[..]).collect::<Vec<_>>();
    let (aggregation_fr, initial) = Pedersen::<E::G1>::scalar_aggregate(&slices, tau, None);
    let (aggregation_g1, _) = Pedersen::<E::G1>::aggregate(commitments, tau, Some(initial));
    // Update proof dependent commitment
    let mut verify = proof.clone();
    let aggregation = aggregation_g1 + vk.ck.batch_g1[0].into_group() * aggregation_fr[0];
    let _aggregation = circuit.aggregation.unwrap();
    let _aggregation = vk.ck.batch_g1[0].into_group() * _aggregation[0]
        + vk.ck.batch_g1[1].into_group() * _aggregation[1];
    assert_eq!(
        aggregation.into_affine(),
        _aggregation.into_affine(),
        "Invalid Aggregation"
    );
    verify.d = (aggregation + verify.d.into_group()).into_affine();

    assert!(
        CCGroth16::<E>::verify(&vk, &public_inputs, &verify).unwrap(),
        "Invalid Proof"
    );
    proof
}

fn zkst_circuit_solidity<E: Pairing>(
    batch_size: usize,
    permuted: &Vec<E::ScalarField>,
    cm_update: &Vec<E::G1Affine>,
    proof_update: &Proof<E>,
    cm_exchange: &Vec<E::G1Affine>,
    proof_exchange: &Proof<E>,
    vk: &VerifyingKey<E>,
) where
    E::G1Affine: Solidity,
    E::G2Affine: Solidity,
    E::ScalarField: Solidity,
{
    println!("const vk = {:?}", vk.to_solidity());
    println!("const ck = {:?}", vk.ck.batch_g1.to_solidity());
    println!(
        "const amount = {:?}",
        vec![permuted[0], permuted[permuted.len() >> 1]].to_solidity()
    );
    println!(
        "const updateCm = {:?}",
        vec![cm_update[batch_size], cm_update[batch_size + 1]].to_solidity()
    );
    println!("const updateProof = {:?}", proof_update.to_solidity());
    println!(
        "const exchangeCm = {:?}",
        vec![cm_exchange[batch_size], cm_exchange[batch_size + 1]].to_solidity()
    );
    println!("const exchangeProof = {:?}", proof_exchange.to_solidity());
    println!("const update = {{ cm: updateCm, proof: updateProof }}");
    println!("const exchange = {{ amount, cm: exchangeCm, proof: exchangeProof }}");
    println!(
        "\nconst batch{} = {{ vk, ck, update, exchange }}",
        batch_size
    );
    println!("\nexport default batch{}", batch_size);
}

pub mod bn254 {
    use crate::tests::{
        utils::{compressed_key_size, format_time},
        LOG_MAX, LOG_MIN, NUM_REPEAT,
    };

    use super::*;
    use ark_relations::r1cs::{ConstraintSystem, OptimizationGoal, SynthesisMode};
    use ark_std::{
        rand::{rngs::StdRng, SeedableRng},
        test_rng,
    };

    type C = ark_bn254::G1Projective;
    type E = ark_bn254::Bn254;
    type F = ark_bn254::Fr;
    type R = StdRng;

    #[test]
    fn batch_commitment_circuit_num_constraints() {
        let mut result: Vec<usize> = vec![];
        for n in *LOG_MIN..=*LOG_MAX {
            let batch_size = 1 << n;

            let cs_timer = start_timer!(|| format!("Batch Size: 2^{}", n));
            let mock = BatchCommitmentCircuit::<C>::mock(batch_size);

            let cs = ConstraintSystem::new_ref();
            cs.set_optimization_goal(OptimizationGoal::Weight);
            cs.set_mode(SynthesisMode::Setup);

            let _ = mock.generate_constraints(cs.clone());

            cs.finalize();
            result.push(cs.num_constraints());
            end_timer!(cs_timer);
        }

        println!("{:?}", result);
    }

    #[test]
    fn batch_commitment_circuit_key_size() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        println!("| log batch | pk | vk | vk (with ck) | ck |");
        println!("| --- | --- | --- | --- | --- |");
        for n in *LOG_MIN..=*LOG_MAX {
            let batch_size = 1 << n;
            let num_aggregation_variables = 2;
            let num_committed_witness_variables =
                num_aggregation_variables + batch_size * num_aggregation_variables;
            let mock = BatchCommitmentCircuit::<C>::mock(batch_size);

            let (pk, vk, _) = CCGroth16::<E>::setup(
                mock,
                num_aggregation_variables,
                num_committed_witness_variables,
                &mut rng,
            )
            .unwrap();

            // key size
            let vk_without_ck = VkWithoutCk::<E>::from(vk.clone());

            println!(
                "| {} | {} | {} | {} | {} |",
                n,
                compressed_key_size(&pk),
                compressed_key_size(&vk),
                compressed_key_size(&vk_without_ck),
                compressed_key_size(&vk.ck)
            );
        }
    }

    #[test]
    fn batch_commitment_circuit_without_key() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        for n in *LOG_MIN..=*LOG_MAX {
            let batch_size = 1 << n;

            let (gen, prv, agg, vrf) =
                process_batch_commitment_circuit::<E, R>(*NUM_REPEAT, batch_size, &mut rng);
            println!(
                "Batch Size: 2^{} Generator: {} Prover: {} Aggregation: {} Verifier: {}",
                n,
                format_time(gen),
                format_time(prv),
                format_time(agg),
                format_time(vrf)
            );
        }
    }

    #[test]
    fn batch_commitment_circuit_commit_and_prove_key_size() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        println!("| log batch | pk | vk | vk (with ck) | ck |");
        println!("| --- | --- | --- | --- | --- |");
        for n in *LOG_MIN..=*LOG_MAX {
            let batch_size = 1 << n;
            let num_aggregation_variables = 2;
            let num_committed_witness_variables =
                num_aggregation_variables + batch_size * num_aggregation_variables;
            let mock = BatchCommitmentCircuit::<C>::mock(batch_size);

            let (mut pk, mut vk, _) = CCGroth16::<E>::setup(
                mock,
                num_aggregation_variables,
                num_committed_witness_variables,
                &mut rng,
            )
            .unwrap();

            let batch_key = pk.vk.ck.batch_g1[..2].to_vec();
            pk.vk.gamma_abc_g1.extend(&batch_key);
            vk.gamma_abc_g1.extend(&batch_key);

            // key size
            let vk_without_ck = VkWithoutCk::<E>::from(vk.clone());

            println!(
                "| {} | {} | {} | {} | {} |",
                n,
                compressed_key_size(&pk),
                compressed_key_size(&vk),
                compressed_key_size(&vk_without_ck),
                compressed_key_size(&vk.ck)
            );
        }
    }

    #[test]
    fn batch_commitment_circuit_commit_and_prove_without_key() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        for n in *LOG_MIN..=*LOG_MAX {
            let batch_size = 1 << n;

            let (gen, prv, agg, vrf) =
                process_batch_commitment_circuit_link::<E, R>(*NUM_REPEAT, batch_size, &mut rng);
            println!(
                "Batch Size: 2^{} Generator: {} Prover: {} Aggregation: {} Verifier: {}",
                n,
                format_time(gen),
                format_time(prv),
                format_time(agg),
                format_time(vrf)
            );
        }
    }

    #[test]
    fn zkst_circuit_scenario() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        for n in *LOG_MIN..=*LOG_MAX {
            let batch_size = 1 << n;

            let (pk, vk, ck) = zkst_circuit_setup::<E, R>(batch_size, &mut rng);

            // update scenario
            let cm_update_curr = test_commitments::<F>(batch_size, 2);
            let cm_update_delta = cm_update_curr.clone();
            let cm_update = [&cm_update_curr[..], &cm_update_delta[..]].concat();

            // commit
            let permuted_update = vec![F::zero(); batch_size];
            let public_inputs_update = vec![F::zero(); batch_size + 1];
            let (cm_update_g1, d_update, tau_update) =
                zkst_circuit_commit(&ck, &public_inputs_update, &cm_update, &mut rng);
            drop(public_inputs_update);

            // prove and verify
            let circuit_update = ZKSTCircuit::new(
                tau_update,
                false,
                permuted_update.clone(),
                cm_update_curr,
                cm_update_delta,
            );
            let proof_update = zkst_circuit_prove_and_verify(
                &pk,
                &vk,
                circuit_update.clone(),
                &cm_update_g1,
                &d_update,
                &mut rng,
            );

            // exchange scenario
            let cm_exchange_curr = vec![vec![F::zero(); 2]; batch_size];
            let cm_exchange_delta = test_commitments::<F>(batch_size, 2);
            let mut permuted_exchange = cfg_iter!(cm_exchange_delta)
                .map(|cm| cm[0])
                .collect::<Vec<F>>();
            permuted_exchange.sort();
            let cm_exchange_delta = cfg_iter!(cm_exchange_delta)
                .map(|cm| vec![-cm[0], -cm[1]])
                .collect::<Vec<Vec<F>>>();
            let cm_exchange = [&cm_exchange_curr[..], &cm_exchange_delta[..]].concat();

            // commit
            let public_inputs_exchange = vec![vec![F::one()], permuted_exchange.clone()].concat();
            let (cm_exchange_g1, d_exchange, tau_exchange) =
                zkst_circuit_commit(&ck, &public_inputs_exchange, &cm_exchange, &mut rng);
            drop(public_inputs_exchange);

            // prove and verify
            let circuit_exchange = ZKSTCircuit::new(
                tau_exchange,
                true,
                permuted_exchange.clone(),
                cm_exchange_curr,
                cm_exchange_delta,
            );
            let proof_exchange = zkst_circuit_prove_and_verify(
                &pk,
                &vk,
                circuit_exchange.clone(),
                &cm_exchange_g1,
                &d_exchange,
                &mut rng,
            );

            println!("update: {}, exchange: {}", tau_update, tau_exchange);

            // print solidity form
            zkst_circuit_solidity(
                batch_size,
                &permuted_exchange,
                &cm_update_g1,
                &proof_update,
                &cm_exchange_g1,
                &proof_exchange,
                &vk,
            );
        }
    }
}
