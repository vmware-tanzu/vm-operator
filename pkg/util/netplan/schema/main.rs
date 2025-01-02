// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

fn main() {
    let schema = schemars::schema_for!(netplan_types::NetplanConfig);
    println!("{}", serde_json::to_string_pretty(&schema).unwrap());
}