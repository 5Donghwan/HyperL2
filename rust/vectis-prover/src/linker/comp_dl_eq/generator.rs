use ark_ec::{AffineRepr, CurveGroup};
use ark_std::Zero;

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use super::{data_structure::*, CompDLEq};

impl<C: CurveGroup> CompDLEq<C> {
    /// Prepare public parameters for the protocol.
    /// if g and g_hat are not of the same length, return an error.
    /// Otherwise, return the public parameters with an extended power-of-two length for g and g_hat.
    pub fn prepare_public_parameters(pp: &PublicParameters<C>) -> Result<PublicParameters<C>, ()> {
        if pp.g.len() != pp.g_hat.len() {
            return Err(());
        }

        let diff = Self::rescale_size(pp.g.len()) - pp.g.len();

        Ok(PublicParameters {
            g: vec![pp.g.clone(), vec![C::Affine::generator(); diff]].concat(),
            g_hat: vec![pp.g_hat.clone(), vec![C::Affine::generator(); diff]].concat(),
        })
    }

    pub fn prepare_witness(witness: &Witness<C>) -> Result<Witness<C>, ()> {
        let diff = Self::rescale_size(witness.z.len()) - witness.z.len();

        Ok(Witness {
            z: vec![witness.z.clone(), vec![C::ScalarField::zero(); diff]].concat(),
        })
    }

    /// Both the prover and verifier need to update the public parameters
    pub fn update_public_parameters(
        pp: &PublicParameters<C>,
        challenge: C::ScalarField,
    ) -> Result<PublicParameters<C>, ()> {
        let update_timer = start_timer!(|| "CompDLEq::Update Public Parameters");

        let mid = pp.g.len() / 2;

        let g_timer = start_timer!(|| "Update G");
        let g = cfg_iter!(pp.g[..mid])
            .zip(&pp.g[mid..])
            .map(|(l, r)| *l * challenge + r)
            .collect::<Vec<_>>();
        end_timer!(g_timer);

        let g_hat_timer = start_timer!(|| "Update G Hat");
        let g_hat = cfg_iter!(pp.g_hat[..mid])
            .zip(&pp.g_hat[mid..])
            .map(|(l, r)| *l * challenge + r)
            .collect::<Vec<_>>();
        end_timer!(g_hat_timer);
        end_timer!(update_timer);

        Ok(PublicParameters {
            g: C::normalize_batch(&g),
            g_hat: C::normalize_batch(&g_hat),
        })
    }

    pub fn update_instance(
        instance: &Instance<C>,
        commitment: &Commitment<C>,
        challenge: C::ScalarField,
    ) -> Result<Instance<C>, ()> {
        let update_timer = start_timer!(|| "CompDLEq::Update Instance");
        let sqr_challenge = challenge * challenge;
        let y = commitment.left + instance.y * challenge + commitment.right * sqr_challenge;
        let y_hat =
            commitment.left_hat + instance.y_hat * challenge + commitment.right_hat * sqr_challenge;
        end_timer!(update_timer);

        Ok(Instance {
            y: y.into_affine(),
            y_hat: y_hat.into_affine(),
        })
    }

    fn rescale_size(l: usize) -> usize {
        if l.is_power_of_two() {
            l
        } else {
            l.next_power_of_two()
        }
    }
}
