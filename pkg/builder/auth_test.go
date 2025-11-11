// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"

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
		"nil context",
		(*pkgctx.WebhookContext)(nil),
		authv1.UserInfo{},
		false,
	),
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
		"is vm op service account",
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
	Entry(
		"is service account in priv user list sans the -HASH suffix",
		&pkgctx.WebhookContext{
			Context: pkgcfg.WithConfig(pkgcfg.Config{
				PrivilegedUsers: "hello,system:serviceaccount:vmware-system-mobility-operator:vmware-system-mobility-operator-controller-manager,fubar",
			}),
		},
		authv1.UserInfo{
			Username: "system:serviceaccount:vmware-system-mobility-operator:vmware-system-mobility-operator-controller-manager",
		},
		true,
	),
	Entry(
		"is in priv user list with -HASH suffix",
		&pkgctx.WebhookContext{
			Context: pkgcfg.WithConfig(pkgcfg.Config{
				PrivilegedUsers: "hello,system:serviceaccount:svc-configuration-HASH:configuration-service-controller-manager,fubar",
			}),
		},
		authv1.UserInfo{
			Username: "system:serviceaccount:svc-configuration-123ab:configuration-service-controller-manager",
		},
		true,
	),
	Entry(
		"is in priv user list with -HASH suffix but user has extra chars",
		&pkgctx.WebhookContext{
			Context: pkgcfg.WithConfig(pkgcfg.Config{
				PrivilegedUsers: "hello,system:serviceaccount:svc-configuration-HASH:configuration-service-controller-manager,fubar",
			}),
		},
		authv1.UserInfo{
			Username: "system:serviceaccount:svc-configuration-invalid-123ab:configuration-service-controller-manager",
		},
		false,
	),
	Entry(
		"is in priv user list with -HASH suffix but user has old style suffix",
		&pkgctx.WebhookContext{
			Context: pkgcfg.WithConfig(pkgcfg.Config{
				PrivilegedUsers: "hello,system:serviceaccount:svc-configuration-HASH:configuration-service-controller-manager,fubar",
			}),
		},
		authv1.UserInfo{
			Username: "system:serviceaccount:svc-configuration-domain-c36:configuration-service-controller-manager",
		},
		false,
	),
)

var _ = Describe("VerifyWebhookRequest", func() {
	var (
		ctx     context.Context
		certDir string
	)

	BeforeEach(func() {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "")
		Expect(err).NotTo(HaveOccurred())
		certDir = tmpDir

		ctx = pkgcfg.WithConfig(pkgcfg.Config{
			WebhookSecretVolumeMountPath: certDir,
		})

		caFilePath := filepath.Join(certDir, "client-ca", "ca.crt")
		caFileDir := filepath.Dir(caFilePath)
		Expect(os.MkdirAll(caFileDir, 0755)).To(Succeed())
		Expect(os.WriteFile(caFilePath, []byte(sampleCert), 0644)).To(Succeed())
	})

	AfterEach(func() {
		if certDir != "" {
			Expect(os.RemoveAll(certDir)).To(Succeed())
		}
		ctx = nil
	})

	When("request has no context key for peer certs", func() {
		It("should return an error", func() {
			err := builder.VerifyWebhookRequest(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	When("request has invalid peer certs", func() {
		It("should return an error", func() {
			block, _ := pem.Decode([]byte(invalidClientCert))
			clientCert, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			ctx = context.WithValue(ctx, builder.RequestClientCertificateContextKey, &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{clientCert},
			})
			err = builder.VerifyWebhookRequest(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	When("request has valid peer certs", func() {
		It("should not return an error", func() {
			block, _ := pem.Decode([]byte(sampleClientCert))
			clientCert, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			ctx = context.WithValue(ctx, builder.RequestClientCertificateContextKey, &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{clientCert},
			})
			err = builder.VerifyWebhookRequest(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
