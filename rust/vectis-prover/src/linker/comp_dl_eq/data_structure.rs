use ark_ec::CurveGroup;
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};

use crate::solidity::Solidity;

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct PublicParameters<C: CurveGroup> {
    pub g: Vec<C::Affine>,
    pub g_hat: Vec<C::Affine>,
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Instance<C: CurveGroup> {
    /// `Y` in the first step, or `Y'` of recursive steps in CompDLEq
    pub y: C::Affine,

    /// `Yˆ` in the first step, or ` Yˆ'` of recursive steps in CompDLEq
    pub y_hat: C::Affine,
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Witness<C: CurveGroup> {
    pub z: Vec<C::ScalarField>,
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Commitment<C: CurveGroup> {
    pub left: C::Affine,
    pub right: C::Affine,
    pub left_hat: C::Affine,
    pub right_hat: C::Affine,
}

impl<C: CurveGroup> Solidity for Commitment<C>
where
    C::Affine: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        vec![
            self.left.to_solidity(),
            self.right.to_solidity(),
            self.left_hat.to_solidity(),
            self.right_hat.to_solidity(),
        ]
        .concat()
    }
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Proof<C: CurveGroup> {
    pub commitments: Vec<Commitment<C>>,
    pub z: Vec<C::ScalarField>,
}
