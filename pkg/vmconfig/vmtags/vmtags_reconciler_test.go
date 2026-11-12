// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmtags_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/vmtags"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const testNamespace = "team-a"

func newVM(name string, uid types.UID, value string) *vmopv1.VirtualMachine {
	vm := builder.DummyVirtualMachine()
	vm.Name = name
	vm.Namespace = testNamespace
	vm.UID = uid
	vm.Labels = nil
	if value != "" {
		vm.Spec.Affinity = &vmopv1.AffinitySpec{
			VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
				RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{{
					TopologyKey: corev1.LabelHostname,
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": value},
					},
				}},
			},
		}
	}
	return vm
}

func ownerUIDs(tag *vspherepolv1.Tag) []string {
	var uids []string
	for _, r := range tag.OwnerReferences {
		uids = append(uids, string(r.UID))
	}
	return uids
}

var _ = Describe("Tag resources", Label(testlabels.Controller), func() {
	var (
		ctx       context.Context
		c         ctrlclient.Client
		funcs     interceptor.Funcs
		initObjs  []ctrlclient.Object
		writes    int
		vmA, vmB  *vmopv1.VirtualMachine
		listTags  func() []vspherepolv1.Tag
		ownedBy   func(uid string) []string
		tagGroup  = schema.GroupResource{Group: vspherepolv1.GroupName, Resource: "tags"}
		reconcile = func(vm *vmopv1.VirtualMachine) ([]vspherepolv1.Tag, error) {
			return vmtags.ReconcileTagCRs(ctx, c, vm)
		}
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		pkgcfg.SetContext(ctx, func(cfg *pkgcfg.Config) {
			cfg.Features.TaggingAPI = true
			cfg.Features.VMAffinityDuringExecution = true
			cfg.Features.VMPlacementPolicies = true
			cfg.Features.VMHardAffinityDuringExecution = true
		})
		funcs = interceptor.Funcs{}
		initObjs = nil
		writes = 0
		vmA = newVM("vm-a", "vm-a-uid", "db")
		vmB = newVM("vm-b", "vm-b-uid", "db")
	})

	JustBeforeEach(func() {
		f := funcs
		create, patch := f.Create, f.Patch
		f.Create = func(ctx context.Context, w ctrlclient.WithWatch, o ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
			writes++
			if create != nil {
				return create(ctx, w, o, opts...)
			}
			return w.Create(ctx, o, opts...)
		}
		f.Patch = func(ctx context.Context, w ctrlclient.WithWatch, o ctrlclient.Object, p ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
			writes++
			if patch != nil {
				return patch(ctx, w, o, p, opts...)
			}
			return w.Patch(ctx, o, p, opts...)
		}
		c = builder.NewFakeClientWithInterceptors(f, initObjs...)
		listTags = func() []vspherepolv1.Tag {
			var l vspherepolv1.TagList
			Expect(c.List(ctx, &l, ctrlclient.InNamespace(testNamespace))).To(Succeed())
			return l.Items
		}
		ownedBy = func(uid string) []string {
			var names []string
			for _, t := range listTags() {
				for _, u := range ownerUIDs(&t) {
					if u == uid {
						names = append(names, t.Spec.Name)
					}
				}
			}
			return names
		}
	})

	Describe("ReconcileTagCRs", func() {
		It("creates the VM's Tags with the VM as sole owner", func() {
			ownedTags, err := reconcile(vmA)
			Expect(err).ToNot(HaveOccurred())
			Expect(ownedTags).To(HaveLen(3))
			Expect(listTags()).To(HaveLen(3))
			for _, t := range listTags() {
				Expect(ownerUIDs(&t)).To(ConsistOf("vm-a-uid"))
			}
		})

		It("does not own anything for a label-only VM", func() {
			vm := newVM("vm-c", "vm-c-uid", "")
			vm.Labels = map[string]string{"app": "db"}
			ownedTags, err := reconcile(vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(ownedTags).To(BeEmpty())
			Expect(listTags()).To(BeEmpty())
		})

		It("writes nothing when reconciled twice", func() {
			_, err := reconcile(vmA)
			Expect(err).ToNot(HaveOccurred())
			writes = 0
			_, err = reconcile(vmA)
			Expect(err).ToNot(HaveOccurred())
			Expect(writes).To(BeZero())
		})

		It("prunes ownership of Tags the spec no longer needs", func() {
			_, err := reconcile(vmA)
			Expect(err).ToNot(HaveOccurred())

			vmA.Spec.Affinity = newVM("x", "x", "web").Spec.Affinity
			_, err = reconcile(vmA)
			Expect(err).ToNot(HaveOccurred())

			// Original, derived target and self Tag of the new policy.
			Expect(ownedBy("vm-a-uid")).To(HaveLen(3))
			Expect(ownedBy("vm-a-uid")).To(ContainElement("app:web"))
			Expect(ownedBy("vm-a-uid")).ToNot(ContainElement("app:db"))
			Expect(ownedBy("vm-a-uid")).ToNot(ContainElement(HaveSuffix("-app:db")))
		})

		Context("an existing Tag has a conflicting identity", func() {
			BeforeEach(func() {
				desired, err := virtualmachine.DesiredAffinityTags(ctx, vmA)
				Expect(err).ToNot(HaveOccurred())
				bad := desired[0].DeepCopy()
				bad.Spec.Kind = vspherepolv1.TagKindAffinityTarget
				initObjs = append(initObjs, bad)
			})

			It("is not adopted", func() {
				_, err := reconcile(vmA)
				Expect(err).To(MatchError(ContainSubstring("conflicting identity")))
				Expect(ownedBy("vm-a-uid")).To(BeEmpty())
			})
		})

		Context("another VM creates a conflicting Tag between our Get and Create", func() {
			BeforeEach(func() {
				funcs.Create = func(ctx context.Context, w ctrlclient.WithWatch, o ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
					racer := o.DeepCopyObject().(*vspherepolv1.Tag)
					racer.OwnerReferences = nil
					racer.Spec.Kind = vspherepolv1.TagKindAffinityTarget
					Expect(controllerutil.SetOwnerReference(vmB, racer, builder.NewScheme())).To(Succeed())
					Expect(w.Create(ctx, racer)).To(Succeed())
					return apierrors.NewAlreadyExists(tagGroup, o.GetName())
				}
			})

			It("returns a conflicting identity error and does not adopt the Tag", func() {
				_, err := reconcile(vmA)
				Expect(err).To(MatchError(ContainSubstring("conflicting identity")))
				Expect(ownedBy("vm-a-uid")).To(BeEmpty())
			})
		})

		Context("another VM creates the Tag between our Get and Create", func() {
			BeforeEach(func() {
				funcs.Create = func(ctx context.Context, w ctrlclient.WithWatch, o ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
					racer := o.DeepCopyObject().(*vspherepolv1.Tag)
					racer.OwnerReferences = nil
					Expect(controllerutil.SetOwnerReference(vmB, racer, builder.NewScheme())).To(Succeed())
					Expect(w.Create(ctx, racer)).To(Succeed())
					return apierrors.NewAlreadyExists(tagGroup, o.GetName())
				}
			})

			It("adopts the Tag instead of failing", func() {
				ownedTags, err := reconcile(vmA)
				Expect(err).ToNot(HaveOccurred())
				Expect(ownedTags).To(HaveLen(3))
				for _, t := range listTags() {
					Expect(ownerUIDs(&t)).To(ConsistOf("vm-b-uid", "vm-a-uid"))
				}
			})
		})

		Context("an owner patch conflicts", func() {
			var conflicts int

			BeforeEach(func() {
				conflicts = 1
				funcs.Patch = func(ctx context.Context, w ctrlclient.WithWatch, o ctrlclient.Object, p ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
					if conflicts > 0 {
						conflicts--
						return apierrors.NewConflict(tagGroup, o.GetName(), fmt.Errorf("injected"))
					}
					return w.Patch(ctx, o, p, opts...)
				}
			})

			It("returns the error and the next reconcile succeeds", func() {
				_, err := reconcile(vmA)
				Expect(err).ToNot(HaveOccurred())
				_, err = reconcile(vmB)
				Expect(err).To(HaveOccurred())
				_, err = reconcile(vmB)
				Expect(err).ToNot(HaveOccurred())
				for _, t := range listTags() {
					Expect(ownerUIDs(&t)).To(ConsistOf("vm-a-uid", "vm-b-uid"))
				}
			})
		})

		Context("a Tag has a stale owner reference from a same-named VM", func() {
			BeforeEach(func() {
				desired, err := virtualmachine.DesiredAffinityTags(ctx, vmA)
				Expect(err).ToNot(HaveOccurred())
				for i := range desired {
					t := desired[i].DeepCopy()
					old := newVM("vm-a", "old-uid", "")
					Expect(controllerutil.SetOwnerReference(old, t, builder.NewScheme())).To(Succeed())
					initObjs = append(initObjs, t)
				}
			})

			It("upserts the reference with the current UID", func() {
				_, err := reconcile(vmA)
				Expect(err).ToNot(HaveOccurred())
				for _, t := range listTags() {
					Expect(ownerUIDs(&t)).To(ConsistOf("vm-a-uid"))
				}
			})
		})
	})

	// Pruning everything is what ReconcileTagCRs does for a VM that no longer
	// declares affinity. A deleted VM's ownership is left to the garbage
	// collector, so these cover the prune branch through that path.
	Describe("pruning all ownership", func() {
		withoutAffinity := func(vm *vmopv1.VirtualMachine) *vmopv1.VirtualMachine {
			vm = vm.DeepCopy()
			vm.Spec.Affinity = nil
			return vm
		}
		prune := func(vm *vmopv1.VirtualMachine) error {
			_, err := reconcile(withoutAffinity(vm))
			return err
		}

		It("keeps a shared Tag's other owners until each one prunes", func() {
			_, err := reconcile(vmA)
			Expect(err).ToNot(HaveOccurred())
			_, err = reconcile(vmB)
			Expect(err).ToNot(HaveOccurred())

			Expect(prune(vmA)).To(Succeed())
			Expect(ownedBy("vm-a-uid")).To(BeEmpty())
			Expect(ownedBy("vm-b-uid")).To(HaveLen(3))

			Expect(prune(vmB)).To(Succeed())
			for _, t := range listTags() {
				Expect(t.OwnerReferences).To(BeEmpty())
			}
		})

		Context("listing fails", func() {
			BeforeEach(func() {
				funcs.List = func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectList, ...ctrlclient.ListOption) error {
					return fmt.Errorf("injected list failure")
				}
			})
			It("propagates the error", func() {
				Expect(prune(vmA)).To(MatchError(ContainSubstring("injected list failure")))
			})
		})

		Context("a listed Tag is already deleted when its owner reference is pruned", func() {
			var patches int
			var gone bool

			BeforeEach(func() {
				patches, gone = 0, false
				funcs.Patch = func(ctx context.Context, w ctrlclient.WithWatch, o ctrlclient.Object, p ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
					if gone {
						patches++
						return apierrors.NewNotFound(tagGroup, o.GetName())
					}
					return w.Patch(ctx, o, p, opts...)
				}
			})
			JustBeforeEach(func() {
				_, err := reconcile(vmA)
				Expect(err).ToNot(HaveOccurred())
				gone = true
			})
			It("treats the missing Tag as pruned", func() {
				Expect(prune(vmA)).To(Succeed())
				Expect(patches).To(BeNumerically(">", 0))
			})
		})

		Context("the owner reference is already gone from a listed Tag", func() {
			BeforeEach(func() {
				desired, err := virtualmachine.DesiredAffinityTags(ctx, vmA)
				Expect(err).ToNot(HaveOccurred())
				for i := range desired {
					initObjs = append(initObjs, desired[i].DeepCopy())
				}
				funcs.List = func(ctx context.Context, w ctrlclient.WithWatch, l ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
					// Simulate a stale cache: return every Tag regardless of owner.
					return w.List(ctx, l, ctrlclient.InNamespace(testNamespace))
				}
			})
			It("skips it without error", func() {
				Expect(prune(vmA)).To(Succeed())
				Expect(writes).To(BeZero())
			})
		})
	})
})
