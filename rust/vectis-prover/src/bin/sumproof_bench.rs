use std::{env, process, time::Instant};

use ark_bn254::{Bn254, Fr};
use ark_std::rand::{rngs::StdRng, SeedableRng};
use vectis::hyperl2::{
    demo_state_transition_batch, prove_batch, setup_sum_preserving_circuit, verify_batch,
};

struct Args {
    batch_size: usize,
    repeats: usize,
    seed: u64,
}

fn parse_args() -> Result<Args, String> {
    let mut batch_size = 1024usize;
    let mut repeats = 1usize;
    let mut seed = 7u64;

    let mut it = env::args().skip(1);
    while let Some(arg) = it.next() {
        match arg.as_str() {
            "--batch-size" => {
                let value = it.next().ok_or("missing value for --batch-size")?;
                batch_size = value.parse().map_err(|_| "invalid --batch-size")?;
            }
            "--repeats" => {
                let value = it.next().ok_or("missing value for --repeats")?;
                repeats = value.parse().map_err(|_| "invalid --repeats")?;
            }
            "--seed" => {
                let value = it.next().ok_or("missing value for --seed")?;
                seed = value.parse().map_err(|_| "invalid --seed")?;
            }
            "--help" | "-h" => {
                return Err(String::new());
            }
            _ => return Err(format!("unknown argument: {arg}")),
        }
    }

    if batch_size == 0 {
        return Err("batch size must be greater than zero".to_string());
    }
    if repeats == 0 {
        return Err("repeats must be greater than zero".to_string());
    }

    Ok(Args {
        batch_size,
        repeats,
        seed,
    })
}

fn usage() {
    eprintln!(
        "usage: cargo run --manifest-path rust/vectis-prover/Cargo.toml --bin sumproof_bench --release -- \\
  --batch-size <N> --repeats <R> [--seed <S>]"
    );
}

fn main() {
    let args = match parse_args() {
        Ok(args) => args,
        Err(message) if message.is_empty() => {
            usage();
            return;
        }
        Err(message) => {
            usage();
            eprintln!("error: {message}");
            process::exit(2);
        }
    };

    let mut rng = StdRng::seed_from_u64(args.seed);
    let setup_started = Instant::now();
    let (pk, vk, ck) = match setup_sum_preserving_circuit::<Bn254, _>(args.batch_size, &mut rng) {
        Ok(value) => value,
        Err(err) => {
            eprintln!("setup failed: {err}");
            process::exit(1);
        }
    };
    let setup_elapsed = setup_started.elapsed();

    let mut total_prove = 0.0f64;
    let mut total_verify = 0.0f64;

    for batch_id in 0..args.repeats {
        let (_, _, input) = match demo_state_transition_batch::<Fr>(0, batch_id as u64, args.batch_size) {
            Ok(value) => value,
            Err(err) => {
                eprintln!("state transition build failed: {err}");
                process::exit(1);
            }
        };

        let prove_started = Instant::now();
        let artifact = match prove_batch::<Bn254, _>(&pk, &ck, input, &mut rng) {
            Ok(value) => value,
            Err(err) => {
                eprintln!("prove failed: {err}");
                process::exit(1);
            }
        };
        total_prove += prove_started.elapsed().as_secs_f64();

        let verify_started = Instant::now();
        let verified = match verify_batch(&vk, &artifact) {
            Ok(value) => value,
            Err(err) => {
                eprintln!("verify failed: {err}");
                process::exit(1);
            }
        };
        total_verify += verify_started.elapsed().as_secs_f64();

        if !verified {
            eprintln!("verification returned false for batch_id={batch_id}");
            process::exit(1);
        }
    }

    let avg_prove = total_prove / args.repeats as f64;
    let avg_verify = total_verify / args.repeats as f64;
    let proofs_for_200k = 200_000f64 / args.batch_size as f64;

    println!("curve=bn254");
    println!("batch_size={}", args.batch_size);
    println!("repeats={}", args.repeats);
    println!("setup_seconds={:.6}", setup_elapsed.as_secs_f64());
    println!("avg_prove_seconds={:.6}", avg_prove);
    println!("avg_verify_seconds={:.6}", avg_verify);
    println!("serial_seconds_for_200k={:.6}", avg_prove * proofs_for_200k);
    println!("serial_verify_seconds_for_10_proofs={:.6}", avg_verify * 10f64);
}
