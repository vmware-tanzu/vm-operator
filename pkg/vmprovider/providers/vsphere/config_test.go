// +build !integration

// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"os"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/simulator"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	. "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

func newConfig(namespace string, vcPNID string, vcPort string, vcCredsSecretName string) (*v1.ConfigMap, *v1.Secret, *VSphereVmProviderConfig) {

	providerConfig := &VSphereVmProviderConfig{
		VcPNID: vcPNID,
		VcPort: vcPort,
		VcCreds: &VSphereVmProviderCredentials{
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
	secret := ProviderCredentialsToSecret(namespace, providerConfig.VcCreds, vcCredsSecretName)
	return configMap, secret, providerConfig
}

var _ = Describe("UpdateVMFolderAndResourcePool", func() {

	Context("when a good provider config exists and namespace has non-empty annotations", func() {
		Specify("provider config is updated with RP and VM folder from annotations", func() {
			namespaceRP := "namespace-test-RP"
			namespaceVMFolder := "namespace-test-vmfolder"
			annotations := make(map[string]string)
			annotations[NamespaceRPAnnotationKey] = namespaceRP
			annotations[NamespaceFolderAnnotationKey] = namespaceVMFolder
			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace", Annotations: annotations}}
			client := clientfake.NewFakeClient(ns)

			providerConfig := &VSphereVmProviderConfig{}
			Expect(UpdateProviderConfigFromNamespace(client, ns.Name, providerConfig)).To(Succeed())
			Expect(providerConfig.ResourcePool).To(Equal(namespaceRP))
			Expect(providerConfig.Folder).To(Equal(namespaceVMFolder))
		})
	})

	Context("when a good provider config exists and namespace does not have annotations", func() {
		Specify("should succeed with providerconfig unmodified", func() {
			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
			client := clientfake.NewFakeClient(ns)

			providerConfig := &VSphereVmProviderConfig{}
			providerConfigRP := "namespace-test-RP"
			providerConfigFolder := "namespace-test-vmfolder"
			providerConfig.ResourcePool = providerConfigRP
			providerConfig.Folder = providerConfigFolder
			Expect(UpdateProviderConfigFromNamespace(client, ns.Name, providerConfig)).To(Succeed())
			Expect(providerConfig.ResourcePool).To(Equal(providerConfigRP))
			Expect(providerConfig.Folder).To(Equal(providerConfigFolder))
		})
	})

	Context("namespace does not exist", func() {
		Specify("returns error", func() {
			client := clientfake.NewFakeClient()
			providerConfig := &VSphereVmProviderConfig{}
			err := UpdateProviderConfigFromNamespace(client, "test-namespace", providerConfig)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal("could not get the namespace: test-namespace: namespaces \"test-namespace\" not found"))
		})
	})

	Context("ResourcePool and Folder not present in namespace annotations", func() {
		It("should not modify config", func() {
			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
			client := clientfake.NewFakeClient(ns)
			providerConfig := VSphereVmProviderConfig{}
			providerConfig.ResourcePool = "foo"
			providerConfig.Folder = "bar"
			providerConfigIn := providerConfig
			err := UpdateProviderConfigFromNamespace(client, "namespace", &providerConfigIn)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(providerConfig).To(Equal(providerConfigIn))
		})
	})
})

var _ = Describe("GetProviderConfigFromConfigMap", func() {

	var (
		configMapIn      *v1.ConfigMap
		secretIn         *v1.Secret
		providerConfigIn *VSphereVmProviderConfig
	)

	BeforeEach(func() {
		Expect(os.Unsetenv(lib.VmopNamespaceEnv)).To(Succeed())
		configMapIn, secretIn, providerConfigIn = newConfig("namespace", "pnid", "port", "secret-name")
	})

	Context("when a base config exists", func() {

		BeforeEach(func() {
			Expect(os.Setenv(lib.VmopNamespaceEnv, "namespace")).To(Succeed())
		})

		Context("when a secret doesn't exist", func() {
			Specify("returns no provider config and an error", func() {
				ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
				client := clientfake.NewFakeClient(configMapIn, ns)

				providerConfig, err := GetProviderConfigFromConfigMap(client, "namespace")
				Expect(err).NotTo(BeNil())
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("ResourcePool and Folder not present in either providerConfig or namespace annotations", func() {
			Specify("should return an error", func() {
				delete(configMapIn.Data, "ResourcePool")
				delete(configMapIn.Data, "Folder")
				ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
				client := clientfake.NewFakeClient(configMapIn, secretIn, ns)

				providerConfig, err := GetProviderConfigFromConfigMap(client, "namespace")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("missing ResourcePool and Folder in ProviderConfig. ResourcePool: , Folder: "))
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("when a secret exists", func() {
			Specify("returns a good provider config", func() {
				ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
				client := clientfake.NewFakeClient(configMapIn, secretIn, ns)

				providerConfig, err := GetProviderConfigFromConfigMap(client, "namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(providerConfigIn))
			})
			Specify("returns a good provider config for no namespace", func() {
				client := clientfake.NewFakeClient(configMapIn, secretIn)
				providerConfig, err := GetProviderConfigFromConfigMap(client, "")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(providerConfigIn))
			})

			DescribeTable("Update VC PNID and VC Port",
				func(newPnid string, newPort string) {
					if newPnid != "" {
						newPnid = providerConfigIn.VcPNID
					}

					if newPort != "" {
						newPort = providerConfigIn.VcPort
					}

					ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
					client := clientfake.NewFakeClient(configMapIn, secretIn, ns)

					Expect(PatchVcURLInConfigMap(client, newPnid, newPort)).To(Succeed())
					providerConfig, err := GetProviderConfigFromConfigMap(client, "")
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.VcPNID).Should(Equal(newPnid))
					Expect(providerConfig.VcPort).Should(Equal(newPort))
				},
				Entry("only VC PNID is updated", "some-pnid", nil),
				Entry("only VC PNID is updated", nil, "some-port"),
				Entry("only VC PNID is updated", "some-pnid", "some-port"),
			)
		})
	})

	Context("when base config does not exist", func() {
		Specify("returns no provider config and an error", func() {
			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
			client := clientfake.NewFakeClient(configMapIn, secretIn, ns)

			providerConfig, err := GetProviderConfigFromConfigMap(client, "namespace")
			Expect(err).NotTo(BeNil())
			Expect(providerConfig).To(BeNil())
		})
	})
})

var _ = Describe("ConfigMapToProviderConfig", func() {

	var (
		configMapIn *v1.ConfigMap
		vcCreds     *VSphereVmProviderCredentials
	)

	BeforeEach(func() {
		Expect(os.Unsetenv(lib.VmopNamespaceEnv)).To(Succeed())
		configMapIn, _, _ = newConfig("namespace", "pnid", "port", "secret-name")
		vcCreds = &VSphereVmProviderCredentials{Username: "some-user", Password: "some-pass"}
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
			Expect(providerConfig.StorageClassRequired).To(Equal(false))
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
			providerConfig   *VSphereVmProviderConfig
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
