// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

use std::env;

use metrics_rs::MetricsLibrary;

fn main() {
    let args: Vec<String> = env::args().collect();

    if args.len() < 2 {
        panic!("Too few argumetns");
    }

    let metrics_mutex = MetricsLibrary::get().expect("MetricsLibrary::get() returned None.");

    let mut metrics = metrics_mutex
        .lock()
        .expect("Failed to lock the MetricsLibrary object");

    let command = args[1].as_str();
    let result = match command {
        "SendToUMA" => {
            if args.len() != 7 {
                panic!("{}: Argument count is not 7", command);
            }
            metrics.send_to_uma(
                &args[2],
                args[3].parse::<i32>().unwrap(),
                args[4].parse::<i32>().unwrap(),
                args[5].parse::<i32>().unwrap(),
                args[6].parse::<i32>().unwrap(),
            )
        }
        "SendEnumToUMA" => {
            if args.len() != 5 {
                panic!("Argument count is not 5");
            }
            metrics.send_enum_to_uma(
                &args[2],
                args[3].parse::<i32>().unwrap(),
                args[4].parse::<i32>().unwrap(),
            )
        }
        "SendRepeatedEnumToUMA" => {
            if args.len() != 6 {
                panic!("Argument count is not 6");
            }
            metrics.send_repeated_enum_to_uma(
                &args[2],
                args[3].parse::<i32>().unwrap(),
                args[4].parse::<i32>().unwrap(),
                args[5].parse::<i32>().unwrap(),
            )
        }
        "SendLinearToUMA" => {
            if args.len() != 5 {
                panic!("Argument count is not 5");
            }
            metrics.send_linear_to_uma(
                &args[2],
                args[3].parse::<i32>().unwrap(),
                args[4].parse::<i32>().unwrap(),
            )
        }
        "SendPercentageToUMA" => {
            if args.len() != 4 {
                panic!("Argument count is not 4");
            }
            metrics.send_percentage_to_uma(&args[2], args[3].parse::<i32>().unwrap())
        }
        "SendSparseToUMA" => {
            if args.len() != 4 {
                panic!("Argument count is not 4");
            }
            metrics.send_sparse_to_uma(&args[2], args[3].parse::<i32>().unwrap())
        }
        "SendUserActionToUMA" => {
            if args.len() != 3 {
                panic!("Argument count is not 3");
            }
            metrics.send_user_action_to_uma(&args[2])
        }
        "SendCrashToUMA" => {
            if args.len() != 3 {
                panic!("Argument count is not 3");
            }
            metrics.send_crash_to_uma(&args[2])
        }
        "SendCrosEventToUMA" => {
            if args.len() != 3 {
                panic!("Argument count is not 3");
            }
            metrics.send_cros_event_to_uma(&args[2])
        }
        _ => {
            panic!("Unknown command");
        }
    };

    match result {
        Ok(()) => println!("{} succeeded", command),
        Err(e) => {
            panic!("{} failed: {}", command, e);
        }
    }
}
