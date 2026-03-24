use super::transcript::TranscriptProtocol;
use ark_std::rand::{CryptoRng, RngCore};

/// Sigma protocol trait for zero-knowledge proof systems.
pub trait SigmaProtocol {
    type PublicParameters;
    type Instance;
    type Witness;
    type Proof;

    type Error: 'static;

    fn setup(pp: &Self::PublicParameters) -> Result<Self::PublicParameters, ()>;

    fn prove<T: TranscriptProtocol, R: RngCore + CryptoRng>(
        pp: &Self::PublicParameters,
        instance: &Self::Instance,
        witness: &Self::Witness,
        transcript: &mut T,
        rng: &mut R,
    ) -> Result<Self::Proof, ()>;

    fn verify<T: TranscriptProtocol>(
        pp: &Self::PublicParameters,
        instance: &Self::Instance,
        proof: &Self::Proof,
        transcript: &mut T,
    ) -> Result<bool, ()>;
}
