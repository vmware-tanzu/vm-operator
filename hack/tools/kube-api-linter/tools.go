//go:build vmop_tools
// +build vmop_tools

// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package tools manages the version of kube-api-linter used to lint this
// project's API types. It is kept in its own module, separate from
// ../tools.go, so that its dependency graph -- which vendors its own copy of
// golangci-lint -- cannot force an upgrade of the golangci-lint version used
// by the main lint-go target.
package tools

import (
	_ "sigs.k8s.io/kube-api-linter/cmd/golangci-lint-kube-api-linter"
)
