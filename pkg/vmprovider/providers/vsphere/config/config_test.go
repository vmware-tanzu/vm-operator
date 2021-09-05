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
	"github.com/vmware/govmomi/simulator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
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

var _ = Describe("UpdateProviderConfigFromZoneAndNamespace", func() {

	Context("when a good provider config exists and zone/namespace info exists", func() {
		Specify("provider config is updated with RP and VM folder from zone", func() {
			namespaceRP := "namespace-test-RP"
			namespaceVMFolder := "namespace-test-vmfolder"
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
			zone := &topologyv1.AvailabilityZone{
				ObjectMeta: metav1.ObjectMeta{
					Name: "homer",
				},
				Spec: topologyv1.AvailabilityZoneSpec{
					ClusterComputeResourceMoId: "cluster",
					Namespaces: map[string]topologyv1.NamespaceInfo{
						ns.Name: {
							PoolMoId:   namespaceRP,
							FolderMoId: namespaceVMFolder,
						},
					},
				},
			}
			client := builder.NewFakeClient(ns, zone)
			providerConfig := &VSphereVMProviderConfig{}
			Expect(UpdateProviderConfigFromZoneAndNamespace(
				ctx, client, zone.Name, ns.Name, providerConfig)).To(Succeed())
			Expect(providerConfig.ResourcePool).To(Equal(namespaceRP))
			Expect(providerConfig.Folder).To(Equal(namespaceVMFolder))
		})
	})

	Context("when a good provider config exists and zone/namespace info does not exist", func() {
		Specify("should succeed with providerconfig unmodified", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
			client := builder.NewFakeClient(ns)
			providerConfig := &VSphereVMProviderConfig{}
			providerConfigRP := "namespace-test-RP"
			providerConfigFolder := "namespace-test-vmfolder"
			providerConfig.ResourcePool = providerConfigRP
			providerConfig.Folder = providerConfigFolder
			Expect(UpdateProviderConfigFromZoneAndNamespace(
				ctx, client, "homer", ns.Name, providerConfig)).To(Succeed())
			Expect(providerConfig.ResourcePool).To(Equal(providerConfigRP))
			Expect(providerConfig.Folder).To(Equal(providerConfigFolder))
		})
	})

	Context("namespace does not exist", func() {
		Specify("returns error", func() {
			client := builder.NewFakeClient()
			providerConfig := &VSphereVMProviderConfig{}
			err := UpdateProviderConfigFromZoneAndNamespace(
				ctx, client, "homer", "test-namespace", providerConfig)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal(
				"could not get the namespace: test-namespace: namespaces \"test-namespace\" not found"))

		})
	})

	Context("ResourcePool and Folder not present in zone/namespace info", func() {
		It("should not modify config", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
			zone := &topologyv1.AvailabilityZone{
				ObjectMeta: metav1.ObjectMeta{
					Name: "homer",
				},
				Spec: topologyv1.AvailabilityZoneSpec{
					ClusterComputeResourceMoId: "cluster",
					Namespaces: map[string]topologyv1.NamespaceInfo{
						ns.Name: {},
					},
				},
			}
			client := builder.NewFakeClient(ns, zone)
			providerConfig := VSphereVMProviderConfig{}
			providerConfig.ResourcePool = "foo"
			providerConfig.Folder = "bar"
			providerConfigIn := providerConfig
			err := UpdateProviderConfigFromZoneAndNamespace(
				ctx, client, zone.Name, ns.Name, &providerConfigIn)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(providerConfig).To(Equal(providerConfigIn))
		})
	})
})

var _ = Describe("GetProviderConfigFromConfigMap", func() {

	var (
		configMapIn      *corev1.ConfigMap
		secretIn         *corev1.Secret
		providerConfigIn *VSphereVMProviderConfig
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
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
				client := builder.NewFakeClient(configMapIn, ns)

				providerConfig, err := GetProviderConfigFromConfigMap(ctx, client, "", "namespace")
				Expect(err).NotTo(BeNil())
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("ResourcePool and Folder not present in either providerConfig or zone/namespace info", func() {
			Specify("should return an error", func() {
				delete(configMapIn.Data, "ResourcePool")
				delete(configMapIn.Data, "Folder")
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
				zone := &topologyv1.AvailabilityZone{
					ObjectMeta: metav1.ObjectMeta{
						Name: "homer",
					},
					Spec: topologyv1.AvailabilityZoneSpec{
						ClusterComputeResourceMoId: "cluster",
						Namespaces: map[string]topologyv1.NamespaceInfo{
							ns.Name: {},
						},
					},
				}
				client := builder.NewFakeClient(configMapIn, secretIn, ns, zone)
				providerConfig, err := GetProviderConfigFromConfigMap(
					ctx, client, zone.Name, ns.Name)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("missing ResourcePool and Folder in ProviderConfig. ResourcePool: , Folder: "))
				Expect(providerConfig).To(BeNil())
			})
		})

		Context("when a secret exists", func() {
			Specify("returns a good provider config", func() {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
				client := builder.NewFakeClient(configMapIn, secretIn, ns)

				providerConfig, err := GetProviderConfigFromConfigMap(ctx, client, "", "namespace")
				Expect(err).To(BeNil())
				Expect(providerConfig).To(Equal(providerConfigIn))
			})
			Specify("returns a good provider config for no namespace", func() {
				client := builder.NewFakeClient(configMapIn, secretIn)
				providerConfig, err := GetProviderConfigFromConfigMap(ctx, client, "", "")
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

					ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
					client := builder.NewFakeClient(configMapIn, secretIn, ns)

					Expect(PatchVcURLInConfigMap(client, newPnid, newPort)).To(Succeed())
					providerConfig, err := GetProviderConfigFromConfigMap(ctx, client, "", "")
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
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace"}}
			client := builder.NewFakeClient(configMapIn, secretIn, ns)

			providerConfig, err := GetProviderConfigFromConfigMap(ctx, client, "", "namespace")
			Expect(err).NotTo(BeNil())
			Expect(providerConfig).To(BeNil())
		})
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
