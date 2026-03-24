pub mod constraints;
pub use constraints::PedersenGadget;

use std::marker::PhantomData;

use ark_ec::{AffineRepr, CurveGroup};
use ark_ff::{BigInteger, Field, PrimeField};
use ark_std::{vec::Vec, Zero};
use sha3::{Digest, Keccak256};

use super::{BatchCommitmentScheme, CommitmentScheme};

#[cfg(feature = "parallel")]
use rayon::prelude::*;

type BasePrimeField<C> = <<C as CurveGroup>::BaseField as Field>::BasePrimeField;

pub struct Pedersen<C: CurveGroup> {
    _group: PhantomData<C>,
}

impl<C: CurveGroup> CommitmentScheme for Pedersen<C> {
    type Scalar = C::ScalarField;
    type Base = C::Affine;
    type Commitment = C::Affine;

    fn commit(committing_key: &[Self::Base], commitments: &[Self::Scalar]) -> Self::Commitment {
        let commitments = cfg_iter!(commitments)
            .map(|cm| cm.into_bigint())
            .collect::<Vec<_>>();
        C::msm_bigint(&committing_key[..], &commitments[..]).into_affine()
    }
}

impl<C: CurveGroup> BatchCommitmentScheme for Pedersen<C> {
    type Challenge = C::ScalarField;

    fn batch_commit(
        batch_key: &[Self::Base],
        commitments: &[&[Self::Scalar]],
    ) -> Vec<Self::Commitment> {
        let commitments_g1 = cfg_iter!(commitments)
            .map(|cm| Pedersen::<C>::commit(batch_key, cm))
            .collect::<Vec<Self::Base>>();

        commitments_g1
    }

    fn challenge(
        public_inputs: &[Self::Scalar],
        commitments: &[Self::Commitment],
        proof_dependent_commitment: &Self::Base,
    ) -> Self::Challenge {
        let mut hasher = Keccak256::new();
        let mut strings = vec![];

        let mut update = |s: &String| {
            let zero = BasePrimeField::<C>::zero();
            let scalar = s.parse::<BasePrimeField<C>>().ok().unwrap_or(zero);
            let bigint = scalar.into_bigint();
            let bytes = bigint.to_bytes_be();
            hasher.update(&bytes);
        };

        public_inputs
            .iter()
            .for_each(|x| strings.push(x.to_string()));

        commitments.iter().for_each(|cm| {
            strings.push(cm.x().unwrap().to_string());
            strings.push(cm.y().unwrap().to_string());
        });

        strings.push(proof_dependent_commitment.x().unwrap().to_string());
        strings.push(proof_dependent_commitment.y().unwrap().to_string());

        strings.iter().for_each(|s| update(s));
        drop(strings);

        let mut raw = vec![0u8; 32];
        raw.copy_from_slice(&hasher.finalize());

        C::ScalarField::from_be_bytes_mod_order(&raw)
    }

    fn aggregate(
        commitments: &[Self::Commitment],
        tau: Self::Challenge,
        initial: Option<Self::Challenge>,
    ) -> (Self::Commitment, Self::Challenge) {
        let mut powers_of_tau = vec![];
        let mut cur = initial.unwrap_or(tau);
        for _ in 0..commitments.len() {
            powers_of_tau.push(cur.into());
            cur *= &tau;
        }

        (
            C::msm_bigint(&commitments[..], &powers_of_tau[..]).into_affine(),
            cur, // next initial
        )
    }

    fn scalar_aggregate(
        commitments: &[&[Self::Scalar]],
        tau: Self::Challenge,
        initial: Option<Self::Challenge>,
    ) -> (Vec<Self::Scalar>, Self::Challenge) {
        // powers_of_tau = [t, ..., t^(max_degree + 1)], len = commitments.len()
        let mut powers_of_tau = vec![];
        let mut cur = initial.unwrap_or(tau);
        for _ in 0..commitments.len() {
            powers_of_tau.push(cur);
            cur *= &tau;
        }

        let len = commitments[0].len();
        let indicies = (0..len).collect::<Vec<usize>>();

        (
            cfg_iter!(indicies)
                .map(|c| {
                    cfg_iter!(commitments)
                        .zip(cfg_iter!(powers_of_tau))
                        .map(|(cm, tau)| cm[*c] * tau)
                        .sum()
                })
                .collect(),
            cur, // next initial
        )
    }
}
