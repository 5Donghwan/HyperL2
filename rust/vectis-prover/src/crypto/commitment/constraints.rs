use ark_ff::Field;
use ark_r1cs_std::alloc::AllocVar;
use ark_relations::r1cs::SynthesisError;
use ark_std::vec::Vec;

use super::BatchCommitmentScheme;

pub trait BatchCommitmentGadget<C: BatchCommitmentScheme, ConstraintF: Field> {
    type ScalarVar: AllocVar<C::Scalar, ConstraintF> + Clone;
    type ChallengeVar: AllocVar<C::Challenge, ConstraintF> + Clone;

    fn aggregate(
        commitments: Vec<Vec<Self::ScalarVar>>,
        tau: Self::ChallengeVar,
        initial: Option<Self::ChallengeVar>,
    ) -> (Vec<Self::ScalarVar>, Self::ChallengeVar);

    fn enforce_equal(
        aggregation: Vec<Self::ScalarVar>,
        commitments: Vec<Vec<Self::ScalarVar>>,
        tau: Self::ChallengeVar,
        initial: Option<Self::ChallengeVar>,
    ) -> Result<(), SynthesisError>;
}
