//go:build e2e_coverage

// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/vmware-tanzu/vm-operator/test/coverage"
)

// initCoverage runs from main, after Go has initialized its coverage metadata.
// It is compiled only into the separate E2E manager image.
func initCoverage() {
	if _, err := coverage.Start(context.Background(), os.Getenv("GOCOVERDIR")); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "start E2E coverage exporter: %v\n", err)
		os.Exit(1)
	}
}
