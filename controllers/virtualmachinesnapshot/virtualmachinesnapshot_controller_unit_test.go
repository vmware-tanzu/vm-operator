// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesnapshot_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesnapshot"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.API,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler              *virtualmachinesnapshot.Reconciler
		vmSnapshot              *vmopv1.VirtualMachineSnapshot
		vm                      *vmopv1.VirtualMachine
		fakeVMProvider          *providerfake.VMProvider
		skipReconcile           bool
		vmSnapshotNamespacedKey types.NamespacedName
	)

	const (
		dummyVMUUID = "unique-vm-id"
		namespace   = "test-namespace"
	)

	BeforeEach(func() {
		initObjects = nil
		skipReconcile = false
		vm = builder.DummyBasicVirtualMachine("dummy-vm", namespace)
		vmSnapshot = builder.DummyVirtualMachineSnapshot(namespace, "snap-1", vm.Name)
		vmSnapshotNamespacedKey = types.NamespacedName{
			Namespace: vmSnapshot.Namespace,
			Name:      vmSnapshot.Name,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		reconciler = virtualmachinesnapshot.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
	})

	Context("Reconcile", func() {
		var (
			err error
		)

		BeforeEach(func() {
			initObjects = append(initObjects, vmSnapshot)
			err = nil
		})

		JustBeforeEach(func() {
			if !skipReconcile {
				_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
			}
		})

		When("vm does not exist", func() {
			It("returns failure", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		When("vm resource exists but not ready", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vm)
			})

			It("returns failure", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("VM hasn't been created and has no uniqueID"))
			})
		})

		When("vm ready with empty current snapshot ", func() {
			BeforeEach(func() {
				vm.Status.UniqueID = dummyVMUUID
				initObjects = append(initObjects, vm)
			})

			It("returns success, and update snapshot's status requested capacity", func() {
				Expect(err).ToNot(HaveOccurred())
				objKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				vmObj := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, objKey, vmObj)).To(Succeed())
				Expect(vmObj.Spec.CurrentSnapshot).To(BeNil())
			})
		})

		When("vm ready, and calling GetSnapshotSize to update snapshot capacity", func() {
			BeforeEach(func() {
				vm.Status.UniqueID = dummyVMUUID
				initObjects = append(initObjects, vm)
				skipReconcile = true
			})
			When("fails", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetSnapshotSizeFn = func(_ context.Context, _ string, _ *vmopv1.VirtualMachine) (int64, error) {
						return 0, errors.New("fubar")
					}
				})

				It("returns error", func() {
					_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("fubar"))
				})
			})

			When("succeeds", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetSnapshotSizeFn = func(_ context.Context, _ string, _ *vmopv1.VirtualMachine) (int64, error) {
						return 1024, nil
					}
				})

				It("returns success and updates status", func() {
					_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
					Expect(err).ToNot(HaveOccurred())
					newVMSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
					Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, newVMSnapshotObj)).To(Succeed())
					Expect(newVMSnapshotObj.Status.Storage).ToNot(BeNil())
					Expect(newVMSnapshotObj.Status.Storage.Used).ToNot(BeNil())
					Expect(newVMSnapshotObj.Status.Storage.Used).To(Equal(ptr.To(resource.MustParse("1Ki"))))
				})
			})
		})

		When("vm ready, with disks", func() {
			When("the disk is a classic disk", func() {
				BeforeEach(func() {
					vm.Status.UniqueID = dummyVMUUID
					vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:      "vm-1-pvc-1",
							Requested: ptr.To(resource.MustParse("1Gi")),
							Type:      vmopv1.VolumeTypeClassic,
						},
					}
					initObjects = append(initObjects, vm)
				})
				It("returns success and updates status with requested capacity with 1 item", func() {
					Expect(err).ToNot(HaveOccurred())
					newVMSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
					Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, newVMSnapshotObj)).To(Succeed())
					Expect(newVMSnapshotObj.Status.Storage).ToNot(BeNil())
					Expect(newVMSnapshotObj.Status.Storage.Requested).ToNot(BeNil())
					Expect(newVMSnapshotObj.Status.Storage.Requested).To(HaveLen(1))
					Expect(newVMSnapshotObj.Status.Storage.Requested[0].StorageClass).To(Equal(vm.Spec.StorageClass))
					Expect(newVMSnapshotObj.Status.Storage.Requested[0].Total).To(Equal(ptr.To(resource.MustParse("1Gi"))))
				})
			})
			When("the VM has a classic disk and a FCD with different storage class", func() {
				BeforeEach(func() {
					vm.Status.UniqueID = dummyVMUUID
					vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:      "vm-1-classic-1",
							Requested: ptr.To(resource.MustParse("1Gi")),
							Type:      vmopv1.VolumeTypeClassic,
						},
						{
							Name:      "vm-1-fcd-1",
							Requested: ptr.To(resource.MustParse("1Gi")),
							Type:      vmopv1.VolumeTypeManaged,
						},
					}
					pvc1 := builder.DummyPersistentVolumeClaim()
					pvc1.Name = "claim1"
					pvc1.Namespace = namespace
					pvc1.Spec.StorageClassName = ptr.To("different-storage-class")
					pvc1.Spec.Resources.Requests = corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					}
					vm.Spec.Volumes = append(vm.Spec.Volumes,
						vmopv1.VirtualMachineVolume{
							Name: "vm-1-fcd-1",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: pvc1.Name,
									},
								},
							},
						})
					initObjects = append(initObjects, vm, pvc1)
				})
				It("returns success and updates status with requested capacity with 2 items", func() {
					Expect(err).ToNot(HaveOccurred())
					newVMSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
					Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, newVMSnapshotObj)).To(Succeed())
					Expect(newVMSnapshotObj.Status.Storage).ToNot(BeNil())
					Expect(newVMSnapshotObj.Status.Storage.Requested).ToNot(BeNil())
					Expect(newVMSnapshotObj.Status.Storage.Requested).To(HaveLen(2))
					Expect(newVMSnapshotObj.Status.Storage.Requested).To(ContainElement(vmopv1.VirtualMachineSnapshotStorageStatusRequested{
						StorageClass: vm.Spec.StorageClass,
						Total:        ptr.To(resource.MustParse("1Gi")),
					}))
					Expect(newVMSnapshotObj.Status.Storage.Requested).To(ContainElement(vmopv1.VirtualMachineSnapshotStorageStatusRequested{
						StorageClass: "different-storage-class",
						Total:        ptr.To(resource.MustParse("1Gi")),
					}))
				})
			})
		})

		When("object does not have finalizer set", func() {
			BeforeEach(func() {
				vmSnapshot.Finalizers = nil
				initObjects = nil
				initObjects = append(initObjects, vmSnapshot)
			})

			It("will set finalizer", func() {
				Expect(err).NotTo(HaveOccurred())
				vmSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
				Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, vmSnapshotObj)).To(Succeed())
				Expect(vmSnapshotObj.GetFinalizers()).To(ContainElement(virtualmachinesnapshot.Finalizer))
			})
		})

		When("snapshot is ready", func() {
			BeforeEach(func() {
				conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
				initObjects = nil
				initObjects = append(initObjects, vmSnapshot)

				vm.Status.UniqueID = dummyVMUUID
				initObjects = append(initObjects, vm)
			})

			It("returns success, and set the annotation to requested", func() {
				Expect(err).ToNot(HaveOccurred())
				vmSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
				Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, vmSnapshotObj)).To(Succeed())
				Expect(vmSnapshotObj.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey]).To(Equal(constants.CSIVSphereVolumeSyncAnnotationValueRequest))
			})

			When("annotation is already set to completed", func() {
				BeforeEach(func() {
					vmSnapshot.Annotations = map[string]string{constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueCompleted}
				})
				It("returns success, and does not change the annotation", func() {
					Expect(err).ToNot(HaveOccurred())
					vmSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
					Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, vmSnapshotObj)).To(Succeed())
					Expect(vmSnapshotObj.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey]).To(Equal(constants.CSIVSphereVolumeSyncAnnotationValueCompleted))
				})
			})

			When("annotation is set to something unknown", func() {
				BeforeEach(func() {
					vmSnapshot.ObjectMeta.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] = "whatever"
				})
				It("returns success, and set the annotation to requested", func() {
					Expect(err).ToNot(HaveOccurred())
					vmSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
					Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, vmSnapshotObj)).To(Succeed())
					Expect(vmSnapshotObj.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey]).To(Equal(constants.CSIVSphereVolumeSyncAnnotationValueRequest))
				})
			})
		})
	})

	Context("ReconcileDelete", func() {
		var (
			err                     error
			now                     metav1.Time
			vmSnapshotNamespacedKey types.NamespacedName
			skipReconcile           bool
		)

		BeforeEach(func() {
			vmSnapshotNamespacedKey = types.NamespacedName{
				Namespace: vmSnapshot.Namespace,
				Name:      vmSnapshot.Name,
			}
			err = nil
			skipReconcile = false
		})

		JustBeforeEach(func() {
			if !skipReconcile {
				_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
			}
		})

		When("A Snapshot is marked for deletion", func() {
			BeforeEach(func() {
				now = metav1.Now()
				vmSnapshot.DeletionTimestamp = &now
				initObjects = append(initObjects, vmSnapshot, vm)
			})

			It("returns success", func() {
				_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
				Expect(err).ToNot(HaveOccurred())
			})

			When("Calling DeleteSnapshot to VC", func() {
				BeforeEach(func() {
					skipReconcile = true
				})
				When("VC returns VirtualMachineNotFound error", func() {
					JustBeforeEach(func() {
						fakeVMProvider.DeleteSnapshotFn = func(_ context.Context, _ *vmopv1.VirtualMachineSnapshot, _ *vmopv1.VirtualMachine, _ bool, _ *bool) (bool, error) {
							return true, nil
						}
					})
					It("returns success", func() {
						_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("VC returns other error", func() {
					JustBeforeEach(func() {
						fakeVMProvider.DeleteSnapshotFn = func(_ context.Context, _ *vmopv1.VirtualMachineSnapshot, _ *vmopv1.VirtualMachine, _ bool, _ *bool) (bool, error) {
							return false, errors.New("fubar")
						}
					})
					It("returns error", func() {
						_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("fubar"))
					})
				})
			})

			When("Calling SyncVMSnapshotTreeStatus", func() {
				BeforeEach(func() {
					skipReconcile = true
				})
				When("it returns error", func() {
					JustBeforeEach(func() {
						fakeVMProvider.SyncVMSnapshotTreeStatusFn = func(_ context.Context, _ *vmopv1.VirtualMachine) error {
							return errors.New("fubar")
						}
					})
					It("returns error", func() {
						_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("fubar"))
					})
				})
			})

			When("VirtualMachine CR is not present", func() {
				BeforeEach(func() {
					// Remove vm from initObjects
					initObjects = nil
					initObjects = append(initObjects, vmSnapshot)
				})
				It("returns success", func() {
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("VirtualMachineSnapshot VMRef is nil", func() {
				BeforeEach(func() {
					vmSnapshot.Spec.VMRef = nil
					initObjects = nil
					initObjects = append(initObjects, vmSnapshot, vm)
				})
				It("returns error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("VirtualMachineSnapshot VMRef is nil"))
				})
			})
		})

		When("Nested Snapshot, one of them is marked for deletion", func() {
			var (
				vmSnapshotL1      *vmopv1.VirtualMachineSnapshot
				vmSnapshotL2      *vmopv1.VirtualMachineSnapshot
				vmSnapshotL3Node1 *vmopv1.VirtualMachineSnapshot
				vmSnapshotL3Node2 *vmopv1.VirtualMachineSnapshot
				parent            *vmopv1.VirtualMachineSnapshot
			)
			BeforeEach(func() {
				//        L1
				//         |
				//        L2
				//       /   \
				//   L3-n1    L3-n2
				vmSnapshotL1 = builder.DummyVirtualMachineSnapshot(namespace, "snap-l1", vm.Name)
				vmSnapshotL2 = builder.DummyVirtualMachineSnapshot(namespace, "snap-l2", vm.Name)
				vmSnapshotL3Node1 = builder.DummyVirtualMachineSnapshot(namespace, "snap-l3-node1", vm.Name)
				vmSnapshotL3Node2 = builder.DummyVirtualMachineSnapshot(namespace, "snap-l3-node2", vm.Name)

				addSnapshotToChildren(vmSnapshotL1, vmSnapshotL2)
				addSnapshotToChildren(vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)

				now = metav1.Now()

				vm.Status.UniqueID = dummyVMUUID
			})

			When("internal snapshot is deleted", func() {
				//        L1
				//         |
				//        L2  <--- current snapshot, deleted
				//       /   \
				//   L3-n1    L3-n2

				// after reconcile
				//        L1 <-- same root
				//       /   \
				//   L3-n1    L3-n2
				BeforeEach(func() {
					vmSnapshotL2.DeletionTimestamp = &now
					vm.Status.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)
					vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)}
					// assign before the reconcile
					parent = vmSnapshotL1
					initObjects = append(initObjects, vm, vmSnapshotL1, vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)
				})
				JustBeforeEach(func() {
					fakeVMProvider.SyncVMSnapshotTreeStatusFn = func(_ context.Context, vm *vmopv1.VirtualMachine) error {
						// update vm status
						vm.Status.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)
						vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)}

						// update vmSnapshotL1's children
						vmSnapshotL1Obj := &vmopv1.VirtualMachineSnapshot{}
						Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshotL1.Name, Namespace: vmSnapshotL1.Namespace}, vmSnapshotL1Obj)).To(Succeed())
						vmSnapshotL1Obj.Status.Children = []vmopv1common.LocalObjectRef{
							*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node1),
							*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node2),
						}
						Expect(ctx.Client.Status().Update(ctx, vmSnapshotL1Obj)).To(Succeed())
						return nil
					}
					_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vmSnapshotL2.Namespace,
							Name:      vmSnapshotL2.Name,
						}})
					// get newest vm object
					objKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
					Expect(ctx.Client.Get(ctx, objKey, vm)).To(Succeed())
					if parent != nil {
						parentSSObjKey := types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}
						Expect(ctx.Client.Get(ctx, parentSSObjKey, parent)).To(Succeed())
					}
				})
				It("returns success, vm current snapshot is updated to root", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Status.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)))
					Expect(vm.Status.RootSnapshots).To(HaveLen(1))
					Expect(vm.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)))
					// check vmSnapshotL1's children
					vmSnapshotL1Obj := &vmopv1.VirtualMachineSnapshot{}
					Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshotL1.Name, Namespace: vmSnapshotL1.Namespace}, vmSnapshotL1Obj)).To(Succeed())
					Expect(vmSnapshotL1Obj.Status.Children).To(HaveLen(2))
					Expect(vmSnapshotL1Obj.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node1)))
					Expect(vmSnapshotL1Obj.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node2)))
				})
			})
			When("root snapshot is deleted", func() {
				//        L1  <--- current snapshot, deleted
				//         |
				//        L2
				//       /   \
				//   L3-n1    L3-n2

				// after reconcile
				//        L2  <--- new root
				//       /   \
				//   L3-n1    L3-n2
				BeforeEach(func() {
					vmSnapshotL1.DeletionTimestamp = &now
					vm.Status.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)
					vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)}
					parent = nil
					initObjects = append(initObjects, vm, vmSnapshotL1, vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)
				})
				JustBeforeEach(func() {
					fakeVMProvider.SyncVMSnapshotTreeStatusFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						vm.Status.CurrentSnapshot = nil
						vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)}
						return nil
					}
					_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vmSnapshotL1.Namespace,
							Name:      vmSnapshotL1.Name,
						}})
					// get newest vm object
					objKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
					Expect(ctx.Client.Get(ctx, objKey, vm)).To(Succeed())
					if parent != nil {
						parentSSObjKey := types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}
						Expect(ctx.Client.Get(ctx, parentSSObjKey, parent)).To(Succeed())
					}
				})
				It("returns success, vm current snapshot is updated to nil", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Status.CurrentSnapshot).To(BeNil())
					Expect(vm.Status.RootSnapshots).To(HaveLen(1))
					Expect(vm.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)))
				})
			})
			When("leaf snapshot is deleted", func() {
				//        L1
				//         |
				//        L2
				//       /   \
				//   L3-n2    L3-n1 <--- current snapshot, deleted

				// after reconcile
				//        L1 <-- same root
				//         |
				//        L2
				//         |
				//       L3-n2
				BeforeEach(func() {
					vmSnapshotL3Node1.DeletionTimestamp = &now
					vm.Status.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node1)
					vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)}
					parent = vmSnapshotL2
					initObjects = append(initObjects, vm, vmSnapshotL1, vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)
				})
				JustBeforeEach(func() {
					fakeVMProvider.SyncVMSnapshotTreeStatusFn = func(_ context.Context, vm *vmopv1.VirtualMachine) error {
						vm.Status.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)
						vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)}

						// update vmSnapshotL2's children
						vmSnapshotL2Obj := &vmopv1.VirtualMachineSnapshot{}
						Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshotL2.Name, Namespace: vmSnapshotL2.Namespace}, vmSnapshotL2Obj)).To(Succeed())
						vmSnapshotL2Obj.Status.Children = []vmopv1common.LocalObjectRef{
							*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node2),
						}
						Expect(ctx.Client.Status().Update(ctx, vmSnapshotL2Obj)).To(Succeed())
						return nil
					}
					_, err = reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vmSnapshotL3Node1.Namespace,
							Name:      vmSnapshotL3Node1.Name,
						}})
					// get newest vm object
					objKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
					Expect(ctx.Client.Get(ctx, objKey, vm)).To(Succeed())
					if parent != nil {
						parentSSObjKey := types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}
						Expect(ctx.Client.Get(ctx, parentSSObjKey, parent)).To(Succeed())
					}
				})
				It("returns success, vm current snapshot is updated to parent", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Status.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)))
					Expect(vm.Status.RootSnapshots).To(HaveLen(1))
					Expect(vm.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)))

					// check vmSnapshotL2's children
					vmSnapshotL2Obj := &vmopv1.VirtualMachineSnapshot{}
					Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshotL2.Name, Namespace: vmSnapshotL2.Namespace}, vmSnapshotL2Obj)).To(Succeed())
					Expect(vmSnapshotL2Obj.Status.Children).To(HaveLen(1))
					Expect(vmSnapshotL2Obj.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node2)))
				})
			})
		})
	})
}

func vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshot *vmopv1.VirtualMachineSnapshot) *vmopv1common.LocalObjectRef {
	return &vmopv1common.LocalObjectRef{
		APIVersion: vmSnapshot.APIVersion,
		Kind:       vmSnapshot.Kind,
		Name:       vmSnapshot.Name,
	}
}

func addSnapshotToChildren(vmSnapshot *vmopv1.VirtualMachineSnapshot, children ...*vmopv1.VirtualMachineSnapshot) {
	for _, child := range children {
		vmSnapshot.Status.Children = append(vmSnapshot.Status.Children, *vmSnapshotCRToLocalObjectRefWithDefaultVersion(child))
	}
}
