/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
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
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).NotTo(BeNil())
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("when a base secret exists", func() {
			Specify("returns a good provider config", func() {
				clientSet := fake.NewSimpleClientset(baseConfigMapIn, baseSecretIn)
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(baseProviderConfigIn))
			})
		})

		Context("also full ns config exists", func() {
			Specify("returns a good provider config for ns", func() {
				clientSet := fake.NewSimpleClientset(baseConfigMapIn, baseSecretIn, nsConfigMapIn, nsSecretIn)
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
					providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
					Expect(err).To(BeNil())
					Expect(providerConfig.VcPNID).To(Equal(baseProviderConfigIn.VcPNID))
				})
			})
		})

		Context("per env variable, but only the ns config exists", func() {
			Specify("returns a good provider config for ns", func() {
				clientSet := fake.NewSimpleClientset(nsConfigMapIn, nsSecretIn)
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
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).NotTo(BeNil())
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("when a ns secret exists", func() {
			Specify("returns a good provider config", func() {
				clientSet := fake.NewSimpleClientset(nsConfigMapIn, nsSecretIn)
				providerConfig, err := GetProviderConfigFromConfigMap(clientSet, "ns-namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(nsProviderConfigIn))
			})
		})

	})

	Context("when neither ns nor base config exist", func() {
		Specify("returns no provider config and an error", func() {
			clientSet := fake.NewSimpleClientset()
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

})
