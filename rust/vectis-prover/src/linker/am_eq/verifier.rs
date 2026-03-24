use ark_ec::{AffineRepr, CurveGroup};
use ark_ff::PrimeField;
use ark_serialize::CanonicalSerialize;

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use crate::crypto::protocol::transcript::TranscriptProtocol;

use super::{AmEq, Commitment, Instance, Proof, PublicParameters};

impl<C: CurveGroup> AmEq<C> {
    pub fn verify_proof(
        pp: &PublicParameters<C>,
        instance: &Instance<C>,
        proof: &Proof<C>,
        transcript: &mut impl TranscriptProtocol,
    ) -> Result<bool, ()> {
        let verifier_timer = start_timer!(|| "AmComEq::Verifier");

        let challenge = Self::compute_e(&proof.c, &proof.commitment, transcript);

        let single_timer = start_timer!(|| "Single Commitment");
        let z = cfg_iter!(proof.z)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let gamma = cfg_iter!(proof.gamma)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let s_real =
            C::msm_bigint(&pp.poly_ck.g, &z[..]) + C::msm_bigint(&pp.poly_ck.h, &gamma[..]);

        let s_expected = proof.commitment.a + proof.c * challenge;
        end_timer!(single_timer);

        let multiple_timer = start_timer!(|| "Multiple Commitment");

        let m_real =
            C::msm_bigint(&pp.coeff_ck.g, &z[..]) + C::msm_bigint(&pp.coeff_ck.h, &gamma[..]);

        let l = instance.c_hat.len();
        let powers_of_tau = {
            let mut acc = vec![];
            let mut curr = instance.tau;
            for _ in 0..l {
                acc.push(curr.into_bigint());
                curr *= instance.tau;
            }
            acc
        };
        let m_aggregated = C::msm_bigint(&instance.c_hat, &powers_of_tau[..]);

        let m_expected = proof.commitment.a_hat + m_aggregated * challenge;

        end_timer!(multiple_timer);

        end_timer!(verifier_timer);

        assert_eq!(
            s_real.into_affine(),
            s_expected.into_affine(),
            "single commit different"
        );
        assert_eq!(
            m_real.into_affine(),
            m_expected.into_affine(),
            "multiple commit different"
        );

        Ok(s_real.into_affine() == s_expected.into() && m_real.into_affine() == m_expected.into())
    }

    pub fn compute_e<T: TranscriptProtocol>(
        c: &C::Affine,
        commitment: &Commitment<C>,
        transcript: &mut T,
    ) -> C::ScalarField {
        let mut bytes = vec![];

        // Append the commitment
        let (x, y) = commitment.a.xy().unwrap();
        let (x_hat, y_hat) = commitment.a_hat.xy().unwrap();
        y_hat.serialize_uncompressed(&mut bytes).unwrap();
        x_hat.serialize_uncompressed(&mut bytes).unwrap();
        y.serialize_uncompressed(&mut bytes).unwrap();
        x.serialize_uncompressed(&mut bytes).unwrap();

        // Append the c
        let (x, y) = c.xy().unwrap();
        y.serialize_uncompressed(&mut bytes).unwrap();
        x.serialize_uncompressed(&mut bytes).unwrap();
        bytes.reverse();
        transcript.append(b"commitment", &bytes);
        transcript.challenge_scalar::<C::ScalarField>(b"challenge")
    }
}
