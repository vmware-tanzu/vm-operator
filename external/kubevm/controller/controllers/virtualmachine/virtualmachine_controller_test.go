// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"
)

const (
	annotationKey = "kube-vm.io/virtual-machine"

	stubGroup   = "infrastructure.stub.kube-vm.io"
	stubVersion = "v1alpha1"
	stubKind    = "StubVirtualMachine"
)

func newStubProvider(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: stubGroup, Version: stubVersion, Kind: stubKind})
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

func newNamespace() string {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-" + uuid.NewString()},
	}
	Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())
	return ns.Name
}

var _ = Describe("Reconcile", func() {
	var (
		ctx        context.Context
		namespace  string
		vmKey      types.NamespacedName
		providerNS string
		provider   *unstructured.Unstructured
		vm         *kubevmv1a1.VirtualMachine
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = newNamespace()
		providerNS = namespace

		vmKey = types.NamespacedName{
			Name:      "vm-" + uuid.NewString(),
			Namespace: namespace,
		}

		provider = newStubProvider(providerNS, "provider-"+uuid.NewString())

		vm = &kubevmv1a1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmKey.Name,
				Namespace: vmKey.Namespace,
			},
			Spec: kubevmv1a1.VirtualMachineSpec{
				InfrastructureRef: kubevmv1a1.ObjectReference{
					APIGroup: stubGroup,
					Kind:     stubKind,
					Name:     provider.GetName(),
				},
			},
		}
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})
	})

	When("the provider object mutually names the generic object back", func() {
		BeforeEach(func() {
			provider.SetAnnotations(map[string]string{annotationKey: vm.Name})
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			Expect(k8sClient.Create(ctx, vm)).To(Succeed())
		})

		It("adopts the provider object, setting a controller owner reference", func() {
			Eventually(func(g Gomega) {
				p := newStubProvider(providerNS, provider.GetName())
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: providerNS, Name: provider.GetName()}, p)).To(Succeed())

				refs := p.GetOwnerReferences()
				g.Expect(refs).To(HaveLen(1))
				g.Expect(refs[0].Name).To(Equal(vm.Name))
				g.Expect(refs[0].Kind).To(Equal("VirtualMachine"))
				g.Expect(*refs[0].Controller).To(BeTrue())
			}).Should(Succeed())
		})

		It("mirrors the provider object's status onto the fixed contract paths", func() {
			p := newStubProvider(providerNS, provider.GetName())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: providerNS, Name: provider.GetName()}, p)).To(Succeed())

			addr := map[string]any{"interface": "eth0", "type": "InternalIP", "address": "10.0.0.9"}
			Expect(unstructured.SetNestedSlice(p.Object, []any{addr}, "status", "addresses")).To(Succeed())
			Expect(unstructured.SetNestedField(p.Object, "PoweredOn", "status", "powerState")).To(Succeed())
			Expect(unstructured.SetNestedField(p.Object, "vm-provider-id", "status", "providerID")).To(Succeed())
			conditions := []any{
				map[string]any{"type": "InfrastructureReady", "status": "True", "reason": "Reported", "message": ""},
			}
			Expect(unstructured.SetNestedSlice(p.Object, conditions, "status", "conditions")).To(Succeed())
			Expect(k8sClient.Status().Update(ctx, p)).To(Succeed())

			Eventually(func(g Gomega) {
				obj := &kubevmv1a1.VirtualMachine{}
				g.Expect(k8sClient.Get(ctx, vmKey, obj)).To(Succeed())
				g.Expect(obj.Status.PowerState).To(Equal(kubevmv1a1.PowerStateOn))
				g.Expect(obj.Status.ProviderID).To(Equal("vm-provider-id"))
				g.Expect(obj.Status.Addresses).To(ConsistOf(kubevmv1a1.VirtualMachineAddress{
					Interface: "eth0",
					Type:      kubevmv1a1.VirtualMachineAddressInternalIP,
					Address:   "10.0.0.9",
				}))
				g.Expect(obj.Status.Ready).To(BeTrue())
			}).Should(Succeed())
		})
	})

	When("the reference is one-sided: the provider object does not name the generic object back", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			Expect(k8sClient.Create(ctx, vm)).To(Succeed())
		})

		It("does not adopt it, and reports no adopted infrastructure", func() {
			Consistently(func(g Gomega) {
				p := newStubProvider(providerNS, provider.GetName())
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: providerNS, Name: provider.GetName()}, p)).To(Succeed())
				g.Expect(p.GetOwnerReferences()).To(BeEmpty())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				obj := &kubevmv1a1.VirtualMachine{}
				g.Expect(k8sClient.Get(ctx, vmKey, obj)).To(Succeed())
				g.Expect(obj.Status.Ready).To(BeFalse())
			}).Should(Succeed())
		})
	})

	When("the provider object already has a controller owner reference naming a different generic object", func() {
		BeforeEach(func() {
			isController := true
			provider.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: kubevmv1a1.GroupVersion.String(),
					Kind:       "VirtualMachine",
					Name:       "some-other-generic-vm",
					UID:        types.UID(uuid.NewString()),
					Controller: &isController,
				},
			})
			provider.SetAnnotations(map[string]string{annotationKey: vm.Name})
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			Expect(k8sClient.Create(ctx, vm)).To(Succeed())
		})

		It("refuses to adopt it", func() {
			Consistently(func(g Gomega) {
				p := newStubProvider(providerNS, provider.GetName())
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: providerNS, Name: provider.GetName()}, p)).To(Succeed())
				g.Expect(p.GetOwnerReferences()).To(HaveLen(1))
				g.Expect(p.GetOwnerReferences()[0].Name).To(Equal("some-other-generic-vm"))
			}).Should(Succeed())
		})
	})

	When("the provider object is deleted directly", func() {
		BeforeEach(func() {
			provider.SetAnnotations(map[string]string{annotationKey: vm.Name})
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			Expect(k8sClient.Create(ctx, vm)).To(Succeed())

			Eventually(func(g Gomega) {
				p := newStubProvider(providerNS, provider.GetName())
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: providerNS, Name: provider.GetName()}, p)).To(Succeed())
				g.Expect(p.GetOwnerReferences()).To(HaveLen(1))
			}).Should(Succeed())

			Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
		})

		It("does not recreate it, and reports no adopted infrastructure", func() {
			Consistently(func(g Gomega) {
				p := newStubProvider(providerNS, provider.GetName())
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: providerNS, Name: provider.GetName()}, p)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})
	})

	When("the generic object is deleted", func() {
		BeforeEach(func() {
			provider.SetAnnotations(map[string]string{annotationKey: vm.Name})
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			Expect(k8sClient.Create(ctx, vm)).To(Succeed())

			Eventually(func(g Gomega) {
				obj := &kubevmv1a1.VirtualMachine{}
				g.Expect(k8sClient.Get(ctx, vmKey, obj)).To(Succeed())
				g.Expect(obj.Finalizers).ToNot(BeEmpty())
			}).Should(Succeed())
		})

		It("deletes the provider object, and only then lets the generic object go away", func() {
			Expect(k8sClient.Delete(ctx, vm)).To(Succeed())

			Eventually(func(g Gomega) {
				p := newStubProvider(providerNS, provider.GetName())
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: providerNS, Name: provider.GetName()}, p)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				obj := &kubevmv1a1.VirtualMachine{}
				err := k8sClient.Get(ctx, vmKey, obj)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
