use std::{fmt::Debug, marker::PhantomData};

use ark_ec::CurveGroup;
use ark_r1cs_std::{boolean::Boolean, fields::FieldVar};
use ark_relations::r1cs::SynthesisError;
use ark_std::ops::Add;

use crate::crypto::commitment::constraints::BatchCommitmentGadget;

use super::Pedersen;

pub struct PedersenGadget<C, FV>
where
    C: CurveGroup,
    FV: FieldVar<C::ScalarField, C::ScalarField>,
{
    _group: PhantomData<C>,
    _field: PhantomData<FV>,
}

impl<C, FV> BatchCommitmentGadget<Pedersen<C>, C::ScalarField> for PedersenGadget<C, FV>
where
    C: CurveGroup,
    FV: FieldVar<C::ScalarField, C::ScalarField> + Clone + Debug + Add,
{
    type ScalarVar = FV;
    type ChallengeVar = FV;

    fn aggregate(
        commitments: Vec<Vec<Self::ScalarVar>>,
        tau: Self::ChallengeVar,
        initial: Option<Self::ChallengeVar>,
    ) -> (Vec<Self::ScalarVar>, Self::ChallengeVar) {
        let mut powers_of_tau = vec![];
        let mut cur = initial.unwrap_or(tau.clone());
        for _ in 0..commitments.len() {
            powers_of_tau.push(cur.clone());
            cur *= &tau;
        }

        let len = commitments[0].len();
        let indicies = 0..len;

        let aggregation = indicies
            .map(|c| {
                commitments
                    .iter()
                    .zip(powers_of_tau.iter())
                    .fold(Self::ScalarVar::zero(), |acc, (cm, tau)| {
                        acc + cm[c].clone() * tau
                    })
            })
            .collect::<Vec<Self::ScalarVar>>();

        (aggregation, cur)
    }

    fn enforce_equal(
        aggregation: Vec<Self::ScalarVar>,
        commitments: Vec<Vec<Self::ScalarVar>>,
        tau: Self::ChallengeVar,
        initial: Option<Self::ChallengeVar>,
    ) -> Result<(), SynthesisError> {
        let mut powers_of_tau = vec![];
        let mut cur = initial.unwrap_or(tau.clone());
        for _ in 0..commitments.len() {
            powers_of_tau.push(cur.clone());
            cur *= &tau;
        }

        let len = commitments[0].len();
        let indicies = 0..len;

        let _aggregation = indicies
            .map(|c| {
                commitments
                    .iter()
                    .zip(powers_of_tau.iter())
                    .fold(Self::ScalarVar::zero(), |acc, (cm, tau)| {
                        acc + cm[c].clone() * tau
                    })
            })
            .collect::<Vec<Self::ScalarVar>>();

        _aggregation
            .iter()
            .zip(aggregation.iter())
            .for_each(|(_aggr, aggr)| {
                _aggr
                    .conditional_enforce_equal(aggr, &Boolean::constant(true))
                    .unwrap()
            });

        Ok(())
    }
}
