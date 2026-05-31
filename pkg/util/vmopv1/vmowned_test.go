// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("IsVMOwnedStorageVM", func() {
	It("should return false for nil VM", func() {
		Expect(vmopv1util.IsVMOwnedStorageVM(nil)).To(BeFalse())
	})

	It("should return false when VM has no annotations", func() {
		vm := &vmopv1.VirtualMachine{}
		Expect(vmopv1util.IsVMOwnedStorageVM(vm)).To(BeFalse())
	})

	It("should return false when annotation is absent", func() {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"some-other": "annotation"},
			},
		}
		Expect(vmopv1util.IsVMOwnedStorageVM(vm)).To(BeFalse())
	})

	It("should return false when annotation value is not 'true'", func() {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{pkgconst.VMOwnedVolumesAnnotation: "false"},
			},
		}
		Expect(vmopv1util.IsVMOwnedStorageVM(vm)).To(BeFalse())
	})

	It("should return true when annotation is 'true'", func() {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{pkgconst.VMOwnedVolumesAnnotation: "true"},
			},
		}
		Expect(vmopv1util.IsVMOwnedStorageVM(vm)).To(BeTrue())
	})
})

var _ = Describe("GetCVIByVolumeID", func() {
	const (
		ns       = "test-ns"
		volumeID = "abc-123"
	)

	var (
		fakeClient ctrlclient.Client
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = builder.NewScheme()
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("should return nil when no CsiVolumeInfo exists for the volume", func() {
		cvi, err := vmopv1util.GetCVIByVolumeID(suite, fakeClient, ns, volumeID)
		Expect(err).ToNot(HaveOccurred())
		Expect(cvi).To(BeNil())
	})

	It("should return the CsiVolumeInfo by deterministic name", func() {
		expected := &cnsv1alpha1.CsiVolumeInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "csi-volume-info-" + volumeID,
				Namespace: ns,
			},
			Spec: cnsv1alpha1.CsiVolumeInfoSpec{VolumeID: volumeID},
		}
		Expect(fakeClient.Create(suite, expected)).To(Succeed())

		cvi, err := vmopv1util.GetCVIByVolumeID(suite, fakeClient, ns, volumeID)
		Expect(err).ToNot(HaveOccurred())
		Expect(cvi).ToNot(BeNil())
		Expect(cvi.Spec.VolumeID).To(Equal(volumeID))
	})
})

var _ = Describe("ShouldUseVMOwnedStoragePath", func() {
	makeVM := func(annotated bool) *vmopv1.VirtualMachine {
		vm := &vmopv1.VirtualMachine{}
		if annotated {
			vm.Annotations = map[string]string{pkgconst.VMOwnedVolumesAnnotation: "true"}
		}
		return vm
	}

	It("returns false when feature gate is disabled", func() {
		ctx := pkgcfg.UpdateContext(suite, func(c *pkgcfg.Config) {
			c.Features.VMOwnedVolumes = false
		})
		Expect(vmopv1util.ShouldUseVMOwnedStoragePath(ctx, makeVM(true))).To(BeFalse())
	})

	It("returns false when VM is not VM-owned", func() {
		ctx := pkgcfg.UpdateContext(suite, func(c *pkgcfg.Config) {
			c.Features.VMOwnedVolumes = true
		})
		Expect(vmopv1util.ShouldUseVMOwnedStoragePath(ctx, makeVM(false))).To(BeFalse())
	})

	It("returns true when feature gate is enabled and VM is VM-owned", func() {
		ctx := pkgcfg.UpdateContext(suite, func(c *pkgcfg.Config) {
			c.Features.VMOwnedVolumes = true
		})
		Expect(vmopv1util.ShouldUseVMOwnedStoragePath(ctx, makeVM(true))).To(BeTrue())
	})
})

var _ = Describe("GetCVIByDiskUUID", func() {
	const (
		ns       = "test-ns"
		diskUUID = "6000C29-abc"
	)

	var (
		fakeClient ctrlclient.Client
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = builder.NewScheme()
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("should return nil when no CsiVolumeInfo has the disk-uuid label", func() {
		cvi, err := vmopv1util.GetCVIByDiskUUID(suite, fakeClient, ns, diskUUID)
		Expect(err).ToNot(HaveOccurred())
		Expect(cvi).To(BeNil())
	})

	It("should return the CsiVolumeInfo matching the disk-uuid label", func() {
		expected := &cnsv1alpha1.CsiVolumeInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "csi-volume-info-vol-1",
				Namespace: ns,
				Labels: map[string]string{
					cnsv1alpha1.CVIDiskUUIDLabel: diskUUID,
				},
			},
			Spec: cnsv1alpha1.CsiVolumeInfoSpec{VolumeID: "vol-1"},
		}
		Expect(fakeClient.Create(suite, expected)).To(Succeed())

		cvi, err := vmopv1util.GetCVIByDiskUUID(suite, fakeClient, ns, diskUUID)
		Expect(err).ToNot(HaveOccurred())
		Expect(cvi).ToNot(BeNil())
		Expect(cvi.Spec.VolumeID).To(Equal("vol-1"))
	})

	It("should return the first match when multiple CsiVolumeInfos share a disk UUID", func() {
		// This is an exceptional case; by design each diskUUID maps to one volume.
		cvi1 := &cnsv1alpha1.CsiVolumeInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "csi-volume-info-vol-2",
				Namespace: ns,
				Labels: map[string]string{cnsv1alpha1.CVIDiskUUIDLabel: diskUUID},
			},
			Spec: cnsv1alpha1.CsiVolumeInfoSpec{VolumeID: "vol-2"},
		}
		Expect(fakeClient.Create(suite, cvi1)).To(Succeed())

		cvi, err := vmopv1util.GetCVIByDiskUUID(suite, fakeClient, ns, diskUUID)
		Expect(err).ToNot(HaveOccurred())
		Expect(cvi).ToNot(BeNil())
	})
})
