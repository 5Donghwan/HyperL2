use std::collections::BTreeMap;

use ark_ff::PrimeField;
use ark_std::vec::Vec;

use super::sum::{BatchMetadata, HyperL2Error, StateTransitionBatchInput};

#[derive(Clone, Debug, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct AccountAddress(pub [u8; 20]);

impl AccountAddress {
    pub fn from_u64(value: u64) -> Self {
        let mut bytes = [0u8; 20];
        bytes[12..].copy_from_slice(&value.to_be_bytes());
        Self(bytes)
    }
}

#[derive(Clone, Debug)]
pub struct FixedSlotMap {
    accounts: Vec<AccountAddress>,
    index_by_address: BTreeMap<AccountAddress, usize>,
}

impl FixedSlotMap {
    pub fn new(accounts: Vec<AccountAddress>) -> Result<Self, HyperL2Error> {
        if accounts.is_empty() {
            return Err(HyperL2Error::InvalidInput(
                "slot map must contain at least one account".to_string(),
            ));
        }

        let mut index_by_address = BTreeMap::new();
        for (index, account) in accounts.iter().cloned().enumerate() {
            if index_by_address.insert(account.clone(), index).is_some() {
                return Err(HyperL2Error::InvalidInput(
                    "slot map contains duplicate account addresses".to_string(),
                ));
            }
        }

        Ok(Self {
            accounts,
            index_by_address,
        })
    }

    pub fn len(&self) -> usize {
        self.accounts.len()
    }

    pub fn is_empty(&self) -> bool {
        self.accounts.is_empty()
    }

    pub fn slot_of(&self, address: &AccountAddress) -> Result<usize, HyperL2Error> {
        self.index_by_address
            .get(address)
            .copied()
            .ok_or_else(|| {
                HyperL2Error::InvalidInput("transaction address missing from slot map".to_string())
            })
    }

    pub fn account_at(&self, index: usize) -> Option<&AccountAddress> {
        self.accounts.get(index)
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct RawTransferTx<F: PrimeField> {
    pub sender: AccountAddress,
    pub receiver: AccountAddress,
    pub amount: F,
}

pub fn build_state_transition_batch<F: PrimeField>(
    slot_map: &FixedSlotMap,
    pre_state_values: &[F],
    txs: &[RawTransferTx<F>],
    metadata: BatchMetadata,
    pre_state_blind: F,
    post_state_blind: F,
) -> Result<StateTransitionBatchInput<F>, HyperL2Error> {
    let batch_size = usize::try_from(metadata.tx_count)
        .map_err(|_| HyperL2Error::InvalidInput("tx_count does not fit usize".to_string()))?;
    if slot_map.len() != batch_size {
        return Err(HyperL2Error::InvalidInput(format!(
            "slot map length {} does not match tx_count {}",
            slot_map.len(),
            batch_size
        )));
    }
    if pre_state_values.len() != slot_map.len() {
        return Err(HyperL2Error::InvalidInput(format!(
            "pre_state_values length {} does not match slot map length {}",
            pre_state_values.len(),
            slot_map.len()
        )));
    }
    if txs.len() != batch_size {
        return Err(HyperL2Error::InvalidInput(format!(
            "tx count {} does not match metadata tx_count {}",
            txs.len(),
            batch_size
        )));
    }

    let mut post_state_values = pre_state_values.to_vec();
    for tx in txs {
        let sender_index = slot_map.slot_of(&tx.sender)?;
        let receiver_index = slot_map.slot_of(&tx.receiver)?;
        post_state_values[sender_index] -= tx.amount;
        post_state_values[receiver_index] += tx.amount;
    }

    Ok(StateTransitionBatchInput {
        lane_id: metadata.lane_id,
        batch_id: metadata.batch_id,
        tx_count: metadata.tx_count,
        pre_state_values: pre_state_values.to_vec(),
        post_state_values,
        pre_state_blind,
        post_state_blind,
    })
}

pub fn demo_state_transition_batch<F: PrimeField>(
    lane_id: u16,
    batch_id: u64,
    batch_size: usize,
) -> Result<(FixedSlotMap, Vec<RawTransferTx<F>>, StateTransitionBatchInput<F>), HyperL2Error> {
    let accounts = (0..batch_size)
        .map(|i| AccountAddress::from_u64((i + 1) as u64))
        .collect::<Vec<_>>();
    let slot_map = FixedSlotMap::new(accounts.clone())?;

    let pre_state_values = (0..batch_size)
        .map(|i| {
            let base = 10_000u64 + ((i % 251) as u64) * 17 + (i as u64 % 19);
            F::from(base)
        })
        .collect::<Vec<_>>();

    let txs = (0..batch_size)
        .map(|i| RawTransferTx {
            sender: accounts[i].clone(),
            receiver: accounts[(i + 1) % batch_size].clone(),
            amount: F::from(((i % 31) + 1) as u64),
        })
        .collect::<Vec<_>>();

    let metadata = BatchMetadata {
        lane_id,
        batch_id,
        tx_count: u32::try_from(batch_size)
            .map_err(|_| HyperL2Error::InvalidInput("batch_size does not fit u32".to_string()))?,
    };
    let pre_state_blind = F::from(0xC0FFEEu64);
    let post_state_blind = F::from(0xBAD5EEDu64);
    let batch = build_state_transition_batch(
        &slot_map,
        &pre_state_values,
        &txs,
        metadata,
        pre_state_blind,
        post_state_blind,
    )?;

    Ok((slot_map, txs, batch))
}

#[cfg(test)]
mod tests {
    use ark_bn254::Fr;

    use super::*;

    #[test]
    fn state_transition_builder_preserves_total_sum() {
        let (_, txs, batch) = demo_state_transition_batch::<Fr>(0, 1, 8).unwrap();
        assert_eq!(txs.len(), 8);
        let pre_sum = batch
            .pre_state_values
            .iter()
            .copied()
            .fold(Fr::from(0u64), |acc, value| acc + value);
        let post_sum = batch
            .post_state_values
            .iter()
            .copied()
            .fold(Fr::from(0u64), |acc, value| acc + value);
        assert_eq!(pre_sum, post_sum);
    }
}
