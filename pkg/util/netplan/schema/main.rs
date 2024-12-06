// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

fn main() {
    let schema = schemars::schema_for!(netplan_types::NetplanConfig);
    println!("{}", serde_json::to_string_pretty(&schema).unwrap());
}