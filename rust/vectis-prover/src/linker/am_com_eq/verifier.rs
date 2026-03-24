use ark_ec::{AffineRepr, CurveGroup};
use ark_ff::PrimeField;
use ark_serialize::CanonicalSerialize;
use ark_std::One;

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use crate::crypto::protocol::transcript::TranscriptProtocol;

use super::{AmComEq, Commitment, Instance, Proof, PublicParameters};

impl<C: CurveGroup> AmComEq<C> {
    pub fn verify_proof(
        pp: &PublicParameters<C>,
        instance: &Instance<C>,
        proof: &Proof<C>,
        transcript: &mut impl TranscriptProtocol,
    ) -> Result<bool, ()> {
        let verifier_timer = start_timer!(|| "AmComEq::Verifier");

        let powers_of_x = Self::compute_powers_of_x(instance, transcript);

        let challenge = Self::compute_e(&proof.commitment, transcript);

        let single_timer = start_timer!(|| "Single Commitment");
        let z = cfg_iter!(proof.z)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let omega = cfg_iter!(proof.omega)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let s_real =
            C::msm_bigint(&pp.poly_ck.g, &z[..]) + C::msm_bigint(&pp.poly_ck.h, &omega[..]);
        drop(omega);

        let s_expected = proof.commitment.a + instance.c * challenge;
        end_timer!(single_timer);

        let multiple_timer = start_timer!(|| "Multiple Commitment");

        let d0 = pp.coeff_ck.g.len();
        let l = powers_of_x.len();

        let d0_indicies = (0..d0).collect::<Vec<_>>();
        let l_indicies = (0..l).collect::<Vec<_>>();

        let aggregated_z = cfg_iter!(d0_indicies)
            .map(|&j| {
                cfg_iter!(l_indicies)
                    .map(|&i| proof.z[d0 * i + j] * powers_of_x[i])
                    .sum::<C::ScalarField>()
                    .into_bigint()
            })
            .collect::<Vec<_>>();
        drop(z);

        let omega_hat = cfg_iter!(proof.omega_hat)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let m_real = C::msm_bigint(&pp.coeff_ck.g, &aggregated_z)
            + C::msm_bigint(&pp.coeff_ck.h, &omega_hat[..]);

        drop(aggregated_z);
        drop(omega_hat);

        let powers_of_x = cfg_iter!(powers_of_x)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let m_expected =
            proof.commitment.a_hat + C::msm_bigint(&instance.c_hat, &powers_of_x[..]) * challenge;

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

    pub fn compute_powers_of_x<T: TranscriptProtocol>(
        instance: &Instance<C>,
        transcript: &mut T,
    ) -> Vec<C::ScalarField> {
        let l = instance.c_hat.len();
        let instance = vec![&[instance.c][..], &instance.c_hat].concat();
        let bytes = cfg_iter!(instance)
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
        transcript.append(b"instance", &bytes);
        let x = transcript.challenge_scalar::<C::ScalarField>(b"challenge");

        let mut powers_of_x = vec![];
        let mut curr = C::ScalarField::one();
        for _ in 0..l {
            powers_of_x.push(curr);
            curr *= x;
        }
        powers_of_x
    }

    pub fn compute_e<T: TranscriptProtocol>(
        commitment: &Commitment<C>,
        transcript: &mut T,
    ) -> C::ScalarField {
        let mut bytes = vec![];
        let (x, y) = commitment.a.xy().unwrap();
        let (x_hat, y_hat) = commitment.a_hat.xy().unwrap();
        y_hat.serialize_uncompressed(&mut bytes).unwrap();
        x_hat.serialize_uncompressed(&mut bytes).unwrap();
        y.serialize_uncompressed(&mut bytes).unwrap();
        x.serialize_uncompressed(&mut bytes).unwrap();
        bytes.reverse();
        transcript.append(b"commitment", &bytes);
        transcript.challenge_scalar::<C::ScalarField>(b"challenge")
    }
}
