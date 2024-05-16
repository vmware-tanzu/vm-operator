// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/env"
)

var _ = Describe(
	"Env",
	Ordered, // All tests in this container run in order of appearance
	func() {

		BeforeEach(func() {
			env.Unset()
		})

		Describe("FromEnv", func() {
			var (
				config pkgcfg.Config
			)

			JustBeforeEach(func() {
				config = pkgcfg.FromEnv()
			})

			AfterEach(func() {
				config = pkgcfg.Config{}
			})

			When("The environment is empty", func() {
				It("Should return a default config", func() {
					Expect(config).To(Equal(pkgcfg.Default()))
				})
			})
			When("All environment variables are set", func() {
				// Please note the following:
				//
				// * It is important that this function does not use the
				//   env package's indirections to the names, but the literal
				//   names themselves. Only this verifies that FromEnv is
				//   behaving as expected.
				//
				// * It is also important that all values be unique to ensure
				//   they match the expected output, otherwise FromEnv could
				//   simply be reading/writing the wrong info, resulting in a
				//   false-positive.
				//
				// * All boolean values must be set to the opposite of their
				//   default value. This means when an FSS is enabled, and its
				//   default value flips from false to true, its value in this
				//   test should be inverted as well.
				BeforeEach(func() {
					Expect(os.Setenv("DEFAULT_VM_CLASS_CONTROLLER_NAME", "100")).To(Succeed())
					Expect(os.Setenv("MAX_CREATE_VMS_ON_PROVIDER", "101")).To(Succeed())
					Expect(os.Setenv("PRIVILEGED_USERS", "102")).To(Succeed())
					Expect(os.Setenv("NETWORK_PROVIDER", "103")).To(Succeed())
					Expect(os.Setenv("LB_PROVIDER", "104")).To(Succeed())
					Expect(os.Setenv("VSPHERE_NETWORKING", "true")).To(Succeed())
					Expect(os.Setenv("CONTENT_API_WAIT_SECS", "105s")).To(Succeed())
					Expect(os.Setenv("JSON_EXTRA_CONFIG", "106")).To(Succeed())
					Expect(os.Setenv("INSTANCE_STORAGE_PV_PLACEMENT_FAILED_TTL", "107h")).To(Succeed())
					Expect(os.Setenv("INSTANCE_STORAGE_JITTER_MAX_FACTOR", "108.0")).To(Succeed())
					Expect(os.Setenv("INSTANCE_STORAGE_SEED_REQUEUE_DURATION", "109h")).To(Succeed())
					Expect(os.Setenv("CONTAINER_NODE", "true")).To(Succeed())
					Expect(os.Setenv("PROFILER_ADDR", "110")).To(Succeed())
					Expect(os.Setenv("RATE_LIMIT_QPS", "111")).To(Succeed())
					Expect(os.Setenv("RATE_LIMIT_BURST", "112")).To(Succeed())
					Expect(os.Setenv("SYNC_PERIOD", "113h")).To(Succeed())
					Expect(os.Setenv("MAX_CONCURRENT_RECONCILES", "114")).To(Succeed())
					Expect(os.Setenv("LEADER_ELECTION_ID", "115")).To(Succeed())
					Expect(os.Setenv("POD_NAME", "116")).To(Succeed())
					Expect(os.Setenv("POD_NAMESPACE", "117")).To(Succeed())
					Expect(os.Setenv("POD_SERVICE_ACCOUNT_NAME", "118")).To(Succeed())
					Expect(os.Setenv("WATCH_NAMESPACE", "119")).To(Succeed())
					Expect(os.Setenv("WEBHOOK_SERVICE_CONTAINER_PORT", "120")).To(Succeed())
					Expect(os.Setenv("WEBHOOK_SERVICE_NAME", "121")).To(Succeed())
					Expect(os.Setenv("WEBHOOK_SERVICE_NAMESPACE", "122")).To(Succeed())
					Expect(os.Setenv("WEBHOOK_SECRET_NAME", "123")).To(Succeed())
					Expect(os.Setenv("WEBHOOK_SECRET_NAMESPACE", "124")).To(Succeed())
					Expect(os.Setenv("FSS_WCP_INSTANCE_STORAGE", "false")).To(Succeed())
					Expect(os.Setenv("FSS_WCP_NAMESPACED_VM_CLASS", "false")).To(Succeed())
					Expect(os.Setenv("FSS_WCP_VMSERVICE_K8S_WORKLOAD_MGMT_API", "true")).To(Succeed())
					Expect(os.Setenv("FSS_WCP_VMSERVICE_ISO_SUPPORT", "true")).To(Succeed())
					Expect(os.Setenv("FSS_WCP_TKG_Multiple_CL", "false")).To(Succeed())
					Expect(os.Setenv("FSS_WCP_VMSERVICE_RESIZE", "true")).To(Succeed())
					Expect(os.Setenv("FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET", "true")).To(Succeed())
					Expect(os.Setenv("CREATE_VM_REQUEUE_DELAY", "125h")).To(Succeed())
					Expect(os.Setenv("POWERED_ON_VM_HAS_IP_REQUEUE_DELAY", "126h")).To(Succeed())
				})
				It("Should return a default config overridden by the environment", func() {
					Expect(config).To(Equal(pkgcfg.Config{
						DefaultVMClassControllerName: "100",
						MaxCreateVMsOnProvider:       101,
						PrivilegedUsers:              "102",
						NetworkProviderType:          "103",
						LoadBalancerProvider:         "104",
						VSphereNetworking:            true,
						ContentAPIWait:               105 * time.Second,
						JSONExtraConfig:              "106",
						InstanceStorage: pkgcfg.InstanceStorage{
							PVPlacementFailedTTL: 107 * time.Hour,
							JitterMaxFactor:      108.0,
							SeedRequeueDuration:  109 * time.Hour,
						},
						ContainerNode:                true,
						ProfilerAddr:                 "110",
						RateLimitQPS:                 111,
						RateLimitBurst:               112,
						SyncPeriod:                   113 * time.Hour,
						MaxConcurrentReconciles:      114,
						LeaderElectionID:             "115",
						PodName:                      "116",
						PodNamespace:                 "117",
						PodServiceAccountName:        "118",
						WatchNamespace:               "119",
						WebhookServiceContainerPort:  120,
						WebhookServiceName:           "121",
						WebhookServiceNamespace:      "122",
						WebhookSecretName:            "123",
						WebhookSecretNamespace:       "124",
						WebhookSecretVolumeMountPath: pkgcfg.Default().WebhookSecretVolumeMountPath,
						Features: pkgcfg.FeatureStates{
							InstanceStorage:    false,
							IsoSupport:         true,
							K8sWorkloadMgmtAPI: true,
							VMResize:           true,
							VMImportNewNet:     true,
						},
						CreateVMRequeueDelay:         125 * time.Hour,
						PoweredOnVMHasIPRequeueDelay: 126 * time.Hour,
					}))
				})
			})
		})
	})
