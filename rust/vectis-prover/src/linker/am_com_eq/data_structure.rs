use ark_ec::CurveGroup;
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};

use crate::solidity::Solidity;

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct CommittingKey<C: CurveGroup> {
    pub g: Vec<C::Affine>,
    pub h: Vec<C::Affine>,
}

impl<C: CurveGroup> Solidity for CommittingKey<C>
where
    C::Affine: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        vec![self.g.to_solidity(), self.h.to_solidity()].concat()
    }
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct PublicParameters<C: CurveGroup> {
    pub poly_ck: CommittingKey<C>,
    pub coeff_ck: CommittingKey<C>,
}

impl<C: CurveGroup> Solidity for PublicParameters<C>
where
    C::Affine: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        let l = self.poly_ck.g.len() / self.coeff_ck.g.len();
        let d0 = self.coeff_ck.g.len();
        let d1 = self.poly_ck.h.len();
        let d2 = self.coeff_ck.h.len();
        let mut v = vec![
            l.to_string(),
            d0.to_string(),
            d1.to_string(),
            d2.to_string(),
        ];
        v.extend(C::generator().into_affine().to_solidity());
        v.extend(self.poly_ck.to_solidity());
        v.extend(self.coeff_ck.to_solidity());
        v
    }
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Instance<C: CurveGroup> {
    pub c: C::Affine,
    pub c_hat: Vec<C::Affine>,
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Witness<C: CurveGroup> {
    pub w: Vec<Vec<C::ScalarField>>,
    pub alpha: Vec<C::ScalarField>,
    pub beta: Vec<Vec<C::ScalarField>>,
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Randomness<C: CurveGroup> {
    pub r: Vec<C::ScalarField>,
    pub delta: Vec<C::ScalarField>,
    pub gamma: Vec<C::ScalarField>,
}

/// Eclipse: AmComEq commitment
#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Commitment<C: CurveGroup> {
    pub a: C::Affine,
    pub a_hat: C::Affine,
}

impl<C: CurveGroup> Solidity for Commitment<C>
where
    C::Affine: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        vec![self.a.to_solidity(), self.a_hat.to_solidity()].concat()
    }
}

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Proof<C: CurveGroup> {
    pub commitment: Commitment<C>,
    pub z: Vec<C::ScalarField>,
    pub omega: Vec<C::ScalarField>,
    pub omega_hat: Vec<C::ScalarField>,
}

impl<C: CurveGroup> Solidity for Proof<C>
where
    C::Affine: Solidity,
    C::ScalarField: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        vec![
            self.z.to_solidity(),
            self.omega.to_solidity(),
            self.omega_hat.to_solidity(),
            self.commitment.to_solidity(),
        ]
        .concat()
    }
}
