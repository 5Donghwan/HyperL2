use ark_ec::CurveGroup;
use ark_ff::PrimeField;

#[cfg(feature = "parallel")]
use rayon::prelude::*;

use super::{data_structure::*, CompAmComEq};

impl<C: CurveGroup> CompAmComEq<C> {
    pub fn prepare_for_comp_dl_eq(
        pp: &PublicParameters<C>,
        instance: &Instance<C>,
        proof: &ACEProof<C>,
        powers_of_x: &[C::ScalarField],
        challenge: C::ScalarField,
    ) -> Result<
        (
            RecursionPublicParameters<C>,
            RecursionInstance<C>,
            RecursionWitness<C>,
        ),
        (),
    > {
        let prepare_timer = start_timer!(|| "Prepare for CompDLEq");

        let g = pp.poly_ck.g.clone();
        let g_hat = cfg_iter!(powers_of_x)
            .map(|x| {
                cfg_iter!(pp.coeff_ck.g)
                    .map(|g| (*g * x).into_affine())
                    .collect::<Vec<_>>()
            })
            .collect::<Vec<_>>()
            .concat();

        let expected_timer = start_timer!(|| "Compute Expected");

        let y_timer = start_timer!(|| "Compute Y");
        let omega = cfg_iter!(proof.omega)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let y =
            proof.commitment.a + instance.c * challenge - C::msm_bigint(&pp.poly_ck.h, &omega[..]);
        drop(omega);
        end_timer!(y_timer);

        let y_hat_timer = start_timer!(|| "Compute Y Hat");
        let powers_of_x = cfg_iter!(powers_of_x)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let omega_hat = cfg_iter!(proof.omega_hat)
            .map(|s| s.into_bigint())
            .collect::<Vec<_>>();
        let y_hat = proof.commitment.a_hat
            + C::msm_bigint(&instance.c_hat, &powers_of_x[..]) * challenge
            - C::msm_bigint(&pp.coeff_ck.h, &omega_hat[..]);
        drop(powers_of_x);
        drop(omega_hat);
        end_timer!(y_hat_timer);
        end_timer!(expected_timer);
        end_timer!(prepare_timer);

        Ok((
            RecursionPublicParameters { g, g_hat },
            RecursionInstance {
                y: y.into_affine(),
                y_hat: y_hat.into_affine(),
            },
            RecursionWitness { z: proof.z.clone() },
        ))
    }
}
