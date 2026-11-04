// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package tag_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/controllers/vspherepolicy/tag"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	testlabels "github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/vmtags"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"Reconcile",
	Label(testlabels.Controller, testlabels.API),
	func() {
		var (
			ctx        context.Context
			k8sClient  ctrlclient.Client
			reconciler *tag.Reconciler
			obj        *vspherepolv1.Tag
			namespace  string
			listCalled bool

			withObjs  []ctrlclient.Object
			withFuncs interceptor.Funcs
		)

		BeforeEach(func() {
			ctx = pkgcfg.NewContextWithDefaultConfig()
			namespace = "test-namespace"
			listCalled = false

			withObjs = nil
			withFuncs = interceptor.Funcs{
				List: func(
					ctx context.Context,
					c ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {
					if _, ok := list.(*vmopv1.VirtualMachineList); ok {
						listCalled = true
					}

					return c.List(ctx, list, opts...)
				},
			}

			obj = &vspherepolv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-bar",
					Namespace: namespace,
				},
				Spec: vspherepolv1.TagSpec{
					Key:        "foo",
					Value:      "bar",
					Name:       "foo:bar",
					Kind:       vspherepolv1.TagKindOriginal,
					Membership: vspherepolv1.TagMembershipSourceLabel,
				},
			}
		})

		JustBeforeEach(func() {
			scheme := runtime.NewScheme()
			Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
			Expect(vspherepolv1.AddToScheme(scheme)).To(Succeed())
			Expect(vmopv1.AddToScheme(scheme)).To(Succeed())

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&vspherepolv1.Tag{}).
				WithObjects(withObjs...).
				WithInterceptorFuncs(withFuncs).
				Build()

			reconciler = tag.NewReconciler(
				ctx,
				k8sClient,
				log.Log.WithName("test"))
		})

		reconcileObj := func() (ctrl.Result, error) {
			return reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      obj.Name,
					Namespace: obj.Namespace,
				},
			})
		}

		getObj := func() *vspherepolv1.Tag {
			var out vspherepolv1.Tag
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      obj.Name,
				Namespace: obj.Namespace,
			}, &out)).To(Succeed())

			return &out
		}

		When("the Tag does not exist", func() {
			It("returns without error", func() {
				result, err := reconcileObj()
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		When("the Tag has an owner", func() {
			BeforeEach(func() {
				obj.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       "VirtualMachine",
						Name:       "some-vm",
						UID:        types.UID("some-vm-uid"),
					},
				}
				withObjs = append(withObjs, obj)
			})

			Context("and the Tag is marked for deletion", func() {
				BeforeEach(func() {
					obj.DeletionTimestamp = new(metav1.Now())
					obj.Finalizers = []string{metav1.FinalizerDeleteDependents}
				})

				It("leaves the terminating Tag unchanged", func() {
					before := getObj().DeepCopy()

					result, err := reconcileObj()
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))
					Expect(getObj()).To(Equal(before))
				})
			})

			Context("and the Tag is a self tag", func() {
				BeforeEach(func() {
					obj.Spec.Kind = vspherepolv1.TagKindGroupedAntiAffinitySelf
					obj.Spec.Membership = vspherepolv1.TagMembershipPolicyRequesters
					obj.Spec.Key, obj.Spec.Value = "", ""
					obj.Labels = map[string]string{"admin": "retained"}
				})
				It("marks Ready without synthesizing a source label or reading peer VMs", func() {
					_, err := reconcileObj()
					Expect(err).ToNot(HaveOccurred())
					after := getObj()
					Expect(after.Labels).To(Equal(map[string]string{"admin": "retained"}))
					Expect(conditions.IsTrue(after, vspherepolv1.ReadyConditionType)).To(BeTrue())
					Expect(after.Status.ObservedGeneration).To(Equal(after.Generation))
					Expect(listCalled).To(BeFalse())
				})
			})

			Context("and the label mirror is missing", func() {
				BeforeEach(func() {
					obj.Labels = nil
				})

				It("corrects the label mirror and marks Ready", func() {
					_, err := reconcileObj()
					Expect(err).ToNot(HaveOccurred())

					after := getObj()
					Expect(after.Labels).To(HaveKeyWithValue("foo", "bar"))
					Expect(after.Status.ObservedGeneration).To(Equal(after.Generation))
					Expect(conditions.IsTrue(after, vspherepolv1.ReadyConditionType)).To(BeTrue())
				})
			})

			Context("and the label mirror already matches", func() {
				BeforeEach(func() {
					obj.Labels = map[string]string{"foo": "bar"}
				})

				It("marks Ready without changing the labels", func() {
					_, err := reconcileObj()
					Expect(err).ToNot(HaveOccurred())

					after := getObj()
					Expect(after.Labels).To(HaveKeyWithValue("foo", "bar"))
					Expect(conditions.IsTrue(after, vspherepolv1.ReadyConditionType)).To(BeTrue())
				})
			})

			Context("and spec.value is empty and the label mirror is missing", func() {
				BeforeEach(func() {
					obj.Spec.Value = ""
					obj.Labels = nil
				})

				It("adds the label key with an empty value rather than skipping the mirror", func() {
					_, err := reconcileObj()
					Expect(err).ToNot(HaveOccurred())

					after := getObj()
					Expect(after.Labels).To(HaveKey("foo"))
					Expect(after.Labels["foo"]).To(BeEmpty())
					Expect(conditions.IsTrue(after, vspherepolv1.ReadyConditionType)).To(BeTrue())
				})
			})

			It("never lists VirtualMachines", func() {
				_, err := reconcileObj()
				Expect(err).ToNot(HaveOccurred())
				Expect(listCalled).To(BeFalse())
			})
		})

		When("the Tag has no owners", func() {
			BeforeEach(func() {
				obj.OwnerReferences = nil
				withObjs = append(withObjs, obj)
			})

			It("deletes the Tag outright, with no terminating window", func() {
				_, err := reconcileObj()
				Expect(err).ToNot(HaveOccurred())

				var out vspherepolv1.Tag
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      obj.Name,
					Namespace: obj.Namespace,
				}, &out)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})

			It("never lists VirtualMachines", func() {
				_, err := reconcileObj()
				Expect(err).ToNot(HaveOccurred())

				Expect(listCalled).To(BeFalse())
			})

			Context("and the delete is captured", func() {
				var gotOpts []ctrlclient.DeleteOption

				BeforeEach(func() {
					withFuncs.Delete = func(
						ctx context.Context,
						c ctrlclient.WithWatch,
						o ctrlclient.Object,
						opts ...ctrlclient.DeleteOption) error {
						gotOpts = opts
						return c.Delete(ctx, o, opts...)
					}
				})

				It("preconditions the delete on the ResourceVersion it read", func() {
					_, err := reconcileObj()
					Expect(err).ToNot(HaveOccurred())

					deleteOpts := &ctrlclient.DeleteOptions{}
					for _, opt := range gotOpts {
						opt.ApplyToDelete(deleteOpts)
					}
					Expect(deleteOpts.Preconditions).ToNot(BeNil())
					Expect(deleteOpts.Preconditions.ResourceVersion).ToNot(BeNil())
					Expect(*deleteOpts.Preconditions.ResourceVersion).To(Equal(obj.ResourceVersion))
				})
			})
		})

		When("the Tag gained an owner between the Get and the Delete", func() {
			BeforeEach(func() {
				obj.OwnerReferences = nil
				withObjs = append(withObjs, obj)
				withFuncs.Delete = func(
					ctx context.Context,
					c ctrlclient.WithWatch,
					o ctrlclient.Object,
					opts ...ctrlclient.DeleteOption) error {
					// Simulate a concurrent VM reconcile adding an owner
					// reference after this reconcile's Get but before its
					// Delete: the fake client's real object no longer
					// matches the ResourceVersion this reconcile read, so a
					// genuine apiserver would reject the precondition with a
					// conflict.
					return apierrors.NewConflict(
						vspherepolv1.GroupVersion.WithResource("tags").GroupResource(),
						o.GetName(),
						errors.New("resourceVersion mismatch"))
				}
			})

			It("propagates the conflict rather than deleting the Tag", func() {
				_, err := reconcileObj()
				Expect(apierrors.IsConflict(err)).To(BeTrue())

				after := getObj()
				Expect(after).ToNot(BeNil())
			})
		})

		When("deleting the zero-owner Tag fails", func() {
			BeforeEach(func() {
				obj.OwnerReferences = nil
				withObjs = append(withObjs, obj)
				withFuncs.Delete = func(
					ctx context.Context,
					c ctrlclient.WithWatch,
					o ctrlclient.Object,
					opts ...ctrlclient.DeleteOption) error {
					return errors.New("fake delete error")
				}
			})

			It("propagates the error", func() {
				_, err := reconcileObj()
				Expect(err).To(HaveOccurred())
			})
		})
	},
)

var _ = Describe("Printer columns and status subresource", Ordered, Label(
	testlabels.Controller,
	testlabels.EnvTest,
	testlabels.API,
), func() {
	var (
		crdSuite *builder.TestSuite
		intgCtx  *builder.IntegrationTestContext
	)

	// envtest bring-up/tear-down is expensive (a full API server restart),
	// so it happens once for this Describe block via BeforeAll/AfterAll
	// rather than per-It in BeforeEach/AfterEach.
	BeforeAll(func() {
		crdSuite = builder.NewTestSuiteForControllerWithContext(
			pkgcfg.NewContextWithDefaultConfig(),
			func(*pkgctx.ControllerManagerContext, ctrlmgr.Manager) error {
				// No indexes or watches are needed: this Describe verifies
				// CRD/API-server behavior (printer columns, status
				// subresource), not reconciler logic.
				return nil
			},
			pkgmgr.InitializeProvidersNoopFn)
		crdSuite.BeforeSuite()
	})

	AfterAll(func() {
		crdSuite.AfterSuite()
		crdSuite = nil
	})

	BeforeEach(func() {
		// IntegrationTestContext.Client talks directly to the envtest API
		// server (test/builder/intg_test_context.go), so Get() immediately
		// after Create()/Update() below observes the write with no cache
		// propagation delay -- unlike the manager's cached client the other
		// Describe blocks in this file use for List-by-index assertions.
		intgCtx = crdSuite.NewIntegrationTestContext()
	})

	AfterEach(func() {
		intgCtx.AfterEach()
		intgCtx = nil
	})

	It("declares the label Key, label Value, and Age printer columns and enables the status subresource", func() {
		crd := crdSuite.GetInstalledCRD("tags.vsphere.policy.vmware.com")
		Expect(crd).ToNot(BeNil())
		Expect(crd.Spec.Versions).ToNot(BeEmpty())

		version := crd.Spec.Versions[0]

		Expect(version.Subresources).ToNot(BeNil())
		Expect(version.Subresources.Status).ToNot(BeNil())

		Expect(version.AdditionalPrinterColumns).To(ConsistOf(
			apiextensionsv1.CustomResourceColumnDefinition{Name: "Tag", Type: "string", JSONPath: ".spec.name"},
			apiextensionsv1.CustomResourceColumnDefinition{Name: "Kind", Type: "string", JSONPath: ".spec.kind"},
			apiextensionsv1.CustomResourceColumnDefinition{
				Name:     "Key",
				Type:     "string",
				JSONPath: ".spec.key",
			},
			apiextensionsv1.CustomResourceColumnDefinition{
				Name:     "Value",
				Type:     "string",
				JSONPath: ".spec.value",
			},
			apiextensionsv1.CustomResourceColumnDefinition{
				Name:     "Age",
				Type:     "date",
				JSONPath: ".metadata.creationTimestamp",
			},
		))
	})

	It("only persists Status through the status subresource, never through a plain Update", func() {
		obj := &vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: intgCtx.Namespace,
				Name:      "status-subresource",
			},
			Spec: vspherepolv1.TagSpec{
				Key:        "foo",
				Value:      "bar",
				Name:       "foo:bar",
				Kind:       vspherepolv1.TagKindOriginal,
				Membership: vspherepolv1.TagMembershipSourceLabel,
			},
		}
		Expect(intgCtx.Client.Create(intgCtx, obj)).To(Succeed())
		key := ctrlclient.ObjectKeyFromObject(obj)

		// A plain Update() that changes both metadata and Status must
		// persist the metadata change but silently drop the Status change:
		// the API server ignores status writes sent through the main
		// resource's Update endpoint once a status subresource is enabled.
		// Spec fields are immutable, so a label stands in for the change.
		var beforeUpdate vspherepolv1.Tag
		Expect(intgCtx.Client.Get(intgCtx, key, &beforeUpdate)).To(Succeed())
		beforeUpdate.Labels = map[string]string{"test": "baz"}
		beforeUpdate.Status.ObservedGeneration = 42
		Expect(intgCtx.Client.Update(intgCtx, &beforeUpdate)).To(Succeed())

		var afterUpdate vspherepolv1.Tag
		Expect(intgCtx.Client.Get(intgCtx, key, &afterUpdate)).To(Succeed())
		Expect(afterUpdate.Labels).To(HaveKeyWithValue("test", "baz"))
		Expect(afterUpdate.Status.ObservedGeneration).To(BeZero())

		// Status().Update() is the only way to persist the Status change.
		afterUpdate.Status.ObservedGeneration = 42
		Expect(intgCtx.Client.Status().Update(intgCtx, &afterUpdate)).To(Succeed())

		var afterStatusUpdate vspherepolv1.Tag
		Expect(intgCtx.Client.Get(intgCtx, key, &afterStatusUpdate)).To(Succeed())
		Expect(afterStatusUpdate.Status.ObservedGeneration).To(Equal(int64(42)))
		// The metadata must be untouched by the status-only write.
		Expect(afterStatusUpdate.Labels).To(HaveKeyWithValue("test", "baz"))
	})
})

// This Describe proves the optimistic-lock merge patch
// operator-best-practices.md requires for fan-in owner-reference writes
// actually does its job against a real API server, not just a fake client:
// two VMs concurrently becoming owners of the same brand-new Tag must both
// survive, rather than one silently overwriting the other's write — the
// exact failure a plain merge patch (no resourceVersion precondition) would
// allow. A real apiserver's conflict detection is what a fake client cannot
// be trusted to reproduce faithfully.
var _ = Describe("Concurrent ownership writes", Label(
	testlabels.Controller,
	testlabels.EnvTest,
	testlabels.API,
), func() {
	var (
		concurrentSuite *builder.TestSuite
		intgCtx         *builder.IntegrationTestContext
		mgrClient       ctrlclient.Client
	)

	BeforeEach(func() {
		concurrentSuite = builder.NewTestSuiteForControllerWithContext(
			pkgcfg.UpdateContext(
				pkgcfg.NewContextWithDefaultConfig(),
				func(config *pkgcfg.Config) {
					config.Features.TaggingAPI = true
					config.Features.VMAffinityDuringExecution = true
					config.Features.VMPlacementPolicies = true
					config.Features.VMHardAffinityDuringExecution = true
				}),
			func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
				return kubeutil.RegisterVMTagsIndexes(ctx, mgr.GetFieldIndexer())
			},
			pkgmgr.InitializeProvidersNoopFn)
		concurrentSuite.BeforeSuite()

		intgCtx = concurrentSuite.NewIntegrationTestContext()
		mgrClient = concurrentSuite.GetManager().GetClient()
	})

	AfterEach(func() {
		intgCtx.AfterEach()
		intgCtx = nil

		concurrentSuite.AfterSuite()
		concurrentSuite = nil
	})

	It("both survive when two VMs concurrently become owners of the same new Tag", func() {
		affinityFor := func(key, value string) *vmopv1.AffinitySpec {
			return &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{key: value},
							},
							TopologyKey: "kubernetes.io/hostname",
						},
					},
				},
			}
		}

		vmA := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: intgCtx.Namespace,
				Name:      "vm-a",
				UID:       types.UID("vm-a-uid"),
				Labels:    map[string]string{"team": "blue"},
			},
			Spec: vmopv1.VirtualMachineSpec{Affinity: affinityFor("team", "blue")},
		}
		vmB := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: intgCtx.Namespace,
				Name:      "vm-b",
				UID:       types.UID("vm-b-uid"),
				Labels:    map[string]string{"team": "blue"},
			},
			Spec: vmopv1.VirtualMachineSpec{Affinity: affinityFor("team", "blue")},
		}

		// reconcileWithRetry mimics what a real controller requeues on a
		// conflict error: try again. ReconcileTagCRs itself does not retry —
		// a conflict from the optimistic lock is expected to propagate so
		// the caller's normal requeue handles it.
		reconcileCtx := pkgcfg.UpdateContext(intgCtx, func(config *pkgcfg.Config) {
			config.Features.TaggingAPI = true
			config.Features.VMAffinityDuringExecution = true
			config.Features.VMPlacementPolicies = true
			config.Features.VMHardAffinityDuringExecution = true
		})
		reconcileWithRetry := func(vm *vmopv1.VirtualMachine) error {
			var lastErr error
			for range 20 {
				_, err := vmtags.ReconcileTagCRs(reconcileCtx, mgrClient, vm)
				if err == nil {
					return nil
				}
				lastErr = err
			}
			return lastErr
		}

		start := make(chan struct{})
		errs := make([]error, 2)

		var wg sync.WaitGroup
		wg.Add(2)
		for i, vm := range []*vmopv1.VirtualMachine{vmA, vmB} {
			go func(i int, vm *vmopv1.VirtualMachine) {
				defer wg.Done()
				<-start
				errs[i] = reconcileWithRetry(vm)
			}(i, vm)
		}
		close(start)
		wg.Wait()

		Expect(errs[0]).ToNot(HaveOccurred())
		Expect(errs[1]).ToNot(HaveOccurred())

		// Both owner-reference writes land in the same API-server-side Tag
		// object, but the informer cache backing mgrClient observes them as
		// two separate watch events. A List that only waits for the Tag to
		// exist can win the race against the second event and read back a
		// stale copy with only the first VM's owner reference, so the owner
		// extraction must be inside the Eventually and re-read on every
		// poll rather than reusing whatever the first successful List saw.
		Eventually(func(g Gomega) {
			var list vspherepolv1.TagList
			g.Expect(mgrClient.List(
				intgCtx,
				&list,
				ctrlclient.InNamespace(intgCtx.Namespace),
			)).To(Succeed())
			// The original Tag and the affinity Tag of the shared policy.
			g.Expect(list.Items).To(HaveLen(2))

			for _, item := range list.Items {
				owners := make([]string, 0, len(item.OwnerReferences))
				for _, ref := range item.OwnerReferences {
					owners = append(owners, string(ref.UID))
				}
				g.Expect(owners).To(ConsistOf("vm-a-uid", "vm-b-uid"))
			}
		}).Should(Succeed())
	})
})

// This Describe proves TagFanOutPredicate's filtering survives real wiring,
// not just the unit-level predicate assertions: it runs the Tag controller
// and the VM controller's Tag watch together against a real manager, and
// checks that a Tag update the Tag controller itself issues — correcting its
// own label mirror — never bumps the label-only VM's resourceVersion. Only
// the Tag's create/delete may do that (kubeutil.TagFanOutPredicate).
var _ = Describe("Fan-out no-op guard", Label(
	testlabels.Controller,
	testlabels.EnvTest,
	testlabels.API,
), func() {
	var (
		fanOutSuite  *builder.TestSuite
		intgCtx      *builder.IntegrationTestContext
		fakeProvider *providerfake.VMProvider
		vm           *vmopv1.VirtualMachine
		vmKey        types.NamespacedName
		reconcileCnt atomic.Int32
	)

	BeforeEach(func() {
		// This file's TestTagController runs RunSpecs directly rather than
		// through TestSuite.Register, so the 10s/100ms envtest default every
		// other integration suite gets (test/builder/test_suite.go Register)
		// is not already in effect here: apply the same default rather than
		// picking a bespoke timeout for this Describe alone.
		SetDefaultEventuallyTimeout(10 * time.Second)
		SetDefaultEventuallyPollingInterval(100 * time.Millisecond)

		virtualmachine.SkipNameValidation = ptr.To(true)

		fakeProvider = providerfake.NewVMProvider()
		reconcileCnt.Store(0)
		fakeProvider.CreateOrUpdateVirtualMachineFn = func(_ context.Context, vm *vmopv1.VirtualMachine) error {
			conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
			reconcileCnt.Add(1)
			return nil
		}

		ctx := ovfcache.WithContext(
			cource.WithContext(
				pkgcfg.UpdateContext(
					pkgcfg.NewContextWithDefaultConfig(),
					func(config *pkgcfg.Config) {
						config.Features.TaggingAPI = true
						config.SyncPeriod = time.Hour
						config.CreateVMRequeueDelay = time.Hour
						config.PoweredOnVMHasIPRequeueDelay = time.Hour
						config.AsyncCreateEnabled = false
						config.AsyncSignalEnabled = false
					})))

		fanOutSuite = builder.NewTestSuiteForControllerWithContext(
			ctx,
			func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
				if err := tag.AddToManager(ctx, mgr); err != nil {
					return err
				}
				return virtualmachine.AddToManager(ctx, mgr)
			},
			func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
				ctx.VMProvider = fakeProvider
				return nil
			})
		fanOutSuite.BeforeSuite()

		intgCtx = fanOutSuite.NewIntegrationTestContext()

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: intgCtx.Namespace,
				Name:      "label-only-vm",
				Labels:    map[string]string{"team": "blue"},
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:    "dummy-image",
				ClassName:    "dummy-class",
				StorageClass: "my-storage-class",
				PowerState:   vmopv1.VirtualMachinePowerStateOn,
			},
		}
		vmKey = types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}
	})

	AfterEach(func() {
		intgCtx.AfterEach()
		intgCtx = nil

		fanOutSuite.AfterSuite()
		fanOutSuite = nil
	})

	It("does not reconcile or write the VM for label-mirror, status, or owner-reference updates", func() {
		Expect(intgCtx.Client.Create(intgCtx, vm)).To(Succeed())

		tagObj := &vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tag-team-blue",
				Namespace: vm.Namespace,
				// Deliberately missing the label mirror, so the Tag
				// controller's ReconcileNormal must correct it after this
				// create settles.
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       "VirtualMachine",
						Name:       "vm-other",
						UID:        types.UID("vm-other-uid"),
					},
				},
			},
			Spec: vspherepolv1.TagSpec{
				Key:        "team",
				Value:      "blue",
				Name:       "team:blue",
				Kind:       vspherepolv1.TagKindOriginal,
				Membership: vspherepolv1.TagMembershipSourceLabel,
			},
		}
		tagKey := types.NamespacedName{Namespace: tagObj.Namespace, Name: tagObj.Name}
		Expect(intgCtx.Client.Create(intgCtx, tagObj)).To(Succeed())

		// Wait for the Tag controller to fully settle: label mirror
		// corrected, status set Ready. The Tag's create event is a
		// legitimate fan-out to the label-only VM, so wait for that to
		// settle too before taking the baseline for the no-op guard below.
		Eventually(func(g Gomega) {
			var t vspherepolv1.Tag
			g.Expect(intgCtx.Client.Get(intgCtx, tagKey, &t)).To(Succeed())
			g.Expect(t.Labels).To(HaveKeyWithValue("team", "blue"))
			g.Expect(conditions.IsTrue(&t, vspherepolv1.ReadyConditionType)).To(BeTrue())
		}).Should(Succeed())

		// Observe provider calls as well as writes: a redundant reconcile may
		// leave resourceVersion unchanged. Drain initial create/status events
		// before changing the Tag, with periodic retries outside this window.
		for _, update := range []string{"label mirror", "status", "owner references"} {
			By("Updating " + update + " without waking the VM")
			var baselineRV string
			var baselineCount int32
			Eventually(func(g Gomega) {
				var v vmopv1.VirtualMachine
				g.Expect(intgCtx.Client.Get(intgCtx, vmKey, &v)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(&v, "vmoperator.vmware.com/virtualmachine")).To(BeTrue())
				g.Expect(conditions.IsTrue(&v, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				baselineRV = v.ResourceVersion
				baselineCount = reconcileCnt.Load()
				g.Expect(baselineCount).To(BeNumerically(">", 0))
				g.Consistently(reconcileCnt.Load, time.Second, 50*time.Millisecond).Should(Equal(baselineCount))
			}).Should(Succeed())

			var t vspherepolv1.Tag
			Expect(intgCtx.Client.Get(intgCtx, tagKey, &t)).To(Succeed())
			originalRV := t.ResourceVersion
			switch update {
			case "label mirror":
				delete(t.Labels, "team")
				Expect(intgCtx.Client.Update(intgCtx, &t)).To(Succeed())
			case "status":
				t.Status.ObservedGeneration = 0
				conditions.MarkFalse(&t, vspherepolv1.ReadyConditionType, "TestUpdate", "testing status-only fan-out")
				Expect(intgCtx.Client.Status().Update(intgCtx, &t)).To(Succeed())
			case "owner references":
				t.OwnerReferences = append(t.OwnerReferences, metav1.OwnerReference{
					APIVersion: vmopv1.GroupVersion.String(),
					Kind:       "VirtualMachine",
					Name:       "vm-another",
					UID:        types.UID("vm-another-uid"),
				})
				Expect(intgCtx.Client.Update(intgCtx, &t)).To(Succeed())
			}

			Eventually(func(g Gomega) {
				var after vspherepolv1.Tag
				g.Expect(intgCtx.Client.Get(intgCtx, tagKey, &after)).To(Succeed())
				g.Expect(after.ResourceVersion).ToNot(Equal(originalRV))
				g.Expect(after.Labels).To(HaveKeyWithValue("team", "blue"))
				g.Expect(after.Status.ObservedGeneration).To(Equal(after.Generation))
				g.Expect(conditions.IsTrue(&after, vspherepolv1.ReadyConditionType)).To(BeTrue())
				if update == "owner references" {
					g.Expect(after.OwnerReferences).To(HaveLen(2))
				}
			}).Should(Succeed(), "waiting for the Tag update and any correction to be observed")

			Consistently(func(g Gomega) {
				var v vmopv1.VirtualMachine
				g.Expect(intgCtx.Client.Get(intgCtx, vmKey, &v)).To(Succeed())
				g.Expect(v.ResourceVersion).To(Equal(baselineRV))
				g.Expect(reconcileCnt.Load()).To(Equal(baselineCount))
			}, time.Second, 50*time.Millisecond).Should(Succeed())
		}
	})
})
