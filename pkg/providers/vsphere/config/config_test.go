// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/credentials"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func configTests() {

	var (
		ctx        *builder.TestContextForVCSim
		testConfig builder.VCSimTestConfig
		cfg        pkgcfg.Config
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
		cfg = pkgcfg.FromContext(ctx)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Describe("GetProviderConfig", func() {

		Context("GetProviderConfig", func() {

			Context("when a secret doesn't exist", func() {
				It("returns no provider config and an error", func() {
					// Note that NewTestContextForVCSim() creates this Secret.
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      pkgcfg.FromContext(ctx).VCCredsSecretName,
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

	Describe("GetDNSInformationFromConfigMap", func() {
		var (
			kubeDNSLBService     *corev1.Service
			kubeadmConfigMap     *corev1.ConfigMap
			vmopNetworkConfigMap *corev1.ConfigMap
		)

		JustBeforeEach(func() {
			// Note that NewTestContextForVCSim() creates this CM.
			vmopNetworkConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.NetworkConfigMapName,
					Namespace: ctx.PodNamespace,
				},
			}
			Expect(ctx.Client.Get(
				ctx,
				ctrlclient.ObjectKeyFromObject(vmopNetworkConfigMap),
				vmopNetworkConfigMap)).To(Succeed())
		})

		AfterEach(func() {
			vmopNetworkConfigMap = nil
			kubeDNSLBService = nil
			kubeadmConfigMap = nil
		})

		It("does not exist", func() {
			Expect(ctx.Client.Delete(ctx, vmopNetworkConfigMap)).To(Succeed())

			_, _, err := config.GetDNSInformationFromConfigMap(ctx, ctx.Client)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("returns empty data", func() {
			vmopNetworkConfigMap.Data = map[string]string{}
			Expect(ctx.Client.Update(ctx, vmopNetworkConfigMap)).To(Succeed())

			nameservers, searchSuffixes, err := config.GetDNSInformationFromConfigMap(ctx, ctx.Client)
			Expect(err).ToNot(HaveOccurred())
			Expect(nameservers).To(BeEmpty())
			Expect(searchSuffixes).To(BeEmpty())
		})

		It("returns error if <worker_dns> is there", func() {
			vmopNetworkConfigMap.Data = map[string]string{
				config.NameserversKey: "<worker_dns>",
			}
			Expect(ctx.Client.Update(ctx, vmopNetworkConfigMap)).To(Succeed())

			_, _, err := config.GetDNSInformationFromConfigMap(ctx, ctx.Client)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("It still contains <worker_dns> key"))
		})

		It("returns expected data", func() {
			vmopNetworkConfigMap.Data = map[string]string{
				config.NameserversKey:    "1.1.1.1 8.8.8.8",
				config.SearchSuffixesKey: "vmware.com google.com",
			}
			Expect(ctx.Client.Update(ctx, vmopNetworkConfigMap)).To(Succeed())

			nameservers, searchSuffixes, err := config.GetDNSInformationFromConfigMap(ctx, ctx.Client)
			Expect(err).ToNot(HaveOccurred())
			Expect(nameservers).To(ConsistOf("1.1.1.1", "8.8.8.8"))
			Expect(searchSuffixes).To(ConsistOf("vmware.com", "google.com"))
		})

		When("using kube-dns", func() {
			JustBeforeEach(func() {
				kubeDNSLBService = &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfg.KubeDNSLBServiceName,
						Namespace: cfg.KubeSystemNamespace,
					},
				}
				Expect(ctx.Client.Create(ctx, kubeDNSLBService)).To(Succeed())
				kubeDNSLBService.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
					{
						IP: "1.2.3.4",
					},
				}
				Expect(ctx.Client.Status().Update(ctx, kubeDNSLBService)).To(Succeed())

				kubeadmConfigMap = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfg.KubeadmConfigMapName,
						Namespace: cfg.KubeSystemNamespace,
					},
					Data: map[string]string{
						cfg.KubeadmClusterConfigKey: kubeadmConfigFile,
					},
				}
				Expect(ctx.Client.Create(ctx, kubeadmConfigMap)).To(Succeed())

				Expect(ctx.Client.Get(
					ctx,
					ctrlclient.ObjectKeyFromObject(vmopNetworkConfigMap),
					vmopNetworkConfigMap)).To(Succeed())

				vmopNetworkConfigMap.Data = map[string]string{
					config.NameserversKey:    "1.1.1.1 8.8.8.8",
					config.SearchSuffixesKey: "vmware.com google.com",
				}
				Expect(ctx.Client.Update(ctx, vmopNetworkConfigMap)).To(Succeed())
			})

			It("should return just the kube-dns data", func() {
				nameservers, searchSuffixes, err := config.GetDNSInformationFromConfigMap(ctx, ctx.Client)
				Expect(err).ToNot(HaveOccurred())
				Expect(nameservers).To(ConsistOf("1.2.3.4"))
				Expect(searchSuffixes).To(ConsistOf("cluster.local"))
			})

			When("there is no data in the vmop configmap", func() {
				JustBeforeEach(func() {
					vmopNetworkConfigMap.Data = map[string]string{}
					Expect(ctx.Client.Update(ctx, vmopNetworkConfigMap)).To(Succeed())
				})
				It("should return just the kube-dns data", func() {
					nameservers, searchSuffixes, err := config.GetDNSInformationFromConfigMap(ctx, ctx.Client)
					Expect(err).ToNot(HaveOccurred())
					Expect(nameservers).To(ConsistOf("1.2.3.4"))
					Expect(searchSuffixes).To(ConsistOf("cluster.local"))
				})
			})

			When("the vmop configmap does not exist", func() {
				JustBeforeEach(func() {
					Expect(ctx.Client.Delete(ctx, vmopNetworkConfigMap)).To(Succeed())
				})
				It("should return just the kube-dns data", func() {
					nameservers, searchSuffixes, err := config.GetDNSInformationFromConfigMap(ctx, ctx.Client)
					Expect(err).ToNot(HaveOccurred())
					Expect(nameservers).To(ConsistOf("1.2.3.4"))
					Expect(searchSuffixes).To(ConsistOf("cluster.local"))
				})
			})
		})
	})
}

var _ = Describe("ConfigMapToProviderConfig", func() {

	var (
		providerCreds    credentials.VSphereVMProviderCredentials
		providerConfigIn *config.VSphereVMProviderConfig
		configMap        *corev1.ConfigMap
	)

	BeforeEach(func() {
		providerCreds = credentials.VSphereVMProviderCredentials{Username: "username", Password: "password"}
		providerConfigIn = &config.VSphereVMProviderConfig{
			VcPNID:                      "my-vc.vmware.com",
			VcPort:                      "433",
			VcCreds:                     providerCreds,
			Datacenter:                  "datacenter-42",
			StorageClassRequired:        false,
			UseInventoryAsContentSource: false,
			CAFilePath:                  "/etc/pki/tls/certs/ca-bundle.crt",
			InsecureSkipTLSVerify:       false,
			Datastore:                   "/DC0/datastore/LocalDS_0",
		}
	})

	JustBeforeEach(func() {
		configMap = config.ProviderConfigToConfigMap("dummy-ns", providerConfigIn)
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

const kubeadmConfigFile = `
apiServer:
  certSANs:
  - 127.0.0.1
  - 10.192.17.69
  - supervisor.default.svc
  extraArgs:
    admission-control-config-file: /etc/vmware/wcp/admission-control.yaml
    anonymous-auth: "false"
    audit-log-compress: "true"
    audit-log-maxage: "30"
    audit-log-maxbackup: "25"
    audit-log-maxsize: "400"
    audit-log-path: /var/log/vmware/audit/kube-apiserver.log
    audit-policy-file: /etc/vmware/wcp/audit-policy.yaml
    authentication-config: /etc/vmware/wcp/authentication-config.yaml
    authorization-config: /etc/vmware/wcp/authorization/kube-apiserver-authorization.yaml
    disable-admission-plugins: ""
    enable-admission-plugins: NamespaceLifecycle,ServiceAccount,NodeRestriction,EventRateLimit,LimitRanger,PersistentVolumeLabel,DefaultStorageClass,DefaultTolerationSeconds,ResourceQuota,ValidatingAdmissionWebhook,PodSecurity,MutatingAdmissionWebhook,StorageObjectInUseProtection
    enable-bootstrap-token-auth: "true"
    encryption-provider-config: /etc/vmware/wcp/encryption-config.yaml
    profiling: "false"
    runtime-config: admissionregistration.k8s.io/v1
    service-account-jwks-uri: https://kubernetes.default.svc.cluster.local/openid/v1/jwks
    service-account-lookup: "true"
    service-cluster-ip-range: 172.24.0.0/16
    tls-cipher-suites: TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
    tls-filter-cert-key: /etc/vmware/wcp/tls/vip.crt,/etc/vmware/wcp/tls/vip.key:interface=eth1,require-src-cidr=0.0.0.0/32
    tls-min-version: VersionTLS12
    tls-sni-cert-key: /etc/kubernetes/pki/apiserver.crt,/etc/kubernetes/pki/apiserver.key
  timeoutForControlPlane: 4m0s
apiVersion: kubeadm.k8s.io/v1beta3
certificatesDir: /etc/kubernetes/pki
clusterName: kubernetes
controlPlaneEndpoint: 10.192.17.69:6443
controllerManager:
  extraArgs:
    client-ca-file: /etc/vmware/wcp/tls/vmca.pem
    feature-gates: RotateKubeletServerCertificate=true
    profiling: "false"
    terminated-pod-gc-threshold: "1000"
    tls-cipher-suites: TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
    tls-min-version: VersionTLS12
dns:
  imageRepository: localhost:5000/vmware
  imageTag: v1.30
etcd:
  local:
    dataDir: /var/lib/etcd
    extraArgs:
      auto-tls: "false"
      cipher-suites: TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
      election-timeout: "6000"
      heartbeat-interval: "600"
      initial-cluster-token: domain-c11
      max-wals: "80"
      strict-reconfig-check: "false"
    imageRepository: vmware
    imageTag: KUSTOMIZE
    peerCertSANs:
    - 10.192.17.69
imageRepository: vmware
kind: ClusterConfiguration
kubernetesVersion: v1.30.10
networking:
  dnsDomain: cluster.local
  serviceSubnet: 172.24.0.0/16
scheduler:
  extraArgs:
    config: /etc/kubernetes/wcp-schedext-scheduler-configuration-v1.yaml
    profiling: "false"
    tls-min-version: VersionTLS12
`
