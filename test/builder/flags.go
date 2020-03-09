// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"flag"
	"os"
	"strings"
)

type TestFlags struct {
	// IntegrationTestsEnabled is set to true with the -enable-integration-tests
	// flag.
	// Defaults to false.
	IntegrationTestsEnabled bool

	// UnitTestsEnabled is set to true with the -enable-unit-tests flag.
	// Defaults to true.
	UnitTestsEnabled bool
}

var (
	flags TestFlags
)

func init() {
	flags = TestFlags{}
	// Create a special flagset used to parse the -enable-integration-tests
	// and -enable-unit-tests flags. A special flagset is used so as not to
	// interfere with whatever Ginkgo or Kubernetes might be doing with the
	// default flagset.
	//
	// Please note that in order for this to work, we must copy the os.Args
	// slice into a new slice, removing any flags except those about which
	// we're concerned and possibly values immediately succeeding those flags,
	// provided the values are not prefixed with a "-" character.
	cmdLine := flag.NewFlagSet("test", flag.PanicOnError)
	var args []string
	for i := 0; i < len(os.Args); i++ {
		if strings.HasPrefix(os.Args[i], "-enable-integration-tests") || strings.HasPrefix(os.Args[i], "-enable-unit-tests") {
			args = append(args, os.Args[i])
		}
	}
	cmdLine.BoolVar(&flags.IntegrationTestsEnabled, "enable-integration-tests", false, "Enables integration tests")
	cmdLine.BoolVar(&flags.UnitTestsEnabled, "enable-unit-tests", true, "Enables unit tests")
	_ = cmdLine.Parse(args)

	// We still need to add the flags to the default flagset, because otherwise
	// Ginkgo will complain that the flags are not recognized.
	flag.Bool("enable-integration-tests", false, "Enables integration tests")
	flag.Bool("enable-unit-tests", true, "Enables unit tests")
}

func GetTestFlags() TestFlags {
	return flags
}
