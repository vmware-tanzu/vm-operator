// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesnapshot_test

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesnapshot"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcile,
	)

	Describe(
		"ReconcileDelete",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcileDelete,
	)
}

func intgTestsReconcile() {
	var (
		ctx        *builder.IntegrationTestContext
		vmSnapshot *vmopv1.VirtualMachineSnapshot
		vm         *vmopv1.VirtualMachine
	)

	const (
		uniqueVMID = "unique-vm-id"
	)

	getVirtualMachine := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.VirtualMachine {
		vm := &vmopv1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = builder.DummyBasicVirtualMachine("dummy-vm", ctx.Namespace)
		vmSnapshot = builder.DummyVirtualMachineSnapshot("snap-1", ctx.Namespace, vm.Name)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			vm.Status.UniqueID = uniqueVMID
			Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vmSnapshot)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, vm)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("vm resource successfully patched with current snapshot", func() {
			vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
			Eventually(func(g Gomega) {
				vmObj := getVirtualMachine(ctx, vmObjKey)
				g.Expect(vmObj).ToNot(BeNil())
				g.Expect(vmObj.Spec.CurrentSnapshot).To(Equal(&vmopv1common.LocalObjectRef{
					APIVersion: "vmoperator.vmware.com/v1alpha4",
					Kind:       "VirtualMachineSnapshot",
					Name:       vmSnapshot.Name,
				}))
			}).Should(Succeed(), "waiting current snapshot to be set on virtualmachine")
		})
	})
}

func intgTestsReconcileDelete() {
	var (
		ctx        context.Context
		vcSimCtx   *builder.IntegrationTestContextForVCSim
		provider   *providerfake.VMProvider
		initEnvFn  builder.InitVCSimEnvFn
		vmSnapshot *vmopv1.VirtualMachineSnapshot
		vm         *vmopv1.VirtualMachine
	)

	const (
		uniqueVMID = "unique-vm-id"
	)

	getVirtualMachine := func(ctx *builder.IntegrationTestContextForVCSim, objKey types.NamespacedName) *vmopv1.VirtualMachine {
		vm := &vmopv1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	JustBeforeEach(func() {
		ctx = logr.NewContext(
			context.Background(),
			textlogger.NewLogger(textlogger.NewConfig(
				textlogger.Verbosity(6),
				textlogger.Output(GinkgoWriter),
			)))

		ctx = pkgcfg.WithContext(ctx, pkgcfg.Default())
		provider = providerfake.NewVMProvider()
		vcSimCtx = builder.NewIntegrationTestContextForVCSim(
			ctx,
			builder.VCSimTestConfig{},
			virtualmachinesnapshot.AddToManager,
			func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
				ctx.VMProvider = provider
				return nil
			},
			initEnvFn)
		Expect(vcSimCtx).ToNot(BeNil())

		vcSimCtx.BeforeEach()
	})

	AfterEach(func() {
		vcSimCtx.AfterEach()
	})

	When("delete snapshot", func() {
		When("snapshot is not nested", func() {
			JustBeforeEach(func() {
				vm = builder.DummyBasicVirtualMachine("dummy-vm", vcSimCtx.NSInfo.Namespace)
				vmSnapshot = builder.DummyVirtualMachineSnapshot("snap-1", vcSimCtx.NSInfo.Namespace, vm.Name)
				Expect(vcSimCtx.Client.Create(ctx, vmSnapshot.DeepCopy())).To(Succeed())
				Expect(vcSimCtx.Client.Create(ctx, vm)).To(Succeed())
				vm.Status.UniqueID = uniqueVMID
				Expect(vcSimCtx.Client.Status().Update(ctx, vm)).To(Succeed())
				vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				Eventually(func(g Gomega) {
					vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
					g.Expect(vmObj).ToNot(BeNil())
					g.Expect(vmObj.Spec.CurrentSnapshot).ToNot(BeNil())
				}).Should(Succeed(), "waiting current snapshot to be set on virtualmachine")
				Expect(vcSimCtx.Client.Delete(ctx, vmSnapshot)).To(Succeed())
			})

			AfterEach(func() {
				err := vcSimCtx.Client.Delete(ctx, vmSnapshot)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
				err = vcSimCtx.Client.Delete(ctx, vm)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			})

			It("set current snapshot to nil", func() {
				vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				Expect(vcSimCtx.Client.Delete(ctx, vmSnapshot)).To(Succeed())
				Eventually(func(g Gomega) {
					vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
					g.Expect(vmObj).ToNot(BeNil())
					g.Expect(vmObj.Spec.CurrentSnapshot).To(BeNil())
					tmpVMSSnapshot := getVirtualMachineSnapshot(vcSimCtx, vmObjKey)
					g.Expect(tmpVMSSnapshot).To(BeNil())
				}).Should(Succeed(), "waiting current snapshot to be deleted")
			})

			When("Calling DeleteSnapshot to VC", func() {
				When("VC returns VirtualMachineNotFound error", func() {
					JustBeforeEach(func() {
						provider.Lock()
						provider.DeleteSnapshotFn = func(_ context.Context, _ *vmopv1.VirtualMachineSnapshot, _ *vmopv1.VirtualMachine, _ bool, _ *bool) (bool, error) {
							return true, nil
						}
						provider.Unlock()
					})
					It("snapshot is deleted", func() {
						vmSnapshotObjKey := types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}
						Eventually(func(g Gomega) {
							tmpVMSSnapshot := getVirtualMachineSnapshot(vcSimCtx, vmSnapshotObjKey)
							g.Expect(tmpVMSSnapshot).To(Not(BeNil()))
						}).Should(Succeed())
					})
				})
				When("there is error from VC when finding VM other than VirtualMachineNotFound", func() {
					JustBeforeEach(func() {
						provider.Lock()
						provider.DeleteSnapshotFn = func(_ context.Context, _ *vmopv1.VirtualMachineSnapshot, _ *vmopv1.VirtualMachine, _ bool, _ *bool) (bool, error) {
							return false, errors.New("fubar")
						}
						provider.Unlock()
					})
					It("snapshot is not deleted and VM is not updated", func() {
						vmSnapshotObjKey := types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}
						Consistently(func(g Gomega) {
							tmpVMSSnapshot := getVirtualMachineSnapshot(vcSimCtx, vmSnapshotObjKey)
							g.Expect(tmpVMSSnapshot).To(Not(BeNil()))
						}).Should(Succeed())
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Consistently(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot).To(Not(BeNil()))
							g.Expect(*vmObj.Spec.CurrentSnapshot).To(Equal(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshot)))
						}).Should(Succeed())
					})
				})
			})
			When("VirtualMachine CR is not present", func() {
				JustBeforeEach(func() {
					Expect(vcSimCtx.Client.Delete(ctx, vm)).To(Succeed())
					vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
					Eventually(func(g Gomega) {
						vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
						g.Expect(vmObj).To(BeNil())
					}).Should(Succeed())
				})
				It("snapshot is deleted", func() {
					vmSnapshotObjKey := types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}
					Eventually(func(g Gomega) {
						tmpVMSSnapshot := getVirtualMachineSnapshot(vcSimCtx, vmSnapshotObjKey)
						g.Expect(tmpVMSSnapshot).To(Not(BeNil()))
					}).Should(Succeed())
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
				vmSnapshotToBeDeleted *vmopv1.VirtualMachineSnapshot
				currentSnapshot       *vmopv1common.LocalObjectRef
				now                   metav1.Time
			)

			markVMSnapshotReady := func(ctx *builder.IntegrationTestContextForVCSim, vmSnapshot *vmopv1.VirtualMachineSnapshot) {
				objKey := types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}
				Expect(ctx.Client.Get(ctx, objKey, vmSnapshot)).To(Succeed())
				patch := ctrlclient.MergeFrom(vmSnapshot.DeepCopy())
				conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
				Expect(ctx.Client.Status().Patch(ctx, vmSnapshot, patch)).To(Succeed())
			}
			addSnapshotToChildren := func(ctx *builder.IntegrationTestContextForVCSim, vmSnapshot *vmopv1.VirtualMachineSnapshot, children ...*vmopv1.VirtualMachineSnapshot) {
				for _, child := range children {
					vmSnapshot.Status.Children = append(vmSnapshot.Status.Children, *vmSnapshotCRToLocalObjectRefWithDefault(child))
				}
				Expect(ctx.Client.Status().Update(ctx, vmSnapshot)).To(Succeed())
				Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, vmSnapshot)).To(Succeed())
			}

			BeforeEach(func() {
				//        L1
				//         |
				//        L2
				//       /   \
				//   L3-n1    L3-n2
				// Create the object here so that it can be customized in each BeforeEach
				vm = builder.DummyBasicVirtualMachine("dummy-vm", "placeholder")
				vmSnapshotL1 = builder.DummyVirtualMachineSnapshot("snap-l1", "placeholder", vm.Name)
				vmSnapshotL2 = builder.DummyVirtualMachineSnapshot("snap-l2", "placeholder", vm.Name)
				vmSnapshotL3Node1 = builder.DummyVirtualMachineSnapshot("snap-l3-node1", "placeholder", vm.Name)
				vmSnapshotL3Node2 = builder.DummyVirtualMachineSnapshot("snap-l3-node2", "placeholder", vm.Name)
				now = metav1.Now()
			})

			JustBeforeEach(func() {
				// Update the namespace to the one in current context
				vm.Namespace, vmSnapshotL1.Namespace, vmSnapshotL2.Namespace,
					vmSnapshotL3Node1.Namespace, vmSnapshotL3Node2.Namespace =
					vcSimCtx.NSInfo.Namespace, vcSimCtx.NSInfo.Namespace,
					vcSimCtx.NSInfo.Namespace, vcSimCtx.NSInfo.Namespace, vcSimCtx.NSInfo.Namespace

				Expect(vcSimCtx.Client.Create(ctx, vm)).To(Succeed())
				// Update the current snapshot after creation. Otherwise will run into "failed to get informer from cache"
				vm.Status.UniqueID = uniqueVMID
				// Update the root snapshots
				vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL1)}
				Expect(vcSimCtx.Client.Status().Update(ctx, vm)).To(Succeed())

				// // Create the object here so that it can be customized in each BeforeEach
				Expect(vcSimCtx.Client.Create(ctx, vmSnapshotL1)).To(Succeed())
				Expect(vcSimCtx.Client.Create(ctx, vmSnapshotL2)).To(Succeed())
				Expect(vcSimCtx.Client.Create(ctx, vmSnapshotL3Node1.DeepCopy())).To(Succeed())
				Expect(vcSimCtx.Client.Create(ctx, vmSnapshotL3Node2.DeepCopy())).To(Succeed())
				// Mark the snapshot as ready so that they won't update CurrentSnapshot
				markVMSnapshotReady(vcSimCtx, vmSnapshotL1)
				markVMSnapshotReady(vcSimCtx, vmSnapshotL2)
				markVMSnapshotReady(vcSimCtx, vmSnapshotL3Node1)
				markVMSnapshotReady(vcSimCtx, vmSnapshotL3Node2)
				// Fetch newest vmSnapshotL1 and vmSnapshotL2 to update children
				addSnapshotToChildren(vcSimCtx, vmSnapshotL1, vmSnapshotL2)
				addSnapshotToChildren(vcSimCtx, vmSnapshotL2, vmSnapshotL3Node1, vmSnapshotL3Node2)

				By("update vm current snapshot to the snapshot to be deleted")
				vm.Spec.CurrentSnapshot = currentSnapshot
				Expect(vcSimCtx.Client.Update(ctx, vm)).To(Succeed())
				vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				Eventually(func(g Gomega) {
					vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
					g.Expect(vmObj).ToNot(BeNil())
					g.Expect(vmObj.Spec.CurrentSnapshot).ToNot(BeNil())
					g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(currentSnapshot.Name))
				}).Should(Succeed(), "waiting current snapshot to be set on virtualmachine")

				By("delete the snapshot")
				Expect(vcSimCtx.Client.Delete(ctx, vmSnapshotToBeDeleted)).To(Succeed())
			})

			AfterEach(func() {
				err := vcSimCtx.Client.Delete(ctx, vmSnapshotL1)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
				err = vcSimCtx.Client.Delete(ctx, vmSnapshotL2)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
				err = vcSimCtx.Client.Delete(ctx, vmSnapshotL3Node1)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
				err = vcSimCtx.Client.Delete(ctx, vmSnapshotL3Node2)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
				err = vcSimCtx.Client.Delete(ctx, vm)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
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
					currentSnapshot = vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL2)
					// assign before the reconcile
					vmSnapshotToBeDeleted = vmSnapshotL2
					parent = vmSnapshotL1
				})
				When("it's the current snapshot", func() {
					It("returns success, vm current snapshot is updated to root", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						By("check vm current snapshot is updated to root")
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL1.Name))
						}).Should(Succeed(), "waiting for current snapshot to be updated to root")

						By("check parent snapshot's children should be updated")
						parentSSObjKey := types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}
						Eventually(func(g Gomega) {
							tmpParent := &vmopv1.VirtualMachineSnapshot{}
							g.Expect(vcSimCtx.Client.Get(ctx, parentSSObjKey, tmpParent)).To(Succeed())
							g.Expect(tmpParent).To(Not(BeNil()))
							g.Expect(tmpParent.Status.Children).To(HaveLen(2))
							g.Expect(tmpParent.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL3Node1)))
							g.Expect(tmpParent.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL3Node2)))
							g.Expect(tmpParent.Status.Children).ToNot(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL2)))
						}).Should(Succeed(), "waiting for parent snapshot's children to be updated")

						By("check vm root snapshots should stay the same")
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj.Status.RootSnapshots).To(HaveLen(1))
							g.Expect(vmObj.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL1)))
						}).Should(Succeed(), "waiting for vm root snapshots to be updated")
					})
				})
				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL1)
					})
					It("returns success, vm current snapshot is not changed", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL1.Name))
						}).Should(Succeed())
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
					currentSnapshot = vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL1)
					vmSnapshotToBeDeleted = vmSnapshotL1
					parent = nil
				})
				When("it's the current snapshot", func() {
					It("returns success, vm current snapshot is updated to nil", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot).To(BeNil())
						}).Should(Succeed())
						By("vm root snapshots should be updated")
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj.Status.RootSnapshots).To(HaveLen(1))
							g.Expect(vmObj.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL2)))
						}).Should(Succeed(), "waiting for vm root snapshots to be updated")
					})
				})
				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						currentSnapshot = vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL2)
					})
					It("returns success, vm current snapshot is not changed", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL2.Name))
						}).Should(Succeed())
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
				//        L1
				//         |
				//        L2
				//         |
				//       L3-n2
				BeforeEach(func() {
					vmSnapshotL3Node1.DeletionTimestamp = &now
					currentSnapshot = vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL3Node1)
					vmSnapshotToBeDeleted = vmSnapshotL3Node1
					parent = vmSnapshotL2
				})
				When("it's the current snapshot", func() {
					It("returns success, vm current snapshot is updated to parent", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL2.Name))
						}).Should(Succeed())
						By("check parent snapshot's children should be updated")
						parentSSObjKey := types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}
						Eventually(func(g Gomega) {
							tmpParent := &vmopv1.VirtualMachineSnapshot{}
							g.Expect(vcSimCtx.Client.Get(ctx, parentSSObjKey, tmpParent)).To(Succeed())
							g.Expect(tmpParent).To(Not(BeNil()))
							g.Expect(tmpParent.Status.Children).To(HaveLen(1))
							g.Expect(tmpParent.Status.Children).ToNot(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL3Node1)))
							g.Expect(tmpParent.Status.Children).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL3Node2)))
						}).Should(Succeed())
						By("check vm root snapshots should be updated")
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj.Status.RootSnapshots).To(HaveLen(1))
							g.Expect(vmObj.Status.RootSnapshots).To(ContainElement(*vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL1)))
						}).Should(Succeed(), "waiting for vm root snapshots to be updated")
					})
				})
				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshotL2)
					})
					It("returns success, vm current snapshot is not changed", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL2.Name))
						}).Should(Succeed())
					})
				})
			})
		})
	})
}

func getVirtualMachineSnapshot(ctx *builder.IntegrationTestContextForVCSim, objKey types.NamespacedName) *vmopv1.VirtualMachineSnapshot {
	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := ctx.Client.Get(ctx, objKey, vmSnapshot); err != nil {
		return nil
	}
	return vmSnapshot
}

// This is a workaround when controller-runtime doesn't set the version and kind if the
// object is created by ctrlClient.Get().
func vmSnapshotCRToLocalObjectRefWithDefault(vmSnapshot *vmopv1.VirtualMachineSnapshot) *vmopv1common.LocalObjectRef {
	kind := vmSnapshot.Kind
	if kind == "" {
		kind = "VirtualMachineSnapshot"
	}
	apiVersion := vmSnapshot.APIVersion
	if apiVersion == "" {
		apiVersion = "vmoperator.vmware.com/v1alpha4"
	}
	return &vmopv1common.LocalObjectRef{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       vmSnapshot.Name,
	}
}
