//go:build e2e_coverage

// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/vmware-tanzu/vm-operator/test/coverage"
)

// initCoverage runs from main, after Go has initialized its coverage metadata.
// It is compiled only into the separate E2E manager image.
func initCoverage() {
	port := coverage.DefaultPort
	if value := os.Getenv("E2E_COVERAGE_PORT"); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 16)
		if err != nil || parsed == 0 {
			_, _ = fmt.Fprintf(os.Stderr, "E2E_COVERAGE_PORT must be between 1 and 65535: %q\n", value)
			os.Exit(1)
		}
		port = uint16(parsed)
	}
	server, err := coverage.Start(context.Background(), port)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "start E2E coverage server: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stderr, "E2E coverage server listening on http://%s/snapshot\n", server.Addr)
}
