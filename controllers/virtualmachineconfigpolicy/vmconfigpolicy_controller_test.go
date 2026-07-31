// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineconfigpolicy_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineconfigpolicy"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/configtarget"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	testNamespace   = "dummy-ns"
	testZoneName    = "zone-1"
	testClusterMoID = "domain-c9"
)

var _ = Describe("VirtualMachineConfigPolicy Controller", Label(testlabels.Controller, testlabels.API), unitTests)

func unitTests() {
	var (
		ctx        context.Context
		k8sClient  ctrlclient.Client
		reconciler *virtualmachineconfigpolicy.Reconciler
		obj        *vimv1.VirtualMachineConfigPolicy
		zone       *topologyv1.Zone
		objReq     ctrl.Request

		withObjs []ctrlclient.Object
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()

		zone = &topologyv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: testZoneName, Namespace: testNamespace},
			Spec: topologyv1.ZoneSpec{
				ManagedVMs: topologyv1.VSphereEntityInfo{
					ClusterMoIDs: []string{testClusterMoID},
				},
			},
		}

		obj = &vimv1.VirtualMachineConfigPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: testZoneName, Namespace: testNamespace},
			Spec: vimv1.VirtualMachineConfigPolicySpec{
				Zone:       testZoneName,
				SyncMode:   vimv1.VirtualMachineConfigPolicySyncModeConfigTarget,
				CreateMode: vimv1.VirtualMachineConfigPolicyModeDeny,
			},
		}
		objReq = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testZoneName}}

		withObjs = []ctrlclient.Object{zone, obj}
	})

	JustBeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(vimv1.AddToScheme(scheme)).To(Succeed())
		Expect(topologyv1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&vimv1.VirtualMachineConfigPolicy{}).
			WithObjects(withObjs...).
			Build()

		reconciler = virtualmachineconfigpolicy.NewReconciler(
			ctx,
			k8sClient,
			log.Log.WithName("virtualmachineconfigpolicy"),
			record.New(events.NewFakeRecorder(100)))
	})

	getPolicy := func() *vimv1.VirtualMachineConfigPolicy {
		var got vimv1.VirtualMachineConfigPolicy
		Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &got)).To(Succeed())

		return &got
	}

	configTarget := func(clusterMoID string, numCPUCores int32, maxMem string) *vimv1.ConfigTarget {
		ct := &vimv1.ConfigTarget{
			ObjectMeta: metav1.ObjectMeta{Name: clusterMoID},
			Spec:       vimv1.ConfigTargetSpec{ID: vimv1.ManagedObjectID{ID: clusterMoID}},
			Status: vimv1.ConfigTargetStatus{
				NumCPUCores:     numCPUCores,
				SupportedMaxMem: resourceQuantityPtr(maxMem),
				SEVSupported:    true,
			},
		}
		pkgcond.MarkTrue(ct, vimv1.ReadyConditionType)

		return ct
	}

	// notReadyConfigTarget returns a ConfigTarget that exists but has not
	// finished populating its status (Ready!=True) -- e.g. the configtarget
	// controller has not reconciled it yet. Its numeric fields are left at
	// their Go zero value, which the sync must not treat as "the cluster
	// supports nothing."
	notReadyConfigTarget := func(clusterMoID string) *vimv1.ConfigTarget {
		return &vimv1.ConfigTarget{
			ObjectMeta: metav1.ObjectMeta{Name: clusterMoID},
			Spec:       vimv1.ConfigTargetSpec{ID: vimv1.ManagedObjectID{ID: clusterMoID}},
		}
	}

	When("the VirtualMachineConfigPolicy does not exist", func() {
		BeforeEach(func() {
			withObjs = []ctrlclient.Object{zone}
		})

		It("returns without error", func() {
			result, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	When("spec.syncMode is Disabled", func() {
		BeforeEach(func() {
			obj.Spec.SyncMode = vimv1.VirtualMachineConfigPolicySyncModeDisabled
			obj.Spec.NumCPUCores = &vimv1.IntRange{Min: 1, Max: 2}
			withObjs = []ctrlclient.Object{zone, obj}
		})

		It("sets Ready=True with reason SyncDisabled and does not modify spec", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(pkgcond.IsTrue(got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(pkgcond.GetReason(got, vimv1.ReadyConditionType)).To(Equal(virtualmachineconfigpolicy.SyncDisabledReason))
			Expect(got.Spec.NumCPUCores).To(Equal(&vimv1.IntRange{Min: 1, Max: 2}))
		})
	})

	When("spec.zone references a non-existent Zone", func() {
		BeforeEach(func() {
			obj.Spec.Zone = "does-not-exist"
			withObjs = []ctrlclient.Object{zone, obj}
		})

		It("sets Ready=False with reason ZoneNotFound", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(pkgcond.IsFalse(got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(pkgcond.GetReason(got, vimv1.ReadyConditionType)).To(Equal(virtualmachineconfigpolicy.ZoneNotFoundReason))
		})
	})

	When("the zone has no cluster to sync from", func() {
		BeforeEach(func() {
			zone.Spec.ManagedVMs.ClusterMoIDs = nil
		})

		It("sets Ready=False with reason ConfigTargetNotFound", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(pkgcond.IsFalse(got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(pkgcond.GetReason(got, vimv1.ReadyConditionType)).To(Equal(virtualmachineconfigpolicy.ConfigTargetNotFoundReason))
			Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))
		})
	})

	When("the zone's cluster has no matching ConfigTarget", func() {
		It("sets Ready=False with reason ConfigTargetNotReady and does not touch spec", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(pkgcond.IsFalse(got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(pkgcond.GetReason(got, vimv1.ReadyConditionType)).To(Equal(virtualmachineconfigpolicy.ConfigTargetNotReadyReason))
			Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))
		})
	})

	When("the zone's ConfigTarget exists but has not finished populating its status", func() {
		BeforeEach(func() {
			withObjs = []ctrlclient.Object{zone, obj, notReadyConfigTarget(testClusterMoID)}
		})

		It("sets Ready=False with reason ConfigTargetNotReady and does not merge its zero-valued fields", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(pkgcond.IsFalse(got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(pkgcond.GetReason(got, vimv1.ReadyConditionType)).To(Equal(virtualmachineconfigpolicy.ConfigTargetNotReadyReason))
			Expect(got.Spec.NumCPUCores).To(BeNil(), "a not-yet-Ready ConfigTarget's zero fields must not be published as capability denials")
		})
	})

	When("a multi-cluster zone has one Ready ConfigTarget and one not yet Ready", func() {
		const secondClusterMoID = "domain-c10"

		BeforeEach(func() {
			zone.Spec.ManagedVMs.ClusterMoIDs = []string{testClusterMoID, secondClusterMoID}
			withObjs = []ctrlclient.Object{
				zone, obj,
				configTarget(testClusterMoID, 8, "64Gi"),
				notReadyConfigTarget(secondClusterMoID),
			}
		})

		It("does not sync from the single Ready target -- that would be wider than the true intersection", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(pkgcond.IsFalse(got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(pkgcond.GetReason(got, vimv1.ReadyConditionType)).To(Equal(virtualmachineconfigpolicy.ConfigTargetNotReadyReason))
			Expect(got.Spec.NumCPUCores).To(BeNil())
		})
	})

	When("a single ConfigTarget exists for the zone's cluster", func() {
		BeforeEach(func() {
			withObjs = []ctrlclient.Object{zone, obj, configTarget(testClusterMoID, 8, "64Gi")}
		})

		It("copies its capacity limits into spec and sets Ready=True", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(pkgcond.IsTrue(got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(got.Spec.NumCPUCores).ToNot(BeNil())
			Expect(got.Spec.NumCPUCores.Max).To(Equal(int32(8)))
			Expect(got.Spec.Memory.Max.Equal(resource.MustParse("64Gi"))).To(BeTrue())
			Expect(got.Spec.SEVSupported).To(BeTrue())
			Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))

			// Non-ConfigTarget-derived fields are untouched.
			Expect(got.Spec.CreateMode).To(Equal(vimv1.VirtualMachineConfigPolicyModeDeny))
		})

		It("does not bump resourceVersion on a second reconcile against an unchanged ConfigTarget", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			rv1 := getPolicy().ResourceVersion

			_, err = reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			rv2 := getPolicy().ResourceVersion
			Expect(rv2).To(Equal(rv1), "reconciling an unchanged ConfigTarget must not re-patch spec")
		})
	})

	When("two ConfigTargets exist for a multi-cluster zone", func() {
		const secondClusterMoID = "domain-c10"

		BeforeEach(func() {
			zone.Spec.ManagedVMs.ClusterMoIDs = []string{testClusterMoID, secondClusterMoID}
			withObjs = []ctrlclient.Object{
				zone, obj,
				configTarget(testClusterMoID, 8, "64Gi"),
				configTarget(secondClusterMoID, 4, "128Gi"),
			}
		})

		It("intersects to the minimum of the per-cluster maxima", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(pkgcond.IsTrue(got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(got.Spec.NumCPUCores.Max).To(Equal(int32(4)))
			Expect(got.Spec.Memory.Max.Equal(resource.MustParse("64Gi"))).To(BeTrue())
		})
	})

	When("toggling syncMode between ConfigTarget and Disabled", func() {
		BeforeEach(func() {
			withObjs = []ctrlclient.Object{zone, obj, configTarget(testClusterMoID, 8, "64Gi")}
		})

		It("converges correctly in both directions", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(getPolicy().Spec.NumCPUCores.Max).To(Equal(int32(8)))

			toDisable := getPolicy()
			toDisable.Spec.SyncMode = vimv1.VirtualMachineConfigPolicySyncModeDisabled
			toDisable.Spec.NumCPUCores.Max = 999
			Expect(k8sClient.Update(ctx, toDisable)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			disabled := getPolicy()
			Expect(pkgcond.GetReason(disabled, vimv1.ReadyConditionType)).To(Equal(virtualmachineconfigpolicy.SyncDisabledReason))
			Expect(disabled.Spec.NumCPUCores.Max).To(Equal(int32(999)), "spec must not change while sync is disabled")

			toEnable := getPolicy()
			toEnable.Spec.SyncMode = vimv1.VirtualMachineConfigPolicySyncModeConfigTarget
			Expect(k8sClient.Update(ctx, toEnable)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(getPolicy().Spec.NumCPUCores.Max).To(Equal(int32(8)), "re-enabling sync converges spec back to ConfigTarget's value")
		})
	})

	When("the policy has extraConfig and other non-ConfigTarget-derived fields set", func() {
		BeforeEach(func() {
			obj.Spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
				Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
					{Type: vimv1.MatchTypeFixed, Key: "some.key"},
				},
			}
			obj.Spec.LatencySensitivityLevels = []vimv1.LatencySensitivityLevel{vimv1.LatencySensitivityLevelHigh}
			withObjs = []ctrlclient.Object{zone, obj, configTarget(testClusterMoID, 8, "64Gi")}
		})

		It("preserves them across a sync", func() {
			_, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())

			got := getPolicy()
			Expect(got.Spec.ExtraConfig).To(Equal(obj.Spec.ExtraConfig))
			Expect(got.Spec.LatencySensitivityLevels).To(Equal(obj.Spec.LatencySensitivityLevels))
		})
	})
}

func resourceQuantityPtr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

var _ = Describe("VirtualMachineConfigPolicy Controller against vcsim",
	Label(testlabels.Controller, testlabels.API, testlabels.EnvTest, testlabels.VCSim),
	vcsimTests)

func vcsimTests() {
	var (
		vcsimCtx    *builder.TestContextForVCSim
		reconciler  *virtualmachineconfigpolicy.Reconciler
		nsInfo      builder.WorkloadNamespaceInfo
		zoneName    string
		clusterMoID string
	)

	BeforeEach(func() {
		vcsimCtx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
		nsInfo = vcsimCtx.CreateWorkloadNamespace()
		zoneName = vcsimCtx.GetFirstZoneName()

		ccr := vcsimCtx.GetFirstClusterFromFirstZone()
		Expect(ccr).ToNot(BeNil())
		clusterMoID = ccr.Reference().Value

		reconciler = virtualmachineconfigpolicy.NewReconciler(
			vcsimCtx,
			vcsimCtx.Client,
			log.Log.WithName("virtualmachineconfigpolicy"),
			vcsimCtx.Recorder)
	})

	AfterEach(func() {
		vcsimCtx.AfterEach()
	})

	populateRealConfigTarget := func() {
		provider := vsphere.NewVSphereVMProviderFromClient(vcsimCtx, vcsimCtx.Client, vcsimCtx.Recorder)

		ct := &vimv1.ConfigTarget{
			ObjectMeta: metav1.ObjectMeta{Name: clusterMoID},
			Spec:       vimv1.ConfigTargetSpec{ID: vimv1.ManagedObjectID{ID: clusterMoID}},
		}
		Expect(vcsimCtx.Client.Create(vcsimCtx, ct)).To(Succeed())

		configTargetResult, descriptors, err := provider.GetVirtualMachineConfigTarget(vcsimCtx, clusterMoID)
		Expect(err).ToNot(HaveOccurred())

		Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(ct), ct)).To(Succeed())
		configtarget.PopulateStatus(ct, configTargetResult, descriptors)
		pkgcond.MarkTrue(ct, vimv1.ReadyConditionType)
		Expect(vcsimCtx.Client.Status().Update(vcsimCtx, ct)).To(Succeed())
	}

	When("a real, populated ConfigTarget exists for the zone's cluster", func() {
		var policy *vimv1.VirtualMachineConfigPolicy

		BeforeEach(func() {
			populateRealConfigTarget()

			policy = &vimv1.VirtualMachineConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: zoneName, Namespace: nsInfo.Namespace},
				Spec:       vimv1.VirtualMachineConfigPolicySpec{Zone: zoneName},
			}
			Expect(vcsimCtx.Client.Create(vcsimCtx, policy)).To(Succeed())
		})

		It("populates spec from the real cluster's capabilities and sets Ready=True", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: policy.Namespace, Name: policy.Name}}
			_, err := reconciler.Reconcile(vcsimCtx, req)
			Expect(err).ToNot(HaveOccurred())

			var got vimv1.VirtualMachineConfigPolicy
			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(policy), &got)).To(Succeed())
			Expect(pkgcond.IsTrue(&got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(got.Spec.NumCPUCores).ToNot(BeNil())
			Expect(got.Spec.NumCPUCores.Max).To(BeNumerically(">", 0))
		})

		It("does not bump resourceVersion on a second reconcile against a real, unchanged ConfigTarget", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: policy.Namespace, Name: policy.Name}}

			_, err := reconciler.Reconcile(vcsimCtx, req)
			Expect(err).ToNot(HaveOccurred())

			var got vimv1.VirtualMachineConfigPolicy
			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(policy), &got)).To(Succeed())
			rv1 := got.ResourceVersion

			_, err = reconciler.Reconcile(vcsimCtx, req)
			Expect(err).ToNot(HaveOccurred())

			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(policy), &got)).To(Succeed())
			Expect(got.ResourceVersion).To(Equal(rv1))
		})
	})

	When("no ConfigTarget exists yet for the zone's cluster", func() {
		var policy *vimv1.VirtualMachineConfigPolicy

		BeforeEach(func() {
			policy = &vimv1.VirtualMachineConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: zoneName, Namespace: nsInfo.Namespace},
				Spec:       vimv1.VirtualMachineConfigPolicySpec{Zone: zoneName},
			}
			Expect(vcsimCtx.Client.Create(vcsimCtx, policy)).To(Succeed())
		})

		It("sets Ready=False with reason ConfigTargetNotReady", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: policy.Namespace, Name: policy.Name}}
			_, err := reconciler.Reconcile(vcsimCtx, req)
			Expect(err).ToNot(HaveOccurred())

			var got vimv1.VirtualMachineConfigPolicy
			Expect(vcsimCtx.Client.Get(vcsimCtx, ctrlclient.ObjectKeyFromObject(policy), &got)).To(Succeed())
			Expect(pkgcond.IsFalse(&got, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(pkgcond.GetReason(&got, vimv1.ReadyConditionType)).To(Equal(virtualmachineconfigpolicy.ConfigTargetNotReadyReason))
		})
	})
}

// The Describe below runs against a real controller-runtime manager (via
// builder.NewIntegrationTestContextForVCSim), unlike the two above which
// call reconciler.Reconcile() directly. It exists specifically to exercise
// AddToManager's ConfigTarget/Zone watches and field-indexed mappers end to
// end: a direct-Reconcile test cannot tell an intentional watch from a
// missing one, since it never goes through the manager's watch wiring at
// all.
//
// This suite deliberately does NOT run the real configtarget controller or
// a real vSphere provider: that combination makes the ConfigTarget's Ready
// transition depend on a live (albeit vcsim-backed) govmomi round trip whose
// duration is not bounded under CI/local resource contention, which made an
// earlier version of this test flaky (timing out anywhere from a few
// seconds to several minutes). Driving the ConfigTarget's status by hand
// keeps the test deterministic while still exercising exactly what this
// Describe exists to prove: that a ConfigTarget status change reaches the
// policy purely through AddToManager's watch wiring, with no manual
// reconcile call.
var _ = Describe("VirtualMachineConfigPolicy Controller watches, against a real manager",
	Label(testlabels.Controller, testlabels.API, testlabels.EnvTest, testlabels.VCSim),
	func() {
		var (
			ctx         context.Context
			vcSimCtx    *builder.IntegrationTestContextForVCSim
			zoneName    string
			clusterMoID string
		)

		BeforeEach(func() {
			ctx = pkgcfg.NewContextWithDefaultConfig()
			ctx = pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VirtualMachineConfigPolicy = true
			})

			vcSimCtx = builder.NewIntegrationTestContextForVCSim(
				ctx,
				builder.VCSimTestConfig{},
				virtualmachineconfigpolicy.AddToManager,
				manager.InitializeProvidersNoopFn,
				nil)
			Expect(vcSimCtx).ToNot(BeNil())

			vcSimCtx.BeforeEach()

			zoneName = vcSimCtx.GetFirstZoneName()

			ccr := vcSimCtx.GetFirstClusterFromFirstZone()
			Expect(ccr).ToNot(BeNil())
			clusterMoID = ccr.Reference().Value
		})

		AfterEach(func() {
			vcSimCtx.AfterEach()
		})

		When("a policy is created before its ConfigTarget becomes Ready", func() {
			var (
				policy *vimv1.VirtualMachineConfigPolicy
				ct     *vimv1.ConfigTarget
			)

			BeforeEach(func() {
				ct = &vimv1.ConfigTarget{
					ObjectMeta: metav1.ObjectMeta{Name: clusterMoID},
					Spec:       vimv1.ConfigTargetSpec{ID: vimv1.ManagedObjectID{ID: clusterMoID}},
				}
				Expect(vcSimCtx.Client.Create(vcSimCtx, ct)).To(Succeed())

				policy = &vimv1.VirtualMachineConfigPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: zoneName, Namespace: vcSimCtx.NSInfo.Namespace},
					Spec:       vimv1.VirtualMachineConfigPolicySpec{Zone: zoneName},
				}
				Expect(vcSimCtx.Client.Create(vcSimCtx, policy)).To(Succeed())
			})

			It("converges once the ConfigTarget is marked Ready, with no manual reconcile", func() {
				// This suite does not go through TestSuite.Register, so the
				// package-wide 10s/100ms Gomega defaults it would otherwise
				// set are not in effect here -- use an explicit interval
				// instead of Gomega's 1s/10ms fallback.
				Eventually(func(g Gomega) {
					var got vimv1.VirtualMachineConfigPolicy
					g.Expect(vcSimCtx.Client.Get(vcSimCtx, ctrlclient.ObjectKeyFromObject(policy), &got)).To(Succeed())
					g.Expect(pkgcond.IsFalse(&got, vimv1.ReadyConditionType)).To(BeTrue())
					g.Expect(pkgcond.GetReason(&got, vimv1.ReadyConditionType)).
						To(Equal(virtualmachineconfigpolicy.ConfigTargetNotReadyReason))
				}, 10*time.Second, 100*time.Millisecond).
					Should(Succeed(), "policy must observe the not-yet-Ready ConfigTarget before it converges")

				ct.Status = vimv1.ConfigTargetStatus{NumCPUCores: 8}
				pkgcond.MarkTrue(ct, vimv1.ReadyConditionType)
				Expect(vcSimCtx.Client.Status().Update(vcSimCtx, ct)).To(Succeed())

				Eventually(func(g Gomega) {
					var got vimv1.VirtualMachineConfigPolicy
					g.Expect(vcSimCtx.Client.Get(vcSimCtx, ctrlclient.ObjectKeyFromObject(policy), &got)).To(Succeed())
					g.Expect(pkgcond.IsTrue(&got, vimv1.ReadyConditionType)).To(BeTrue())
					g.Expect(got.Spec.NumCPUCores).ToNot(BeNil())
					g.Expect(got.Spec.NumCPUCores.Max).To(Equal(int32(8)))
				}, 10*time.Second, 100*time.Millisecond).
					Should(Succeed(), "the ConfigTarget watch must enqueue the policy once the ConfigTarget is marked Ready")
			})
		})
	})
