use ark_ec::CurveGroup;

use crate::{
    crypto::protocol::transcript::TranscriptProtocol,
    linker::{am_com_eq::AmComEq, comp_dl_eq::CompDLEq},
};

use super::{CDEProof, CompAmComEq, Instance, Proof, PublicParameters};

impl<C: CurveGroup> CompAmComEq<C> {
    pub fn verify_proof<T: TranscriptProtocol>(
        pp: &PublicParameters<C>,
        instance: &Instance<C>,
        proof: &Proof<C>,
        transcript: &mut T,
    ) -> Result<bool, ()> {
        if proof.ace.z.len() != 2 {
            return Err(());
        }

        let verifier_timer = start_timer!(|| "CompAmComEq::Verify");

        let powers_of_x = AmComEq::compute_powers_of_x(instance, transcript);
        let challenge = AmComEq::compute_e(&proof.ace.commitment, transcript);

        let (pp, instance, witness) =
            Self::prepare_for_comp_dl_eq(pp, instance, &proof.ace, &powers_of_x, challenge)?;

        let cde_proof = CDEProof {
            commitments: proof.commitments.clone(),
            z: witness.z.clone(),
        };
        let proof = CompDLEq::verify_proof(&pp, &instance, &cde_proof, transcript)?;

        end_timer!(verifier_timer);
        Ok(proof)
    }
}
