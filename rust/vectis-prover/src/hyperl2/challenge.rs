use ark_ec::{pairing::Pairing, AffineRepr};
use ark_ff::{BigInteger, PrimeField};
use sha3::{Digest, Keccak256};

use super::sum::{BatchMetadata, HyperL2Error};

pub const HYPERL2_SUM_BATCH_DOMAIN: &[u8] = b"HyperL2.SumPreservingBatch.v1";

fn append_be_bytes(
    hasher: &mut Keccak256,
    bytes: &[u8],
) {
    hasher.update(bytes);
}

fn append_prime_field<F: PrimeField>(
    hasher: &mut Keccak256,
    value: &F,
) {
    let bytes = value.into_bigint().to_bytes_be();
    let mut padded = [0u8; 32];
    let start = padded.len() - bytes.len();
    padded[start..].copy_from_slice(&bytes);
    append_be_bytes(hasher, &padded);
}

fn append_g1_point<E: Pairing>(
    hasher: &mut Keccak256,
    point: &E::G1Affine,
) -> Result<(), HyperL2Error>
where
    <E::G1Affine as AffineRepr>::BaseField: PrimeField,
{
    let x = point.x().ok_or_else(|| {
        HyperL2Error::InvalidInput("point at infinity is not allowed in batch challenge".to_string())
    })?;
    let y = point.y().ok_or_else(|| {
        HyperL2Error::InvalidInput("point at infinity is not allowed in batch challenge".to_string())
    })?;
    append_prime_field(hasher, x);
    append_prime_field(hasher, y);
    Ok(())
}

pub fn compute_batch_challenge<E: Pairing>(
    metadata: &BatchMetadata,
    prev_state_commitment: &E::G1Affine,
    next_state_commitment: &E::G1Affine,
    proof_dependent_commitment: &E::G1Affine,
) -> Result<E::ScalarField, HyperL2Error>
where
    <E::G1Affine as AffineRepr>::BaseField: PrimeField,
{
    let mut hasher = Keccak256::new();
    append_be_bytes(&mut hasher, HYPERL2_SUM_BATCH_DOMAIN);
    append_be_bytes(&mut hasher, &metadata.lane_id.to_be_bytes());
    append_be_bytes(&mut hasher, &metadata.batch_id.to_be_bytes());
    append_be_bytes(&mut hasher, &metadata.tx_count.to_be_bytes());
    append_g1_point::<E>(&mut hasher, prev_state_commitment)?;
    append_g1_point::<E>(&mut hasher, next_state_commitment)?;
    append_g1_point::<E>(&mut hasher, proof_dependent_commitment)?;

    let digest = hasher.finalize();
    Ok(E::ScalarField::from_be_bytes_mod_order(&digest))
}
