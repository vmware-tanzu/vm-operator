//go:build !e2e_coverage

// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

// initCoverage is a no-op in production builds.
func initCoverage() {}
