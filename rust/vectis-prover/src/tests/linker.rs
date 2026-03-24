use std::time::Instant;

use ark_ec::{pairing::Pairing, AffineRepr, CurveGroup};
use ark_ff::PrimeField;
use ark_r1cs_std::{
    alloc::AllocVar,
    boolean::Boolean,
    eq::EqGadget,
    fields::{fp::FpVar, FieldVar},
    ToBitsGadget,
};
use ark_relations::r1cs::{ConstraintSynthesizer, ConstraintSystemRef, SynthesisError};
use ark_std::{
    rand::{CryptoRng, RngCore},
    vec::Vec,
    One, UniformRand,
};

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use crate::{
    crypto::{
        commitment::{pedersen::Pedersen, CommitmentScheme},
        protocol::{
            sigma::SigmaProtocol,
            transcript::{sha3::SHA3Base, TranscriptProtocol},
        },
    },
    gro::{CCGroth16, Commitment, ProvingKey},
    linker::{
        am_com_eq::{
            data_structure::{CommittingKey, Instance, PublicParameters, Witness},
            AmComEq,
        },
        comp_am_com_eq::CompAmComEq,
    },
    snark::{CircuitSpecificSetupCCSNARK, CCSNARK},
    solidity::Solidity,
};

use super::utils::Average;

#[derive(Clone)]
struct LinkerCircuit<C: CurveGroup> {
    // committed witness
    msg: Option<Vec<C::ScalarField>>,
}

impl<C: CurveGroup> LinkerCircuit<C> {
    pub fn new(msg: Vec<Vec<C::ScalarField>>) -> Self {
        let msg = cfg_iter!(msg).map(|m| m[0].clone()).collect::<Vec<_>>();
        Self { msg: Some(msg) }
    }

    pub fn mock(batch_size: usize) -> Self {
        Self {
            msg: Some(vec![C::ScalarField::one(); batch_size]),
        }
    }
}

impl<C: CurveGroup> ConstraintSynthesizer<C::ScalarField> for LinkerCircuit<C> {
    fn generate_constraints(
        self,
        cs: ConstraintSystemRef<C::ScalarField>,
    ) -> ark_relations::r1cs::Result<()> {
        let msg = Vec::<FpVar<C::ScalarField>>::new_witness(cs.clone(), || {
            self.msg.ok_or_else(|| SynthesisError::AssignmentMissing)
        })?;

        // let e_sum = msg.iter().step_by(2).sum::<FpVar<C::ScalarField>>();
        // let o_sum = msg.iter().skip(1).step_by(2).sum::<FpVar<C::ScalarField>>();
        // let _ = e_sum.clone() * o_sum.clone();
        // let _ = o_sum.clone() * e_sum.clone();

        // Age Check
        let age_limit_bytes: [u8; 8] = (std::u64::MAX - 1).to_le_bytes();
        let age_limit = C::ScalarField::from_le_bytes_mod_order(&age_limit_bytes);
        let start = msg.len() >> 9;
        msg[start..].iter().for_each(|age| {
            let age_bits = age.to_non_unique_bits_le().unwrap();
            Boolean::enforce_smaller_or_equal_than_le(&age_bits, age_limit.into_bigint()).unwrap();
        });

        Ok(())
    }
}

fn linker_setup<C: CurveGroup, R: RngCore + CryptoRng>(
    l: usize,
    d0: usize,
    d1: usize,
    d2: usize,
    rng: &mut R,
) -> (PublicParameters<C>, Instance<C>, Witness<C>) {
    let ld = l * d0;

    let g = vec![C::Affine::rand(rng); ld];
    let h = vec![C::Affine::rand(rng); d1];

    let g_hat = vec![C::Affine::rand(rng); d0];
    let h_hat = vec![C::Affine::rand(rng); d2];

    let w = vec![vec![C::ScalarField::rand(rng); d0]; l];
    let alpha = vec![C::ScalarField::rand(rng); d1];
    let beta = vec![vec![C::ScalarField::rand(rng); d2]; l];

    let w_flat = cfg_iter!(w).flat_map(|w_i| w_i.clone()).collect::<Vec<_>>();
    let c = Pedersen::<C>::commit(&g, &w_flat) + Pedersen::<C>::commit(&h, &alpha);
    let c_hat = cfg_iter!(w)
        .zip(&beta)
        .map(|(w_i, beta_i)| {
            Pedersen::<C>::commit(&g_hat, w_i) + Pedersen::<C>::commit(&h_hat, beta_i)
        })
        .collect::<Vec<_>>();

    (
        PublicParameters {
            poly_ck: CommittingKey { g, h },
            coeff_ck: CommittingKey { g: g_hat, h: h_hat },
        },
        Instance {
            c: c.into_affine(),
            c_hat: C::normalize_batch(&c_hat),
        },
        Witness { w, alpha, beta },
    )
}

fn process_linker<C: CurveGroup, R: RngCore + CryptoRng>(
    repeat: usize,
    l: usize,
    d0: usize,
    d1: usize,
    d2: usize,
    rng: &mut R,
) -> (u128, u128) {
    let mut prover = vec![];
    let mut verifier = vec![];
    for _ in 0..repeat {
        let (pp, instance, witness) = linker_setup::<C, _>(l, d0, d1, d2, rng);
        let mut transcript = SHA3Base::new(true);

        // prove
        let prv_instant = Instant::now();
        let proof = CompAmComEq::<C>::prove(&pp, &instance, &witness, &mut transcript, rng)
            .expect("proof failed");
        prover.push(prv_instant.elapsed().as_micros());
        drop(witness);

        // verify
        let vry_instant = Instant::now();
        let mut transcript = SHA3Base::new(true);
        assert!(CompAmComEq::<C>::verify(&pp, &instance, &proof, &mut transcript).unwrap());
        verifier.push(vry_instant.elapsed().as_micros());
    }
    (prover.average(), verifier.average())
}

fn cp_link_setup<E: Pairing, R: RngCore + CryptoRng>(
    l: usize,
    rng: &mut R,
) -> (
    PublicParameters<E::G1>,
    Instance<E::G1>,
    Witness<E::G1>,
    ProvingKey<E>,
    Commitment<E>,
) {
    let (d0, d2) = (1, 1);
    let g_hat = (0..d0).map(|_| E::G1Affine::rand(rng)).collect::<Vec<_>>();
    let h_hat = (0..d2).map(|_| E::G1Affine::rand(rng)).collect::<Vec<_>>();

    let w = (0..l)
        .map(|_| {
            (0..d0)
                .map(|_| E::ScalarField::rand(rng))
                .collect::<Vec<_>>()
        })
        .collect::<Vec<_>>();
    let beta = (0..l)
        .map(|_| {
            (0..d2)
                .map(|_| E::ScalarField::rand(rng))
                .collect::<Vec<_>>()
        })
        .collect::<Vec<_>>();

    let mock = LinkerCircuit::<E::G1>::mock(l);
    let (pk, _, ck) = CCGroth16::<E>::setup(mock, 0, l, rng).unwrap();

    let w_flat = cfg_iter!(w).map(|w_i| w_i[0].clone()).collect::<Vec<_>>();
    let c = CCGroth16::<E>::commit(&ck, &w_flat, rng).unwrap();
    let alpha = vec![c.opening];
    let c_hat = cfg_iter!(w)
        .zip(&beta)
        .map(|(w_i, beta_i)| {
            Pedersen::<E::G1>::commit(&g_hat, w_i) + Pedersen::<E::G1>::commit(&h_hat, beta_i)
        })
        .collect::<Vec<_>>();

    (
        PublicParameters {
            poly_ck: CommittingKey {
                g: ck.proof_dependent_g1.clone(),
                h: vec![ck.gamma_eta_g1.clone()],
            },
            coeff_ck: CommittingKey { g: g_hat, h: h_hat },
        },
        Instance {
            c: c.cm,
            c_hat: E::G1::normalize_batch(&c_hat),
        },
        Witness { w, alpha, beta },
        pk,
        c,
    )
}

fn cp_link_compressed_sigma<E: Pairing, R: RngCore + CryptoRng>(
    repeat: usize,
    l: usize,
    rng: &mut R,
) -> (u128, u128)
where
    E::G1Affine: Solidity,
    E::G2Affine: Solidity,
    <E::G1Affine as AffineRepr>::ScalarField: Solidity,
{
    let mut prover = vec![];
    let mut verifier = vec![];
    for _ in 0..repeat {
        let (pp, instance, witness, pk, commitment) = cp_link_setup::<E, _>(l, rng);

        let circuit = LinkerCircuit::<E::G1>::new(witness.w.clone());

        // prove
        let prv_instant = Instant::now();
        let mut transcript = SHA3Base::new(false);
        let lego_proof =
            CCGroth16::<E>::prove(&pk, circuit.clone(), &commitment, rng).expect("proof failed");

        let eclipse_proof =
            CompAmComEq::<E::G1>::prove(&pp, &instance, &witness, &mut transcript, rng)
                .expect("proof failed");
        prover.push(prv_instant.elapsed().as_micros());
        drop(witness);

        // verify
        let vry_instant = Instant::now();
        let mut transcript = SHA3Base::new(false);
        // assert!(
        //     CCGroth16::<E>::verify(&pk.vk, &[], &lego_proof).unwrap(),
        //     "lego proof failed"
        // );

        // assert!(
        //     CompAmComEq::<E::G1>::verify(&pp, &instance, &eclipse_proof, &mut transcript).unwrap(),
        //     "eclipse proof failed"
        // );
        verifier.push(vry_instant.elapsed().as_micros());

        if repeat == 1 {
            println!("const vk = {:?}", pk.vk.to_solidity());
            println!("const pp = {:?}", pp.to_solidity());

            println!("const lego_proof = {:?}", lego_proof.to_solidity());
            println!("const eclipse_proof = {:?}", eclipse_proof.to_solidity());

            println!("const single = {:?}", instance.c.to_solidity());
            println!("const multi = {:?}", instance.c_hat.to_solidity());

            println!(
                "const batch{} = {{ vk, pp, lego_proof, eclipse_proof, single, multi }}",
                l
            );
            println!("export default batch{}", l);
        }
    }
    (prover.average(), verifier.average())
}

pub mod bn254 {
    use crate::tests::{
        utils::{compressed_key_size, format_time, parse_env},
        LOG_MAX, LOG_MIN, NUM_REPEAT,
    };

    use super::*;
    use ark_std::{
        rand::{rngs::StdRng, SeedableRng},
        test_rng,
    };
    use lazy_static::lazy_static;

    type E = ark_bn254::Bn254;
    type C = ark_bn254::G1Projective;
    type R = StdRng;

    lazy_static! {
        pub static ref D0: usize = parse_env("D0").expect("Failed to parse D0");
        pub static ref D1: usize = parse_env("D1").expect("Failed to parse D1");
        pub static ref D2: usize = parse_env("D2").expect("Failed to parse D2");
    }

    #[test]
    fn simple_am_com_eq_scenario() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());

        let l = 1 << *LOG_MIN;
        let (pp, instance, witness) = linker_setup::<C, _>(l, *D0, *D1, *D2, &mut rng);

        // prove
        let mut transcript = SHA3Base::new(false);
        let proof = AmComEq::<C>::prove(&pp, &instance, &witness, &mut transcript, &mut rng)
            .expect("proof failed");

        // verify
        let mut transcript = SHA3Base::new(false);
        assert!(AmComEq::<C>::verify(&pp, &instance, &proof, &mut transcript).unwrap());
    }

    #[test]
    fn comp_am_com_eq_scenario() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        for n in *LOG_MIN..=*LOG_MAX {
            let l = 1 << n;

            let (prv, vrf) = process_linker::<C, R>(*NUM_REPEAT, l, *D0, *D1, *D2, &mut rng);
            println!(
                "Batch Size: 2^{} Prover: {} Verifier: {}",
                n,
                format_time(prv),
                format_time(vrf)
            );
        }
    }

    #[test]
    fn cp_link_scenario() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        for n in *LOG_MIN..=*LOG_MAX {
            let l = 1 << n;

            let (prv, vrf) = cp_link_compressed_sigma::<E, R>(*NUM_REPEAT, l, &mut rng);
            println!(
                "Batch Size: 2^{} Prover: {} Verifier: {}",
                n,
                format_time(prv),
                format_time(vrf)
            );
        }
    }

    #[test]
    fn cp_link_key_size() {
        let mut rng = R::seed_from_u64(test_rng().next_u64());
        println!("| log batch | pk | vk |");
        println!("| --- | --- | --- |");
        for n in *LOG_MIN..=*LOG_MAX {
            let batch_size = 1 << n;
            let num_aggregation_variables = 0;
            let num_committed_witness_variables = num_aggregation_variables + batch_size;
            let mock = LinkerCircuit::<C>::mock(batch_size);

            let (mut pk, _, _) = CCGroth16::<E>::setup(
                mock,
                num_aggregation_variables,
                num_committed_witness_variables,
                &mut rng,
            )
            .unwrap();

            let batch_key = vec![
                <E as Pairing>::G1Affine::rand(&mut rng),
                <E as Pairing>::G1Affine::rand(&mut rng),
            ];
            pk.vk.ck.batch_g1.extend(batch_key);

            println!(
                "| {} | {} | {} |",
                n,
                compressed_key_size(&pk),
                compressed_key_size(&pk.vk),
            );
        }
    }
}
