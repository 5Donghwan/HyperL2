use ark_ec::twisted_edwards::{Affine, Projective, TECurveConfig};

use super::Solidity;

impl<P: TECurveConfig> Solidity for Affine<P>
where
    P::BaseField: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        [self.x.to_solidity(), self.y.to_solidity()].concat()
    }
}

impl<P: TECurveConfig> Solidity for Projective<P>
where
    P::BaseField: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        [
            self.x.to_solidity(),
            self.y.to_solidity(),
            self.z.to_solidity(),
        ]
        .concat()
    }
}
