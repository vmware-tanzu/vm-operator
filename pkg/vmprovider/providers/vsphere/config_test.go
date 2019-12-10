// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	"os"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

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
		Datacenter:   "/DC0",
		ResourcePool: "/DC0/host/DC0_C0/Resources",
		Folder:       "/DC0/vm",
		Datastore:    "/DC0/datastore/LocalDS_0",
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
			ns, err = clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace", Annotations: annotations}})
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
			ns, err = clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
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
			ns, err = clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
			Expect(err).ShouldNot(HaveOccurred())
			providerConfig := &VSphereVmProviderConfig{}
			err = UpdateVMFolderAndRPInProviderConfig(clientSet, "ns-namespace", providerConfig)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal("Invalid resourcepool/folder in providerConfig. ResourcePool: , Folder: "))
		})
	})
})

var _ = Describe("GetProviderConfigFromConfigMap", func() {

	var (
		baseConfigMapIn      *v1.ConfigMap
		baseSecretIn         *v1.Secret
		baseProviderConfigIn *VSphereVmProviderConfig
		nsConfigMapIn        *v1.ConfigMap
		nsSecretIn           *v1.Secret
		nsProviderConfigIn   *VSphereVmProviderConfig
	)

	BeforeEach(func() {
		os.Unsetenv(VmopNamespaceEnv)
		baseConfigMapIn, baseSecretIn, baseProviderConfigIn = newConfig("base-namespace", "base-pnid", "base-port", "base-secret-name")
		nsConfigMapIn, nsSecretIn, nsProviderConfigIn = newConfig("ns-namespace", "ns-pnid", "ns-port", "ns-secret-name")
	})

	Context("when a base config exists", func() {

		BeforeEach(func() {
			os.Setenv(VmopNamespaceEnv, "base-namespace")
		})

		Context("when a base secret doesn't exist", func() {
			Specify("returns no provider config and an error", func() {
				clientSet := fake.NewSimpleClientset(baseConfigMapIn)
				_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
				Expect(err).To(BeNil())
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).NotTo(BeNil())
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("when a base secret exists", func() {
			Specify("returns a good provider config", func() {
				clientSet := fake.NewSimpleClientset(baseConfigMapIn, baseSecretIn)
				_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
				Expect(err).To(BeNil())
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(baseProviderConfigIn))
			})
			Specify("returns a good provider config for no namespace", func() {
				clientSet := fake.NewSimpleClientset(baseConfigMapIn, baseSecretIn)
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(baseProviderConfigIn))
			})
		})

		Context("also full ns config exists", func() {
			Specify("returns a good provider config for ns", func() {
				clientSet := fake.NewSimpleClientset(baseConfigMapIn, baseSecretIn, nsConfigMapIn, nsSecretIn)
				_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
				Expect(err).To(BeNil())
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(nsProviderConfigIn))
			})

		})

		Context("also sparse ns config exists", func() {
			Context("where ns config is missing VcPNID", func() {
				Specify("returns a good provider config with VcPNID from base", func() {
					delete(nsConfigMapIn.Data, "VcPNID")
					clientSet := fake.NewSimpleClientset(baseConfigMapIn, baseSecretIn, nsConfigMapIn, nsSecretIn)
					_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
					Expect(err).To(BeNil())
					providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
					Expect(err).To(BeNil())
					Expect(providerConfig.VcPNID).To(Equal(baseProviderConfigIn.VcPNID))
				})
			})
		})

		Context("per env variable, but only the ns config exists", func() {
			Specify("returns a good provider config for ns", func() {
				clientSet := fake.NewSimpleClientset(nsConfigMapIn, nsSecretIn)
				_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
				Expect(err).To(BeNil())
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(nsProviderConfigIn))
			})
		})

	})

	Context("when a ns config exists", func() {

		Context("when a ns secret doesn't exist", func() {
			Specify("returns no provider config and an error", func() {
				clientSet := fake.NewSimpleClientset(nsConfigMapIn)
				_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
				Expect(err).To(BeNil())
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).NotTo(BeNil())
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("when a ns secret exists", func() {
			Specify("returns a good provider config", func() {
				clientSet := fake.NewSimpleClientset(nsConfigMapIn, nsSecretIn)
				_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
				Expect(err).To(BeNil())
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(nsProviderConfigIn))
			})
		})

	})

	Context("when neither ns nor base config exist", func() {
		Specify("returns no provider config and an error", func() {
			clientSet := fake.NewSimpleClientset()
			_, err := clientSet.CoreV1().Namespaces().Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-namespace"}})
			Expect(err).To(BeNil())
			providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
			Expect(err).NotTo(BeNil())
			Expect(providerConfig).To(BeNil())
		})
	})

})

var _ = Describe("configMapsToProviderConfig", func() {

	var (
		baseConfigMapIn *v1.ConfigMap
		nsConfigMapIn   *v1.ConfigMap
		vcCreds         *VSphereVmProviderCredentials
	)

	BeforeEach(func() {
		os.Unsetenv(VmopNamespaceEnv)
		baseConfigMapIn, _, _ = newConfig("base-namespace", "base-pnid", "base-port", "base-secret-name")
		nsConfigMapIn, _, _ = newConfig("ns-namespace", "ns-pnid", "ns-port", "ns-secret-name")
		vcCreds = &VSphereVmProviderCredentials{"some-user", "some-pass"}
	})

	Context("when same key is in both config", func() {
		Specify("use the key from the ns config", func() {
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.VcPNID).To(Equal(nsConfigMapIn.Data["VcPNID"]))
		})
	})

	Context("when same key is only in ns config", func() {
		Specify("use the key from the ns config", func() {
			delete(baseConfigMapIn.Data, "VcPNID")
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.VcPNID).To(Equal(nsConfigMapIn.Data["VcPNID"]))
		})
	})

	Context("when same key is only in base config", func() {
		Specify("use the key from the base config", func() {
			delete(nsConfigMapIn.Data, "VcPNID")
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.VcPNID).To(Equal(baseConfigMapIn.Data["VcPNID"]))
		})
	})

	Context("when vcPNID is unset on both configs", func() {
		Specify("return an error", func() {
			delete(nsConfigMapIn.Data, "VcPNID")
			delete(baseConfigMapIn.Data, "VcPNID")
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).NotTo(BeNil())
			Expect(providerConfig).To(BeNil())
		})
	})

	Context("when ResourcePool is unset on both configs", func() {
		Specify("return a provider config with the ResourcePool field empty", func() {
			delete(nsConfigMapIn.Data, "ResourcePool")
			delete(baseConfigMapIn.Data, "ResourcePool")
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.ResourcePool).To(Equal(""))
		})
	})

	Context("when vcCreds is unset", func() {
		Specify("return an error", func() {
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, nil)
			Expect(err).NotTo(BeNil())
			Expect(providerConfig).To(BeNil())
		})
	})

	Context("when StorageClassRequired is unset on both configs", func() {
		Specify("return a provider config with the StorageClassRequired equals to false", func() {
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.StorageClassRequired).To(Equal(false))
		})
	})

	Context("when StorageClassRequired is set on base config", func() {
		Specify("return a provider config with the StorageClassRequired equals to true", func() {
			baseConfigMapIn.Data["StorageClassRequired"] = strconv.FormatBool(true)
			delete(nsConfigMapIn.Data, "StorageClassRequired")
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.StorageClassRequired).To(Equal(true))
		})
	})

	Context("when StorageClassRequired is set on ns config", func() {
		Specify("return a provider config with the StorageClassRequired equals to true", func() {
			delete(baseConfigMapIn.Data, "StorageClassRequired")
			nsConfigMapIn.Data["StorageClassRequired"] = strconv.FormatBool(true)
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.StorageClassRequired).To(Equal(true))
		})
	})

	Context("when StorageClassRequired is set on both configs", func() {
		Specify("return a provider config with the StorageClassRequired equals to false", func() {
			baseConfigMapIn.Data["StorageClassRequired"] = strconv.FormatBool(true)
			nsConfigMapIn.Data["StorageClassRequired"] = strconv.FormatBool(false)
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.StorageClassRequired).To(Equal(false))
		})
		Specify("return a provider config with the StorageClassRequired equals to true", func() {
			baseConfigMapIn.Data["StorageClassRequired"] = "false"
			nsConfigMapIn.Data["StorageClassRequired"] = strconv.FormatBool(true)
			providerConfig, err := ConfigMapsToProviderConfig(baseConfigMapIn, nsConfigMapIn, vcCreds)
			Expect(err).To(BeNil())
			Expect(providerConfig.StorageClassRequired).To(Equal(true))
		})
	})

})
