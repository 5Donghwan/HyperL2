pub use crate::linker::am_com_eq::data_structure::{Commitment, CommittingKey, PublicParameters};
use ark_ec::CurveGroup;
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};

use crate::solidity::Solidity;

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Instance<C: CurveGroup> {
    pub c_hat: Vec<C::Affine>,
    pub tau: C::ScalarField,
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Witness<C: CurveGroup> {
    pub w: Vec<C::ScalarField>,
    pub alpha: Vec<C::ScalarField>,
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Randomness<C: CurveGroup> {
    pub r: Vec<C::ScalarField>,
    pub beta: Vec<C::ScalarField>,
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Proof<C: CurveGroup> {
    // claim that c is committed exactly same (w, alpha) as c_hat
    pub c: C::Affine,
    pub commitment: Commitment<C>,
    pub z: Vec<C::ScalarField>,
    pub gamma: Vec<C::ScalarField>,
}

impl<C: CurveGroup> Solidity for Proof<C>
where
    C::Affine: Solidity,
    C::ScalarField: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        vec![
            self.c.to_solidity(),
            self.z.to_solidity(),
            self.gamma.to_solidity(),
            self.commitment.to_solidity(),
        ]
        .concat()
    }
}
