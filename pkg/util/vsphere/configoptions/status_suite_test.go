// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configoptions_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestConfigOptions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config options mapping")
}
