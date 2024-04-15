// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/credentials"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func configTests() {

	var (
		ctx        *builder.TestContextForVCSim
		testConfig builder.VCSimTestConfig
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Describe("GetProviderConfig", func() {

		Context("GetProviderConfig", func() {

			Context("when a secret doesn't exist", func() {
				It("returns no provider config and an error", func() {
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "vmop-vcsim-dummy-creds",
							Namespace: ctx.PodNamespace,
						},
					}
					Expect(ctx.Client.Delete(ctx, secret)).To(Succeed())

					providerConfig, err := config.GetProviderConfig(ctx, ctx.Client)
					Expect(err).To(HaveOccurred())
					Expect(providerConfig).To(BeNil())
				})
			})

			Context("when a secret exists", func() {
				It("returns a good provider config", func() {
					_, err := config.GetProviderConfig(ctx, ctx.Client)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})

	Describe("UpdateVcInConfigMap", func() {

		Context("UpdateVcInConfigMap", func() {
			DescribeTable("Update VC PNID and VC Port",
				func(newPnid string, newPort string) {
					expectedUpdated := newPnid != "" || newPort != ""

					providerConfig, err := config.GetProviderConfig(ctx, ctx.Client)
					Expect(err).ToNot(HaveOccurred())
					if newPnid == "" {
						newPnid = providerConfig.VcPNID
					}
					if newPort == "" {
						newPort = providerConfig.VcPort
					}

					updated, err := config.UpdateVcInConfigMap(ctx, ctx.Client, newPnid, newPort)
					Expect(err).ToNot(HaveOccurred())
					Expect(updated).To(Equal(expectedUpdated))

					providerConfig, err = config.GetProviderConfig(ctx, ctx.Client)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.VcPNID).To(Equal(newPnid))
					Expect(providerConfig.VcPort).To(Equal(newPort))
				},
				Entry("only VC PNID is updated", "some-pnid", nil),
				Entry("only VC Port is updated", nil, "some-port"),
				Entry("both VC PNID and Port are updated", "some-pnid", "some-port"),
				Entry("neither VC PNID and Port are updated", nil, nil),
			)
		})
	})
}

var _ = Describe("ConfigMapToProviderConfig", func() {

	var (
		providerCreds    *credentials.VSphereVMProviderCredentials
		providerConfigIn *config.VSphereVMProviderConfig
		configMap        *corev1.ConfigMap
	)

	BeforeEach(func() {
		providerCreds = &credentials.VSphereVMProviderCredentials{Username: "username", Password: "password"}
		providerConfigIn = &config.VSphereVMProviderConfig{
			VcPNID:                      "my-vc.vmware.com",
			VcPort:                      "433",
			VcCreds:                     providerCreds,
			Datacenter:                  "datacenter-42",
			StorageClassRequired:        false,
			UseInventoryAsContentSource: false,
			CAFilePath:                  "/etc/pki/tls/certs/ca-bundle.crt",
			InsecureSkipTLSVerify:       false,
			ResourcePool:                "resourcepool-42",
			Folder:                      "folder-42",
			Datastore:                   "/DC0/datastore/LocalDS_0",
		}
	})

	JustBeforeEach(func() {
		configMap = config.ProviderConfigToConfigMap("dummy-ns", providerConfigIn, "dummy-secrets")
	})

	It("provider config is correctly extracted from the ConfigMap", func() {
		providerConfig, err := config.ConfigMapToProviderConfig(configMap, providerCreds)
		Expect(err).ToNot(HaveOccurred())
		Expect(providerConfig.VcPNID).To(Equal(configMap.Data["VcPNID"]))
		Expect(providerConfig.VcPort).To(Equal(configMap.Data["VcPort"]))
	})

	Context("when VcPNID is unset in configMap", func() {
		It("return an error", func() {
			delete(configMap.Data, "VcPNID")
			providerConfig, err := config.ConfigMapToProviderConfig(configMap, providerCreds)
			Expect(err).To(HaveOccurred())
			Expect(providerConfig).To(BeNil())
		})
	})

	Context("StorageClassRequired", func() {
		It("StorageClassRequired is unset in configMap", func() {
			providerConfig, err := config.ConfigMapToProviderConfig(configMap, providerCreds)
			Expect(err).ToNot(HaveOccurred())
			Expect(providerConfig.StorageClassRequired).To(BeFalse())
		})

		Context("StorageClassRequired is set in configMap", func() {
			BeforeEach(func() {
				providerConfigIn.StorageClassRequired = true
			})

			It("StorageClassRequired is true in config", func() {
				providerConfig, err := config.ConfigMapToProviderConfig(configMap, providerCreds)
				Expect(err).ToNot(HaveOccurred())
				Expect(providerConfig.StorageClassRequired).To(BeTrue())
			})
		})
	})

	Describe("Tests for TLS configuration", func() {

		Context("when no TLS configuration is specified", func() {
			It("defaults to using TLS with the system root CA", func() {
				providerConfig, err := config.ConfigMapToProviderConfig(configMap, providerCreds)
				Expect(err).NotTo(HaveOccurred())

				Expect(providerConfig.InsecureSkipTLSVerify).To(BeFalse())
				Expect(providerConfig.CAFilePath).To(Equal("/etc/pki/tls/certs/ca-bundle.crt"))
			})
		})

		Context("when the config chooses to ignore TLS verification", func() {
			BeforeEach(func() {
				providerConfigIn.InsecureSkipTLSVerify = true
			})

			It("sets the insecure flag in the provider config", func() {
				providerConfig, err := config.ConfigMapToProviderConfig(configMap, providerCreds)
				Expect(err).NotTo(HaveOccurred())
				Expect(providerConfig.InsecureSkipTLSVerify).To(BeTrue())
			})
		})

		Context("when the TLS settings in the Config do not pars", func() {
			It("returns an error when parsing the ConfigMa", func() {
				configMap.Data["InsecureSkipTLSVerify"] = "bogus"
				_, err := config.ConfigMapToProviderConfig(configMap, providerCreds)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the config chooses to use TLS verification and overrides the CA file path", func() {
			BeforeEach(func() {
				providerConfigIn.CAFilePath = "/etc/a/new/ca/bundle.crt"
			})

			It("uses the new CA path", func() {
				providerConfig, err := config.ConfigMapToProviderConfig(configMap, providerCreds)
				Expect(err).ToNot(HaveOccurred())
				Expect(providerConfig.CAFilePath).To(Equal("/etc/a/new/ca/bundle.crt"))
			})
		})
	})
})
