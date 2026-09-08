// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kubevm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/kubevm"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

const (
	genericName  = "my-generic-vm"
	providerName = "my-vm"
	namespace    = "my-namespace"
)

func newProviderVM(annotationValue string) *vmopv1.VirtualMachine {
	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      providerName,
			Namespace: namespace,
		},
	}
	if annotationValue != "" {
		vm.Annotations = map[string]string{
			kubevm.AnnotationKey: annotationValue,
		}
	}
	return vm
}

func newGenericVM(infraRefName string) *kubevmv1a1.VirtualMachine {
	return &kubevmv1a1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      genericName,
			Namespace: namespace,
		},
		Spec: kubevmv1a1.VirtualMachineSpec{
			InfrastructureRef: kubevmv1a1.ObjectReference{
				APIGroup: vmopv1.GroupName,
				Kind:     "VirtualMachine",
				Name:     infraRefName,
			},
		},
	}
}

var _ = Describe("IsMutuallyLinked", func() {
	It("returns true when both objects name each other", func() {
		vm := newProviderVM(genericName)
		generic := newGenericVM(providerName)
		Expect(kubevm.IsMutuallyLinked(vm, generic)).To(BeTrue())
	})

	It("returns false when the provider object carries no annotation", func() {
		vm := newProviderVM("")
		generic := newGenericVM(providerName)
		Expect(kubevm.IsMutuallyLinked(vm, generic)).To(BeFalse())
	})

	It("returns false when the annotation names a different generic object", func() {
		vm := newProviderVM("some-other-generic-vm")
		generic := newGenericVM(providerName)
		Expect(kubevm.IsMutuallyLinked(vm, generic)).To(BeFalse())
	})

	It("returns false when the generic object's infrastructureRef names a different provider object", func() {
		vm := newProviderVM(genericName)
		generic := newGenericVM("some-other-vm")
		Expect(kubevm.IsMutuallyLinked(vm, generic)).To(BeFalse())
	})

	It("returns false when the generic object's infrastructureRef points at a different group or kind", func() {
		vm := newProviderVM(genericName)
		generic := newGenericVM(providerName)
		generic.Spec.InfrastructureRef.APIGroup = "example.com"
		Expect(kubevm.IsMutuallyLinked(vm, generic)).To(BeFalse())
	})
})

var _ = Describe("ConflictingOwner", func() {
	It("returns nil when there is no controller owner reference", func() {
		vm := newProviderVM(genericName)
		generic := newGenericVM(providerName)
		Expect(kubevm.ConflictingOwner(vm, generic)).To(Succeed())
	})

	It("returns nil when the existing controller owner reference names the same generic object", func() {
		vm := newProviderVM(genericName)
		generic := newGenericVM(providerName)
		vm.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: kubevmv1a1.GroupVersion.String(),
				Kind:       "VirtualMachine",
				Name:       genericName,
				UID:        types.UID("abc"),
				Controller: ptr.To(true),
			},
		}
		Expect(kubevm.ConflictingOwner(vm, generic)).To(Succeed())
	})

	It("returns an error when the existing controller owner reference names a different generic object", func() {
		vm := newProviderVM(genericName)
		generic := newGenericVM(providerName)
		vm.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: kubevmv1a1.GroupVersion.String(),
				Kind:       "VirtualMachine",
				Name:       "some-other-generic-vm",
				UID:        types.UID("abc"),
				Controller: ptr.To(true),
			},
		}
		Expect(kubevm.ConflictingOwner(vm, generic)).To(HaveOccurred())
	})

	It("ignores non-controller owner references", func() {
		vm := newProviderVM(genericName)
		generic := newGenericVM(providerName)
		vm.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: kubevmv1a1.GroupVersion.String(),
				Kind:       "VirtualMachine",
				Name:       "some-other-generic-vm",
				UID:        types.UID("abc"),
				Controller: ptr.To(false),
			},
		}
		Expect(kubevm.ConflictingOwner(vm, generic)).To(Succeed())
	})
})

var _ = Describe("ApplyDelegatedFields", func() {
	var (
		vm      *vmopv1.VirtualMachine
		generic *kubevmv1a1.VirtualMachine
	)

	BeforeEach(func() {
		vm = newProviderVM(genericName)
		generic = newGenericVM(providerName)
		generic.Spec.InstanceType = &kubevmv1a1.InstanceTypeSpec{Name: "best-effort-2xlarge"}
		generic.Spec.BootDisk = &kubevmv1a1.BootDiskSpec{
			Source: kubevmv1a1.DiskSource{
				Image: &kubevmv1a1.ObjectReference{
					APIGroup: vmopv1.GroupName,
					Kind:     "VirtualMachineImage",
					Name:     "vmi-0123456789abcdef0",
				},
			},
			StorageClassName: "my-storage-class",
		}
		generic.Spec.PowerState = kubevmv1a1.PowerStateOn
	})

	It("fills every empty delegated field from the generic object", func() {
		kubevm.ApplyDelegatedFields(vm, generic)

		Expect(vm.Spec.ClassName).To(Equal("best-effort-2xlarge"))
		Expect(vm.Spec.Image).To(Equal(&vmopv1.VirtualMachineImageRef{
			Kind: "VirtualMachineImage",
			Name: "vmi-0123456789abcdef0",
		}))
		Expect(vm.Spec.StorageClass).To(Equal("my-storage-class"))
		Expect(vm.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
	})

	It("does not overwrite a field the user already set", func() {
		vm.Spec.ClassName = "user-set-class"
		vm.Spec.StorageClass = "user-set-storage-class"
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff

		kubevm.ApplyDelegatedFields(vm, generic)

		Expect(vm.Spec.ClassName).To(Equal("user-set-class"))
		Expect(vm.Spec.StorageClass).To(Equal("user-set-storage-class"))
		Expect(vm.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
	})

	It("leaves a field empty when the generic object's corresponding field is empty", func() {
		generic.Spec.InstanceType = nil
		generic.Spec.BootDisk = nil

		kubevm.ApplyDelegatedFields(vm, generic)

		Expect(vm.Spec.ClassName).To(BeEmpty())
		Expect(vm.Spec.Image).To(BeNil())
		Expect(vm.Spec.StorageClass).To(BeEmpty())
	})
})

var _ = Describe("DelegatedFieldsUpToDate", func() {
	var (
		vm      *vmopv1.VirtualMachine
		generic *kubevmv1a1.VirtualMachine
	)

	BeforeEach(func() {
		vm = newProviderVM(genericName)
		vm.Spec.ClassName = "best-effort-2xlarge"
		vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
			Kind: "VirtualMachineImage",
			Name: "vmi-0123456789abcdef0",
		}
		vm.Spec.StorageClass = "my-storage-class"

		generic = newGenericVM(providerName)
		generic.Spec.InstanceType = &kubevmv1a1.InstanceTypeSpec{Name: "best-effort-2xlarge"}
		generic.Spec.BootDisk = &kubevmv1a1.BootDiskSpec{
			Source: kubevmv1a1.DiskSource{
				Image: &kubevmv1a1.ObjectReference{
					APIGroup: vmopv1.GroupName,
					Kind:     "VirtualMachineImage",
					Name:     "vmi-0123456789abcdef0",
				},
			},
			StorageClassName: "my-storage-class",
		}
	})

	It("returns true when the persisted values still match the generic object", func() {
		Expect(kubevm.DelegatedFieldsUpToDate(vm, generic)).To(BeTrue())
	})

	It("returns false when the instance type name diverges", func() {
		generic.Spec.InstanceType.Name = "best-effort-4xlarge"
		Expect(kubevm.DelegatedFieldsUpToDate(vm, generic)).To(BeFalse())
	})

	It("returns false when the image reference diverges", func() {
		generic.Spec.BootDisk.Source.Image.Name = "vmi-fedcba9876543210f"
		Expect(kubevm.DelegatedFieldsUpToDate(vm, generic)).To(BeFalse())
	})

	It("returns false when the storage class diverges", func() {
		generic.Spec.BootDisk.StorageClassName = "some-other-storage-class"
		Expect(kubevm.DelegatedFieldsUpToDate(vm, generic)).To(BeFalse())
	})
})
