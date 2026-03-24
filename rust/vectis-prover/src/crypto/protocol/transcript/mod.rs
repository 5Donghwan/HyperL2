use ark_ff::PrimeField;

pub mod sha3;

pub trait TranscriptProtocol: Clone {
    /// Create a new transcript with the given `use_label` flag.
    fn new(use_label: bool) -> Self;

    /// Append an `item` with the given `label`.
    fn append(&mut self, label: &'static [u8], item: &[u8]);

    /// Compute a `label`ed challenge variable.
    fn challenge_scalar<F: PrimeField>(&mut self, label: &'static [u8]) -> F;
}
