use ark_ec::CurveGroup;
use ark_ff::PrimeField;

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use crate::crypto::protocol::transcript::TranscriptProtocol;

use super::{data_structure::*, CompDLEq};

impl<C: CurveGroup> CompDLEq<C> {
    pub fn compute_depth_commitment_from_updated_parameters(
        pp: &PublicParameters<C>,
        witness: &Witness<C>,
    ) -> Result<Commitment<C>, ()> {
        if witness.z.len() != pp.g.len() || witness.z.len() != pp.g_hat.len() {
            return Err(());
        }
        let commit_timer = start_timer!(|| "CompDLEq::Depth Commit");
        let mid = witness.z.len() / 2;

        let z_bigint = cfg_iter!(witness.z)
            .map(|z| z.into_bigint())
            .collect::<Vec<_>>();

        let lr_timer = start_timer!(|| "Compute LR");
        let left = C::msm_bigint(&pp.g[mid..], &z_bigint[..mid]);
        let right = C::msm_bigint(&pp.g[..mid], &z_bigint[mid..]);

        end_timer!(lr_timer);

        let lr_hat_timer = start_timer!(|| "Compute LR Hat");
        let left_hat = C::msm_bigint(&pp.g_hat[mid..], &z_bigint[..mid]);
        let right_hat = C::msm_bigint(&pp.g_hat[..mid], &z_bigint[mid..]);

        end_timer!(lr_hat_timer);
        end_timer!(commit_timer);

        Ok(Commitment {
            left: left.into_affine(),
            right: right.into_affine(),
            left_hat: left_hat.into_affine(),
            right_hat: right_hat.into_affine(),
        })
    }

    /// Update the witness with the challenge.
    /// Prover also needs to update the witness with the challenge.
    pub fn update_witness(
        witness: &Witness<C>,
        challenge: C::ScalarField,
    ) -> Result<Witness<C>, ()> {
        let witness_timer = start_timer!(|| "Update witness");
        let mid = witness.z.len() / 2;
        let z = cfg_iter!(witness.z[..mid])
            .zip(&witness.z[mid..])
            .map(|(l, r)| *l + challenge * r)
            .collect::<Vec<_>>();
        end_timer!(witness_timer);

        Ok(Witness { z })
    }

    pub fn create_proof(
        pp: &PublicParameters<C>,
        instance: &Instance<C>,
        witness: &Witness<C>,
        transcript: &mut impl TranscriptProtocol,
    ) -> Result<Proof<C>, ()> {
        let threshold = 2;
        if pp.g.len() != pp.g_hat.len() {
            return Err(());
        }

        let prover_timer = start_timer!(|| "CompDLEq::Prover");
        let mut commitments = vec![];

        let mut pp = pp.clone();
        let mut instance = instance.clone();
        let mut witness = Self::prepare_witness(witness)?;
        while pp.g.len() > threshold {
            let commitment = Self::compute_depth_commitment_from_updated_parameters(&pp, &witness)?;

            let challenge = Self::compute_challenge(&commitment, transcript);

            pp = Self::update_public_parameters(&pp, challenge)?;
            instance = Self::update_instance(&instance, &commitment, challenge)?;
            witness = Self::update_witness(&witness, challenge)?;
            commitments.push(commitment);
        }

        end_timer!(prover_timer);

        Ok(Proof {
            commitments,
            z: witness.z,
        })
    }
}
