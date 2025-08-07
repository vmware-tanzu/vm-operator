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
	vimtypes "github.com/vmware/govmomi/vim25/types"
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
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
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
}

func intgTestsReconcile() {
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

	BeforeEach(func() {
		provider = providerfake.NewVMProvider()
	})

	JustBeforeEach(func() {
		ctx = logr.NewContext(
			context.Background(),
			textlogger.NewLogger(textlogger.NewConfig(
				textlogger.Verbosity(6),
				textlogger.Output(GinkgoWriter),
			)))

		ctx = pkgcfg.WithContext(ctx, pkgcfg.Default())
		ctx = cource.WithContext(ctx)
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

	Context("ReconcileNormal", func() {
		When("snapshot is ready", func() {
			BeforeEach(func() {
				initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
					By("create vm in k8s")
					vm = builder.DummyBasicVirtualMachine("dummy-vm", vcSimCtx.NSInfo.Namespace)
					Expect(vcSimCtx.Client.Create(ctx, vm)).To(Succeed())
					vm.Status.UniqueID = uniqueVMID
					Expect(vcSimCtx.Client.Status().Update(ctx, vm)).To(Succeed())
					By("create snapshot in k8s")
					vmSnapshot = builder.DummyVirtualMachineSnapshot(vcSimCtx.NSInfo.Namespace, "snap-1", vm.Name)
					Expect(vcSimCtx.Client.Create(ctx, vmSnapshot.DeepCopy())).To(Succeed())
				}
			})
			JustBeforeEach(func() {
				By("mark snapshot as ready")
				vmSnapshotObjKey := types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}
				Expect(vcSimCtx.Client.Get(ctx, vmSnapshotObjKey, vmSnapshot)).To(Succeed())
				patch := ctrlclient.MergeFrom(vmSnapshot.DeepCopy())
				conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
				Expect(vcSimCtx.Client.Status().Patch(ctx, vmSnapshot, patch)).To(Succeed(), "mark snapshot as ready")
			})

			It("returns success, and set the annotation to requested", func() {
				Eventually(func(g Gomega) {
					vmSnapshotObj := &vmopv1.VirtualMachineSnapshot{}
					Expect(vcSimCtx.Client.Get(ctx, types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, vmSnapshotObj)).To(Succeed())
					g.Expect(conditions.IsTrue(vmSnapshotObj, vmopv1.VirtualMachineSnapshotReadyCondition)).To(BeTrue())
					g.Expect(vmSnapshotObj.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey]).To(Equal(constants.CSIVSphereVolumeSyncAnnotationValueRequest))
				}).Should(Succeed())
			})
		})
	})

	Context("ReconcileDelete", func() {
		When("snapshot is not nested", func() {
			BeforeEach(func() {
				initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
					By("create vm and snapshot in k8s")
					vm = builder.DummyBasicVirtualMachine("dummy-vm", vcSimCtx.NSInfo.Namespace)
					vmSnapshot = builder.DummyVirtualMachineSnapshot(vcSimCtx.NSInfo.Namespace, "snap-1", vm.Name)
					Expect(vcSimCtx.Client.Create(ctx, vmSnapshot.DeepCopy())).To(Succeed())
					Expect(vcSimCtx.Client.Create(ctx, vm)).To(Succeed())
					vm.Status.UniqueID = uniqueVMID
					Expect(vcSimCtx.Client.Status().Update(ctx, vm)).To(Succeed())
					vm.Spec.CurrentSnapshot = newLocalObjectRefWithSnapshotName(vmSnapshot.Name)
					Expect(vcSimCtx.Client.Update(ctx, vm)).To(Succeed())
				}
			})

			JustBeforeEach(func() {
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
					BeforeEach(func() {
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
					BeforeEach(func() {
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
							g.Expect(*vmObj.Spec.CurrentSnapshot).To(Equal(*newLocalObjectRefWithSnapshotName(vmSnapshot.Name)))
						}).Should(Succeed())
					})
				})
			})

			When("Calling GetParentSnapshot to VC", func() {
				When("VC error", func() {
					BeforeEach(func() {
						provider.Lock()
						provider.GetParentSnapshotFn = func(_ context.Context, _ string, _ *vmopv1.VirtualMachine) (*vimtypes.VirtualMachineSnapshotTree, error) {
							return nil, errors.New("fubar")
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
							g.Expect(*vmObj.Spec.CurrentSnapshot).To(Equal(*newLocalObjectRefWithSnapshotName(vmSnapshot.Name)))
						}).Should(Succeed())
					})
				})
			})

			When("VirtualMachine CR is not present", func() {
				BeforeEach(func() {
					initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
						By("only create snapshot in k8s")
						vmSnapshot = builder.DummyVirtualMachineSnapshot(vcSimCtx.NSInfo.Namespace, "snap-1", vm.Name)
						Expect(vcSimCtx.Client.Create(ctx, vmSnapshot.DeepCopy())).To(Succeed())
					}
				})
				It("snapshot is deleted", func() {
					By("VM is not presented")
					vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
					Consistently(func(g Gomega) {
						vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
						g.Expect(vmObj).To(BeNil())
					}).Should(Succeed())

					By("Snapshot is deleted")
					vmSnapshotObjKey := types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}
					Eventually(func(g Gomega) {
						tmpVMSSnapshot := getVirtualMachineSnapshot(vcSimCtx, vmSnapshotObjKey)
						g.Expect(tmpVMSSnapshot).To(BeNil())
					}).Should(Succeed())
				})
			})

			When("Snapshot's VMRef is nil", func() {
				BeforeEach(func() {
					initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
						By("create vm and snapshot in k8s")
						vm = builder.DummyBasicVirtualMachine("dummy-vm", vcSimCtx.NSInfo.Namespace)
						vmSnapshot = builder.DummyVirtualMachineSnapshot(vcSimCtx.NSInfo.Namespace, "snap-1", vm.Name)
						By("set snapshot's vmref to nil")
						vmSnapshot.Spec.VMRef = nil
						Expect(vcSimCtx.Client.Create(ctx, vmSnapshot.DeepCopy())).To(Succeed())
						Expect(vcSimCtx.Client.Create(ctx, vm)).To(Succeed())
						vm.Status.UniqueID = uniqueVMID
						Expect(vcSimCtx.Client.Status().Update(ctx, vm)).To(Succeed())
						vm.Spec.CurrentSnapshot = newLocalObjectRefWithSnapshotName(vmSnapshot.Name)
						Expect(vcSimCtx.Client.Update(ctx, vm)).To(Succeed())
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}

						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
						}).Should(Succeed(), "waiting current snapshot to be set on virtualmachine")
					}
				})
				It("snapshot is not deleted", func() {
					vmSnapshotObjKey := types.NamespacedName{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}
					Consistently(func(g Gomega) {
						tmpVMSSnapshot := getVirtualMachineSnapshot(vcSimCtx, vmSnapshotObjKey)
						g.Expect(tmpVMSSnapshot).To(Not(BeNil()))
					}).Should(Succeed())
				})
			})
		})

		When("Nested Snapshot, one of them is marked for deletion", func() {
			var (
				vmSnapshotL1        *vmopv1.VirtualMachineSnapshot
				vmSnapshotL2        *vmopv1.VirtualMachineSnapshot
				vmSnapshotL3Node1   *vmopv1.VirtualMachineSnapshot
				vmSnapshotL3Node2   *vmopv1.VirtualMachineSnapshot
				currentSnapshotName string
			)

			const (
				vmSnapshotL1Name      = "snap-l1"
				vmSnapshotL2Name      = "snap-l2"
				vmSnapshotL3Node1Name = "snap-l3-node1"
				vmSnapshotL3Node2Name = "snap-l3-node2"
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
					vmSnapshot.Status.Children = append(vmSnapshot.Status.Children, *newLocalObjectRefWithSnapshotName(child.Name))
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
				initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
					vm = builder.DummyBasicVirtualMachine("dummy-vm", ctx.NSInfo.Namespace)
					vmSnapshotL1 = builder.DummyVirtualMachineSnapshot(ctx.NSInfo.Namespace, vmSnapshotL1Name, vm.Name)
					vmSnapshotL2 = builder.DummyVirtualMachineSnapshot(ctx.NSInfo.Namespace, vmSnapshotL2Name, vm.Name)
					vmSnapshotL3Node1 = builder.DummyVirtualMachineSnapshot(ctx.NSInfo.Namespace, vmSnapshotL3Node1Name, vm.Name)
					vmSnapshotL3Node2 = builder.DummyVirtualMachineSnapshot(ctx.NSInfo.Namespace, vmSnapshotL3Node2Name, vm.Name)

					Expect(vcSimCtx.Client.Create(ctx, vm)).To(Succeed())
					// Update the current snapshot after creation. Otherwise will run into "failed to get informer from cache"
					vm.Status.UniqueID = uniqueVMID
					// Update the root snapshots
					vm.Status.RootSnapshots = []vmopv1common.LocalObjectRef{*newLocalObjectRefWithSnapshotName(vmSnapshotL1.Name)}
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
				}
			})

			JustBeforeEach(func() {
				By("update vm current snapshot")
				vm.Spec.CurrentSnapshot = newLocalObjectRefWithSnapshotName(currentSnapshotName)
				Expect(vcSimCtx.Client.Update(ctx, vm)).To(Succeed())
				vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				Eventually(func(g Gomega) {
					vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
					g.Expect(vmObj).ToNot(BeNil())
					g.Expect(vmObj.Spec.CurrentSnapshot).ToNot(BeNil())
					g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(currentSnapshotName))
				}).Should(Succeed(), "waiting current snapshot to be set on virtualmachine")
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
					provider.GetParentSnapshotFn = func(_ context.Context, _ string, _ *vmopv1.VirtualMachine) (*vimtypes.VirtualMachineSnapshotTree, error) {
						return &vimtypes.VirtualMachineSnapshotTree{
							Name: vmSnapshotL1.Name,
						}, nil
					}
				})

				JustBeforeEach(func() {
					By("delete the snapshot")
					Expect(vcSimCtx.Client.Delete(ctx, vmSnapshotL2)).To(Succeed())
				})

				When("it's the current snapshot", func() {
					BeforeEach(func() {
						currentSnapshotName = vmSnapshotL2Name
					})

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
						parent := vmSnapshotL1
						parentSSObjKey := types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}
						Eventually(func(g Gomega) {
							tmpParent := &vmopv1.VirtualMachineSnapshot{}
							g.Expect(vcSimCtx.Client.Get(ctx, parentSSObjKey, tmpParent)).To(Succeed())
							g.Expect(tmpParent).To(Not(BeNil()))
							g.Expect(tmpParent.Status.Children).To(HaveLen(2))
							g.Expect(tmpParent.Status.Children).To(ContainElement(*newLocalObjectRefWithSnapshotName(vmSnapshotL3Node1Name)))
							g.Expect(tmpParent.Status.Children).To(ContainElement(*newLocalObjectRefWithSnapshotName(vmSnapshotL3Node2Name)))
							g.Expect(tmpParent.Status.Children).ToNot(ContainElement(*newLocalObjectRefWithSnapshotName(vmSnapshotL2Name)))
						}).Should(Succeed(), "waiting for parent snapshot's children to be updated")

						By("check vm root snapshots should stay the same")
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj.Status.RootSnapshots).To(HaveLen(1))
							g.Expect(vmObj.Status.RootSnapshots).To(ContainElement(*newLocalObjectRefWithSnapshotName(vmSnapshotL1Name)))
						}).Should(Succeed(), "waiting for vm root snapshots to be updated")
					})
				})

				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						currentSnapshotName = vmSnapshotL1Name
					})

					It("returns success, vm current snapshot is not changed", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL1Name))
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

				JustBeforeEach(func() {
					By("delete the snapshot")
					Expect(vcSimCtx.Client.Delete(ctx, vmSnapshotL1)).To(Succeed())
				})

				When("it's the current snapshot", func() {
					BeforeEach(func() {
						currentSnapshotName = vmSnapshotL1Name
					})
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
							g.Expect(vmObj.Status.RootSnapshots).To(ContainElement(*newLocalObjectRefWithSnapshotName(vmSnapshotL2Name)))
						}).Should(Succeed(), "waiting for vm root snapshots to be updated")
					})
				})

				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						currentSnapshotName = vmSnapshotL2Name
					})
					It("returns success, vm current snapshot is not changed", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL2Name))
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
					provider.GetParentSnapshotFn = func(_ context.Context, _ string, _ *vmopv1.VirtualMachine) (*vimtypes.VirtualMachineSnapshotTree, error) {
						return &vimtypes.VirtualMachineSnapshotTree{
							Name: vmSnapshotL2.Name,
						}, nil
					}
				})

				JustBeforeEach(func() {
					By("delete the snapshot")
					Expect(vcSimCtx.Client.Delete(ctx, vmSnapshotL3Node1)).To(Succeed())
				})

				When("it's the current snapshot", func() {
					BeforeEach(func() {
						currentSnapshotName = vmSnapshotL3Node1Name
					})
					It("returns success, vm current snapshot is updated to parent", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL2Name))
						}).Should(Succeed())
						By("check parent snapshot's children should be updated")
						parent := vmSnapshotL2
						parentSSObjKey := types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}
						Eventually(func(g Gomega) {
							tmpParent := &vmopv1.VirtualMachineSnapshot{}
							g.Expect(vcSimCtx.Client.Get(ctx, parentSSObjKey, tmpParent)).To(Succeed())
							g.Expect(tmpParent).To(Not(BeNil()))
							g.Expect(tmpParent.Status.Children).To(HaveLen(1))
							g.Expect(tmpParent.Status.Children).ToNot(ContainElement(*newLocalObjectRefWithSnapshotName(vmSnapshotL3Node1Name)))
							g.Expect(tmpParent.Status.Children).To(ContainElement(*newLocalObjectRefWithSnapshotName(vmSnapshotL3Node2Name)))
						}).Should(Succeed())
						By("check vm root snapshots should be updated")
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj.Status.RootSnapshots).To(HaveLen(1))
							g.Expect(vmObj.Status.RootSnapshots).To(ContainElement(*newLocalObjectRefWithSnapshotName(vmSnapshotL1.Name)))
						}).Should(Succeed(), "waiting for vm root snapshots to be updated")
					})
				})

				When("it's not the current snapshot", func() {
					BeforeEach(func() {
						currentSnapshotName = vmSnapshotL2Name
					})
					It("returns success, vm current snapshot is not changed", func() {
						vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
						Eventually(func(g Gomega) {
							vmObj := getVirtualMachine(vcSimCtx, vmObjKey)
							g.Expect(vmObj).ToNot(BeNil())
							g.Expect(vmObj.Spec.CurrentSnapshot.Name).To(Equal(vmSnapshotL2Name))
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
func newLocalObjectRefWithSnapshotName(name string) *vmopv1common.LocalObjectRef {
	return &vmopv1common.LocalObjectRef{
		APIVersion: "vmoperator.vmware.com/v1alpha4",
		Kind:       "VirtualMachineSnapshot",
		Name:       name,
	}
}
