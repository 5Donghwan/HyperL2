use super::TranscriptProtocol;
use ark_ff::{BigInteger, PrimeField};
use sha3::{Digest, Keccak256};

#[derive(Clone)]
pub struct SHA3Base {
    bytes: Vec<u8>,
    use_label: bool,
}

impl TranscriptProtocol for SHA3Base {
    fn new(use_label: bool) -> Self {
        SHA3Base {
            bytes: Vec::new(),
            use_label,
        }
    }

    fn append(&mut self, label: &'static [u8], item: &[u8]) {
        if self.use_label {
            self.bytes.extend_from_slice(label);
        }
        self.bytes.extend_from_slice(item);
    }

    fn challenge_scalar<F: PrimeField>(&mut self, label: &'static [u8]) -> F {
        if self.use_label {
            self.bytes.extend_from_slice(label);
        }
        let mut hasher = Keccak256::new();
        hasher.update(&self.bytes);
        self.bytes.clear();
        let bytes = hasher.finalize().to_vec();
        let challenge = F::from_be_bytes_mod_order(&bytes);
        self.bytes.extend(challenge.into_bigint().to_bytes_be());
        challenge
    }
}
