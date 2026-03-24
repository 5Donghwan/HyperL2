use ark_ec::short_weierstrass::{Affine, Projective, SWCurveConfig};

use super::Solidity;

impl<P: SWCurveConfig> Solidity for Affine<P>
where
    P::BaseField: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        [self.x.to_solidity(), self.y.to_solidity()].concat()
    }
}

impl<P: SWCurveConfig> Solidity for Projective<P>
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
