// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	proberfake "github.com/vmware-tanzu/vm-operator/pkg/prober/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	instancestoragetestutil "github.com/vmware-tanzu/vm-operator/test/instancestorage"
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
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler          *virtualmachine.Reconciler
		fakeVMProvider      *providerfake.VMProvider
		vmCtx               *vmopContext.VirtualMachineContext
		vm                  *vmopv1alpha1.VirtualMachine
		contentSource       *vmopv1alpha1.ContentSource
		clProvider          *vmopv1alpha1.ContentLibraryProvider
		vmClass             *vmopv1alpha1.VirtualMachineClass
		vmImage             *vmopv1alpha1.VirtualMachineImage
		vmMetaDataConfigMap *corev1.ConfigMap
		vmMetaDataSecret    *corev1.Secret
		vmResourcePolicy    *vmopv1alpha1.VirtualMachineSetResourcePolicy
		storageClass        *storagev1.StorageClass
		resourceQuota       *corev1.ResourceQuota

		fakeProbeManager *proberfake.ProberManager

		// Various FSS. This should be manipulated atomically to avoid races where
		// the controller is trying to read this _while_ the tests are updating it.
		vmServiceFSS   uint32
		faultDomainFSS uint32
	)

	BeforeEach(func() {
		// Modify the helper function to return the custom value of the FSS
		lib.IsVMServiceFSSEnabled = func() bool {
			return atomic.LoadUint32(&vmServiceFSS) != 0
		}

		lib.IsWcpFaultDomainsFSSEnabled = func() bool {
			return atomic.LoadUint32(&faultDomainFSS) != 0
		}

		vmClass = &vmopv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vmclass",
			},
		}

		contentSource = &vmopv1alpha1.ContentSource{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-contentsource",
			},
		}

		// For ContentSourceBindings Condition tests, we need to add an OwnerRef to the VM image to point to the ContentLibraryProvider.
		// The controller uses this Ref to know which content library this image is part of.
		clProvider = &vmopv1alpha1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-contentlibraryprovider",
				OwnerReferences: []metav1.OwnerReference{{
					Name: contentSource.Name,
					Kind: "ContentSource",
					// UID:  contentSource.UID,
				}},
			},
			Spec: vmopv1alpha1.ContentLibraryProviderSpec{
				UUID: "dummy-cl-uuid",
			},
		}

		vmImage = &vmopv1alpha1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-image",
				OwnerReferences: []metav1.OwnerReference{{
					Name: clProvider.Name,
					Kind: "ContentLibraryProvider",
					// UID:  clProvider.UID,
				}},
			},
		}

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dummy-vm",
				Namespace:  "dummy-ns",
				Labels:     map[string]string{},
				Finalizers: []string{finalizer},
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ClassName: vmClass.Name,
				ImageName: vmImage.Name,
			},
		}

		vmMetaDataConfigMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm-metadata",
				Namespace: vm.Namespace,
			},
			Data: map[string]string{
				"foo": "bar",
			},
		}

		vmMetaDataSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm-metadata",
				Namespace: vm.Namespace,
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
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
			ctx.VMProvider,
			fakeProbeManagerIf,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		fakeProbeManager = fakeProbeManagerIf.(*proberfake.ProberManager)

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
		fakeVMProvider = nil
	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, vm, vmClass, vmImage, clProvider, contentSource)
		})

		When("the WCP_VMService FSS is enabled", func() {
			var oldVMServiceFSSState uint32

			var vmClassBinding *vmopv1alpha1.VirtualMachineClassBinding
			var contentSourceBinding *vmopv1alpha1.ContentSourceBinding

			validateNoVMClassBindingCondition := func(vm *vmopv1alpha1.VirtualMachine) {
				msg := fmt.Sprintf("Namespace does not have access to VirtualMachineClass. className: %s, namespace: %s",
					vm.Spec.ClassName, vm.Namespace)

				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(
						vmopv1alpha1.VirtualMachinePrereqReadyCondition,
						vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
						vmopv1alpha1.ConditionSeverityError,
						msg),
				}
				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			}

			validateNoContentSourceBindingCondition := func(vm *vmopv1alpha1.VirtualMachine, clUUID string) {
				msg := fmt.Sprintf("Namespace does not have access to VirtualMachineImage. imageName: %v, contentLibraryUUID: %v, namespace: %v",
					vm.Spec.ImageName, clUUID, vm.Namespace)

				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(
						vmopv1alpha1.VirtualMachinePrereqReadyCondition,
						vmopv1alpha1.ContentSourceBindingNotFoundReason,
						vmopv1alpha1.ConditionSeverityError,
						msg),
				}

				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			}

			BeforeEach(func() {
				oldVMServiceFSSState = vmServiceFSS
				atomic.StoreUint32(&vmServiceFSS, 1)

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

				contentSourceBinding = &vmopv1alpha1.ContentSourceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake-contentsource-binding",
						Namespace: vm.Namespace,
					},
					ContentSourceRef: vmopv1alpha1.ContentSourceReference{
						APIVersion: vmopv1alpha1.SchemeGroupVersion.Group,
						Name:       contentSource.Name,
						Kind:       reflect.TypeOf(contentSource).Elem().Name(),
					},
				}
			})

			AfterEach(func() {
				atomic.StoreUint32(&vmServiceFSS, oldVMServiceFSSState)
			})

			Context("No VirtualMachineClassBindings exist in namespace", func() {
				It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vm.Spec.ClassName, vm.Namespace)))
					validateNoVMClassBindingCondition(vmCtx.VM)
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

					validateNoVMClassBindingCondition(vmCtx.VM)
				})
			})

			When("A missing VirtualMachineClassBinding is added to the namespace in the subsequent reconcile", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, contentSourceBinding)
				})

				It("successfully reconciles and marks the VirtualMachinePrereqReady Condition to True", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					bindingNotFoundError := fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vm.Spec.ClassName, vm.Namespace)
					Expect(err).To(MatchError(bindingNotFoundError))

					validateNoVMClassBindingCondition(vmCtx.VM)

					By("VirtualMachineClassBinding is added to the namespace")
					Expect(ctx.Client.Create(ctx, vmClassBinding)).To(Succeed())

					By("Reconciling again")
					err = reconciler.ReconcileNormal(vmCtx)
					Expect(err).NotTo(HaveOccurred())

					expectedCondition := vmopv1alpha1.Conditions{
						*conditions.TrueCondition(vmopv1alpha1.VirtualMachinePrereqReadyCondition),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("No ContentSourceBindings exist in the namespace", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClassBinding)
				})

				It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					msg := fmt.Sprintf("Namespace does not have access to VirtualMachineImage. imageName: %v, contentLibraryUUID: %v, namespace: %v",
						vm.Spec.ImageName, clProvider.Spec.UUID, vm.Namespace)

					csBindingNotFoundErr := fmt.Errorf(msg)
					Expect(err).To(MatchError(csBindingNotFoundErr))

					validateNoContentSourceBindingCondition(vmCtx.VM, clProvider.Spec.UUID)
				})
			})

			When("ContentSourceBinding is not present for the content library corresponding to the VM iamge", func() {
				BeforeEach(func() {
					contentSourceBinding.ContentSourceRef.Name = "blah-blah-binding"
					initObjects = append(initObjects, vmClassBinding, contentSourceBinding)
				})

				It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					msg := fmt.Sprintf("Namespace does not have access to VirtualMachineImage. imageName: %v, contentLibraryUUID: %v, namespace: %v",
						vm.Spec.ImageName, clProvider.Spec.UUID, vm.Namespace)

					csBindingNotFoundErr := fmt.Errorf(msg)
					Expect(err).To(MatchError(csBindingNotFoundErr))

					validateNoContentSourceBindingCondition(vmCtx.VM, clProvider.Spec.UUID)
				})
			})

			When("A missing ContentSourceBinding is added to the namespace in the subsequent reconcile", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClassBinding)
				})

				It("successfully reconciles and marks the VirtualMachinePrereqReady Condition to True", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					msg := fmt.Sprintf("Namespace does not have access to VirtualMachineImage. imageName: %v, contentLibraryUUID: %v, namespace: %v",
						vm.Spec.ImageName, clProvider.Spec.UUID, vm.Namespace)

					csBindingNotFoundErr := fmt.Errorf(msg)
					Expect(err).To(MatchError(csBindingNotFoundErr))

					validateNoContentSourceBindingCondition(vmCtx.VM, clProvider.Spec.UUID)

					By("ContentSourceBinding is added to the namespace")
					Expect(ctx.Client.Create(ctx, contentSourceBinding)).To(Succeed())

					By("Reconciling again")
					err = reconciler.ReconcileNormal(vmCtx)
					Expect(err).NotTo(HaveOccurred())

					expectedCondition := vmopv1alpha1.Conditions{
						*conditions.TrueCondition(vmopv1alpha1.VirtualMachinePrereqReadyCondition),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("ContentSourceBindings and VirtualMachineClassBindings are present", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClassBinding, contentSourceBinding)
				})

				It("marks the VirtualMachinePreReq Condition as True", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).NotTo(HaveOccurred())

					expectedCondition := vmopv1alpha1.Conditions{
						*conditions.TrueCondition(vmopv1alpha1.VirtualMachinePrereqReadyCondition),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})
		})

		When("the WCP_FAULT_DOMAINS FSS is enabled", func() {
			var oldFaultDomainFSSState uint32

			BeforeEach(func() {
				oldFaultDomainFSSState = faultDomainFSS
				atomic.StoreUint32(&faultDomainFSS, 1)
			})

			AfterEach(func() {
				atomic.StoreUint32(&faultDomainFSS, oldFaultDomainFSSState)
			})

			It("Returns error when no AZs exist", func() {
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(topology.ErrNoAvailabilityZones))
				Expect(vmCtx.VM.Labels).ToNot(HaveKey(topology.KubernetesTopologyZoneLabelKey))
			})

			When("VM already has AZ assigned", func() {
				JustBeforeEach(func() {
					vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = "mars-east-1"
				})

				It("VM has same AZ assigned", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
					Expect(vmCtx.VM.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, "mars-east-1"))
				})
			})

			When("AZ exist", func() {
				var az *topologyv1.AvailabilityZone

				BeforeEach(func() {
					az = builder.DummyAvailabilityZone()
					initObjects = append(initObjects, az)
				})

				It("Assigns zone to VM that does not have one already assigned", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
					Expect(vmCtx.VM.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, az.Name))
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

		Context("VirtualMachineImage specified in the VM spec does not exist", func() {
			It("returns error and sets the VirtualMachinePrereqReady Condition to false", func() {
				vmCtx.VM.Spec.ImageName = "non-existent-image"
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).To(HaveOccurred())
				Expect(apiErrors.IsNotFound(err)).To(BeTrue())

				err = apiErrors.NewNotFound(schema.ParseGroupResource("virtualmachineimages.vmoperator.vmware.com"), vmCtx.VM.Spec.ImageName)
				msg := fmt.Sprintf("Failed to get VirtualMachineImage %s: %s", vmCtx.VM.Spec.ImageName, err)
				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(
						vmopv1alpha1.VirtualMachinePrereqReadyCondition,
						vmopv1alpha1.VirtualMachineImageNotFoundReason,
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

		When("number of reconciler workers creating VirtualMachines on the provider are more than the configured threshold", func() {
			var isCalled int32

			It("does not call into the provider to create the new VM", func() {
				intgFakeVMProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) error {
					atomic.AddInt32(&isCalled, 1)
					return nil
				}

				// Simulate a reconciler that does not have any threads available to update the status of existing VMs.
				reconciler.NumVMsBeingCreatedOnProvider = 1
				reconciler.MaxConcurrentCreateVMsOnProvider = 0

				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(isCalled).To(Equal(int32(0)))
			})
		})

		It("will return error when provider fails to create VM", func() {
			// Simulate an error during VM create
			fakeVMProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) error {
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
			fakeVMProvider.UpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) error {
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
				initObjects = []client.Object{vm}
			})

			It("return an error", func() {
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		When("Instance Storage related", func() {
			orgIsInstanceStorageFSSEnabled := lib.IsInstanceStorageFSSEnabled
			BeforeEach(func() {
				lib.IsInstanceStorageFSSEnabled = func() bool {
					return true
				}
			})
			AfterEach(func() {
				lib.IsInstanceStorageFSSEnabled = orgIsInstanceStorageFSSEnabled
			})

			It("Instance Storage FSS is disabled", func() {
				lib.IsInstanceStorageFSSEnabled = func() bool {
					return false
				}
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(instancestoragetestutil.GetConfiguredInstanceVolumes(vmCtx.VM)).To(HaveLen(0))
				Expect(vmCtx.VM.GetLabels()).ToNot(HaveKey(constants.InstanceStorageLabelKey))
			})

			It("Virtual Machine is created", func() {
				vmCtx.VM.Status.Phase = vmopv1alpha1.Created
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(instancestoragetestutil.GetConfiguredInstanceVolumes(vmCtx.VM)).To(HaveLen(0))
				Expect(vmCtx.VM.GetLabels()).ToNot(HaveKey(constants.InstanceStorageLabelKey))
			})

			It("Instance Storage is not configured in VM Spec and Instance Volume is not added in VM Class", func() {
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(instancestoragetestutil.GetConfiguredInstanceVolumes(vmCtx.VM)).To(HaveLen(0))
				Expect(vmCtx.VM.GetLabels()).ToNot(HaveKey(constants.InstanceStorageLabelKey))
			})

			When("Instance Volume is added in VM Class", func() {
				BeforeEach(func() {
					vmClass.Spec.Hardware.InstanceStorage = builder.DummyInstanceStorage()
					initObjects = []client.Object{}
					initObjects = append(initObjects, vm, vmClass, vmImage, clProvider, contentSource)
				})

				It("Instance Volumes should be added", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
					Expect(instancestoragetestutil.GetConfiguredInstanceVolumes(vmCtx.VM)).ToNot(HaveLen(0))
					Expect(instancestoragetestutil.InstanceVolumeEqComparator(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)).Should(BeTrue())
				})

				It("Instance Storage is already configured in VM Spec", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
					instanceVolumesBefore := instancestoragetestutil.GetConfiguredInstanceVolumes(vmCtx.VM)
					Expect(instanceVolumesBefore).ToNot(HaveLen(0))
					Expect(instancestoragetestutil.InstanceVolumeEqComparator(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)).Should(BeTrue())

					// Instance Storage is already configured, should not patch again
					err = reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
					instanceVolumesAfter := instancestoragetestutil.GetConfiguredInstanceVolumes(vmCtx.VM)
					Expect(instanceVolumesAfter).ToNot(HaveLen(0))
					Expect(instanceVolumesAfter).Should(HaveLen(len(instanceVolumesBefore)))
					Expect(instancestoragetestutil.InstanceVolumeEqComparator(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)).Should(BeTrue())
				})

				It("No host recommendation returned, Selected Node annotations should not be added", func() {
					fakeVMProvider.GetCompatibleHostsFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) ([]string, error) {
						return []string{}, nil
					}
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
					Expect(vmCtx.VM.GetAnnotations()).ToNot(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
					Expect(vmCtx.VM.GetAnnotations()).ToNot(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
				})

				It("Selected Node annotations should be added", func() {
					fakeVMProvider.GetCompatibleHostsFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) ([]string, error) {
						return []string{"host-28", "host-88", "host-64"}, nil
					}
					fakeVMProvider.GetHostNetworkInfoFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, hostMoID string) (string, error) {
						return "sc2-rdops-vm05-dhcp-186-183.eng.vmware.com", nil
					}
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
					Expect(vmCtx.VM.GetAnnotations()).To(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
					Expect(vmCtx.VM.GetAnnotations()).To(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
				})

				It("Selected Node annotation is already added, PVC is bound then VM status should be created", func() {
					vmCtx.VM.Annotations = make(map[string]string)
					vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = "host-88"
					vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = "sc2-rdops-vm05-dhcp-186-183.eng.vmware.com"
					vmCtx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey] = lib.TrueString
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
					Expect(instancestoragetestutil.GetConfiguredInstanceVolumes(vmCtx.VM)).ToNot(HaveLen(0))
					Expect(instancestoragetestutil.InstanceVolumeEqComparator(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)).Should(BeTrue())
					// When volume bound annotation is set, VM's status.phase is set to Created.
					Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))
				})
			})
		})

		When("VM Metadata is specified via a ConfigMap", func() {
			BeforeEach(func() {
				vm.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
					ConfigMapName: vmMetaDataConfigMap.Name,
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
					initObjects = append(initObjects, vmMetaDataConfigMap)
				})

				It("returns success", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		When("VM Metadata is specified via a Secret", func() {
			BeforeEach(func() {
				vm.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
					SecretName: vmMetaDataSecret.Name,
					Transport:  "transport",
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
					initObjects = append(initObjects, vmMetaDataSecret)
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
						fakeVMProvider.IsVirtualMachineSetResourcePolicyReadyFn = func(ctx context.Context, azName string, rp *vmopv1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
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
						fakeVMProvider.IsVirtualMachineSetResourcePolicyReadyFn = func(ctx context.Context, azName string, rp *vmopv1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
							return true, nil
						}
					})

					When("VM ResourcePolicy is not in DELETING State", func() {
						It("returns success", func() {
							err := reconciler.ReconcileNormal(vmCtx)
							Expect(err).ToNot(HaveOccurred())
						})
					})

					When("VM ResourcePolicy is in deleting state", func() {
						BeforeEach(func() {
							t := metav1.Now()
							vmResourcePolicy.SetDeletionTimestamp(&t)
						})

						It("returns an error", func() {
							err := reconciler.ReconcileNormal(vmCtx)
							Expect(err.Error()).To(ContainSubstring("cannot create VirtualMachine with its resource policy in DELETING state"))
						})
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
			fakeVMProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) error {
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
			initObjects = append(initObjects, vm, vmClass, vmImage, clProvider, contentSource)
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

			vmExists, err := fakeVMProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeFalse())

			expectEvent(ctx, "DeleteSuccess")
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Deleted))
		})

		It("will emit corresponding event during delete failure", func() {
			// Simulate delete failure
			fakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine) error {
				return errors.New(providerError)
			}
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())

			vmExists, err := fakeVMProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeTrue())

			expectEvent(ctx, "DeleteFailure")
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Deleting))
		})

		It("Should not remove from Prober Manager if ReconcileDelete fails", func() {
			// Simulate delete failure
			fakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine) error {
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
