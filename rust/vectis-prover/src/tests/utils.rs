use ark_serialize::CanonicalSerialize;
use dotenv::dotenv;
use std::{env, str::FromStr};

pub trait Average<T> {
    fn average(&self) -> T;
}

impl Average<u128> for Vec<u128> {
    fn average(&self) -> u128 {
        let sum: u128 = self.iter().sum();
        if self.is_empty() {
            0
        } else {
            sum / (self.len() as u128)
        }
    }
}

pub trait Transpose<T> {
    fn transpose(&self) -> Vec<T>;
}

impl<T: Clone> Transpose<Vec<T>> for Vec<T> {
    fn transpose(&self) -> Vec<Vec<T>> {
        self.into_iter().map(|x| vec![x.clone()]).collect()
    }
}

pub fn format_time(microseconds: u128) -> String {
    if microseconds >= 1_000_000 {
        let seconds = microseconds as f64 / 1_000_000.0;
        format!("{:.3} s", seconds)
    } else if microseconds >= 1_000 {
        let milliseconds = microseconds as f64 / 1_000.0;
        format!("{:.3} ms", milliseconds)
    } else {
        format!("{} µs", microseconds)
    }
}

pub fn parse_env<T: FromStr>(key: &'static str) -> Result<T, T::Err> {
    dotenv().ok();
    let var = env::var(key).expect(format!("{} not set", key).as_str());
    var.parse()
}

pub fn compressed_key_size<K: CanonicalSerialize>(key: &K) -> usize {
    let mut buffer = vec![];
    key.serialize_compressed(&mut buffer).unwrap();
    buffer.len()
}
