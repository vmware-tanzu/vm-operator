// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
)

// https://github.com/kubernetes-sigs/cluster-api/blob/5ac19dc6a5f78f98282f13d5159dcb2d91e4d89f/test/e2e/common.go#L45
func Byf(format string, a ...any) {
	By(fmt.Sprintf(format, a...))
}
