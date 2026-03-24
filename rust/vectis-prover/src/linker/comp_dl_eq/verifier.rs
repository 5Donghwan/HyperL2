use ark_ec::{AffineRepr, CurveGroup};
use ark_ff::PrimeField;
use ark_serialize::CanonicalSerialize;
use ark_std::{vec::Vec, One, Zero};

use crate::crypto::protocol::transcript::TranscriptProtocol;

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use super::{Commitment, CompDLEq, Instance, Proof, PublicParameters};

impl<C: CurveGroup> CompDLEq<C> {
    pub fn verify_proof(
        pp: &PublicParameters<C>,
        instance: &Instance<C>,
        proof: &Proof<C>,
        transcript: &mut impl TranscriptProtocol,
    ) -> Result<bool, ()> {
        let threshold = 2;
        if proof.z.len() != threshold || pp.g.len() != pp.g_hat.len() {
            return Err(());
        }

        let verifier_timer = start_timer!(|| "CompDLEq::Verifier");
        let mut pp = pp.clone();
        let mut instance = instance.clone();
        let mut challenges = vec![];
        proof.commitments.iter().for_each(|commitment| {
            let challenge = Self::compute_challenge(commitment, transcript);
            challenges.push(challenge);

            instance = Self::update_instance(&instance, commitment, challenge).unwrap();
        });
        pp = Self::update_public_parameters_once(&pp, &challenges).unwrap();

        if proof.z.len() != pp.g.len() {
            return Err(());
        }
        let y_real = (pp.g[0] * proof.z[0] + pp.g[1] * proof.z[1]).into_affine();
        let y_hat_real = (pp.g_hat[0] * proof.z[0] + pp.g_hat[1] * proof.z[1]).into_affine();
        end_timer!(verifier_timer);

        Ok(instance.y == y_real && instance.y_hat == y_hat_real)
    }

    /// Verifier could update the public parameters faster
    pub fn update_public_parameters_once(
        pp: &PublicParameters<C>,
        challenges: &[C::ScalarField],
    ) -> Result<PublicParameters<C>, ()> {
        let update_timer = start_timer!(|| "CompDLEq::Update Public Parameters");

        let challenges = {
            let n = 1 << challenges.len();
            let mut arr = cfg_into_iter!(0..n)
                .map(|mut b| {
                    let mut k = C::ScalarField::one();
                    let mut i = challenges.len() - 1;
                    while b > 0 {
                        if (b & 1) > 0 {
                            k *= challenges[i];
                        }
                        b >>= 1;
                        i -= 1;
                    }
                    k.into_bigint()
                })
                .collect::<Vec<_>>();
            arr.reverse();
            arr
        };

        let g = Self::fold(&pp.g, &challenges);
        let g_hat = Self::fold(&pp.g_hat, &challenges);
        end_timer!(update_timer);

        Ok(PublicParameters { g, g_hat })
    }

    fn fold(
        generators: &[C::Affine],
        factors: &[<C::ScalarField as PrimeField>::BigInt],
    ) -> Vec<C::Affine> {
        if (generators.len() != factors.len() << 1) {
            println!("error");
        }
        assert_eq!(
            generators.len(),
            factors.len() << 1,
            "Fold: length mismatch"
        );
        let left = cfg_iter!(generators)
            .step_by(2)
            .map(|l| l.clone())
            .collect::<Vec<_>>();
        let right = cfg_iter!(generators)
            .skip(1)
            .step_by(2)
            .map(|r| r.clone())
            .collect::<Vec<_>>();
        let g = vec![
            C::msm_bigint(&left, &factors[..]),
            C::msm_bigint(&right, &factors[..]),
        ];
        C::normalize_batch(&g)
    }

    pub fn compute_challenge(
        commitment: &Commitment<C>,
        transcript: &mut impl TranscriptProtocol,
    ) -> C::ScalarField {
        let commitment = vec![
            commitment.left,
            commitment.right,
            commitment.left_hat,
            commitment.right_hat,
        ];

        let bytes = cfg_iter!(commitment)
            .map(|p| {
                let mut _bytes = vec![];
                let (x, y) = p.xy().unwrap();
                y.serialize_uncompressed(&mut _bytes).unwrap();
                x.serialize_uncompressed(&mut _bytes).unwrap();
                _bytes.reverse();
                _bytes
            })
            .flatten()
            .collect::<Vec<_>>();

        transcript.append(b"commitments", &bytes[..]);
        transcript.challenge_scalar(b"challenge")
    }
}
