// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package spq_test

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

func TestKube(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StoragePolicyQuota Util Test Suite")
}

const (
	defaultNamespace = "default"
	myStorageClass   = "my-storage-class"
	fakeString       = "fake"
	sc1PolicyID      = "my-storage-policy-id-1"
	sc2PolicyID      = "my-storage-policy-id-2"
)
