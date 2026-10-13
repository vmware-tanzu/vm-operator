// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
)

var _ = DescribeTable("isCAPVServiceAccount",
	Label(
		testlabels.Validation,
		testlabels.Webhook,
	),
	func(username string, expected bool) {
		Expect(isCAPVServiceAccount(username)).To(Equal(expected))
	},
	// Old style: svc-tkg-domain-<cluster-mob-id>
	Entry("old tkg namespace with cluster mob id, default sa",
		"system:serviceaccount:svc-tkg-domain-c52:default", true),
	Entry("old tkg namespace with cluster mob id, capv-manager sa",
		"system:serviceaccount:svc-tkg-domain-c52:capv-manager", true),
	// New style: svc-tkg-<random>
	Entry("new tkg namespace with random hash, default sa",
		"system:serviceaccount:svc-tkg-123ab:default", true),
	Entry("new tkg namespace with random hash, capv-manager sa",
		"system:serviceaccount:svc-tkg-123ab:capv-manager", true),
	// Negative cases
	Entry("vks prefix namespace (not used, should not match)",
		"system:serviceaccount:svc-vks-123ab:capv-manager", false),
	Entry("unrelated supervisor service, default sa",
		"system:serviceaccount:svc-configuration-123ab:default", false),
	Entry("unrelated supervisor service, capv-manager sa",
		"system:serviceaccount:svc-configuration-123ab:capv-manager", false),
	Entry("tkg namespace with wrong sa",
		"system:serviceaccount:svc-tkg-123ab:other-sa", false),
	Entry("default namespace",
		"system:serviceaccount:default:default", false),
	Entry("sso user",
		"sso:admin@vsphere.local", false),
	Entry("empty username",
		"", false),
)
