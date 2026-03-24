use std::env;

use ark_bn254::{Bn254, Fr};
use ark_std::rand::{rngs::StdRng, SeedableRng};
use serde_json::{json, Value};
use vectis::{
    gro::{Proof, VerifyingKey},
    hyperl2::{demo_state_transition_batch, prove_batch, setup_sum_preserving_circuit},
    solidity::Solidity,
};

fn parse_arg<T: std::str::FromStr>(flag: &str, default: T) -> T {
    let args = env::args().collect::<Vec<_>>();
    args.windows(2)
        .find_map(|window| {
            if window[0] == flag {
                window[1].parse::<T>().ok()
            } else {
                None
            }
        })
        .unwrap_or(default)
}

fn g1_json<T: Solidity>(point: &T) -> Value {
    let encoded = point.to_solidity();
    json!({
        "x": encoded[0],
        "y": encoded[1],
    })
}

fn g2_json<T: Solidity>(point: &T) -> Value {
    let encoded = point.to_solidity();
    json!({
        "x": [encoded[0].clone(), encoded[1].clone()],
        "y": [encoded[2].clone(), encoded[3].clone()],
    })
}

fn proof_json<E: ark_ec::pairing::Pairing>(proof: &Proof<E>) -> Value
where
    E::G1Affine: Solidity,
    E::G2Affine: Solidity,
{
    json!({
        "a": g1_json(&proof.a),
        "b": g2_json(&proof.b),
        "c": g1_json(&proof.c),
        "d": g1_json(&proof.d),
    })
}

fn vk_json<E: ark_ec::pairing::Pairing>(vk: &VerifyingKey<E>) -> Value
where
    E::G1Affine: Solidity,
    E::G2Affine: Solidity,
{
    assert_eq!(vk.gamma_abc_g1.len(), 2, "expected one public input (tau)");
    json!({
        "alphaG1": g1_json(&vk.alpha_g1),
        "betaG2": g2_json(&vk.beta_g2),
        "gammaG2": g2_json(&vk.gamma_g2),
        "deltaG2": g2_json(&vk.delta_g2),
        "gammaAbcG1": [
            g1_json(&vk.gamma_abc_g1[0]),
            g1_json(&vk.gamma_abc_g1[1]),
        ],
    })
}

fn main() {
    let batch_size = parse_arg("--batch-size", 20_000usize);
    let lane_count = parse_arg("--lane-count", 10usize);
    let batch_id_base = parse_arg("--batch-id-base", 1u64);
    let seed = parse_arg("--seed", 7u64);

    let mut setup_rng = StdRng::seed_from_u64(seed);
    let (pk, vk, ck) = setup_sum_preserving_circuit::<Bn254, _>(batch_size, &mut setup_rng).unwrap();

    let mut artifacts = Vec::with_capacity(lane_count);
    let mut lane_heads = Vec::with_capacity(lane_count);

    for lane in 0..lane_count {
        let lane_id = u16::try_from(lane).expect("lane index does not fit u16");
        let batch_id = batch_id_base + u64::try_from(lane).expect("lane index does not fit u64");
        let (_, _, input) = demo_state_transition_batch::<Fr>(lane_id, batch_id, batch_size).unwrap();
        let mut prove_rng = StdRng::seed_from_u64(seed + 1 + lane as u64);
        let artifact = prove_batch::<Bn254, _>(&pk, &ck, input, &mut prove_rng).unwrap();

        lane_heads.push(json!({
            "laneId": artifact.lane_id,
            "initialHead": g1_json(&artifact.prev_state_commitment),
        }));
        artifacts.push(json!({
            "laneId": artifact.lane_id,
            "batchId": artifact.batch_id,
            "txCount": artifact.tx_count,
            "prevStateCommitment": g1_json(&artifact.prev_state_commitment),
            "nextStateCommitment": g1_json(&artifact.next_state_commitment),
            "proof": proof_json(&artifact.proof),
        }));
    }

    let output = json!({
        "notes": {
            "domain": "HyperL2.SumPreservingBatch.v1",
            "g2_encoding": "x = [c1, c0], y = [c1, c0]",
            "public_inputs": 1,
            "batch_size": batch_size,
            "lane_count": lane_count,
            "verified_tx_total": batch_size * lane_count,
        },
        "verifyingKey": vk_json(&vk),
        "laneHeads": lane_heads,
        "verifyTenBatchesInput": {
            "artifacts": artifacts,
        }
    });

    println!("{}", serde_json::to_string_pretty(&output).unwrap());
}
