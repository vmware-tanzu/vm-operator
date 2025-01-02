// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	authv1 "k8s.io/api/authentication/v1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
)

var _ = DescribeTable("IsPrivilegedAccount",
	func(ctx *pkgctx.WebhookContext, userInfo authv1.UserInfo, expected bool) {
		Ω(builder.IsPrivilegedAccount(ctx, userInfo)).To(Equal(expected))
	},
	Entry(
		"empty inputs",
		&pkgctx.WebhookContext{Context: pkgcfg.NewContext()},
		authv1.UserInfo{},
		false,
	),
	Entry(
		"belongs to system:masters group",
		&pkgctx.WebhookContext{},
		authv1.UserInfo{
			Groups: []string{"system:masters"},
		},
		true,
	),
	Entry(
		"is kubernetes-admin user",
		&pkgctx.WebhookContext{},
		authv1.UserInfo{
			Username: "kubernetes-admin",
		},
		true,
	),
	Entry(
		"is in priv user list",
		&pkgctx.WebhookContext{
			Context: pkgcfg.WithConfig(pkgcfg.Config{
				PrivilegedUsers: "hello,world,fubar",
			}),
		},
		authv1.UserInfo{
			Username: "world",
		},
		true,
	),
	Entry(
		"is service account",
		&pkgctx.WebhookContext{
			Context:            pkgcfg.NewContext(),
			Namespace:          "my-ns",
			ServiceAccountName: "my-svc-acct",
		},
		authv1.UserInfo{
			Username: "system:serviceaccount:my-ns:my-svc-acct",
		},
		true,
	),
)
