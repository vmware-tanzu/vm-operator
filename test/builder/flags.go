// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"flag"
	"os"
	"strconv"
	"strings"
)

// testFlags contains the configurations we'd like to get from the command line,
// that could be used to tune tests behavior.
type testFlags struct {
	// rootDir is the root directory of the checked-out project and is set with
	// the -root-dir flag.
	// Defaults to ../../
	RootDir string

	// integrationTestsEnabled is set to true with the -enable-integration-tests flag.
	// Defaults to false.
	IntegrationTestsEnabled bool

	// unitTestsEnabled is set to true with the -enable-unit-tests flag.
	// Defaults to true.
	UnitTestsEnabled bool
}

var (
	flags testFlags
)

func init() {
	flags = testFlags{}
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

	eitFlagDefaultValue, _ := strconv.ParseBool(os.Getenv("ENABLE_INTEGRATION_TESTS"))
	var eutFlagDefaultValue bool
	if v, ok := os.LookupEnv("ENABLE_UNIT_TESTS"); ok {
		eutFlagDefaultValue, _ = strconv.ParseBool(v)
	} else {
		eutFlagDefaultValue = true
	}

	cmdLine.BoolVar(
		&flags.IntegrationTestsEnabled,
		"enable-integration-tests",
		eitFlagDefaultValue,
		"Enables integration tests")
	cmdLine.BoolVar(
		&flags.UnitTestsEnabled,
		"enable-unit-tests",
		eutFlagDefaultValue,
		"Enables unit tests")
	_ = cmdLine.Parse(args)

	// We still need to add the flags to the default flagset, because otherwise
	// Ginkgo will complain that the flags are not recognized.
	flag.Bool(
		"enable-integration-tests",
		eitFlagDefaultValue,
		"Enables integration tests")
	flag.Bool(
		"enable-unit-tests",
		eutFlagDefaultValue,
		"Enables unit tests")

	flag.StringVar(&flags.RootDir, "root-dir", "../..", "Root project directory")
}
