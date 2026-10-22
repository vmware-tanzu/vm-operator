// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmtags_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/vmtags"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const testNamespace = "test-ns"

// newVM returns a minimal VM with the given name/UID, carrying the given
// labels, and referencing the given label pairs via a required VM-affinity
// term (TopologyKey deliberately not zone/hostname, so the term is eligible
// regardless of the VMPlacementPolicies/VMAffinityDuringExecution flags).
func newVM(name string, uid types.UID, labels map[string]string, referenced ...virtualmachine.LabelPair) *vmopv1.VirtualMachine {
	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      name,
			UID:       uid,
			Labels:    labels,
		},
	}

	if len(referenced) > 0 {
		var terms []vmopv1.VMAffinityTerm
		for _, pair := range referenced {
			terms = append(terms, vmopv1.VMAffinityTerm{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{pair.Key: pair.Value},
				},
				TopologyKey: "custom-topology-key",
			})
		}

		vm.Spec.Affinity = &vmopv1.AffinitySpec{
			VMAffinity: &vmopv1.VMAffinitySpec{
				RequiredDuringSchedulingPreferredDuringExecution: terms,
			},
		}
	}

	return vm
}

func getTag(k8sClient ctrlclient.Client, name string) *vspherepolv1.Tag {
	tag := &vspherepolv1.Tag{}
	err := k8sClient.Get(
		context.Background(),
		ctrlclient.ObjectKey{Namespace: testNamespace, Name: name},
		tag)
	if err != nil {
		return nil
	}
	return tag
}

func ownerUIDs(tag *vspherepolv1.Tag) []string {
	uids := make([]string, 0, len(tag.OwnerReferences))
	for _, ref := range tag.OwnerReferences {
		uids = append(uids, string(ref.UID))
	}
	return uids
}

var _ = Describe("ReconcileTagCRs", func() {
	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		vm        *vmopv1.VirtualMachine
		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		withObjs = nil
		withFuncs = interceptor.Funcs{}
		vm = newVM("vm-a", "vm-a-uid", map[string]string{"team": "blue"},
			virtualmachine.LabelPair{Key: "team", Value: "blue"})
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	When("a panic is expected", func() {
		When("ctx is nil", func() {
			It("panics", func() {
				var nilCtx context.Context
				fn := func() {
					_, _ = vmtags.ReconcileTagCRs(nilCtx, k8sClient, vm)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})

		When("k8sClient is nil", func() {
			It("panics", func() {
				fn := func() {
					_, _ = vmtags.ReconcileTagCRs(ctx, nil, vm)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})

		When("vm is nil", func() {
			It("panics", func() {
				fn := func() {
					_, _ = vmtags.ReconcileTagCRs(ctx, k8sClient, nil)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})
	})

	When("the VM references and carries a label pair it does not yet own", func() {
		It("creates the Tag with this VM as its sole owner", func() {
			ownedTags, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(ownedTags).To(HaveLen(1))

			name := virtualmachine.TagResourceName(testNamespace, "team", "blue")
			Expect(ownedTags[0].Name).To(Equal(name))
			Expect(ownedTags[0].Namespace).To(Equal(testNamespace))
			Expect(ownedTags[0].Labels).To(Equal(map[string]string{"team": "blue"}))
			Expect(ownedTags[0].Spec).To(Equal(vspherepolv1.TagSpec{Key: "team", Value: "blue"}))

			tag := getTag(k8sClient, name)
			Expect(tag).ToNot(BeNil())
			Expect(ownerUIDs(tag)).To(ConsistOf("vm-a-uid"))
		})
	})

	When("the VM references a pair but does not carry it", func() {
		BeforeEach(func() {
			vm = newVM("vm-a", "vm-a-uid", map[string]string{"team": "red"},
				virtualmachine.LabelPair{Key: "team", Value: "blue"})
		})

		It("creates the Tag with this VM as its sole owner, even though it does not carry the pair", func() {
			ownedTags, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(ownedTags).To(HaveLen(1))

			name := virtualmachine.TagResourceName(testNamespace, "team", "blue")
			Expect(ownedTags[0].Name).To(Equal(name))
			Expect(ownedTags[0].Spec).To(Equal(vspherepolv1.TagSpec{Key: "team", Value: "blue"}))

			tag := getTag(k8sClient, name)
			Expect(tag).ToNot(BeNil())
			Expect(ownerUIDs(tag)).To(ConsistOf("vm-a-uid"))
		})
	})

	When("a Tag for the pair is already owned by another VM", func() {
		var otherOwnedTag *vspherepolv1.Tag

		BeforeEach(func() {
			other := newVM("vm-other", "vm-other-uid", nil)
			otherOwnedTag = &vspherepolv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      virtualmachine.TagResourceName(testNamespace, "team", "blue"),
					Namespace: testNamespace,
					Labels:    map[string]string{"team": "blue"},
				},
				Spec: vspherepolv1.TagSpec{Key: "team", Value: "blue"},
			}
			Expect(controllerutil.SetOwnerReference(
				other, otherOwnedTag, builder.NewScheme())).To(Succeed())

			withObjs = append(withObjs, otherOwnedTag)
		})

		It("appends this VM's owner reference rather than replacing the existing one", func() {
			_, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())

			tag := getTag(k8sClient, otherOwnedTag.Name)
			Expect(tag).ToNot(BeNil())
			Expect(ownerUIDs(tag)).To(ConsistOf("vm-other-uid", "vm-a-uid"))
		})
	})

	When("the existing Tag's spec does not match the pair its derived name implies", func() {
		BeforeEach(func() {
			withObjs = append(withObjs, &vspherepolv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      virtualmachine.TagResourceName(testNamespace, "team", "blue"),
					Namespace: testNamespace,
				},
				Spec: vspherepolv1.TagSpec{Key: "team", Value: "green"},
			})
		})

		It("returns an error rather than adopting the Tag", func() {
			_, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).To(MatchError(ContainSubstring("expected")))
		})
	})

	When("the VM previously owned a pair it no longer references", func() {
		JustBeforeEach(func() {
			// First pass: the VM references and carries "team:blue", so it
			// becomes the Tag's owner.
			_, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())

			name := virtualmachine.TagResourceName(testNamespace, "team", "blue")
			Expect(ownerUIDs(getTag(k8sClient, name))).To(ConsistOf("vm-a-uid"))

			// Second pass: the VM stops referencing the pair via affinity,
			// but still carries the label.
			vm.Spec.Affinity = nil
		})

		It("removes this VM's ownership on the next reconcile, even though it still carries the label", func() {
			ownedTags, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(ownedTags).To(BeEmpty())

			name := virtualmachine.TagResourceName(testNamespace, "team", "blue")
			Expect(ownerUIDs(getTag(k8sClient, name))).To(BeEmpty())
		})
	})

	When("the VM is deleted", func() {
		JustBeforeEach(func() {
			_, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
		})

		It("ReleaseOwnership removes this VM's ownership of every Tag it owns", func() {
			name := virtualmachine.TagResourceName(testNamespace, "team", "blue")
			Expect(ownerUIDs(getTag(k8sClient, name))).To(ConsistOf("vm-a-uid"))

			Expect(vmtags.ReleaseOwnership(ctx, k8sClient, vm)).To(Succeed())

			Expect(ownerUIDs(getTag(k8sClient, name))).To(BeEmpty())
		})
	})

	When("reconciling twice with nothing changed", func() {
		JustBeforeEach(func() {
			_, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
		})

		It("writes nothing on the second reconcile", func() {
			name := virtualmachine.TagResourceName(testNamespace, "team", "blue")
			before := getTag(k8sClient, name)
			Expect(before).ToNot(BeNil())

			writes := 0
			withFuncs = interceptor.Funcs{
				Create: func(ctx context.Context, c ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
					writes++
					return c.Create(ctx, obj, opts...)
				},
				Patch: func(ctx context.Context, c ctrlclient.WithWatch, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
					writes++
					return c.Patch(ctx, obj, patch, opts...)
				},
			}
			k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, before)

			_, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(writes).To(Equal(0))

			after := getTag(k8sClient, name)
			Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
		})
	})

	When("another VM concurrently creates the Tag between our Get and our Create", func() {
		BeforeEach(func() {
			name := virtualmachine.TagResourceName(testNamespace, "team", "blue")

			withFuncs.Create = func(ctx context.Context, c ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
				other := newVM("vm-other", "vm-other-uid", nil)
				racer := &vspherepolv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: testNamespace,
						Labels:    map[string]string{"team": "blue"},
					},
					Spec: vspherepolv1.TagSpec{Key: "team", Value: "blue"},
				}
				Expect(controllerutil.SetOwnerReference(
					other, racer, builder.NewScheme())).To(Succeed())
				Expect(c.Create(ctx, racer)).To(Succeed())

				return apierrors.NewAlreadyExists(
					schema.GroupResource{Group: vspherepolv1.GroupName, Resource: "tags"}, name)
			}
		})

		It("re-gets the Tag and adopts it rather than returning a hard error", func() {
			ownedTags, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(ownedTags).To(HaveLen(1))

			name := virtualmachine.TagResourceName(testNamespace, "team", "blue")
			tag := getTag(k8sClient, name)
			Expect(tag).ToNot(BeNil())
			Expect(ownerUIDs(tag)).To(ConsistOf("vm-other-uid", "vm-a-uid"))
		})
	})

	When("the Tag carries a stale owner reference from a same-named VM with a different UID", func() {
		var name string

		BeforeEach(func() {
			name = virtualmachine.TagResourceName(testNamespace, "team", "blue")

			stale := newVM("vm-a", "vm-a-old-uid", nil)
			tag := &vspherepolv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: testNamespace,
					Labels:    map[string]string{"team": "blue"},
				},
				Spec: vspherepolv1.TagSpec{Key: "team", Value: "blue"},
			}
			Expect(controllerutil.SetOwnerReference(
				stale, tag, builder.NewScheme())).To(Succeed())

			withObjs = append(withObjs, tag)
		})

		It("upserts the ref with the VM's current UID rather than leaving it stale", func() {
			_, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())

			tag := getTag(k8sClient, name)
			Expect(tag).ToNot(BeNil())
			Expect(ownerUIDs(tag)).To(ConsistOf("vm-a-uid"))
		})
	})

	When("the UID index returns a Tag whose ref was already removed (stale cache read)", func() {
		BeforeEach(func() {
			// A Tag with no owner reference for this VM at all, standing in
			// for the real scenario: a prior reconcile's patch already
			// removed the ref, but the informer cache backing this List
			// has not yet observed it and still surfaces the Tag under the
			// VM's UID index entry.
			stale := &vspherepolv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      virtualmachine.TagResourceName(testNamespace, "team", "blue"),
					Namespace: testNamespace,
				},
				Spec: vspherepolv1.TagSpec{Key: "team", Value: "blue"},
			}

			withFuncs.List = func(ctx context.Context, c ctrlclient.WithWatch, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
				tagList, ok := list.(*vspherepolv1.TagList)
				if !ok {
					return c.List(ctx, list, opts...)
				}
				tagList.Items = []vspherepolv1.Tag{*stale}
				return nil
			}

			// The VM does not reference or carry the pair, so
			// pruneStaleOwnership runs with an empty keepNames set.
			vm = newVM("vm-a", "vm-a-uid", nil)
		})

		It("skips the already-absent ref instead of erroring", func() {
			ownedTags, err := vmtags.ReconcileTagCRs(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(ownedTags).To(BeEmpty())
		})
	})
})

var _ = Describe("ReleaseOwnership", func() {
	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		vm        *vmopv1.VirtualMachine
		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		withObjs = nil
		withFuncs = interceptor.Funcs{}
		vm = newVM("vm-a", "vm-a-uid", nil)
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	When("the VM owns no Tags", func() {
		It("succeeds and writes nothing", func() {
			Expect(vmtags.ReleaseOwnership(ctx, k8sClient, vm)).To(Succeed())
		})
	})

	When("listing Tags owned by the VM fails", func() {
		BeforeEach(func() {
			withFuncs.List = func(ctx context.Context, c ctrlclient.WithWatch, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
				return errors.New("boom")
			}
		})

		It("propagates the error", func() {
			Expect(vmtags.ReleaseOwnership(ctx, k8sClient, vm)).To(
				MatchError(ContainSubstring("boom")))
		})
	})

	When("patching a Tag's owner references fails", func() {
		BeforeEach(func() {
			tag := &vspherepolv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      virtualmachine.TagResourceName(testNamespace, "team", "blue"),
					Namespace: testNamespace,
				},
				Spec: vspherepolv1.TagSpec{Key: "team", Value: "blue"},
			}
			Expect(controllerutil.SetOwnerReference(vm, tag, builder.NewScheme())).To(Succeed())
			withObjs = append(withObjs, tag)

			withFuncs.Patch = func(ctx context.Context, c ctrlclient.WithWatch, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
				return errors.New("boom")
			}
		})

		It("propagates the error", func() {
			Expect(vmtags.ReleaseOwnership(ctx, k8sClient, vm)).To(
				MatchError(ContainSubstring("boom")))
		})
	})
})
