// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func AssertAttachmentSpecFromVMVol(
	vm *vmopv1.VirtualMachine,
	vmVol vmopv1.VirtualMachineVolume,
	attachment *cnsv1alpha1.CnsNodeVmAttachment) {

	GinkgoHelper()

	Expect(attachment.Spec.NodeUUID).To(Equal(vm.Status.BiosUUID))
	Expect(attachment.Spec.VolumeName).To(Equal(vmVol.PersistentVolumeClaim.ClaimName))

	ownerRefs := attachment.GetOwnerReferences()
	Expect(ownerRefs).To(HaveLen(1))
	ownerRef := ownerRefs[0]
	Expect(ownerRef.Name).To(Equal(vm.Name))
	Expect(ownerRef.Controller).ToNot(BeNil())
	Expect(*ownerRef.Controller).To(BeTrue())
}

func AssertVMVolStatusFromAttachment(
	vmVol vmopv1.VirtualMachineVolume,
	attachment *cnsv1alpha1.CnsNodeVmAttachment,
	vmVolStatus vmopv1.VirtualMachineVolumeStatus) {

	GinkgoHelper()

	diskUUID := attachment.Status.AttachmentMetadata[cnsv1alpha1.AttributeFirstClassDiskUUID]

	Expect(vmVolStatus.Name).To(Equal(vmVol.Name))
	Expect(vmVolStatus.Attached).To(Equal(attachment.Status.Attached))
	Expect(vmVolStatus.DiskUUID).To(Equal(diskUUID))
	Expect(vmVolStatus.Error).To(Equal(attachment.Status.Error))
}

func AssertVMVolStatusFromAttachmentDetaching(
	vmVol vmopv1.VirtualMachineVolume,
	attachment *cnsv1alpha1.CnsNodeVmAttachment,
	vmVolStatus vmopv1.VirtualMachineVolumeStatus) {

	GinkgoHelper()

	diskUUID := attachment.Status.AttachmentMetadata[cnsv1alpha1.AttributeFirstClassDiskUUID]

	Expect(vmVolStatus.Name).To(Equal(vmVol.Name + ":detaching"))
	Expect(vmVolStatus.Attached).To(Equal(attachment.Status.Attached))
	Expect(vmVolStatus.DiskUUID).To(Equal(diskUUID))
	Expect(vmVolStatus.Error).To(Equal(attachment.Status.Error))
}

func GetCNSAttachmentForVolumeName(ctx *builder.UnitTestContextForController, vm *vmopv1.VirtualMachine, volumeName string) *cnsv1alpha1.CnsNodeVmAttachment {

	GinkgoHelper()

	objectKey := client.ObjectKey{Name: util.CNSAttachmentNameForVolume(vm.Name, volumeName), Namespace: vm.Namespace}
	attachment := &cnsv1alpha1.CnsNodeVmAttachment{}

	err := ctx.Client.Get(ctx, objectKey, attachment)
	if err == nil {
		return attachment
	}

	if apierrors.IsNotFound(err) {
		return nil
	}

	Expect(err).ToNot(HaveOccurred())
	return nil
}
