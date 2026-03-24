use ark_ec::CurveGroup;
use ark_std::rand::{CryptoRng, RngCore};

use crate::{
    crypto::protocol::transcript::TranscriptProtocol,
    linker::{am_com_eq::AmComEq, comp_dl_eq::CompDLEq},
};

use super::{data_structure::*, CompAmComEq};

impl<C: CurveGroup> CompAmComEq<C> {
    pub fn creat_proof_with_combined<R: RngCore + CryptoRng>(
        pp: &PublicParameters<C>,
        instance: &Instance<C>,
        witness: &Witness<C>,
        transcript: &mut impl TranscriptProtocol,
        rng: &mut R,
    ) -> Result<Proof<C>, ()> {
        let prover_timer = start_timer!(|| "CompAmComEq::Prover");

        let powers_of_x = AmComEq::compute_powers_of_x(instance, transcript);
        let (randomness, commitment) = AmComEq::create_random_commitment(pp, &powers_of_x, rng)?;
        let challenge = AmComEq::compute_e(&commitment, transcript);
        let ace_proof = AmComEq::create_proof_with_assignment(
            pp,
            witness,
            &randomness,
            &commitment,
            &powers_of_x,
            challenge,
        )?;

        let (pp, instance, witness) =
            Self::prepare_for_comp_dl_eq(pp, instance, &ace_proof, &powers_of_x, challenge)?;
        let cde_proof = CompDLEq::create_proof(&pp, &instance, &witness, transcript)?;

        end_timer!(prover_timer);
        Ok(Proof {
            commitments: cde_proof.commitments,
            ace: ACEProof {
                commitment,
                z: cde_proof.z,
                omega: ace_proof.omega,
                omega_hat: ace_proof.omega_hat,
            },
        })
    }
}
