// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storage_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/storage"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("GetVMStorageData", func() {

	var (
		initObjects  []client.Object
		client       client.Client
		storageClass *storagev1.StorageClass
		pvc          *corev1.PersistentVolumeClaim

		vmCtx pkgctx.VirtualMachineContext
	)

	BeforeEach(func() {
		vm := builder.DummyBasicVirtualMachine("storage-test", "storage-test-ns")
		vmCtx = pkgctx.VirtualMachineContext{
			Context: context.Background(),
			Logger:  logf.Log,
			VM:      vm,
		}

		storageClass = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-class",
			},
			Parameters: map[string]string{
				"storagePolicyID": "id1",
			},
		}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pvc1",
				Namespace: vmCtx.VM.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: new(storageClass.Name),
			},
		}
	})

	JustBeforeEach(func() {
		client = builder.NewFakeClient(initObjects...)
	})

	AfterEach(func() {
		initObjects = nil
	})

	Context("VM StorageClass does not exist", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.StorageClass = storageClass.Name
		})

		It("returns error", func() {
			_, err := storage.GetVMStorageData(vmCtx, client)
			Expect(err).To(MatchError(`storageclasses.storage.k8s.io "my-class" not found`))
		})
	})

	Context("VM with PVC", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, storageClass)

			vmCtx.VM.Spec.Volumes = append(vmCtx.VM.Spec.Volumes, vmopv1.VirtualMachineVolume{
				Name: "volume1",
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc.Name,
						},
					},
				},
			})
		})

		When("PVC does not exist", func() {
			It("returns error", func() {
				_, err := storage.GetVMStorageData(vmCtx, client)
				Expect(err).To(MatchError(`persistentvolumeclaims "pvc1" not found`))
			})
		})

		When("PVC exists", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, pvc)
			})

			It("returns success", func() {
				data, err := storage.GetVMStorageData(vmCtx, client)
				Expect(err).ToNot(HaveOccurred())
				Expect(data).ToNot(BeNil())
				Expect(data.StorageClasses).To(HaveLen(1))
				Expect(data.StorageClasses).To(HaveKey("my-class"))
				Expect(data.StorageClassToPolicyID).To(HaveLen(1))
				Expect(data.StorageClassToPolicyID).To(HaveKeyWithValue("my-class", "id1"))
				Expect(data.PVCs).To(HaveLen(1))
				Expect(data.PVCs[0].Name).To(Equal("pvc1"))
			})
		})
	})
})
