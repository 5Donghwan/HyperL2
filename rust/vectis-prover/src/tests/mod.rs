mod circuit;
mod linker;
mod utils;
mod vectis;

use lazy_static::lazy_static;
use utils::parse_env;

// Test environment variables
lazy_static! {
    pub static ref STATISTICS: bool = parse_env("STATISTICS").expect("Failed to parse STATISTICS");
    /// Number of repetitions for each test (if statistics are enabled, repeat only once)
    pub static ref NUM_REPEAT: usize = if *STATISTICS {
        parse_env("NUM_REPEAT").expect("Failed to parse NUM_REPEAT")
    } else {
        1
    };
    /// Minimum log of the number batch commitments
    pub static ref LOG_MIN: usize = parse_env("LOG_MIN").expect("Failed to parse LOG_MIN");
    /// Maximum log of the number batch commitments
    pub static ref LOG_MAX: usize = parse_env("LOG_MAX").expect("Failed to parse LOG_MAX");
    /// Maximum log of the number batch commitments
    pub static ref CP_LINK: bool = parse_env("CP_LINK").expect("Failed to parse CP_LINK");
}
