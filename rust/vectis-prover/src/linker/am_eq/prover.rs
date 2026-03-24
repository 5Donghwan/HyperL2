use ark_ec::CurveGroup;
use ark_ff::{PrimeField, UniformRand};
use ark_std::rand::Rng;

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use crate::crypto::protocol::transcript::TranscriptProtocol;

use super::{AmEq, Commitment, Instance, Proof, PublicParameters, Randomness, Witness};

impl<C: CurveGroup> AmEq<C> {
    /// Generate a random and compute the commitment
    /// Returns the randomness and the commitment
    pub fn create_random_commitment(
        pp: &PublicParameters<C>,
        rng: &mut impl Rng,
    ) -> Result<(Randomness<C>, Commitment<C>), ()> {
        let commit_timer = start_timer!(|| "AmComEq::Commit");
        let d1 = pp.poly_ck.g.len();
        let d2 = pp.poly_ck.h.len();

        let random_timer = start_timer!(|| "Generate Random");
        let r = (0..d1).map(|_| C::ScalarField::rand(rng)).collect();
        let beta = (0..d2).map(|_| C::ScalarField::rand(rng)).collect();

        let random = Randomness { r, beta };

        end_timer!(random_timer);

        let commitment = Self::create_commitment_from_random(pp, &random)?;
        end_timer!(commit_timer);

        Ok((random, commitment))
    }

    /// Compute the commitment from the randomness
    pub fn create_commitment_from_random(
        pp: &PublicParameters<C>,
        random: &Randomness<C>,
    ) -> Result<Commitment<C>, ()> {
        let a_timer = start_timer!(|| "Compute A");
        let r = cfg_iter!(random.r)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let beta = cfg_iter!(random.beta)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let a = C::msm_bigint(&pp.poly_ck.g, &r[..]) + C::msm_bigint(&pp.poly_ck.h, &beta[..]);

        end_timer!(a_timer);

        let a_hat_timer = start_timer!(|| "Compute A hat");

        let a_hat =
            C::msm_bigint(&pp.coeff_ck.g, &r[..]) + C::msm_bigint(&pp.coeff_ck.h, &beta[..]);

        end_timer!(a_hat_timer);

        Ok(Commitment {
            a: a.into(),
            a_hat: a_hat.into(),
        })
    }

    pub fn create_proof_with_assignment(
        _pp: &PublicParameters<C>,
        witness: &Witness<C>,
        randomness: &Randomness<C>,
        c: &C::Affine,
        commitment: &Commitment<C>,
        challenge: C::ScalarField,
    ) -> Result<Proof<C>, ()> {
        let z_timer = start_timer!(|| "Compute Z");
        let z = cfg_iter!(randomness.r)
            .zip(&witness.w)
            .map(|(&r, w)| r + challenge * w)
            .collect();
        end_timer!(z_timer);

        let gamma_timer = start_timer!(|| "Compute γ");
        let gamma = cfg_iter!(randomness.beta)
            .zip(&witness.alpha)
            .map(|(&beta, alpha)| beta + challenge * alpha)
            .collect();
        end_timer!(gamma_timer);

        Ok(Proof {
            c: c.clone(),
            commitment: commitment.clone(),
            z,
            gamma,
        })
    }

    /// Create a proof
    pub fn create_proof(
        pp: &PublicParameters<C>,
        _instance: &Instance<C>,
        witness: &Witness<C>,
        transcript: &mut impl TranscriptProtocol,
        rng: &mut impl Rng,
    ) -> Result<Proof<C>, ()> {
        let proof_timer = start_timer!(|| "AmComEq::Prover");

        let (randomness, commitment) = Self::create_random_commitment(pp, rng)?;
        let c_timer = start_timer!(|| "Compute C");
        let w = cfg_iter!(witness.w)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let alpha = cfg_iter!(witness.alpha)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let c = C::msm_bigint(&pp.poly_ck.g, &w[..]) + C::msm_bigint(&pp.poly_ck.h, &alpha[..]);
        end_timer!(c_timer);

        let c = c.into();
        let challenge = Self::compute_e(&c, &commitment, transcript);

        let proof = Self::create_proof_with_assignment(
            pp,
            witness,
            &randomness,
            &c,
            &commitment,
            challenge,
        )?;

        end_timer!(proof_timer);

        Ok(proof)
    }
}
