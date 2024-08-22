// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package annotations_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/klog/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func init() {
	klog.SetOutput(GinkgoWriter)
	logf.SetLogger(klog.Background())
}

func TestPtr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Annotations Util Test Suite")
}
