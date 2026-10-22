// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crd_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgcrd "github.com/vmware-tanzu/vm-operator/pkg/crd"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

const (
	storagePoliciesCRD = "storagepolicies.infra.vmware.com"
)

var (
	basesNonGated = []string{
		"clustervirtualmachineimages.vmoperator.vmware.com",
		"contentlibraryproviders.vmoperator.vmware.com",
		"contentsourcebindings.vmoperator.vmware.com",
		"contentsources.vmoperator.vmware.com",
		"virtualmachineclassbindings.vmoperator.vmware.com",
		"virtualmachineclasses.vmoperator.vmware.com",
		"virtualmachinereservedprofiles.vmoperator.vmware.com",
		"virtualmachineimages.vmoperator.vmware.com",
		"virtualmachinepublishrequests.vmoperator.vmware.com",
		"virtualmachines.vmoperator.vmware.com",
		"virtualmachineservices.vmoperator.vmware.com",
		"virtualmachinesetresourcepolicies.vmoperator.vmware.com",
		"virtualmachinewebconsolerequests.vmoperator.vmware.com",
		"webconsolerequests.vmoperator.vmware.com",
	}

	basesVMGroups = []string{
		"virtualmachinegrouppublishrequests.vmoperator.vmware.com",
		"virtualmachinegroups.vmoperator.vmware.com",
	}

	basesSnapshots = []string{
		"virtualmachinesnapshots.vmoperator.vmware.com",
	}

	basesFastDeploy = []string{
		"virtualmachineimagecaches.vmoperator.vmware.com",
	}

	basesImmutableClasses = []string{
		"virtualmachineclassinstances.vmoperator.vmware.com",
	}

	basesK8sWorkloadMgmtAPI = []string{
		"virtualmachinereplicasets.vmoperator.vmware.com",
	}

	basesAll = slices.Concat(
		basesNonGated,
		basesFastDeploy,
		basesImmutableClasses,
		basesSnapshots,
		basesVMGroups,
		basesK8sWorkloadMgmtAPI,
	)

	externalBYOK = []string{
		"encryptionclasses.encryption.vmware.com",
	}

	externalVSpherePolicy = []string{
		"computepolicies.vsphere.policy.vmware.com",
		"policyevaluations.vsphere.policy.vmware.com",
		"requiredduringexecutionvmplacementpolicies.vsphere.policy.vmware.com",
		"tagpolicies.vsphere.policy.vmware.com",
	}

	externalTagAPI = []string{
		"tags.vsphere.policy.vmware.com",
	}

	externalEnvironmentBrowser = []string{
		"configtargets.vim.vmware.com",
		"virtualmachineconfigoptions.vim.vmware.com",
		"virtualmachineguestoptions.vim.vmware.com",
	}

	externalVMEviction = []string{
		"automaticvmevictionpolicies.vsphere.policy.vmware.com",
		"besteffortrestartpolicies.vsphere.policy.vmware.com",
	}

	externalControlledRebalancing = []string{
		"controlledrebalancingpolicies.vsphere.policy.vmware.com",
	}

	externalAll = slices.Concat(
		externalBYOK,
		externalVSpherePolicy,
		externalTagAPI,
		externalEnvironmentBrowser,
		externalVMEviction,
		externalControlledRebalancing,
		[]string{storagePoliciesCRD},
	)
)

func init() {
	slices.Sort(basesAll)
	basesAll = slices.Compact(basesAll)
}

func assertCRDsConsistOf[T any](
	crds []T,
	expectedNames ...string) {

	GinkgoHelper()

	slices.Sort(expectedNames)
	expectedNames = slices.Compact(expectedNames)

	Expect(expectedNames).To(HaveLen(len(crds)))

	actualNames := make([]string, len(crds))
	for i := range crds {
		switch tCRD := (any)(crds[i]).(type) {
		case unstructured.Unstructured:
			actualNames[i] = tCRD.GetName()
		case apiextensionsv1.CustomResourceDefinition:
			actualNames[i] = tCRD.GetName()
		case *unstructured.Unstructured:
			actualNames[i] = tCRD.GetName()
		case *apiextensionsv1.CustomResourceDefinition:
			actualNames[i] = tCRD.GetName()
		}

	}

	Expect(actualNames).To(ConsistOf(expectedNames))
}

var _ = Describe("UnstructuredBases", func() {
	It("should get the expected crds", func() {
		crds, err := pkgcrd.UnstructuredBases()
		Expect(err).ToNot(HaveOccurred())
		assertCRDsConsistOf(crds, basesAll...)
	})
})

var _ = Describe("UnstructuredExternal", func() {
	It("should get the expected crds", func() {
		crds, err := pkgcrd.UnstructuredExternal()
		Expect(err).ToNot(HaveOccurred())
		assertCRDsConsistOf(crds, externalAll...)
	})
})

var _ = Describe("Install", func() {
	var (
		ctx    context.Context
		client ctrlclient.Client
	)

	BeforeEach(func() {
		ctx = pkgcfg.WithConfig(pkgcfg.Config{
			CRDCleanupEnabled: false,
			Features: pkgcfg.FeatureStates{
				FastDeploy:         false,
				ImmutableClasses:   false,
				VMGroups:           false,
				VMSnapshots:        false,
				K8sWorkloadMgmtAPI: false,
			},
		})

		scheme := runtime.NewScheme()
		Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		client = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()
	})

	JustBeforeEach(func() {
		Expect(pkgcrd.Install(ctx, client, nil)).To(Succeed())
	})

	DescribeTable("removes the obsolete policy CRD on upgrade",
		func(enabled bool) {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VirtualMachineConfigPolicy = enabled
			})
			obj := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "virtualmachineconfigpolicies.vim.vmware.com"},
			}
			Expect(client.Create(ctx, obj)).To(Succeed())
			Expect(pkgcrd.Install(ctx, client, nil)).To(Succeed())
			err := client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), obj)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			var list apiextensionsv1.CustomResourceDefinitionList
			Expect(client.List(ctx, &list)).To(Succeed())
			expected := slices.Clone(basesNonGated)
			if enabled {
				expected = append(expected, externalEnvironmentBrowser...)
			}
			assertCRDsConsistOf(list.Items, expected...)
		},
		Entry("with the environment browser enabled", true),
		Entry("with the environment browser disabled", false),
	)

	It("should stop writing after one update with cleanup disabled", func() {
		// Creation removes disabled feature fields, while updates with cleanup
		// disabled retain fields from the embedded schema. Allow that first update.
		recordingClient := &writeRecordingClient{Client: client}
		Expect(pkgcrd.Install(ctx, recordingClient, nil)).To(Succeed())
		Expect(recordingClient.writes).To(ConsistOf(
			"patch/virtualmachines.vmoperator.vmware.com",
			"patch/virtualmachineservices.vmoperator.vmware.com",
		))

		for range 2 {
			recordingClient.writes = nil
			Expect(pkgcrd.Install(ctx, recordingClient, nil)).To(Succeed())
			Expect(recordingClient.writes).To(BeEmpty())
		}
	})

	When("CRDs are installed again", func() {
		var recordingClient *writeRecordingClient

		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.CRDCleanupEnabled = true
			})
		})

		JustBeforeEach(func() {
			recordingClient = &writeRecordingClient{Client: client}
		})

		It("should skip writes when CRDs are unchanged", func() {
			Expect(pkgcrd.Install(ctx, recordingClient, nil)).To(Succeed())
			Expect(recordingClient.writes).To(BeEmpty())
		})

		It("should preserve conversion configuration without patching", func() {
			key := ctrlclient.ObjectKey{Name: "virtualmachines.vmoperator.vmware.com"}
			var obj apiextensionsv1.CustomResourceDefinition
			Expect(client.Get(ctx, key, &obj)).To(Succeed())
			conversion := &apiextensionsv1.CustomResourceConversion{
				Strategy: apiextensionsv1.WebhookConverter,
				Webhook: &apiextensionsv1.WebhookConversion{
					ConversionReviewVersions: []string{"v1"},
					ClientConfig: &apiextensionsv1.WebhookClientConfig{
						Service: &apiextensionsv1.ServiceReference{
							Name: "webhook", Namespace: "default",
							Path: ptr.To("/convert"), Port: ptr.To(int32(443)),
						},
					},
				},
			}
			obj.Spec.Conversion = conversion
			obj.Labels = map[string]string{"test": "preserved"}
			Expect(client.Update(ctx, &obj)).To(Succeed())

			Expect(pkgcrd.Install(ctx, recordingClient, nil)).To(Succeed())
			Expect(recordingClient.writes).To(BeEmpty())
			Expect(client.Get(ctx, key, &obj)).To(Succeed())
			Expect(obj.Spec.Conversion).To(Equal(conversion))
			Expect(obj.Labels).To(HaveKeyWithValue("test", "preserved"))
		})

		It("should patch a changed spec and skip it on the next installation", func() {
			key := ctrlclient.ObjectKey{Name: "virtualmachines.vmoperator.vmware.com"}
			var expected apiextensionsv1.CustomResourceDefinition
			Expect(client.Get(ctx, key, &expected)).To(Succeed())
			obj := expected.DeepCopy()
			obj.Spec.Names.ShortNames = []string{"changed"}
			Expect(client.Update(ctx, obj)).To(Succeed())

			Expect(pkgcrd.Install(ctx, recordingClient, nil)).To(Succeed())
			Expect(recordingClient.writes).To(Equal([]string{"patch/" + key.Name}))
			Expect(client.Get(ctx, key, obj)).To(Succeed())
			Expect(obj.Spec).To(Equal(expected.Spec))

			recordingClient.writes = nil
			Expect(pkgcrd.Install(ctx, recordingClient, nil)).To(Succeed())
			Expect(recordingClient.writes).To(BeEmpty())
		})
	})

	AfterEach(func() {
		ctx = nil
		client = nil
	})

	assertFieldForCRD := func(crdName string, expected bool, fields ...string) {
		GinkgoHelper()

		obj := unstructured.Unstructured{
			Object: map[string]any{},
		}
		obj.SetAPIVersion("apiextensions.k8s.io/v1")
		obj.SetKind("CustomResourceDefinition")
		obj.SetName(crdName)

		Expect(client.Get(
			ctx,
			ctrlclient.ObjectKeyFromObject(&obj),
			&obj)).To(Succeed())

		versions, _, err := unstructured.NestedSlice(
			obj.Object, "spec", "versions")
		Expect(err).ToNot(HaveOccurred())

		hasField := false
		for j := range versions {
			v := versions[j].(map[string]any)
			_, okay, err := unstructured.NestedFieldNoCopy(
				v,
				fields...)
			Expect(err).ToNot(HaveOccurred())
			if okay {
				hasField = okay
				break
			}
		}
		Expect(hasField).To(Equal(expected))
	}

	assertField := func(expected bool, fields ...string) {
		GinkgoHelper()
		assertFieldForCRD("virtualmachines.vmoperator.vmware.com", expected, fields...)
	}

	assertVMSvcField := func(expected bool, fields ...string) {
		GinkgoHelper()
		assertFieldForCRD("virtualmachineservices.vmoperator.vmware.com", expected, fields...)
	}

	assertVMGroupField := func(expected bool, fields ...string) {
		GinkgoHelper()
		assertFieldForCRD("virtualmachinegroups.vmoperator.vmware.com", expected, fields...)
	}

	assertCELRulesContaining := func(crdName string, celPath []string, fieldRef string, expectPresent bool) {
		GinkgoHelper()

		obj := unstructured.Unstructured{Object: map[string]any{}}
		obj.SetAPIVersion("apiextensions.k8s.io/v1")
		obj.SetKind("CustomResourceDefinition")
		obj.SetName(crdName)
		Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(&obj), &obj)).To(Succeed())

		versions, _, err := unstructured.NestedSlice(obj.Object, "spec", "versions")
		Expect(err).ToNot(HaveOccurred())

		found := false
		for j := range versions {
			v := versions[j].(map[string]any)
			rules, _, _ := unstructured.NestedSlice(v, celPath...)
			for _, r := range rules {
				ruleMap, ok := r.(map[string]any)
				if !ok {
					continue
				}
				if ruleText, _ := ruleMap["rule"].(string); strings.Contains(ruleText, fieldRef) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		Expect(found).To(Equal(expectPresent))
	}

	When("no crds are installed", func() {
		When("no capabilities are enabled", func() {
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, basesNonGated...)
			})

			DescribeTable("vm api should not have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(false, fields...)
				},
				Entry("bootOptions", "bootOptions"),
				Entry("class", "class"),
				Entry("currentSnapshotName", "currentSnapshotName"),
				Entry("groupName", "groupName"),
				Entry("policies", "policies"),
				Entry("linuxPrep expire password", "bootstrap.linuxPrep.expirePasswordAfterNextLogin"),
				Entry("linuxPrep root password", "bootstrap.linuxPrep.password"),
				Entry("linuxPrep script text ", "bootstrap.linuxPrep.scriptText"),
				Entry("sysprep expire password", "bootstrap.sysprep.sysprep.expirePasswordAfterNextLogin"),
				Entry("sysprep script text", "bootstrap.sysprep.sysprep.scriptText"),
				Entry("ideControllers", "hardware.ideControllers"),
				Entry("nvmeControllers", "hardware.nvmeControllers"),
				Entry("sataControllers", "hardware.sataControllers"),
				Entry("scsiControllers", "hardware.scsiControllers"),
				Entry("cdrom's controllerBusNumber", "hardware.cdrom.[].controllerBusNumber"),
				Entry("cdrom's controllerType", "hardware.cdrom.[].controllerType"),
				Entry("cdrom's unitNumber", "hardware.cdrom.[].unitNumber"),
				Entry("volumes pvc applicationType", "volumes.[].applicationType"),
				Entry("volumes pvc controllerBusNumber", "volumes.[].controllerBusNumber"),
				Entry("volumes pvc controllerType", "volumes.[].controllerType"),
				Entry("volumes pvc diskMode", "volumes.[].diskMode"),
				Entry("volumes pvc sharingMode", "volumes.[].sharingMode"),
				Entry("volumes pvc unitNumber", "volumes.[].unitNumber"),
				Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
				Entry("advanced preferHtEnabled", "advanced.preferHtEnabled"),
				Entry("advanced hugePages1GEnabled", "advanced.hugePages1GEnabled"),
				Entry("advanced timeTrackerLowLatencyEnabled", "advanced.timeTrackerLowLatencyEnabled"),
				Entry("advanced cpuAffinityExclusiveNoStatsEnabled", "advanced.cpuAffinityExclusiveNoStatsEnabled"),
				Entry("advanced vmxSwapEnabled", "advanced.vmxSwapEnabled"),
				Entry("advanced pnumaNodeAffinity", "advanced.pnumaNodeAffinity"),
				Entry("advanced extraConfig", "advanced.extraConfig"),
				Entry("network interface type", "network.interfaces.[].type"),
				Entry("network interface vnumaNodeID", "network.interfaces.[].vnumaNodeID"),
				Entry("network interface vmxnet3", "network.interfaces.[].vmxnet3"),
				Entry("network interface advancedProperties", "network.interfaces.[].advancedProperties"),
				Entry("network interface ipamModes", "network.interfaces.[].ipamModes"),
				Entry("resources", "resources"),
				Entry("cpuAdvanced", "cpuAdvanced"),
				Entry("memoryAdvanced", "memoryAdvanced"),
			)

			DescribeTable("vm service api should not have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertVMSvcField(false, fields...)
				},
				Entry("ipFamilies", "ipFamilies"),
				Entry("ipFamilyPolicy", "ipFamilyPolicy"),
			)

			It("vm api should not have CEL rules referencing vmxnet3", func() {
				assertCELRulesContaining(
					"virtualmachines.vmoperator.vmware.com",
					specCELPath("network.interfaces.[]"),
					"vmxnet3",
					false,
				)
			})

			It("vm api should not have CEL rules referencing ipamModes", func() {
				assertCELRulesContaining(
					"virtualmachines.vmoperator.vmware.com",
					specCELPath("network.interfaces.[]"),
					"ipamModes",
					false,
				)
			})

			It("vm service api should not have CEL rules referencing ipFamilies", func() {
				assertCELRulesContaining(
					"virtualmachineservices.vmoperator.vmware.com",
					specCELPath(""),
					"ipFamilies",
					false,
				)
			})

			DescribeTable("vm api should not have status fields",
				func(field string) {
					fields := statusFieldPath(field)
					assertField(false, fields...)
				},
				Entry("currentSnapshot", "currentSnapshot"),
				Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
				Entry("rootSnapshots", "rootSnapshots"),
				Entry("policies", "policies"),
				Entry("volumes diskMode", "volumes.[].diskMode"),
				Entry("volumes sharingMode", "volumes.[].sharingMode"),
				Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
				Entry("volumes controllerType", "volumes.[].controllerType"),
				Entry("hardware controllers", "hardware.controllers"),
				Entry("extraConfig", "extraConfig"),
			)
		})

		When("byok is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.BringYourOwnEncryptionKey = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, append(slices.Concat(basesNonGated, externalBYOK), storagePoliciesCRD)...)
			})
		})

		When("vSphere policies are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VSpherePolicies = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalVSpherePolicy)...)
			})

			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("policies", "policies"),
			)

			DescribeTable("vm api should have status fields",
				func(field string) {
					fields := statusFieldPath(field)
					assertField(true, fields...)
				},
				Entry("policies", "policies"),
			)
		})

		When("VMEviction is enabled without vSphere policies", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMEviction = true
				})
			})
			It("should not install the VM eviction policy crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, basesNonGated...)
			})
		})

		When("vSphere policies are enabled without VMEviction", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VSpherePolicies = true
				})
			})
			It("should not install the VM eviction policy crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalVSpherePolicy)...)
			})
		})

		When("vSphere policies and VMEviction are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VSpherePolicies = true
					config.Features.VMEviction = true
				})
			})
			It("should get the expected crds, including both VM eviction policy crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalVSpherePolicy, externalVMEviction)...)
			})
		})

		When("ControlledRebalancingPolicy is enabled without vSphere policies", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.ControlledRebalancingPolicy = true
				})
			})
			It("should not install the controlled rebalancing policy crd", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, basesNonGated...)
			})
		})

		When("vSphere policies are enabled without ControlledRebalancingPolicy", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VSpherePolicies = true
				})
			})
			It("should not install the controlled rebalancing policy crd", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalVSpherePolicy)...)
			})
		})

		When("vSphere policies and ControlledRebalancingPolicy are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VSpherePolicies = true
					config.Features.ControlledRebalancingPolicy = true
				})
			})
			It("should get the expected crds, including the controlled rebalancing policy crd", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalVSpherePolicy, externalControlledRebalancing)...)
			})
		})

		When("TaggingAPI is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.TaggingAPI = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalTagAPI)...)
			})
		})

		When("groups are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMGroups = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesVMGroups)...)
			})

			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("bootOptions", "bootOptions"),
				Entry("groupName", "groupName"),
			)

			It("vmgroup api should not have powerOffDelay without TelcoVMServiceAPI", func() {
				assertVMGroupField(false, specFieldPath("bootOrder.[].powerOffDelay")...)
			})
		})

		When("groups and TelcoVMServiceAPI are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMGroups = true
					config.Features.TelcoVMServiceAPI = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesVMGroups)...)
			})

			It("vmgroup api should have powerOffDelay", func() {
				assertVMGroupField(true, specFieldPath("bootOrder.[].powerOffDelay")...)
			})
		})

		When("K8sWorkloadMgmtAPI is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.K8sWorkloadMgmtAPI = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesK8sWorkloadMgmtAPI)...)
			})
		})

		When("snapshots are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMSnapshots = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesSnapshots)...)
			})

			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("currentSnapshotName", "currentSnapshotName"),
			)

			DescribeTable("vm api should have status fields",
				func(field string) {
					fields := statusFieldPath(field)
					assertField(true, fields...)
				},
				Entry("currentSnapshot", "currentSnapshot"),
				Entry("rootSnapshots", "rootSnapshots"),
			)
		})

		When("immutable classes are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.ImmutableClasses = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, basesImmutableClasses)...)
			})
			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("class", "class"),
			)
		})

		When("Guest customization VCD parity is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.GuestCustomizationVCDParity = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, basesNonGated...)
			})
			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("linuxPrep expire password", "bootstrap.linuxPrep.expirePasswordAfterNextLogin"),
				Entry("linuxPrep root password", "bootstrap.linuxPrep.password"),
				Entry("linuxPrep script text ", "bootstrap.linuxPrep.scriptText"),
				Entry("sysprep expire password", "bootstrap.sysprep.sysprep.expirePasswordAfterNextLogin"),
				Entry("sysprep script text", "bootstrap.sysprep.sysprep.scriptText"),
			)
		})

		When("VM extra config capability is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.TelcoVMServiceAPI = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, basesNonGated...)
			})
			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("advanced preferHtEnabled", "advanced.preferHtEnabled"),
				Entry("advanced hugePages1GEnabled", "advanced.hugePages1GEnabled"),
				Entry("advanced timeTrackerLowLatencyEnabled", "advanced.timeTrackerLowLatencyEnabled"),
				Entry("advanced cpuAffinityExclusiveNoStatsEnabled", "advanced.cpuAffinityExclusiveNoStatsEnabled"),
				Entry("advanced vmxSwapEnabled", "advanced.vmxSwapEnabled"),
				Entry("advanced pnumaNodeAffinity", "advanced.pnumaNodeAffinity"),
				Entry("advanced extraConfig", "advanced.extraConfig"),
				Entry("network interface type", "network.interfaces.[].type"),
				Entry("network interface vnumaNodeID", "network.interfaces.[].vnumaNodeID"),
				Entry("network interface vmxnet3", "network.interfaces.[].vmxnet3"),
				Entry("network interface advancedProperties", "network.interfaces.[].advancedProperties"),
				Entry("resources", "resources"),
				Entry("cpuAdvanced", "cpuAdvanced"),
				Entry("memoryAdvanced", "memoryAdvanced"),
			)
			DescribeTable("vm api should have status fields",
				func(field string) {
					fields := statusFieldPath(field)
					assertField(true, fields...)
				},
				Entry("extraConfig", "extraConfig"),
			)

			It("vm api should have CEL rules referencing vmxnet3", func() {
				assertCELRulesContaining(
					"virtualmachines.vmoperator.vmware.com",
					specCELPath("network.interfaces.[]"),
					"vmxnet3",
					true,
				)
			})
		})

		When("VM network unit numbers capability is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMNetworkUnitNumbers = true
				})
			})

			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
			)

			DescribeTable("vm api should have status fields",
				func(field string) {
					fields := statusFieldPath(field)
					assertField(true, fields...)
				},
				Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
			)
		})

		When("VM shared disks (OracleRAC) is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMSharedDisks = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, append(basesNonGated, storagePoliciesCRD)...)
			})
			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("ideControllers", "hardware.ideControllers"),
				Entry("nvmeControllers", "hardware.nvmeControllers"),
				Entry("sataControllers", "hardware.sataControllers"),
				Entry("scsiControllers", "hardware.scsiControllers"),
				Entry("cdrom's controllerBusNumber", "hardware.cdrom.[].controllerBusNumber"),
				Entry("cdrom's controllerType", "hardware.cdrom.[].controllerType"),
				Entry("cdrom's unitNumber", "hardware.cdrom.[].unitNumber"),
				Entry("volumes applicationType", "volumes.[].applicationType"),
				Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
				Entry("volumes controllerType", "volumes.[].controllerType"),
				Entry("volumes diskMode", "volumes.[].diskMode"),
				Entry("volumes sharingMode", "volumes.[].sharingMode"),
				Entry("volumes unitNumber", "volumes.[].unitNumber"),
			)
			DescribeTable("vm api should have status fields",
				func(field string) {
					fields := statusFieldPath(field)
					assertField(true, fields...)
				},
				Entry("volumes diskMode", "volumes.[].diskMode"),
				Entry("volumes sharingMode", "volumes.[].sharingMode"),
				Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
				Entry("volumes controllerType", "volumes.[].controllerType"),
				Entry("hardware controllers", "hardware.controllers"),
			)
		})

		When("AllDisksArePVCs is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.AllDisksArePVCs = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, append(basesNonGated, storagePoliciesCRD)...)
			})
			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("ideControllers", "hardware.ideControllers"),
				Entry("nvmeControllers", "hardware.nvmeControllers"),
				Entry("sataControllers", "hardware.sataControllers"),
				Entry("scsiControllers", "hardware.scsiControllers"),
				Entry("cdrom's controllerBusNumber", "hardware.cdrom.[].controllerBusNumber"),
				Entry("cdrom's controllerType", "hardware.cdrom.[].controllerType"),
				Entry("cdrom's unitNumber", "hardware.cdrom.[].unitNumber"),
				Entry("volumes applicationType", "volumes.[].applicationType"),
				Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
				Entry("volumes controllerType", "volumes.[].controllerType"),
				Entry("volumes diskMode", "volumes.[].diskMode"),
				Entry("volumes sharingMode", "volumes.[].sharingMode"),
				Entry("volumes unitNumber", "volumes.[].unitNumber"),
			)
			DescribeTable("vm api should have status fields",
				func(field string) {
					fields := statusFieldPath(field)
					assertField(true, fields...)
				},
				Entry("volumes diskMode", "volumes.[].diskMode"),
				Entry("volumes sharingMode", "volumes.[].sharingMode"),
				Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
				Entry("volumes controllerType", "volumes.[].controllerType"),
				Entry("hardware controllers", "hardware.controllers"),
			)
		})

		When("WorkloadIPv6 is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.WorkloadIPv6 = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, basesNonGated...)
			})
			DescribeTable("vm api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertField(true, fields...)
				},
				Entry("network interface ipamModes", "network.interfaces.[].ipamModes"),
			)
			DescribeTable("vm service api should have spec fields",
				func(field string) {
					fields := specFieldPath(field)
					assertVMSvcField(true, fields...)
				},
				Entry("ipFamilies", "ipFamilies"),
				Entry("ipFamilyPolicy", "ipFamilyPolicy"),
			)

			It("vm api should have CEL rules referencing ipamModes", func() {
				assertCELRulesContaining(
					"virtualmachines.vmoperator.vmware.com",
					specCELPath("network.interfaces.[]"),
					"ipamModes",
					true,
				)
			})

			It("vm service api should have CEL rules referencing ipFamilies", func() {
				assertCELRulesContaining(
					"virtualmachineservices.vmoperator.vmware.com",
					specCELPath(""),
					"ipFamilies",
					true,
				)
			})
		})

		When("fast deploy is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.FastDeploy = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, append(slices.Concat(basesNonGated, basesFastDeploy), storagePoliciesCRD)...)
			})
		})

		When("the environment browser is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VirtualMachineConfigPolicy = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesNonGated, externalEnvironmentBrowser)...)
			})
		})

		When("all features are enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.FastDeploy = true
					config.Features.ImmutableClasses = true
					config.Features.VMGroups = true
					config.Features.VMSnapshots = true
					config.Features.VSpherePolicies = true
					config.Features.VMEviction = true
					config.Features.ControlledRebalancingPolicy = true
					config.Features.BringYourOwnEncryptionKey = true
					config.Features.GuestCustomizationVCDParity = true
					config.Features.TelcoVMServiceAPI = true
					config.Features.VirtualMachineConfigPolicy = true
					config.Features.K8sWorkloadMgmtAPI = true
					config.Features.TaggingAPI = true
					config.Features.VMNetworkUnitNumbers = true
				})
			})
			It("should get the expected crds", func() {
				var obj apiextensionsv1.CustomResourceDefinitionList
				Expect(client.List(ctx, &obj)).To(Succeed())
				assertCRDsConsistOf(obj.Items, slices.Concat(basesAll, externalAll)...)
			})
		})
	})

	When("crds were already installed with caps enabled and with conversion info", func() {
		var (
			crc apiextensionsv1.CustomResourceConversion
		)

		BeforeEach(func() {

			crc = apiextensionsv1.CustomResourceConversion{
				Strategy: apiextensionsv1.WebhookConverter,
				Webhook: &apiextensionsv1.WebhookConversion{
					ConversionReviewVersions: []string{"v1"},
					ClientConfig: &apiextensionsv1.WebhookClientConfig{
						URL: ptr.To("http://127.0.0.1"),
						Service: &apiextensionsv1.ServiceReference{
							Namespace: "default",
							Name:      "webhook",
							Path:      ptr.To("/convert"),
							Port:      ptr.To(int32(443)),
						},
					},
				},
			}

			Expect(pkgcrd.Install(
				pkgcfg.WithConfig(pkgcfg.Config{
					Features: pkgcfg.FeatureStates{
						FastDeploy:                  true,
						ImmutableClasses:            true,
						VMGroups:                    true,
						VMSnapshots:                 true,
						VSpherePolicies:             true,
						VMEviction:                  true,
						ControlledRebalancingPolicy: true,
						BringYourOwnEncryptionKey:   true,
						VirtualMachineConfigPolicy:  true,
						K8sWorkloadMgmtAPI:          true,
						TaggingAPI:                  true,
						VMNetworkUnitNumbers:        true,
					},
				}),
				client,
				func(kind string, obj *unstructured.Unstructured) error {
					if err := unstructured.SetNestedMap(
						obj.Object,
						map[string]any{
							"strategy": string(crc.Strategy),
							"webhook": map[string]any{
								"clientConfig": map[string]any{
									"url": *crc.Webhook.ClientConfig.URL,
									"service": map[string]any{
										"namespace": crc.Webhook.ClientConfig.Service.Namespace,
										"name":      crc.Webhook.ClientConfig.Service.Name,
										"path":      *crc.Webhook.ClientConfig.Service.Path,
									},
								},
							},
						},
						"spec",
						"conversion"); err != nil {
						return err
					}

					if err := unstructured.SetNestedStringSlice(
						obj.Object,
						crc.Webhook.ConversionReviewVersions,
						"spec",
						"conversion",
						"webhook",
						"conversionReviewVersions"); err != nil {
						return err
					}

					if err := unstructured.SetNestedField(
						obj.Object,
						int64(*crc.Webhook.ClientConfig.Service.Port),
						"spec",
						"conversion",
						"webhook",
						"clientConfig",
						"service",
						"port"); err != nil {
						return err
					}

					return nil

				})).To(Succeed())

			// Verify the CRDs were installed.
			var obj apiextensionsv1.CustomResourceDefinitionList
			Expect(client.List(ctx, &obj)).To(Succeed())
			assertCRDsConsistOf(obj.Items, slices.Concat(basesAll, externalAll)...)
			for i := range obj.Items {
				ExpectWithOffset(1, obj.Items[i].Spec.Conversion).ToNot(BeNil())
				ExpectWithOffset(1, *obj.Items[i].Spec.Conversion).To(Equal(crc))
			}
		})

		When("CRD cleanup is disabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.CRDCleanupEnabled = false
				})
			})

			When("no capabilities are enabled", func() {
				It("should get the expected crds", func() {
					var obj apiextensionsv1.CustomResourceDefinitionList
					Expect(client.List(ctx, &obj)).To(Succeed())
					assertCRDsConsistOf(obj.Items, slices.Concat(basesAll, externalAll)...)
					for i := range obj.Items {
						ExpectWithOffset(1, obj.Items[i].Spec.Conversion).ToNot(BeNil())
						ExpectWithOffset(1, *obj.Items[i].Spec.Conversion).To(Equal(crc))
					}
				})

				DescribeTable("vm api should have removed spec fields",
					func(field string) {
						fields := specFieldPath(field)
						assertField(true, fields...)
					},
					Entry("bootOptions", "bootOptions"),
					Entry("class", "class"),
					Entry("currentSnapshotName", "currentSnapshotName"),
					Entry("groupName", "groupName"),
					Entry("policies", "policies"),
					Entry("linuxPrep expire password", "bootstrap.linuxPrep.expirePasswordAfterNextLogin"),
					Entry("linuxPrep root password", "bootstrap.linuxPrep.password"),
					Entry("linuxPrep script text ", "bootstrap.linuxPrep.scriptText"),
					Entry("sysprep expire password", "bootstrap.sysprep.sysprep.expirePasswordAfterNextLogin"),
					Entry("sysprep script text", "bootstrap.sysprep.sysprep.scriptText"),
					Entry("ideControllers", "hardware.ideControllers"),
					Entry("nvmeControllers", "hardware.nvmeControllers"),
					Entry("sataControllers", "hardware.sataControllers"),
					Entry("scsiControllers", "hardware.scsiControllers"),
					Entry("cdrom's controllerBusNumber", "hardware.cdrom.[].controllerBusNumber"),
					Entry("cdrom's controllerType", "hardware.cdrom.[].controllerType"),
					Entry("cdrom's unitNumber", "hardware.cdrom.[].unitNumber"),
					Entry("volumes applicationType", "volumes.[].applicationType"),
					Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
					Entry("volumes controllerType", "volumes.[].controllerType"),
					Entry("volumes diskMode", "volumes.[].diskMode"),
					Entry("volumes sharingMode", "volumes.[].sharingMode"),
					Entry("volumes unitNumber", "volumes.[].unitNumber"),
					Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
				)

				DescribeTable("vm api should have removed status fields",
					func(field string) {
						fields := statusFieldPath(field)
						assertField(true, fields...)
					},
					Entry("currentSnapshot", "currentSnapshot"),
					Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
					Entry("rootSnapshots", "rootSnapshots"),
					Entry("policies", "policies"),
					Entry("volumes diskMode", "volumes.[].diskMode"),
					Entry("volumes sharingMode", "volumes.[].sharingMode"),
					Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
					Entry("volumes controllerType", "volumes.[].controllerType"),
					Entry("hardware controllers", "hardware.controllers"),
				)
			})
		})

		When("CRD cleanup is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.CRDCleanupEnabled = true
				})
			})
			When("no capabilities are enabled", func() {
				It("should get the expected crds", func() {
					var obj apiextensionsv1.CustomResourceDefinitionList
					Expect(client.List(ctx, &obj)).To(Succeed())
					assertCRDsConsistOf(obj.Items, basesNonGated...)
					for i := range obj.Items {
						ExpectWithOffset(1, obj.Items[i].Spec.Conversion).ToNot(BeNil())
						ExpectWithOffset(1, *obj.Items[i].Spec.Conversion).To(Equal(crc))
					}
				})

				DescribeTable("vm api should have removed spec fields",
					func(field string) {
						fields := specFieldPath(field)
						assertField(false, fields...)
					},
					Entry("bootOptions", "bootOptions"),
					Entry("class", "class"),
					Entry("currentSnapshotName", "currentSnapshotName"),
					Entry("groupName", "groupName"),
					Entry("policies", "policies"),
					Entry("linuxPrep expire password", "bootstrap.linuxPrep.expirePasswordAfterNextLogin"),
					Entry("linuxPrep root password", "bootstrap.linuxPrep.password"),
					Entry("linuxPrep script text ", "bootstrap.linuxPrep.scriptText"),
					Entry("sysprep expire password", "bootstrap.sysprep.sysprep.expirePasswordAfterNextLogin"),
					Entry("sysprep script text", "bootstrap.sysprep.sysprep.scriptText"),
					Entry("ideControllers", "hardware.ideControllers"),
					Entry("nvmeControllers", "hardware.nvmeControllers"),
					Entry("sataControllers", "hardware.sataControllers"),
					Entry("scsiControllers", "hardware.scsiControllers"),
					Entry("cdrom's controllerBusNumber", "hardware.cdrom.[].controllerBusNumber"),
					Entry("cdrom's controllerType", "hardware.cdrom.[].controllerType"),
					Entry("cdrom's unitNumber", "hardware.cdrom.[].unitNumber"),
					Entry("volumes applicationType", "volumes.[].applicationType"),
					Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
					Entry("volumes controllerType", "volumes.[].controllerType"),
					Entry("volumes diskMode", "volumes.[].diskMode"),
					Entry("volumes sharingMode", "volumes.[].sharingMode"),
					Entry("volumes unitNumber", "volumes.[].unitNumber"),
					Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
					Entry("advanced preferHtEnabled", "advanced.preferHtEnabled"),
					Entry("advanced hugePages1GEnabled", "advanced.hugePages1GEnabled"),
					Entry("advanced timeTrackerLowLatencyEnabled", "advanced.timeTrackerLowLatencyEnabled"),
					Entry("advanced cpuAffinityExclusiveNoStatsEnabled", "advanced.cpuAffinityExclusiveNoStatsEnabled"),
					Entry("advanced vmxSwapEnabled", "advanced.vmxSwapEnabled"),
					Entry("advanced pnumaNodeAffinity", "advanced.pnumaNodeAffinity"),
					Entry("advanced extraConfig", "advanced.extraConfig"),
					Entry("network interface type", "network.interfaces.[].type"),
					Entry("network interface vnumaNodeID", "network.interfaces.[].vnumaNodeID"),
					Entry("network interface vmxnet3", "network.interfaces.[].vmxnet3"),
					Entry("network interface advancedProperties", "network.interfaces.[].advancedProperties"),
				)

				DescribeTable("vm api should have removed status fields",
					func(field string) {
						fields := statusFieldPath(field)
						assertField(false, fields...)
					},
					Entry("currentSnapshot", "currentSnapshot"),
					Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
					Entry("rootSnapshots", "rootSnapshots"),
					Entry("policies", "policies"),
					Entry("volumes diskMode", "volumes.[].diskMode"),
					Entry("volumes sharingMode", "volumes.[].sharingMode"),
					Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
					Entry("volumes controllerType", "volumes.[].controllerType"),
					Entry("hardware controllers", "hardware.controllers"),
					Entry("extraConfig", "extraConfig"),
				)

				When("one of the crds is already deleted", func() {
					BeforeEach(func() {
						obj := apiextensionsv1.CustomResourceDefinition{
							ObjectMeta: metav1.ObjectMeta{
								Name: "virtualmachinegroups.vmoperator.vmware.com",
							},
						}
						Expect(client.Delete(ctx, &obj)).To(Succeed())
					})

					It("should get the expected crds", func() {
						var obj apiextensionsv1.CustomResourceDefinitionList
						Expect(client.List(ctx, &obj)).To(Succeed())
						assertCRDsConsistOf(obj.Items, basesNonGated...)
						for i := range obj.Items {
							ExpectWithOffset(1, obj.Items[i].Spec.Conversion).ToNot(BeNil())
							ExpectWithOffset(1, *obj.Items[i].Spec.Conversion).To(Equal(crc))
						}
					})

					DescribeTable("vm api should have removed spec fields",
						func(field string) {
							fields := specFieldPath(field)
							assertField(false, fields...)
						},
						Entry("bootOptions", "bootOptions"),
						Entry("class", "class"),
						Entry("currentSnapshotName", "currentSnapshotName"),
						Entry("groupName", "groupName"),
						Entry("policies", "policies"),
						Entry("linuxPrep expire password", "bootstrap.linuxPrep.expirePasswordAfterNextLogin"),
						Entry("linuxPrep root password", "bootstrap.linuxPrep.password"),
						Entry("linuxPrep script text ", "bootstrap.linuxPrep.scriptText"),
						Entry("sysprep expire password", "bootstrap.sysprep.sysprep.expirePasswordAfterNextLogin"),
						Entry("sysprep script text", "bootstrap.sysprep.sysprep.scriptText"),
						Entry("ideControllers", "hardware.ideControllers"),
						Entry("nvmeControllers", "hardware.nvmeControllers"),
						Entry("sataControllers", "hardware.sataControllers"),
						Entry("scsiControllers", "hardware.scsiControllers"),
						Entry("cdrom's controllerBusNumber", "hardware.cdrom.[].controllerBusNumber"),
						Entry("cdrom's controllerType", "hardware.cdrom.[].controllerType"),
						Entry("cdrom's unitNumber", "hardware.cdrom.[].unitNumber"),
						Entry("volumes applicationType", "volumes.[].applicationType"),
						Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
						Entry("volumes controllerType", "volumes.[].controllerType"),
						Entry("volumes diskMode", "volumes.[].diskMode"),
						Entry("volumes sharingMode", "volumes.[].sharingMode"),
						Entry("volumes unitNumber", "volumes.[].unitNumber"),
						Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
						Entry("advanced preferHtEnabled", "advanced.preferHtEnabled"),
						Entry("advanced hugePages1GEnabled", "advanced.hugePages1GEnabled"),
						Entry("advanced timeTrackerLowLatencyEnabled", "advanced.timeTrackerLowLatencyEnabled"),
						Entry("advanced cpuAffinityExclusiveNoStatsEnabled", "advanced.cpuAffinityExclusiveNoStatsEnabled"),
						Entry("advanced vmxSwapEnabled", "advanced.vmxSwapEnabled"),
						Entry("advanced pnumaNodeAffinity", "advanced.pnumaNodeAffinity"),
						Entry("advanced extraConfig", "advanced.extraConfig"),
						Entry("network interface type", "network.interfaces.[].type"),
						Entry("network interface vnumaNodeID", "network.interfaces.[].vnumaNodeID"),
						Entry("network interface vmxnet3", "network.interfaces.[].vmxnet3"),
						Entry("network interface advancedProperties", "network.interfaces.[].advancedProperties"),
					)

					DescribeTable("vm api should have removed status fields",
						func(field string) {
							fields := statusFieldPath(field)
							assertField(false, fields...)
						},
						Entry("currentSnapshot", "currentSnapshot"),
						Entry("network interface unitNumber", "network.interfaces.[].unitNumber"),
						Entry("rootSnapshots", "rootSnapshots"),
						Entry("policies", "policies"),
						Entry("volumes diskMode", "volumes.[].diskMode"),
						Entry("volumes sharingMode", "volumes.[].sharingMode"),
						Entry("volumes controllerBusNumber", "volumes.[].controllerBusNumber"),
						Entry("volumes controllerType", "volumes.[].controllerType"),
						Entry("hardware controllers", "hardware.controllers"),
						Entry("extraConfig", "extraConfig"),
					)
				})
			})
		})
	})
})

func specFieldPath(fieldPath string) []string {
	fieldNames := strings.Split(fieldPath, ".")
	return buildFieldPath("spec", fieldNames...)
}

func statusFieldPath(fieldPath string) []string {
	fieldNames := strings.Split(fieldPath, ".")
	return buildFieldPath("status", fieldNames...)
}

// specCELPath returns the path to x-kubernetes-validations for a schema object
// relative to a CRD version entry. An empty fieldPath returns the spec-level
// validations path; otherwise fieldPath is a dot-separated chain where "[]"
// denotes array items (e.g. "network.interfaces.[]").
func specCELPath(fieldPath string) []string {
	if fieldPath == "" {
		return []string{"schema", "openAPIV3Schema", "properties", "spec", "x-kubernetes-validations"}
	}
	fieldNames := strings.Split(fieldPath, ".")
	path := buildFieldPath("spec", fieldNames...)
	return append(path, "x-kubernetes-validations")
}

func buildFieldPath(parentField string, fieldNames ...string) []string {
	result := []string{"schema", "openAPIV3Schema", "properties", parentField, "properties"}
	result = append(result, fieldNames[0])
	for _, name := range fieldNames[1:] {
		// Use "[]" to indicate previous element is an array type.
		if name == "[]" {
			result = append(result, "items")
			continue
		}
		result = append(result, "properties", name)
	}
	return result
}

// These tests apply actual AutomaticVMEvictionPolicy/BestEffortRestartPolicy
// resources to a real kube-apiserver to verify the generated CRDs' kubebuilder
// markers (required fields, string length bounds, defaulting, status
// subresource) are enforced. The fake client used elsewhere skips OpenAPI
// schema validation entirely, so only a real apiserver can catch a marker
// that was dropped or miscopied during code generation.
var _ = Describe(
	"AutomaticVMEvictionPolicy and BestEffortRestartPolicy schema",
	Label(testlabels.EnvTest),
	func() {

		var (
			ctx       context.Context
			client    ctrlclient.Client
			namespace string
		)

		BeforeEach(func() {
			ctx = pkgcfg.WithConfig(pkgcfg.Config{
				CRDCleanupEnabled: true,
				Features: pkgcfg.FeatureStates{
					VSpherePolicies: true,
					VMEviction:      true,
				},
			})
			Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())

			// The apiserver establishes a newly created CRD's REST endpoint
			// asynchronously. The typed client built below defaults to a
			// dynamic RESTMapper that discovers GVK-to-resource mappings
			// from the apiserver, so building it before the CRD is
			// Established can race ahead and miss the new resource. This
			// is unrelated to AddToScheme below, which only registers Go
			// types in-memory and never talks to the apiserver.
			for _, crdName := range []string{
				"automaticvmevictionpolicies.vsphere.policy.vmware.com",
				"besteffortrestartpolicies.vsphere.policy.vmware.com",
			} {
				Eventually(func(g Gomega) {
					crd := &apiextensionsv1.CustomResourceDefinition{}
					g.Expect(envTestClient.Get(
						ctx,
						ctrlclient.ObjectKey{Name: crdName},
						crd)).To(Succeed())

					established := false
					for _, cond := range crd.Status.Conditions {
						if cond.Type == apiextensionsv1.Established &&
							cond.Status == apiextensionsv1.ConditionTrue {
							established = true
						}
					}
					g.Expect(established).To(BeTrue())
				}).Should(Succeed())
			}

			scheme := runtime.NewScheme()
			Expect(vspherepolv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			var err error
			client, err = ctrlclient.New(envTestEnv.Config, ctrlclient.Options{Scheme: scheme})
			Expect(err).ToNot(HaveOccurred())

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "vmevacuation-crd-test-"},
			}
			Expect(client.Create(ctx, ns)).To(Succeed())
			namespace = ns.Name
		})

		newAutomaticVMEvictionPolicy := func(policyID string) *vspherepolv1.AutomaticVMEvictionPolicy {
			return &vspherepolv1.AutomaticVMEvictionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "auto-host-evac-",
					Namespace:    namespace,
				},
				Spec: vspherepolv1.AutomaticVMEvictionPolicySpec{
					PolicyID: policyID,
				},
			}
		}

		newBestEffortRestartPolicy := func(policyID string) *vspherepolv1.BestEffortRestartPolicy {
			return &vspherepolv1.BestEffortRestartPolicy{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "best-effort-restart-",
					Namespace:    namespace,
				},
				Spec: vspherepolv1.BestEffortRestartPolicySpec{
					PolicyID: policyID,
				},
			}
		}

		It("should accept a valid AutomaticVMEvictionPolicy and default enforcementMode", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			Expect(client.Create(ctx, obj)).To(Succeed())
			Expect(obj.Spec.EnforcementMode).To(Equal(vspherepolv1.PolicyEnforcementModeMandatory))
		})

		It("should accept a valid BestEffortRestartPolicy and default enforcementMode", func() {
			obj := newBestEffortRestartPolicy("policy-1")
			Expect(client.Create(ctx, obj)).To(Succeed())
			Expect(obj.Spec.EnforcementMode).To(Equal(vspherepolv1.PolicyEnforcementModeMandatory))
		})

		It("should reject an AutomaticVMEvictionPolicy missing policyID", func() {
			obj := newAutomaticVMEvictionPolicy("")
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject an AutomaticVMEvictionPolicy with a policyID over 64 characters", func() {
			obj := newAutomaticVMEvictionPolicy(strings.Repeat("a", 65))
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject a BestEffortRestartPolicy with a policyID over 64 characters", func() {
			obj := newBestEffortRestartPolicy(strings.Repeat("a", 65))
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject an AutomaticVMEvictionPolicy with a description over 1024 characters", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			obj.Spec.Description = strings.Repeat("a", 1025)
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject an invalid enforcementMode value", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			obj.Spec.EnforcementMode = "NotAValidMode"
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should ignore status updates submitted through the main endpoint", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			Expect(client.Create(ctx, obj)).To(Succeed())

			obj.Status.ObservedGeneration = 42
			Expect(client.Update(ctx, obj)).To(Succeed())

			fetched := &vspherepolv1.AutomaticVMEvictionPolicy{}
			Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), fetched)).To(Succeed())
			Expect(fetched.Status.ObservedGeneration).To(BeZero())
		})

		It("should persist status conditions written through the status subresource", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			Expect(client.Create(ctx, obj)).To(Succeed())

			obj.Status.Conditions = []metav1.Condition{
				{
					Type:               vspherepolv1.ReadyConditionType,
					Status:             metav1.ConditionTrue,
					Reason:             "Ready",
					Message:            "",
					LastTransitionTime: metav1.Now(),
					ObservedGeneration: obj.Generation,
				},
			}
			Expect(client.Status().Update(ctx, obj)).To(Succeed())

			fetched := &vspherepolv1.AutomaticVMEvictionPolicy{}
			Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), fetched)).To(Succeed())
			Expect(fetched.Status.Conditions).To(HaveLen(1))
			Expect(fetched.Status.Conditions[0].Type).To(Equal(vspherepolv1.ReadyConditionType))
		})
	},
)

// These tests apply mutated CRD schemas to a real kube-apiserver and verify
// acceptance. The fake-client unit tests cannot catch CEL compilation errors
// since the fake client skips schema validation — only a real apiserver runs
// the CEL type-checker at CRD apply time.
var _ = Describe(
	"Install against real API server",
	Label(testlabels.EnvTest),
	func() {
		DescribeTable("removes an installed policy CRD and its objects",
			func(enabled bool) {
				ctx := pkgcfg.WithConfig(pkgcfg.Config{
					Features: pkgcfg.FeatureStates{VirtualMachineConfigPolicy: enabled},
				})
				obsolete := &apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "virtualmachineconfigpolicies.vim.vmware.com"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Group: "vim.vmware.com",
						Names: apiextensionsv1.CustomResourceDefinitionNames{
							Plural: "virtualmachineconfigpolicies", Kind: "VirtualMachineConfigPolicy",
						},
						Scope: apiextensionsv1.NamespaceScoped,
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
							Name: "v1alpha1", Served: true, Storage: true,
							Schema: &apiextensionsv1.CustomResourceValidation{
								OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object"},
							},
						}},
					},
				}
				Expect(envTestClient.Create(ctx, obsolete)).To(Succeed())
				DeferCleanup(func() {
					Expect(ctrlclient.IgnoreNotFound(envTestClient.Delete(ctx, obsolete))).To(Succeed())
				})
				policy := &unstructured.Unstructured{}
				policy.SetAPIVersion("vim.vmware.com/v1alpha1")
				policy.SetKind("VirtualMachineConfigPolicy")
				policy.SetName("upgrade-policy")
				policy.SetNamespace("default")
				Eventually(func() error {
					return envTestClient.Create(ctx, policy)
				}).Should(Succeed())

				Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
				Eventually(func() bool {
					return apierrors.IsNotFound(envTestClient.Get(ctx,
						ctrlclient.ObjectKeyFromObject(obsolete), obsolete))
				}).WithTimeout(30 * time.Second).Should(BeTrue())
				// Re-installation must not recreate the removed API.
				Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
				Expect(apierrors.IsNotFound(envTestClient.Get(ctx,
					ctrlclient.ObjectKeyFromObject(obsolete), obsolete))).To(BeTrue())
				if enabled {
					for _, name := range externalEnvironmentBrowser {
						var crd apiextensionsv1.CustomResourceDefinition
						Expect(envTestClient.Get(ctx, ctrlclient.ObjectKey{Name: name}, &crd)).To(Succeed())
					}
				}
			},
			Entry("with the environment browser enabled", true),
			Entry("with the environment browser disabled", false),
		)

		DescribeTable("should skip writes after API server defaulting and convergence",
			func(cleanup, enabled bool) {
				ctx := pkgcfg.WithConfig(pkgcfg.Config{
					CRDCleanupEnabled: cleanup,
					Features:          featureStates(enabled),
				})
				if cleanup && !enabled {
					// Exercise deletion even when this entry runs in isolation.
					Expect(pkgcrd.Install(pkgcfg.WithConfig(pkgcfg.Config{
						Features: featureStates(true),
					}), envTestClient, nil)).To(Succeed())
				}
				Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
				if cleanup && !enabled {
					// CRD deletion is asynchronous. Wait for finalization before
					// checking that subsequent installs no longer issue deletes.
					Eventually(func(g Gomega) {
						var crds apiextensionsv1.CustomResourceDefinitionList
						g.Expect(envTestClient.List(ctx, &crds)).To(Succeed())
						for _, crd := range crds.Items {
							g.Expect(crd.DeletionTimestamp.IsZero()).To(BeTrue(),
								"CRD %s is still terminating", crd.Name)
						}
					}).WithTimeout(30 * time.Second).Should(Succeed())
				}
				if !cleanup {
					// Creation always removes disabled feature fields, so allow
					// one update to converge when cleanup is disabled.
					Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
				}
				recordingClient := &writeRecordingClient{Client: envTestClient}
				for range 2 {
					recordingClient.writes = nil
					Expect(pkgcrd.Install(ctx, recordingClient, nil)).To(Succeed())
					Expect(recordingClient.writes).To(BeEmpty())
				}
			},
			Entry("with cleanup and capabilities enabled", true, true),
			Entry("with cleanup enabled and capabilities disabled", true, false),
			Entry("with cleanup disabled and capabilities enabled", false, true),
			Entry("with cleanup and capabilities disabled", false, false),
		)

		It("should accept CRDs with all capabilities disabled", func() {
			ctx := pkgcfg.WithConfig(pkgcfg.Config{
				CRDCleanupEnabled: true,
				Features:          pkgcfg.FeatureStates{},
			})
			// The kube-apiserver will reject the CRD if any x-kubernetes-validations
			// entry references a field that was removed from the schema.
			Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
		})

		It("should accept CRDs with all capabilities enabled", func() {
			ctx := pkgcfg.WithConfig(pkgcfg.Config{
				CRDCleanupEnabled: true,
				Features:          featureStates(true),
			})
			Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
		})

		It("should accept CRDs when all features are enabled then disabled", func() {
			install := func(enabled bool) error {
				return pkgcrd.Install(
					pkgcfg.WithConfig(pkgcfg.Config{
						CRDCleanupEnabled: true,
						Features:          featureStates(enabled),
					}),
					envTestClient,
					nil)
			}
			// Reset to a clean state; earlier specs leave CRDs installed.
			// CRD presence checks are flaky and hence not doing those.
			Expect(install(false)).To(Succeed())
			Expect(install(true)).To(Succeed())
			Expect(install(false)).To(Succeed())
		})

		When("K8sWorkloadMgmtAPI is disabled after being enabled", func() {
			const replicaSetCRD = "virtualmachinereplicasets.vmoperator.vmware.com"

			newReplicaSetCR := func() *unstructured.Unstructured {
				return &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "vmoperator.vmware.com/v1alpha6",
						"kind":       "VirtualMachineReplicaSet",
						"metadata": map[string]any{
							"name":      "test-rs",
							"namespace": "default",
						},
						"spec": map[string]any{
							"replicas": int64(0),
							"selector": map[string]any{
								"matchLabels": map[string]any{"app": "test-rs"},
							},
						},
					},
				}
			}

			getReplicaSetCRD := func() error {
				var obj apiextensionsv1.CustomResourceDefinition
				return envTestClient.Get(
					context.Background(),
					ctrlclient.ObjectKey{Name: replicaSetCRD},
					&obj)
			}

			BeforeEach(func() {
				ctx := pkgcfg.WithConfig(pkgcfg.Config{
					Features: pkgcfg.FeatureStates{
						K8sWorkloadMgmtAPI: true,
					},
				})
				Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
				Eventually(getReplicaSetCRD).Should(Succeed())

				// Create a CR so the CRD is in use when it is disabled.
				Eventually(func() error {
					return envTestClient.Create(
						context.Background(), newReplicaSetCR())
				}).Should(Succeed())
			})

			AfterEach(func() {
				// Ignore errors since the CRD may have been deleted.
				_ = envTestClient.Delete(context.Background(), newReplicaSetCR())
			})

			It("should delete the CRD when cleanup is enabled", func() {
				ctx := pkgcfg.WithConfig(pkgcfg.Config{
					CRDCleanupEnabled: true,
					Features:          pkgcfg.FeatureStates{},
				})
				Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
				Eventually(func() bool {
					return apierrors.IsNotFound(getReplicaSetCRD())
				}).Should(BeTrue())
			})

			It("should keep the CRD when cleanup is disabled", func() {
				ctx := pkgcfg.WithConfig(pkgcfg.Config{
					CRDCleanupEnabled: false,
					Features:          pkgcfg.FeatureStates{},
				})
				Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
				Consistently(getReplicaSetCRD).Should(Succeed())
			})
		})
	},
)

// featureStates returns a FeatureStates with every bool field set to the
// given value.
func featureStates(enabled bool) pkgcfg.FeatureStates {
	var fs pkgcfg.FeatureStates
	v := reflect.ValueOf(&fs).Elem()
	for _, f := range v.Fields() {
		if f.Kind() == reflect.Bool {
			f.SetBool(enabled)
		}
	}
	fs.VirtualMachineConfigPolicy = true
	return fs
}

// writeRecordingClient records write attempts while preserving the underlying client's behavior.
type writeRecordingClient struct {
	ctrlclient.Client
	writes []string
}

func (c *writeRecordingClient) Create(
	ctx context.Context,
	obj ctrlclient.Object,
	opts ...ctrlclient.CreateOption,
) error {
	c.writes = append(c.writes, "create/"+obj.GetName())
	return c.Client.Create(ctx, obj, opts...)
}

func (c *writeRecordingClient) Update(
	ctx context.Context,
	obj ctrlclient.Object,
	opts ...ctrlclient.UpdateOption,
) error {
	c.writes = append(c.writes, "update/"+obj.GetName())
	return c.Client.Update(ctx, obj, opts...)
}

func (c *writeRecordingClient) Delete(
	ctx context.Context,
	obj ctrlclient.Object,
	opts ...ctrlclient.DeleteOption,
) error {
	c.writes = append(c.writes, "delete/"+obj.GetName())
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *writeRecordingClient) Patch(
	ctx context.Context,
	obj ctrlclient.Object,
	patch ctrlclient.Patch,
	opts ...ctrlclient.PatchOption,
) error {
	c.writes = append(c.writes, "patch/"+obj.GetName())
	return c.Client.Patch(ctx, obj, patch, opts...)
}
