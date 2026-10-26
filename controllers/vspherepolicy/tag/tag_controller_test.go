// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package tag_test

import (
	"context"
	"errors"
	"sync"

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
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/vspherepolicy/tag"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	testlabels "github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
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
					Key:   "foo",
					Value: "bar",
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

// This Describe exercises kubeutil.RegisterVMTagsIndexes through a real
// manager cache rather than a fake client, so each index's field selector
// semantics are proven against the real informer machinery it is registered
// on in production, not an approximation of it. It registers the indexes
// directly, without the VM controller that owns that call on a Supervisor,
// because the subject here is the indexes themselves.
var _ = Describe("Field indexes", Label(
	testlabels.Controller,
	testlabels.EnvTest,
	testlabels.API,
), func() {
	var (
		idxSuite  *builder.TestSuite
		ns1Ctx    *builder.IntegrationTestContext
		mgrClient ctrlclient.Client
	)

	BeforeEach(func() {
		idxSuite = builder.NewTestSuiteForControllerWithContext(
			pkgcfg.NewContextWithDefaultConfig(),
			func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
				return kubeutil.RegisterVMTagsIndexes(ctx, mgr.GetFieldIndexer())
			},
			pkgmgr.InitializeProvidersNoopFn)
		idxSuite.BeforeSuite()

		ns1Ctx = idxSuite.NewIntegrationTestContext()

		// List with MatchingFields must go through the manager's cache: the
		// direct API-server client IntegrationTestContext.Client uses for
		// writes below has no notion of these custom field indexes and
		// rejects the field selector outright.
		mgrClient = idxSuite.GetManager().GetClient()
	})

	AfterEach(func() {
		ns1Ctx.AfterEach()
		ns1Ctx = nil

		idxSuite.AfterSuite()
		idxSuite = nil
	})

	newTag := func(ns, name, key, value string) *vspherepolv1.Tag {
		return &vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
			Spec: vspherepolv1.TagSpec{
				Key:   key,
				Value: value,
			},
		}
	}

	It("metadata.ownerReferences.uid returns a VM's owned Tags", func() {
		ownedByA := newTag(ns1Ctx.Namespace, "owned-by-a", "team", "blue")
		ownedByA.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: vmopv1.GroupVersion.String(),
				Kind:       "VirtualMachine",
				Name:       "vm-a",
				UID:        types.UID("vm-a-uid"),
			},
		}

		ownedByB := newTag(ns1Ctx.Namespace, "owned-by-b", "team", "red")
		ownedByB.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: vmopv1.GroupVersion.String(),
				Kind:       "VirtualMachine",
				Name:       "vm-b",
				UID:        types.UID("vm-b-uid"),
			},
		}

		Expect(ns1Ctx.Client.Create(ns1Ctx, ownedByA)).To(Succeed())
		Expect(ns1Ctx.Client.Create(ns1Ctx, ownedByB)).To(Succeed())

		var list vspherepolv1.TagList
		Eventually(func(g Gomega) {
			g.Expect(mgrClient.List(
				ns1Ctx,
				&list,
				ctrlclient.InNamespace(ns1Ctx.Namespace),
				ctrlclient.MatchingFields{kubeutil.TagOwnerReferencesUIDIndexKey: "vm-a-uid"},
			)).To(Succeed())
			g.Expect(list.Items).To(HaveLen(1))
		}).Should(Succeed())

		Expect(list.Items[0].Name).To(Equal("owned-by-a"))
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
			pkgcfg.NewContextWithDefaultConfig(),
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
							// Deliberately not zone/hostname, so the term is
							// eligible regardless of the
							// VMPlacementPolicies/VMAffinityDuringExecution
							// flags (mirrors newVM in
							// vmtags_reconciler_test.go).
							TopologyKey: "custom-topology-key",
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
		reconcileWithRetry := func(vm *vmopv1.VirtualMachine) error {
			var lastErr error
			for range 20 {
				_, err := vmtags.ReconcileTagCRs(intgCtx, mgrClient, vm)
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
			g.Expect(list.Items).To(HaveLen(1))

			owners := make([]string, 0, len(list.Items[0].OwnerReferences))
			for _, ref := range list.Items[0].OwnerReferences {
				owners = append(owners, string(ref.UID))
			}
			g.Expect(owners).To(ConsistOf("vm-a-uid", "vm-b-uid"))
		}).Should(Succeed())
	})
})

// This Describe proves the Tag CRD's generated printer columns and status
// subresource are what tag_types.go's kubebuilder markers declare, not just
// something asserted at the marker level: it reads back the installed
// apiextensionsv1.CustomResourceDefinition from the real envtest environment
// (test/builder/test_suite.go TestSuite.GetInstalledCRD, populated from the
// checked-in config/crd/external-crds manifest), and it exercises the
// status subresource split against a real API server: Status is
// a subresource, so a plain Update() to the main resource cannot touch it,
// and only Status().Update() can.
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
				Key:   "foo",
				Value: "bar",
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
