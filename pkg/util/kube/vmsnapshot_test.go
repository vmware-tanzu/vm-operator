// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("CalculateReservedForSnapshot", func() {
	var (
		ctx          context.Context
		client       ctrlclient.Client
		withObjects  []ctrlclient.Object
		withFuncs    interceptor.Funcs
		namespace    string
		vmClass      *vmopv1.VirtualMachineClass
		storageClass *storagev1.StorageClass
		vm           *vmopv1.VirtualMachine
		vmSnapshot   *vmopv1.VirtualMachineSnapshot
		requested    []vmopv1.VirtualMachineSnapshotStorageStatusRequested
		err          error
		size10GB     = *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI)
		size20GB     = *resource.NewQuantity(20*1024*1024*1024, resource.BinarySI)
	)

	const (
		pvcName1 = "claim1"
		pvcName2 = "claim2"
	)
	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		namespace = "default-namespace"
		withFuncs = interceptor.Funcs{}
		withObjects = []ctrlclient.Object{
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
		}

		vmClass = builder.DummyVirtualMachineClass("vm-class")
		vmClass.Spec.Hardware.Memory = size10GB
		vmClass.Namespace = namespace
		vm = builder.DummyBasicVirtualMachine("dummy-vm", namespace)
		vm.Spec.ClassName = vmClass.Name
		vmSnapshot = builder.DummyVirtualMachineSnapshot(namespace, "my-snapshot", vm.Name)
		storageClass = builder.DummyStorageClass()
		vm.Spec.StorageClass = storageClass.Name
		addVolumeToVM(vm, "", "classic-disk", "", size10GB, false, false)
	})

	JustBeforeEach(func() {
		withObjects = append(withObjects, vmSnapshot, vm, storageClass, vmClass)

		client = builder.NewFakeClientWithInterceptors(withFuncs, withObjects...)

		requested, err = kubeutil.CalculateReservedForSnapshotPerStorageClass(ctx, client, logr.Discard(), *vmSnapshot)
	})

	When("There is a VM points to a storage class", func() {
		It("sum the disk of VM", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(requested).To(HaveLen(1))
			Expect(requested[0].StorageClass).To(Equal(storageClass.Name))
			Expect(requested[0].Total.Value()).To(Equal(size10GB.Value()))
		})
	})

	When("snapshot.spec.memory is true", func() {
		BeforeEach(func() {
			vmSnapshot.Spec.Memory = true
		})
		It("should include the VM's memory in the requested capacity list", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(requested).To(HaveLen(1))
			Expect(requested[0].StorageClass).To(Equal(storageClass.Name))
			Expect(requested[0].Total.Value()).To(Equal(size20GB.Value()))
		})
	})

	When("VM is not found", func() {
		BeforeEach(func() {
			vmSnapshot.Spec.VMName = "unknown-vm"
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get VM"))
		})
	})

	When("VM has a FCD attached but the PVC is not found", func() {
		BeforeEach(func() {
			addVolumeToVM(vm, "unknown-pvc", "fcd-1", "", size10GB, true, false)
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get pvc"))
		})
	})

	When("VM has a FCD attached but the PVC's storage class is not set", func() {
		BeforeEach(func() {
			pvc1 := builder.DummyPersistentVolumeClaim()
			pvc1.Name = "claim1"
			pvc1.Namespace = namespace
			pvc1.Spec.StorageClassName = nil
			addVolumeToVM(vm, pvc1.Name, "fcd-1", "", resource.Quantity{}, true, false)
			Expect(vm.Status.Volumes).To(HaveLen(2))
			vm.Status.Volumes[1].Requested = nil

			withObjects = append(withObjects, pvc1)
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("storage class is not set for pvc claim1"))
		})
	})

	When("VM has a FCD attached but the PVC's requested is nil", func() {
		BeforeEach(func() {
			pvc1 := builder.DummyPersistentVolumeClaim()
			pvc1.Name = "claim1"
			pvc1.Namespace = namespace
			pvc1.Spec.Resources.Requests = nil
			addVolumeToVM(vm, pvc1.Name, "fcd-1", "", resource.Quantity{}, true, false)
			Expect(vm.Status.Volumes).To(HaveLen(2))
			vm.Status.Volumes[1].Requested = nil

			withObjects = append(withObjects, pvc1)
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requests is not set for pvc claim1"))
		})
	})

	When("VMClass is not found", func() {
		BeforeEach(func() {
			vmSnapshot.Spec.Memory = true
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Spec.ClassName = "unknown-vm-class"
		})
		It("should return error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get VMClass"))
		})
	})

	When("There is a VM with a storage class, also a FCD with same storage class", func() {
		var pvc1 *corev1.PersistentVolumeClaim
		BeforeEach(func() {
			addVolumeToVM(vm, pvcName1, "fcd-1", "", size10GB, true, false)
			pvc1 = builder.DummyPersistentVolumeClaim()
			pvc1.Name = pvcName1
			pvc1.Namespace = namespace
			pvc1.Spec.StorageClassName = ptr.To(storageClass.Name)
			pvc1.Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: size10GB,
			}
			withObjects = append(withObjects, pvc1)
		})

		It("should return 1 item in the requested capacity list", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(requested).To(HaveLen(1))
			Expect(requested[0].StorageClass).To(Equal(storageClass.Name))
			Expect(requested[0].Total.Value()).To(Equal(size20GB.Value()))
		})
	})

	When("There is a VM with a storage class, also a FCD with same storage class and the FCD is an instance storage volume", func() {
		BeforeEach(func() {
			addVolumeToVM(vm, pvcName1, "fcd-1", storageClass.Name, size10GB, true, true)
		})

		It("should return 1 item in the requested capacity list", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(requested).To(HaveLen(1))
			Expect(requested[0].StorageClass).To(Equal(storageClass.Name))
			Expect(requested[0].Total.Value()).To(Equal(size20GB.Value()))
		})

		When("there is another FCD as instance storage volume that points to a different storage class", func() {
			var storageClass2 = "storage-class-2"
			BeforeEach(func() {
				addVolumeToVM(vm, pvcName1, "fcd-2", storageClass2, size10GB, true, true)
			})
			It("should return 2 items in the requested capacity list", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(requested).To(HaveLen(2))
				Expect(requested).To(ContainElement(vmopv1.VirtualMachineSnapshotStorageStatusRequested{
					StorageClass: storageClass.Name,
					Total:        ptr.To(size20GB),
				}))
				Expect(requested).To(ContainElement(vmopv1.VirtualMachineSnapshotStorageStatusRequested{
					StorageClass: storageClass2,
					Total:        ptr.To(size10GB),
				}))
			})
		})
	})

	When("There is a VM with a storage class, also a FCD with same storage class and another PVC with different storage class", func() {
		var (
			pvc1, pvc2    *corev1.PersistentVolumeClaim
			storageClass2 = "storage-class-2"
		)
		BeforeEach(func() {
			pvc1 = builder.DummyPersistentVolumeClaim()
			pvc1.Name = pvcName1
			pvc1.Namespace = namespace
			pvc1.Spec.StorageClassName = ptr.To(storageClass.Name)
			pvc1.Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: size10GB,
			}
			pvc2 = builder.DummyPersistentVolumeClaim()
			pvc2.Name = pvcName2
			pvc2.Namespace = namespace
			pvc2.Spec.StorageClassName = ptr.To(storageClass2)
			pvc2.Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: size10GB,
			}

			addVolumeToVM(vm, pvc1.Name, "fcd-1", "", size10GB, true, false)
			addVolumeToVM(vm, pvc2.Name, "fcd-2", "", size10GB, true, false)

			withObjects = append(withObjects, pvc1, pvc2)
		})

		It("should return 2 items in the requested capacity list", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(requested).To(HaveLen(2))
			Expect(requested).To(ContainElement(vmopv1.VirtualMachineSnapshotStorageStatusRequested{
				StorageClass: storageClass.Name,
				Total:        ptr.To(size20GB),
			}))
			Expect(requested).To(ContainElement(vmopv1.VirtualMachineSnapshotStorageStatusRequested{
				StorageClass: storageClass2,
				Total:        ptr.To(size10GB),
			}))
		})
	})
})

var _ = Describe("PatchSnapshotSuccessStatus", func() {
	var (
		k8sClient   ctrlclient.Client
		initObjects []ctrlclient.Object

		vmCtx pkgctx.VirtualMachineContext
	)

	BeforeEach(func() {
		vm := builder.DummyBasicVirtualMachine("test-vm", "dummy-ns")

		vmCtx = pkgctx.VirtualMachineContext{
			Context: context.Background(),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClient(initObjects...)
	})

	AfterEach(func() {
		k8sClient = nil
		initObjects = nil
	})

	Context("PatchSnapshotStatus", func() {
		var (
			vmSnapshot *vmopv1.VirtualMachineSnapshot
			snapNode   *vimtypes.VirtualMachineSnapshotTree
		)

		BeforeEach(func() {
			timeout, _ := time.ParseDuration("1h35m")
			vmSnapshot = &vmopv1.VirtualMachineSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "vmoperator.vmware.com/v1alpha5",
					Kind:       "VirtualMachineSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snap-1",
					Namespace: vmCtx.VM.Namespace,
				},
				Spec: vmopv1.VirtualMachineSnapshotSpec{
					VMName: vmCtx.VM.Name,
					Quiesce: &vmopv1.QuiesceSpec{
						Timeout: &metav1.Duration{Duration: timeout},
					},
				},
			}
		})

		When("snapshot patched with vm info and created condition", func() {
			BeforeEach(func() {
				vmCtx.VM.Status = vmopv1.VirtualMachineStatus{
					UniqueID: "dummyID",
				}

				snapNode = &vimtypes.VirtualMachineSnapshotTree{
					Snapshot: vimtypes.ManagedObjectReference{
						Value: "snap-103",
					},
				}

				initObjects = append(initObjects, vmSnapshot)
			})

			It("succeeds", func() {
				err := kubeutil.PatchSnapshotSuccessStatus(
					vmCtx,
					logr.Discard(),
					k8sClient,
					vmSnapshot,
					snapNode,
					vmCtx.VM.Spec.PowerState)
				Expect(err).ToNot(HaveOccurred())

				snapObj := &vmopv1.VirtualMachineSnapshot{}
				Expect(k8sClient.Get(
					vmCtx,
					ctrlclient.ObjectKey{
						Name:      vmSnapshot.Name,
						Namespace: vmSnapshot.Namespace},
					snapObj)).To(Succeed())
				Expect(snapObj.Status.UniqueID).To(Equal(snapNode.Snapshot.Value))
				Expect(snapObj.Status.Quiesced).To(BeTrue())
				Expect(snapObj.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				Expect(conditions.IsTrue(snapObj, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())
			})

			DescribeTable("memory and power state", func(
				memory bool,
				powerState vmopv1.VirtualMachinePowerState,
				expectedPowerState vmopv1.VirtualMachinePowerState) {

				if memory {
					vmSnapshot.Spec.Memory = true
					// When a VM is off, can't take snapshot with memory on VC UI.
					if powerState == vmopv1.VirtualMachinePowerStateOn {
						snapNode.State = vimtypes.VirtualMachinePowerStatePoweredOn
					}
				} else {
					vmSnapshot.Spec.Memory = false
					snapNode.State = vimtypes.VirtualMachinePowerStatePoweredOff
				}

				Expect(kubeutil.PatchSnapshotSuccessStatus(vmCtx,
					logr.Discard(),
					k8sClient,
					vmSnapshot,
					snapNode,
					powerState)).To(Succeed())

				snapObj := &vmopv1.VirtualMachineSnapshot{}
				Expect(k8sClient.Get(vmCtx,
					ctrlclient.ObjectKey{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, snapObj)).
					To(Succeed())
				Expect(snapObj.Status.PowerState).To(Equal(expectedPowerState))
			},
				Entry("memory is true, VM is on, expected power state is on",
					true, vmopv1.VirtualMachinePowerStateOn, vmopv1.VirtualMachinePowerStateOn),
				Entry("memory is false, VM is on, expected power state is off",
					false, vmopv1.VirtualMachinePowerStateOn, vmopv1.VirtualMachinePowerStateOff),
				// This option is disabled on the VC UI
				Entry("memory is true, VM is off, expected power state is off",
					true, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff),
				Entry("memory is false, VM is off, expected power state is off",
					false, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff),
			)
		})
	})
})

func addVolumeToVM(vm *vmopv1.VirtualMachine, pvcName, volumeName, storageClass string, size resource.Quantity, isFCD, isInstanceStorage bool) {
	if isFCD {
		vm.Spec.Volumes = append(vm.Spec.Volumes,
			vmopv1.VirtualMachineVolume{
				Name: volumeName,
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			})
		if isInstanceStorage {
			// change the added volume to instance storage volume
			vm.Spec.Volumes[len(vm.Spec.Volumes)-1].PersistentVolumeClaim.InstanceVolumeClaim = &vmopv1.InstanceVolumeClaimVolumeSource{
				StorageClass: storageClass,
				Size:         size,
			}
		}
	}
	volumeType := vmopv1.VolumeTypeClassic
	if isFCD {
		volumeType = vmopv1.VolumeTypeManaged
	}
	vm.Status.Volumes = append(vm.Status.Volumes,
		vmopv1.VirtualMachineVolumeStatus{
			Name:      volumeName,
			Requested: &size,
			Type:      volumeType,
		})
}
