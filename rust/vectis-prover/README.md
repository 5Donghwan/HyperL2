# VECTIS

- **TODO**: Refactor batch ccSNARK into VECTIS

This is implementation of the VECTIS protocol

## HyperL2 Extension

This vendored copy also includes a HyperL2-specific module at `src/hyperl2/`:

- `SumPreservingBatchCircuit`: proves that two Pedersen vector commitments `D1` and `D2` open to pre/post state vectors with equal sums
- `prove_batch` / `verify_batch`: batch artifact API for `20k tx -> 1 proof`
- `state`: fixed slot-map and `raw transfer -> pre/post state` builder
- `src/bin/sumproof_bench.rs`: simple BN254 benchmark entrypoint

See [docs/hyperl2-sumproof.md](docs/hyperl2-sumproof.md) for the exact witness layout and L1 verification model.

## Directory Structure

| Directory            | Description                                                    |
| -------------------- | -------------------------------------------------------------- |
| `src/`               | Contains the source code for the batch ccSNARK protocol        |
| ┣`crypto/`           | Contains the cryptographic primitives used in the protocol     |
| ┃┣`commitmemt/`      | Batch commitment scheme                                        |
| ┃┃┣`pedersen/`       | Pedersen commitment scheme                                     |
| ┃┃┃┣`constraints.rs` | Gadget for the Pedersen commitment                             |
| ┃┃┃┗`mod.rs`         | Implementation of the Pedersen commitment scheme               |
| ┃┃┣`constraints.rs`  | Trait of the batch commitment gadget                           |
| ┃┃┗`mod.rs`          | Trait of the batch commitment scheme                           |
| ┣`gro/`              | Implementation of the ccGrooth16 (LegoSNARK with Batch Commit) |
| ┣`solidity/`         | Implementation of useful utils to format data                  |

## ccGro16 with Public Inputs

$$
A \cdot B = \alpha \cdot \beta + C \cdot \delta + (PI + D) \cdot \gamma
$$

- All public inputs must be challenges

## Batch Commitment Scheme

The aggregation check ensures that each commitment was made correctly:

$$
A = T \cdot M
$$

- $b$: denotes the total numbers of batches
- $n = \sum_{i}{n_i}$ where $n_i$ denotes the length of messages and openings of the $i$-th batch
- $m = \sum_{i}{m_i}$ where $m_i$ denotes the size of the $i$-th batch
- $M_i$: $n_i \times m_i$ matrix (each row denotes the composition of a commitment)
```math
\begin{equation} M = \text{diag}(M_i) =
\begin{bmatrix}
  M_1 & 0 & \cdots & 0 \\
  0 & M_2 & \cdots & 0 \\
  \vdots & \vdots & \ddots & \vdots \\
  0 & 0 & \cdots & M_b
\end{bmatrix} \end{equation}_{n \times m}
```
- $A$: denotes the aggregation vector
- Where $\tau$ is a challenge which received from the verifier:
```math
T = \begin{bmatrix} \tau & \tau^2 & \cdots & \tau^n \end{bmatrix}
```
- Committing key is a vector of $m$ group elements:
```math
\mathsf{ck} = \begin{bmatrix} g_1 & g_2 & \cdots & g_m \end{bmatrix}
```

The product $\mathsf{ck} \cdot M^{\top}$ produces a vector $CM$, which consists of $n$ commitments. The verifier can check $A$ by verifying $\mathsf{ck} \cdot A^{\top} = CM \cdot T^{\top}$. Additionally, if $M_i$ does not need to be hidden, it can act as public inputs.

If, there is no reason to separate the committing key, the matrix $M$ can be compressed as follows (where $j$ is the index where  $m_i$  is maximized):

$$
M = \begin{bmatrix}
  M_1 & 0 \\
  M_2 & 0 \\
  \vdots & \vdots \\
  M_j \\
  \vdots & \vdots \\
  M_b & 0
\end{bmatrix}_{n \times \max(m_i)}
$$

In this context, the zeros ($0$) are matrices of appropriate dimensions to match the size differences.

### Steps

**Circuit**

- All the aggregated values must be at the front of the committed witness

**Prover**

1. Use `CCGroth16::<E>::commit` to commit the proof-dependent commitment
2. Use `Pedersen::<C>::batch_commit` to calculate the commitments
3. Use `Pedersen::<C>::challenge` to retrieve the challenge for aggregation.

**Verifier**

1. Use `Pedersen::<C>::challenge` to retrieve the challenge for aggregation.
2. Use `Pedersen::<C>::aggregate` to aggregate the commitments.
3. Update the proof-dependent commitment by adding the aggregation of the commitments.

```rust
// Aggregate inputs
let transposed = public_inputs.transpose();
let slices = cfg_iter!(transposed).map(|x| &x[..]).collect::<Vec<_>>();
let (aggregation_fr, initial) = Pedersen::<E::G1>::scalar_aggregate(&slices, tau, None);
// Aggregate commitments
let (aggregation_g1, _) = Pedersen::<C>::aggregate(&commitments, tau, Some(initial));
// Update proof dependent commitment
let aggregation = aggregation_g1 + vk.ck.batch_g1[0].into_group() * aggregation_fr[0];
proof.d = (proof.d.into_group() + aggregation).into_affine();
```
