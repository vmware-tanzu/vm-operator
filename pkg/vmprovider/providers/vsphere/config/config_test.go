// +build !integration

// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi/simulator"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	. "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/credentials"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func newConfig(namespace string, vcPNID string, vcPort string, vcCredsSecretName string) (*corev1.ConfigMap, *corev1.Secret, *VSphereVMProviderConfig) {
	providerConfig := &VSphereVMProviderConfig{
		VcPNID: vcPNID,
		VcPort: vcPort,
		VcCreds: &credentials.VSphereVMProviderCredentials{
			Username: "some-user",
			Password: "some-pass",
		},
		Datacenter:            simulator.Map.Any("Datacenter").Reference().Value,
		ResourcePool:          simulator.Map.Any("ResourcePool").Reference().Value,
		Folder:                simulator.Map.Any("Folder").Reference().Value,
		Datastore:             "/DC0/datastore/LocalDS_0",
		InsecureSkipTLSVerify: false,
		CAFilePath:            "/etc/pki/tls/certs/ca-bundle.crt",
	}

	configMap := ProviderConfigToConfigMap(namespace, providerConfig, vcCredsSecretName)
	secret := credentials.ProviderCredentialsToSecret(namespace, providerConfig.VcCreds, vcCredsSecretName)
	return configMap, secret, providerConfig
}

var _ = Describe("GetProviderConfig", func() {

	var (
		configMapIn        *corev1.ConfigMap
		secretIn           *corev1.Secret
		providerConfigIn   *VSphereVMProviderConfig
		savedVmopNamespace string
	)

	BeforeEach(func() {
		configMapIn, secretIn, providerConfigIn = newConfig("config-namespace-1", "pnid-1", "port-1", "secret-name-1")

		savedVmopNamespace = os.Getenv(lib.VmopNamespaceEnv)
		Expect(os.Setenv(lib.VmopNamespaceEnv, configMapIn.Namespace)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Setenv(lib.VmopNamespaceEnv, savedVmopNamespace))
	})

	Context("when a base config exists", func() {

		Context("when a secret doesn't exist", func() {
			Specify("returns no provider config and an error", func() {
				client := builder.NewFakeClient(configMapIn)

				providerConfig, err := GetProviderConfig(ctx, client)
				Expect(err).NotTo(BeNil())
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("when a secret exists", func() {
			Specify("returns a good provider config", func() {
				client := builder.NewFakeClient(configMapIn, secretIn)
				providerConfig, err := GetProviderConfig(ctx, client)
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(providerConfigIn))
			})
		})
	})
})

var _ = Describe("GetProviderConfigForNamespace", func() {

	var (
		configMapIn        *corev1.ConfigMap
		secretIn           *corev1.Secret
		providerConfigIn   *VSphereVMProviderConfig
		savedVmopNamespace string
	)

	BeforeEach(func() {
		configMapIn, secretIn, providerConfigIn = newConfig("config-namespace-2", "pnid-2", "port-2", "secret-name-2")

		delete(configMapIn.Data, "Cluster")
		delete(configMapIn.Data, "ResourcePool")
		delete(configMapIn.Data, "Folder")

		savedVmopNamespace = os.Getenv(lib.VmopNamespaceEnv)
		Expect(os.Setenv(lib.VmopNamespaceEnv, configMapIn.Namespace)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Setenv(lib.VmopNamespaceEnv, savedVmopNamespace))
	})

	// GetNamespaceRPAndFolder() handles IsFaultDomainsFSSEnabled which will be false for these tests.
	// so set the Namespace annotations.

	Context("when a good provider config exists and zone/namespace info exists", func() {
		Specify("provider config has RP and VM folder from zone/namespace", func() {
			namespaceRP := "namespace-test-RP"
			namespaceVMFolder := "namespace-test-vmfolder"

			providerConfigIn.ResourcePool = namespaceRP
			providerConfigIn.Folder = namespaceVMFolder

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "namespace",
					Annotations: map[string]string{
						topology.NamespaceRPAnnotationKey:     namespaceRP,
						topology.NamespaceFolderAnnotationKey: namespaceVMFolder,
					},
				},
			}

			client := builder.NewFakeClient(configMapIn, secretIn, ns)
			providerConfig, err := GetProviderConfigForNamespace(ctx, client, "", ns.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(providerConfig).To(Equal(providerConfigIn))
		})
	})

	Context("when a good provider config exists and zone/namespace info does not exist", func() {
		Specify("should return an error", func() {
			client := builder.NewFakeClient()

			providerConfig, err := GetProviderConfigForNamespace(ctx, client, "", "invalid-namespace")
			Expect(err).To(HaveOccurred())
			Expect(providerConfig).To(BeNil())
		})
	})

	Context("namespace does not exist", func() {
		Specify("returns error", func() {
			client := builder.NewFakeClient(configMapIn, secretIn)
			providerConfig, err := GetProviderConfigForNamespace(ctx, client, "", "invalid-namespace")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("availability zone default missing info for namespace invalid-namespace"))
			Expect(providerConfig).To(BeNil())
		})
	})
})

var _ = Describe("UpdateVcInConfigMap", func() {

	var (
		configMapIn        *corev1.ConfigMap
		secretIn           *corev1.Secret
		providerConfigIn   *VSphereVMProviderConfig
		savedVmopNamespace string
	)

	BeforeEach(func() {
		configMapIn, secretIn, providerConfigIn = newConfig("config-namespace-3", "pnid-3", "port-3", "secret-name-3")

		savedVmopNamespace = os.Getenv(lib.VmopNamespaceEnv)
		Expect(os.Setenv(lib.VmopNamespaceEnv, configMapIn.Namespace)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Setenv(lib.VmopNamespaceEnv, savedVmopNamespace))
	})

	Context("UpdateVcInConfigMap", func() {
		DescribeTable("Update VC PNID and VC Port",
			func(newPnid string, newPort string) {
				expectUpdate := newPnid != "" || newPort != ""

				if newPnid == "" {
					newPnid = providerConfigIn.VcPNID
				}
				if newPort == "" {
					newPort = providerConfigIn.VcPort
				}

				client := builder.NewFakeClient(configMapIn, secretIn)

				updated, err := UpdateVcInConfigMap(ctx, client, newPnid, newPort)
				Expect(err).ToNot(HaveOccurred())
				Expect(updated).To(Equal(expectUpdate))

				providerConfig, err := GetProviderConfig(ctx, client)
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

var _ = Describe("ConfigMapToProviderConfig", func() {

	var (
		configMapIn *corev1.ConfigMap
		vcCreds     *credentials.VSphereVMProviderCredentials
	)

	BeforeEach(func() {
		Expect(os.Unsetenv(lib.VmopNamespaceEnv)).To(Succeed())
		configMapIn, _, _ = newConfig("namespace", "pnid", "port", "secret-name")
		vcCreds = &credentials.VSphereVMProviderCredentials{Username: "some-user", Password: "some-pass"}
	})

	It("verifies that a config is correctly extracted from the configMap", func() {
		providerConfig, err := ConfigMapToProviderConfig(configMapIn, vcCreds)
		Expect(err).To(BeNil())
		Expect(providerConfig.VcPNID).To(Equal(configMapIn.Data["VcPNID"]))
		Expect(providerConfig.VcPort).To(Equal(configMapIn.Data["VcPort"]))
	})

	Context("when vcPNID is unset in configMap", func() {
		Specify("return an error", func() {
			delete(configMapIn.Data, "VcPNID")
			providerConfig, err := ConfigMapToProviderConfig(configMapIn, vcCreds)
			Expect(err).NotTo(BeNil())
			Expect(providerConfig).To(BeNil())
		})
	})

	Context("StorageClassRequired", func() {
		Specify("StorageClassRequired is unset in configMap", func() {
			providerConfig, err := ConfigMapToProviderConfig(configMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.StorageClassRequired).To(BeFalse())
		})

		DescribeTable("StorageClass from configMap is set in provider config",
			func(expected bool) {
				configMapIn.Data["StorageClassRequired"] = strconv.FormatBool(expected)
				providerConfig, err := ConfigMapToProviderConfig(configMapIn, vcCreds)
				Expect(err).To(BeNil())
				Expect(providerConfig.StorageClassRequired).To(Equal(expected))
			},
			Entry("StorageClass set to false", false),
			Entry("StorageClass set to true", true),
		)
	})

	Describe("Tests for TLS configuration", func() {
		var (
			providerConfig   *VSphereVMProviderConfig
			expectErrToOccur bool
			err              error
		)
		JustBeforeEach(func() {
			// We're not testing anything related to per-namespace ConfigMaps, just make them identical
			// to the base ConfigMap.
			providerConfig, err = ConfigMapToProviderConfig(configMapIn, vcCreds)
			if expectErrToOccur {
				Expect(err).To(HaveOccurred())
				Expect(providerConfig).To(BeNil())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})
		// In most tests, we don't expect errors to occur - make that the default case.
		AfterEach(func() {
			expectErrToOccur = false
		})
		Context("when no TLS configuration is specified", func() {
			It("defaults to using TLS with the system root CA", func() {
				Expect(providerConfig.InsecureSkipTLSVerify).To(BeFalse())
				Expect(providerConfig.CAFilePath).To(Equal("/etc/pki/tls/certs/ca-bundle.crt"))
			})
		})
		Context("when the config chooses to ignore TLS verification", func() {
			BeforeEach(func() {
				configMapIn.Data["InsecureSkipTLSVerify"] = "true"
			})
			It("sets the insecure flag in the provider config", func() {
				Expect(providerConfig.InsecureSkipTLSVerify).To(BeTrue())
			})
		})
		Context("when the config chooses to use TLS verification and overrides the CA file path", func() {
			BeforeEach(func() {
				configMapIn.Data["CAFilePath"] = "/etc/a/new/ca/bundle.crt"
				configMapIn.Data["InsecureSkipTLSVerify"] = "false"
			})
			It("unsets the insecure flag in the provider config", func() {
				Expect(providerConfig.InsecureSkipTLSVerify).To(BeFalse())
			})
			It("uses the new CA path", func() {
				Expect(providerConfig.CAFilePath).To(Equal("/etc/a/new/ca/bundle.crt"))
			})
		})
		Context("when the TLS settings in the Config do not parse", func() {
			BeforeEach(func() {
				expectErrToOccur = true
				configMapIn.Data["InsecureSkipTLSVerify"] = "Not_a_boolean"
			})
			It("returns an error when parsing the ConfigMap", func() {})
		})
	})
})
