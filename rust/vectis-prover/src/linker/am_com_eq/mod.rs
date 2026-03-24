use std::marker::PhantomData;

use ark_ec::CurveGroup;
use ark_std::rand::{CryptoRng, RngCore};

pub mod data_structure;
use crate::crypto::protocol::{sigma::SigmaProtocol, transcript::TranscriptProtocol};

pub use self::data_structure::*;

mod errors;
mod prover;
mod verifier;

pub struct AmComEq<C: CurveGroup> {
    _group: PhantomData<C>,
}

impl<C: CurveGroup> SigmaProtocol for AmComEq<C> {
    type PublicParameters = PublicParameters<C>;
    type Instance = Instance<C>;
    type Witness = Witness<C>;
    type Proof = Proof<C>;
    type Error = ();

    fn setup(pp: &Self::PublicParameters) -> Result<Self::PublicParameters, ()> {
        Ok(pp.clone())
    }

    fn prove<T: TranscriptProtocol, R: RngCore + CryptoRng>(
        pp: &Self::PublicParameters,
        instance: &Self::Instance,
        witness: &Self::Witness,
        transcript: &mut T,
        rng: &mut R,
    ) -> Result<Self::Proof, ()> {
        Self::create_proof(pp, instance, witness, transcript, rng)
    }

    fn verify<T: TranscriptProtocol>(
        pp: &Self::PublicParameters,
        instance: &Self::Instance,
        proof: &Self::Proof,
        transcript: &mut T,
    ) -> Result<bool, ()> {
        Self::verify_proof(pp, instance, proof, transcript)
    }
}
