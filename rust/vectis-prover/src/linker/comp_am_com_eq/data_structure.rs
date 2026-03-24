use ark_ec::CurveGroup;
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};

pub use crate::linker::am_com_eq::{Instance, PublicParameters, Witness};
pub(super) use crate::linker::{
    am_com_eq::Proof as ACEProof,
    comp_dl_eq::{
        Commitment as RecursionCommitment, Instance as RecursionInstance, Proof as CDEProof,
        PublicParameters as RecursionPublicParameters, Witness as RecursionWitness,
    },
};
use crate::solidity::Solidity;

#[derive(Clone, Debug, PartialEq, CanonicalSerialize, CanonicalDeserialize)]
pub struct Proof<C: CurveGroup> {
    pub commitments: Vec<RecursionCommitment<C>>,
    pub ace: ACEProof<C>,
}

impl<C: CurveGroup> Solidity for Proof<C>
where
    C::Affine: Solidity,
    C::ScalarField: Solidity,
{
    fn to_solidity(&self) -> Vec<String> {
        vec![self.ace.to_solidity(), self.commitments.to_solidity()].concat()
    }
}
