// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"flag"
)

// testFlags contains the configurations we'd like to get from the command line,
// that could be used to tune tests behavior.
type testFlags struct {
	// rootDir is the root directory of the checked-out project and is set with
	// the -root-dir flag.
	// Defaults to ../../
	RootDir string
}

var (
	flags testFlags
)

func init() {
	flags = testFlags{}

	// We still need to add the flags to the default flagset, because otherwise
	// Ginkgo will complain that the flags are not recognized.
	flag.StringVar(&flags.RootDir, "root-dir", "../..", "Root project directory")
}
