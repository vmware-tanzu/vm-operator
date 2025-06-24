// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesnapshot_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesnapshot"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
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

		reconciler     *virtualmachinesnapshot.Reconciler
		vmSnapshot     *vmopv1.VirtualMachineSnapshot
		vm             *vmopv1.VirtualMachine
		fakeVMProvider *providerfake.VMProvider
	)

	const (
		dummyVMUUID = "unique-vm-id"
		namespace   = "test-namespace"
	)

	BeforeEach(func() {
		initObjects = nil
		vm = builder.DummyBasicVirtualMachine("dummy-vm", namespace)

		vmSnapshot = builder.DummyVirtualMachineSnapshot("snap-1", namespace, vm.Name)
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
			err = nil
			initObjects = append(initObjects, vmSnapshot)
		})

		JustBeforeEach(func() {
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: vmSnapshot.Namespace,
					Name:      vmSnapshot.Name,
				}})
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

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				objKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				vmObj := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, objKey, vmObj)).To(Succeed())

				Expect(vmObj.Spec.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshot)))
			})
		})

		When("vm ready with different current snapshot", func() {
			BeforeEach(func() {
				vm.Spec.CurrentSnapshot = &vmopv1common.LocalObjectRef{
					APIVersion: vmSnapshot.APIVersion,
					Kind:       vmSnapshot.Kind,
					Name:       "dummy-diff-snapshot",
				}
				vm.Status.UniqueID = dummyVMUUID
				initObjects = append(initObjects, vm)
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				objKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				vmObj := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, objKey, vmObj)).To(Succeed())

				Expect(vmObj.Spec.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshot)))
			})
		})

		When("vm ready with matching current snapshot name", func() {
			BeforeEach(func() {
				vm.Status.UniqueID = dummyVMUUID
				vm.Spec.CurrentSnapshot = &vmopv1common.LocalObjectRef{
					APIVersion: vmSnapshot.APIVersion,
					Kind:       vmSnapshot.Kind,
					Name:       vmSnapshot.Name,
				}
				initObjects = append(initObjects, vm)
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("object does not have finalizer set", func() {
			BeforeEach(func() {
				vmSnapshot.Finalizers = nil
			})

			It("will set finalizer", func() {
				Expect(err).NotTo(HaveOccurred())
				vmSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
				Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, vmSnapshotObj)).To(Succeed())
				Expect(vmSnapshotObj.GetFinalizers()).To(ContainElement(virtualmachinesnapshot.Finalizer))
			})
		})
	})

	Context("ReconcileDelete", func() {
		var (
			err                     error
			now                     metav1.Time
			vmSnapshotNamespacedKey types.NamespacedName
		)

		BeforeEach(func() {
			vmSnapshotNamespacedKey = types.NamespacedName{
				Namespace: vmSnapshot.Namespace,
				Name:      vmSnapshot.Name,
			}
			err = nil
		})

		When("A Snapshot is marked for deletion", func() {
			BeforeEach(func() {
				now = metav1.Now()
				vmSnapshot.DeletionTimestamp = &now
				initObjects = append(initObjects, vmSnapshot, vm)
			})

			It("returns success", func() {
				_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
				Expect(err).ToNot(HaveOccurred())
			})

			When("Calling DeleteSnapshot to VC", func() {
				When("VC returns VirtualMachineNotFound error", func() {
					JustBeforeEach(func() {
						fakeVMProvider.DeleteSnapshotFn = func(_ context.Context, _ *vmopv1.VirtualMachineSnapshot, _ *vmopv1.VirtualMachine, _ bool, _ *bool) (bool, error) {
							return true, nil
						}
					})
					It("returns success", func() {
						_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
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
						_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("fubar"))
					})
				})
			})

			When("VirtualMachine CR is not present", func() {
				BeforeEach(func() {
					initObjects = initObjects[:len(initObjects)-1]
				})
				It("returns success", func() {
					_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: vmSnapshotNamespacedKey})
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		When("Nested Snapshot, one of them is marked for deletion", func() {
			var (
				vmSnapshotL1          *vmopv1.VirtualMachineSnapshot
				vmSnapshotL2          *vmopv1.VirtualMachineSnapshot
				vmSnapshotL3Node1     *vmopv1.VirtualMachineSnapshot
				vmSnapshotL3Node2     *vmopv1.VirtualMachineSnapshot
				parent                *vmopv1.VirtualMachineSnapshot
				vmSnapshotToReconcile *vmopv1.VirtualMachineSnapshot
			)
			BeforeEach(func() {
				//        L1
				//         |
				//        L2
				//       /   \
				//   L3-n1    L3-n2
				vmSnapshotL1 = builder.DummyVirtualMachineSnapshot("snap-l1", namespace, vm.Name)
				vmSnapshotL2 = builder.DummyVirtualMachineSnapshot("snap-l2", namespace, vm.Name)
				vmSnapshotL3Node1 = builder.DummyVirtualMachineSnapshot("snap-l3-node1", namespace, vm.Name)
				vmSnapshotL3Node2 = builder.DummyVirtualMachineSnapshot("snap-l3-node2", namespace, vm.Name)

				addSnapshotToChildren(vmSnapshotL1, vmSnapshotL2)
				addSnapshotToChildren(vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)

				now = metav1.Now()

				vm.Status.UniqueID = dummyVMUUID
			})

			JustBeforeEach(func() {
				// constructing the new key since vmSnapshot is updated in each test
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: vmSnapshotToReconcile.Namespace,
						Name:      vmSnapshotToReconcile.Name,
					}})
				// get newest vm object
				objKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				Expect(ctx.Client.Get(ctx, objKey, vm)).To(Succeed())
				if parent != nil {
					parentSSObjKey := types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}
					Expect(ctx.Client.Get(ctx, parentSSObjKey, parent)).To(Succeed())
				}
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
					vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)
					vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)}
					// assign before the reconcile
					vmSnapshotToReconcile = vmSnapshotL2
					parent = vmSnapshotL1
					initObjects = append(initObjects, vm, vmSnapshotL1, vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)
				})
				When("it's the current snapshot", func() {
					It("returns success, vm current snapshot is updated to root", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(vm.Spec.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)))
						Expect(vm.Status.RootSnapshots).To(HaveLen(1))
						Expect(vm.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)))
						By("check parent snapshot's children should be updated")
						Expect(parent).To(Not(BeNil()))
						Expect(parent.Status.Children).To(HaveLen(2))
						Expect(parent.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node1)))
						Expect(parent.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node2)))
						Expect(parent.Status.Children).ToNot(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)))
					})
				})
				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)
					})
					It("returns success, vm current snapshot is not changed", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(vm.Spec.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)))
					})
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
					vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)
					vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)}
					vmSnapshotToReconcile = vmSnapshotL1
					parent = nil
					initObjects = append(initObjects, vm, vmSnapshotL1, vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)
				})
				When("it's the current snapshot", func() {
					It("returns success, vm current snapshot is updated to nil", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(vm.Spec.CurrentSnapshot).To(BeNil())
						Expect(vm.Status.RootSnapshots).To(HaveLen(1))
						Expect(vm.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)))
					})
				})
				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)
					})
					It("returns success, vm current snapshot is not changed", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(vm.Spec.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)))
					})
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
					vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node1)
					vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)}
					vmSnapshotToReconcile = vmSnapshotL3Node1
					parent = vmSnapshotL2
					initObjects = append(initObjects, vm, vmSnapshotL1, vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)
				})
				When("it's the current snapshot", func() {
					It("returns success, vm current snapshot is updated to parent", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(vm.Spec.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)))
						By("check parent snapshot's children should be updated")
						Expect(parent).To(Not(BeNil()))
						Expect(parent.Status.Children).To(HaveLen(1))
						Expect(parent.Status.Children).ToNot(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node1)))
						Expect(parent.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL3Node2)))
						Expect(vm.Status.RootSnapshots).To(HaveLen(1))
						Expect(vm.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL1)))
					})
				})
				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)
					})
					It("returns success, vm current snapshot is not changed", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(vm.Spec.CurrentSnapshot).To(Equal(vmSnapshotCRToLocalObjectRefWithDefaultVersion(vmSnapshotL2)))
					})
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
