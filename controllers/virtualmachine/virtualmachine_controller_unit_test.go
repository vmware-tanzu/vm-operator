// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	proberfake "github.com/vmware-tanzu/vm-operator/pkg/prober/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}

const finalizer = "virtualmachine.vmoperator.vmware.com"

func unitTestsReconcile() {
	const (
		providerError = "provider error"
	)

	var (
		initObjects []runtime.Object
		ctx         *builder.UnitTestContextForController

		reconciler       *virtualmachine.VirtualMachineReconciler
		fakeVmProvider   *providerfake.FakeVmProvider
		vmCtx            *vmopContext.VirtualMachineContext
		vm               *vmopv1alpha1.VirtualMachine
		vmClass          *vmopv1alpha1.VirtualMachineClass
		vmImage          *vmopv1alpha1.VirtualMachineImage
		vmMetaData       *corev1.ConfigMap
		vmResourcePolicy *vmopv1alpha1.VirtualMachineSetResourcePolicy
		storageClass     *storagev1.StorageClass
		resourceQuota    *corev1.ResourceQuota

		fakeProbeManager *proberfake.FakeProberManager
	)

	BeforeEach(func() {
		vmClass = &vmopv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vmclass",
			},
		}

		vmImage = &vmopv1alpha1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-image",
			},
		}

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dummy-vm",
				Namespace:  "dummy-ns",
				Finalizers: []string{finalizer},
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ClassName: vmClass.Name,
				ImageName: vmImage.Name,
			},
		}

		vmMetaData = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm-metadata",
				Namespace: vm.Namespace,
			},
			Data: map[string]string{
				"foo": "bar",
			},
		}

		vmResourcePolicy = &vmopv1alpha1.VirtualMachineSetResourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm-rp",
				Namespace: vm.Namespace,
			},
			Spec: vmopv1alpha1.VirtualMachineSetResourcePolicySpec{
				ResourcePool: vmopv1alpha1.ResourcePoolSpec{Name: "fooRP"},
				Folder:       vmopv1alpha1.FolderSpec{Name: "fooFolder"},
			},
		}

		storageClass = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-storage-class",
			},
			Provisioner: "foo",
			Parameters: map[string]string{
				"storagePolicyID": "id42",
			},
		}

		rlName := storageClass.Name + ".storageclass.storage.k8s.io/persistentvolumeclaims"

		resourceQuota = &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-resource-quota",
				Namespace: vm.Namespace,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceName(rlName): resource.MustParse("1"),
				},
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		// Explicitly set the max reconciler threads, otherwise it defaults to 0 and the reconciler thinks
		// no VM creations are allowed.
		ctx.MaxConcurrentReconciles = 1
		fakeProbeManagerIf := proberfake.NewFakeProberManager()
		reconciler = virtualmachine.NewReconciler(
			ctx.Client,
			ctx.MaxConcurrentReconciles,
			ctx.Logger,
			ctx.Recorder,
			ctx.VmProvider,
			fakeProbeManagerIf,
		)
		fakeVmProvider = ctx.VmProvider.(*providerfake.FakeVmProvider)
		fakeProbeManager = fakeProbeManagerIf.(*proberfake.FakeProberManager)

		vmCtx = &vmopContext.VirtualMachineContext{
			Context: ctx,
			Logger:  ctx.Logger.WithName(vm.Name),
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmCtx = nil
		reconciler = nil
		fakeVmProvider = nil
	})

	Context("getCLUUID", func() {

		cl := &vmopv1alpha1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cl",
			},
			Spec: vmopv1alpha1.ContentLibraryProviderSpec{
				UUID: "dummy-cl-uuid",
			},
		}

		When("the VirtualMachine Spec's VirtualMachineImage does not exist", func() {
			It("returns an error", func() {
				clUUID, err := reconciler.GetCLUUID(vmCtx)
				Expect(clUUID).To(BeEmpty())
				Expect(err).To(HaveOccurred())
				Expect(apiErrors.IsNotFound(err)).To(BeTrue())
			})
		})

		When("VirtualMachineImage does not have an OwnerReference", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vmImage, cl)
			})

			It("returns an empty content library UUID", func() {
				clUUID, err := reconciler.GetCLUUID(vmCtx)
				Expect(clUUID).To(BeEmpty())
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("VirtualMachineImage has an OwnerReference", func() {
			BeforeEach(func() {
				vmImage.OwnerReferences = []metav1.OwnerReference{
					{
						Kind: "ContentLibraryProvider",
						Name: "dummy-cl",
					},
				}

				initObjects = append(initObjects, vmImage, cl)
			})

			It("returns the content library UUID from the OwnerReference", func() {
				clUUID, err := reconciler.GetCLUUID(vmCtx)
				Expect(clUUID).To(Equal(cl.Spec.UUID))
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, vm, vmClass, vmImage)
		})

		When("the WCP_VMService FSS is enabled", func() {
			var oldVMServiceEnableFunc func() bool
			var vmClassBinding *vmopv1alpha1.VirtualMachineClassBinding

			BeforeEach(func() {
				oldVMServiceEnableFunc = lib.IsVMServiceFSSEnabled
				lib.IsVMServiceFSSEnabled = func() bool {
					return true
				}

				vmClassBinding = &vmopv1alpha1.VirtualMachineClassBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-class-binding",
						Namespace: vm.Namespace,
					},
					ClassRef: vmopv1alpha1.ClassReference{
						APIVersion: vmopv1alpha1.SchemeGroupVersion.Group,
						Name:       vm.Spec.ClassName,
						Kind:       reflect.TypeOf(vmClass).Elem().Name(),
					},
				}
			})

			AfterEach(func() {
				lib.IsVMServiceFSSEnabled = oldVMServiceEnableFunc
			})

			Context("No VirtualMachineClassBindings exist in namespace", func() {
				It("return an error and sets VirtualMacinePreReqReady Condition to false", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vm.Spec.ClassName, vm.Namespace)))

					msg := fmt.Sprintf("Namespace does not have access to VirtualMachineClass. className: %s, namespace: %s",
						vmCtx.VM.Spec.ClassName, vmCtx.VM.Namespace)

					expectedCondition := vmopv1alpha1.Conditions{
						*conditions.FalseCondition(
							vmopv1alpha1.VirtualMachinePrereqReadyCondition,
							vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
							vmopv1alpha1.ConditionSeverityError,
							msg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			Context("VirtualMachineBinding is not present for VM Class", func() {
				BeforeEach(func() {
					vmClassBinding.ClassRef.Name = "blah-blah-binding"
					initObjects = append(initObjects, vmClassBinding)
				})

				It("returns an error and sets the VirtualMachinePrereqReady Condition to false, ", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vm.Spec.ClassName, vm.Namespace)))

					msg := fmt.Sprintf("Namespace does not have access to VirtualMachineClass. className: %s, namespace: %s",
						vmCtx.VM.Spec.ClassName, vmCtx.VM.Namespace)
					expectedCondition := vmopv1alpha1.Conditions{
						*conditions.FalseCondition(
							vmopv1alpha1.VirtualMachinePrereqReadyCondition,
							vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
							vmopv1alpha1.ConditionSeverityError,
							msg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			Context("VirtualMachineClass and VirtualMachineClassBinding exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClassBinding)
				})

				It("should successfully reconcile the VM, add finalizer, verify the phase and Conditions", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
					Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))

					expectedCondition := vmopv1alpha1.Conditions{
						*conditions.TrueCondition(vmopv1alpha1.VirtualMachinePrereqReadyCondition),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("A missing VirtualMachineClassBinding is added to the namespace in the subsequent reconcile", func() {
				It("successfully reconciles and marks the VirtualMachinePrereqReady Condition to True", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vm.Spec.ClassName, vm.Namespace)))

					msg := fmt.Sprintf("Namespace does not have access to VirtualMachineClass. className: %s, namespace: %s",
						vmCtx.VM.Spec.ClassName, vmCtx.VM.Namespace)

					expectedCondition := vmopv1alpha1.Conditions{
						*conditions.FalseCondition(
							vmopv1alpha1.VirtualMachinePrereqReadyCondition,
							vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
							vmopv1alpha1.ConditionSeverityError,
							msg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))

					By("VirtualMachineClassBinding is added to the namespace")
					Expect(ctx.Client.Create(ctx, vmClassBinding)).To(Succeed())

					By("Reconciling again")
					err = reconciler.ReconcileNormal(vmCtx)
					Expect(err).NotTo(HaveOccurred())

					expectedCondition = vmopv1alpha1.Conditions{
						*conditions.TrueCondition(vmopv1alpha1.VirtualMachinePrereqReadyCondition),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})
		})

		Context("VirtualMachineClass does not exist for the class specified in the VM spec", func() {
			It("returns error and sets the VirtualMachinePrereqReady Condition to false", func() {
				vmCtx.VM.Spec.ClassName = "non-existent-class"
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).To(HaveOccurred())
				Expect(apiErrors.IsNotFound(err)).To(BeTrue())

				err = apiErrors.NewNotFound(schema.ParseGroupResource("virtualmachineclasses.vmoperator.vmware.com"), vmCtx.VM.Spec.ClassName)
				msg := fmt.Sprintf("Failed to get VirtualMachineClass %s: %s", vmCtx.VM.Spec.ClassName, err)
				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(
						vmopv1alpha1.VirtualMachinePrereqReadyCondition,
						vmopv1alpha1.VirtualMachineClassNotFoundReason,
						vmopv1alpha1.ConditionSeverityError,
						msg),
				}
				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			})
		})

		When("object does not have finalizer set", func() {
			BeforeEach(func() {
				vm.Finalizers = nil
			})

			It("will set finalizer", func() {
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
			})
		})

		It("will have finalizer set upon successful reconciliation", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))
		})

		When("number of reconcilers creating VirtualMachines on the provider are more than the configured threshold", func() {

			var isCalled int32

			It("does not call into the provider to create the new VM", func() {
				intgFakeVmProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
					atomic.AddInt32(&isCalled, 1)
					return nil
				}

				// Simulate a recociler that does not have any threads available to update the status of existing VMs.
				reconciler.NumVMsBeingCreatedOnProvider = 1
				reconciler.MaxConcurrentCreateVMsOnProvider = 0

				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(isCalled).To(Equal(int32(0)))
			})
		})

		It("will return error when provider fails to create VM", func() {
			// Simulate an error during VM create
			fakeVmProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
				return errors.New(providerError)
			}

			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(providerError))
			expectEvent(ctx, "CreateFailure")
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Creating))
		})

		It("will return error when provider fails to update VM", func() {
			// Simulate an error after the VM is created.
			fakeVmProvider.UpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
				return errors.New(providerError)
			}

			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(providerError))
			expectEvent(ctx, "UpdateFailure")
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))
		})

		It("can be called multiple times", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))

			err = reconciler.ReconcileNormal(vmCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
		})

		When("VM Class does not exist", func() {
			BeforeEach(func() {
				initObjects = []runtime.Object{vm}
			})

			It("return an error", func() {
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		When("VM Metadata is specified", func() {
			BeforeEach(func() {
				vm.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
					ConfigMapName: vmMetaData.Name,
					Transport:     "transport",
				}
			})

			When("VM Metadata does not exist", func() {
				It("return an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
				})
			})

			When("VM Metadata exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmMetaData)
				})

				It("returns success", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		When("VM ResourcePolicy is specified", func() {
			BeforeEach(func() {
				vm.Spec.ResourcePolicyName = vmResourcePolicy.Name
			})

			When("VM ResourcePolicy does not exist", func() {
				It("returns an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
				})
			})

			When("VM ResourcePolicy exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmResourcePolicy)
				})

				When("VM ResourcePolicy is not ready", func() {
					It("returns an error", func() {
						err := reconciler.ReconcileNormal(vmCtx)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("VirtualMachineSetResourcePolicy is not yet ready"))
					})
				})

				When("VM ResourcePolicy exists check returns error", func() {
					errMsg := "exists error"
					JustBeforeEach(func() {
						fakeVmProvider.DoesVirtualMachineSetResourcePolicyExistFn = func(ctx context.Context, rp *vmopv1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
							return false, errors.New(errMsg)
						}
					})

					It("returns an error", func() {
						err := reconciler.ReconcileNormal(vmCtx)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(errMsg))
					})
				})

				When("VM ResourcePolicy is ready", func() {
					JustBeforeEach(func() {
						fakeVmProvider.DoesVirtualMachineSetResourcePolicyExistFn = func(ctx context.Context, rp *vmopv1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
							return true, nil
						}
					})

					It("returns success", func() {
						err := reconciler.ReconcileNormal(vmCtx)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})
		})

		When("VM StorageClass is specified", func() {
			BeforeEach(func() {
				vm.Spec.StorageClass = storageClass.Name
			})

			When("StorageClass does not exist", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, resourceQuota)
				})

				It("returns an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
				})
			})

			When("StorageClass exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, storageClass, resourceQuota)
				})

				It("returns success", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		It("Should not call add to Prober Manager if ReconcileNormal fails", func() {
			// Simulate an error during VM create
			fakeVmProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
				return errors.New(providerError)
			}

			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(fakeProbeManager.IsAddToProberManagerCalled).Should(BeFalse())
		})

		It("Should call add to Prober Manager if ReconcileNormal succeeds", func() {
			fakeProbeManager.AddToProberManagerFn = func(vm *vmopv1alpha1.VirtualMachine) {
				fakeProbeManager.IsAddToProberManagerCalled = true
			}

			Expect(reconciler.ReconcileNormal(vmCtx)).Should(Succeed())
			Expect(fakeProbeManager.IsAddToProberManagerCalled).Should(BeTrue())
		})
	})

	Context("ReconcileDelete", func() {

		BeforeEach(func() {
			initObjects = append(initObjects, vm, vmClass, vmImage)
		})

		JustBeforeEach(func() {
			// Create the VM to be deleted
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))
		})

		It("will delete the created VM and emit corresponding event", func() {
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).NotTo(HaveOccurred())

			vmExists, err := fakeVmProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeFalse())

			expectEvent(ctx, "DeleteSuccess")
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Deleted))
		})

		It("will emit corresponding event during delete failure", func() {
			// Simulate delete failure
			fakeVmProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine) error {
				return errors.New(providerError)
			}
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())

			vmExists, err := fakeVmProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeTrue())

			expectEvent(ctx, "DeleteFailure")
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Deleting))
		})

		It("Should not remove from Prober Manager if ReconcileDelete fails", func() {
			// Simulate delete failure
			fakeVmProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine) error {
				return errors.New(providerError)
			}

			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(fakeProbeManager.IsRemoveFromProberManagerCalled).Should(BeFalse())
		})

		It("Should remove from Prober Manager if ReconcileDelete succeeds", func() {
			fakeProbeManager.RemoveFromProberManagerFn = func(vm *vmopv1alpha1.VirtualMachine) {
				fakeProbeManager.IsRemoveFromProberManagerCalled = true
			}

			Expect(reconciler.ReconcileDelete(vmCtx)).Should(Succeed())
			Expect(fakeProbeManager.IsRemoveFromProberManagerCalled).Should(BeTrue())
		})
	})
}

func expectEvent(ctx *builder.UnitTestContextForController, eventStr string) {
	var event string
	// This does not work if we have more than one events and the first one does not match.
	EventuallyWithOffset(1, ctx.Events).Should(Receive(&event))
	eventComponents := strings.Split(event, " ")
	ExpectWithOffset(1, eventComponents[1]).To(Equal(eventStr))
}
