//go:build vmop_tools
// +build vmop_tools

// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package tools manages the version of tooling used to test this module.
package tools

import (
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"
)
