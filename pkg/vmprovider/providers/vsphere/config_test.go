// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
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
	"k8s.io/client-go/kubernetes/fake"

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
	var (
		ns  *v1.Namespace
		err error
	)
	Context("when a good provider config exists and namespace has non-empty annotations", func() {
		Specify("provider config is updated with RP and VM folder from annotations", func() {
			clientSet := fake.NewSimpleClientset()
			namespaceRP := "namespace-test-RP"
			namespaceVMFolder := "namesapce-test-vmfolder"
			annotations := make(map[string]string)
			annotations[NamespaceRPAnnotationKey] = namespaceRP
			annotations[NamespaceFolderAnnotationKey] = namespaceVMFolder
			ns, err = clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace", Annotations: annotations}})
			Expect(err).ShouldNot(HaveOccurred())
			providerConfig := &VSphereVmProviderConfig{}
			Expect(UpdateVMFolderAndRPInProviderConfig(clientSet, ns.Name, providerConfig)).To(Succeed())
			Expect(providerConfig.ResourcePool).To(Equal(namespaceRP))
			Expect(providerConfig.Folder).To(Equal(namespaceVMFolder))
		})
	})
	Context("when a good provider config exists and namespace does not have annotations", func() {
		Specify("should succeed with providerconfig unmodified", func() {
			clientSet := fake.NewSimpleClientset()
			ns, err = clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}})
			Expect(err).ShouldNot(HaveOccurred())
			providerConfig := &VSphereVmProviderConfig{}
			providerConfigRP := "namespace-test-RP"
			providerConfigFolder := "namesapce-test-vmfolder"
			providerConfig.ResourcePool = providerConfigRP
			providerConfig.Folder = providerConfigFolder
			Expect(UpdateVMFolderAndRPInProviderConfig(clientSet, ns.Name, providerConfig)).To(Succeed())
			Expect(providerConfig.ResourcePool).To(Equal(providerConfigRP))
			Expect(providerConfig.Folder).To(Equal(providerConfigFolder))
		})
	})
	Context("namespace does not exist", func() {
		Specify("returns error", func() {
			clientSet := fake.NewSimpleClientset()
			providerConfig := &VSphereVmProviderConfig{}
			err = UpdateVMFolderAndRPInProviderConfig(clientSet, "test-namespace", providerConfig)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal("could not find the namespace: test-namespace: namespaces \"test-namespace\" not found"))
		})
	})
	Context("ResourcePool and Folder not present in either providerConfig or namespace annotations", func() {
		It("should return an error", func() {
			clientSet := fake.NewSimpleClientset()
			ns, err = clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}})
			Expect(err).ShouldNot(HaveOccurred())
			providerConfig := &VSphereVmProviderConfig{}
			err = UpdateVMFolderAndRPInProviderConfig(clientSet, "namespace", providerConfig)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal("Invalid resourcepool/folder in providerConfig. ResourcePool: , Folder: "))
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
		os.Unsetenv(lib.VmopNamespaceEnv)
		configMapIn, secretIn, providerConfigIn = newConfig("namespace", "pnid", "port", "secret-name")
	})

	Context("when a base config exists", func() {

		BeforeEach(func() {
			os.Setenv(lib.VmopNamespaceEnv, "namespace")
		})

		Context("when a secret doesn't exist", func() {
			Specify("returns no provider config and an error", func() {
				clientSet := fake.NewSimpleClientset(configMapIn)
				_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}})
				Expect(err).To(BeNil())
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "namespace")
				Expect(err).NotTo(BeNil())
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("when a secret exists", func() {
			Specify("returns a good provider config", func() {
				clientSet := fake.NewSimpleClientset(configMapIn, secretIn)
				_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}})
				Expect(err).To(BeNil())
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(providerConfigIn))
			})
			Specify("returns a good provider config for no namespace", func() {
				clientSet := fake.NewSimpleClientset(configMapIn, secretIn)
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(providerConfigIn))
			})
			Specify("update pnid for no namespace", func() {
				clientSet := fake.NewSimpleClientset(configMapIn, secretIn)
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(providerConfigIn))

				newPnid := providerConfig.VcPNID + "-02"
				err = PatchPnidInConfigMap(clientSet, newPnid)
				Expect(err).NotTo(HaveOccurred())
				providerConfig, _ = GetProviderConfigFromConfigMap(clientSet, "")
				Expect(providerConfig.VcPNID).Should(Equal(newPnid))
			})

		})
	})

	Context("when base config does not exist", func() {
		Specify("returns no provider config and an error", func() {
			clientSet := fake.NewSimpleClientset()
			_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}})
			Expect(err).To(BeNil())
			providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "namespace")
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
		os.Unsetenv(lib.VmopNamespaceEnv)
		configMapIn, _, _ = newConfig("namespace", "pnid", "port", "secret-name")
		vcCreds = &VSphereVmProviderCredentials{"some-user", "some-pass"}
	})

	It("verifies that a config is correctly extracted from the configMap", func() {
		providerConfig, err := ConfigMapToProviderConfig(configMapIn, vcCreds)
		Expect(err).To(BeNil())
		Expect(providerConfig.VcPNID).To(Equal(configMapIn.Data["VcPNID"]))
	})

	Context("when vcPNID is unset in configMap", func() {
		Specify("return an error", func() {
			delete(configMapIn.Data, "VcPNID")
			providerConfig, err := ConfigMapToProviderConfig(configMapIn, vcCreds)
			Expect(err).NotTo(BeNil())
			Expect(providerConfig).To(BeNil())
		})
	})

	Context("when vcCreds is unset", func() {
		Specify("return an error", func() {
			providerConfig, err := ConfigMapToProviderConfig(configMapIn, nil)
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
			expectErrToOccur bool = false
			err              error
		)
		JustBeforeEach(func() {
			// We're not testing anything related to per-namespace ConfigMaps, just make them identical
			//  to the base ConfigMap.
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
