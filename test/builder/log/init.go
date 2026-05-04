// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"github.com/onsi/ginkgo/v2"

	"k8s.io/klog/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// init setups the logging so the output is shown during the tests. Note
// that test/builder imports this package so generally only need to import
// this directly for more standalone tests.
func init() {
	klog.InitFlags(nil)
	klog.SetOutput(ginkgo.GinkgoWriter)
	logf.SetLogger(klog.Background())

	// TODO(akutz) This is kind of handy, but ultimately it makes discerning
	//             test-specific logs from regular log non-trivial.
	// GinkgoLogr = klog.Background()
}
