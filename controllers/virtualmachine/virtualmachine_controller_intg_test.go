// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	const (
		storageClassName = "foo-class"
	)

	var (
		ctx *builder.IntegrationTestContext

		vm                     *vmopv1alpha1.VirtualMachine
		vmKey                  types.NamespacedName
		vmImage                *vmopv1alpha1.VirtualMachineImage
		vmClass                *vmopv1alpha1.VirtualMachineClass
		contentsource          *vmopv1alpha1.ContentSource
		contentLibraryProvider *vmopv1alpha1.ContentLibraryProvider
		metadataConfigMap      *corev1.ConfigMap
		storageClass           *storagev1.StorageClass
		resourceQuota          *corev1.ResourceQuota

		// FSS values. These are manipulated atomically to avoid races where the
		// controller is trying to read this _while_ the tests are updating it.
		vmServiceFSS       uint32
		instanceStorageFSS uint32
	)

	lib.IsVMServiceFSSEnabled = func() bool {
		return atomic.LoadUint32(&vmServiceFSS) != 0
	}

	lib.IsInstanceStorageFSSEnabled = func() bool {
		return atomic.LoadUint32(&instanceStorageFSS) != 0
	}

	BeforeEach(func() {
		atomic.StoreUint32(&vmServiceFSS, 0)
		atomic.StoreUint32(&instanceStorageFSS, 0)

		ctx = suite.NewIntegrationTestContext()

		vmClass = &vmopv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "small",
			},
			Spec: vmopv1alpha1.VirtualMachineClassSpec{
				Hardware: vmopv1alpha1.VirtualMachineClassHardware{
					Cpus:            4,
					Memory:          resource.MustParse("1Mi"),
					InstanceStorage: builder.DummyInstanceStorage(),
				},
				Policies: vmopv1alpha1.VirtualMachineClassPolicies{
					Resources: vmopv1alpha1.VirtualMachineClassResources{
						Requests: vmopv1alpha1.VirtualMachineResourceSpec{
							Cpu:    resource.MustParse("1000Mi"),
							Memory: resource.MustParse("100Mi"),
						},
						Limits: vmopv1alpha1.VirtualMachineResourceSpec{
							Cpu:    resource.MustParse("2000Mi"),
							Memory: resource.MustParse("200Mi"),
						},
					},
				},
			},
		}

		contentLibraryProvider = &vmopv1alpha1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-contentlibraryprovider",
			},
			Spec: vmopv1alpha1.ContentLibraryProviderSpec{
				UUID: "dummy-cl-uuid",
			},
		}

		contentsource = &vmopv1alpha1.ContentSource{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-contentsource",
			},
			Spec: vmopv1alpha1.ContentSourceSpec{
				ProviderRef: vmopv1alpha1.ContentProviderReference{
					Name: contentLibraryProvider.Name,
					Kind: "ContentLibraryProvider",
				},
			},
		}

		vmImage = &vmopv1alpha1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-image",
			},
		}

		storageClass = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: storageClassName,
			},
			Provisioner: "foo",
			Parameters: map[string]string{
				"storagePolicyID": "foo",
			},
		}

		metadataConfigMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-metadata",
				Namespace: ctx.Namespace,
			},
			Data: map[string]string{
				"someKey": "someValue",
			},
		}

		resourceQuota = &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-rq",
				Namespace: ctx.Namespace,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					storageClassName + ".storageclass.storage.k8s.io/persistentvolumeclaims": resource.MustParse("1"),
					"simple-class" + ".storageclass.storage.k8s.io/persistentvolumeclaims":   resource.MustParse("1"),
					"limits.cpu":    resource.MustParse("2"),
					"limits.memory": resource.MustParse("2Gi"),
				},
			},
		}

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm",
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ImageName:    "dummy-image",
				ClassName:    vmClass.Name,
				PowerState:   vmopv1alpha1.VirtualMachinePoweredOn,
				StorageClass: storageClass.Name,
				VmMetadata: &vmopv1alpha1.VirtualMachineMetadata{
					Transport:     vmopv1alpha1.VirtualMachineMetadataOvfEnvTransport,
					ConfigMapName: metadataConfigMap.Name,
				},
			},
		}
		vmKey = types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getVirtualMachine := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1alpha1.VirtualMachine {
		vm := &vmopv1alpha1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	waitForVirtualMachineFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		Eventually(func() []string {
			if vm := getVirtualMachine(ctx, objKey); vm != nil {
				return vm.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizer), "waiting for VirtualMachine finalizer")
	}

	waitForVirtualMachineInstanceStorage := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		var vm *vmopv1alpha1.VirtualMachine
		Eventually(func() bool {
			if vm = getVirtualMachine(ctx, objKey); vm != nil {
				return instancestorage.IsConfigured(vm)
			}
			return false
		}).Should(BeTrue(), "waiting for VirtualMachine instance storage volumes to be added")

		expectInstanceStorageVolumes(vm, vmClass.Spec.Hardware.InstanceStorage)
	}

	markVMForInstanceStoragePVCBound := func(objKey types.NamespacedName) {
		vm := getVirtualMachine(ctx, objKey)
		Expect(vm).ToNot(BeNil())
		vmCopy := vm.DeepCopy()
		if vm.Annotations == nil {
			vm.Annotations = map[string]string{}
		}
		vm.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey] = ""
		Expect(ctx.Client.Patch(ctx, vm, ctrlruntime.MergeFrom(vmCopy))).To(Succeed())
	}

	waitForVirtualMachineInstanceStorageSelectedNode := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		Eventually(func() bool {
			if vm := getVirtualMachine(ctx, objKey); vm != nil {
				_, snaExists := vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey]
				_, snmaExists := vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey]
				return snaExists && snmaExists
			}
			return false
		}).Should(BeTrue(), "waiting for VirtualMachine instance storage selected node annotations to be added")
	}

	Context("Reconcile", func() {
		dummyBiosUUID := "biosUUID42"
		dummyInstanceUUID := "instanceUUID1234"

		BeforeEach(func() {
			intgFakeVMProvider.Lock()
			intgFakeVMProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, _ vmprovider.VMConfigArgs) error {
				vm.Status.BiosUUID = dummyBiosUUID
				vm.Status.InstanceUUID = dummyInstanceUUID
				return nil
			}
			intgFakeVMProvider.Unlock()

			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())
			Expect(ctx.Client.Create(ctx, vmImage)).To(Succeed())
			Expect(ctx.Client.Create(ctx, contentLibraryProvider)).To(Succeed())
			Expect(ctx.Client.Create(ctx, contentsource)).To(Succeed())
			Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
			Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
			Expect(ctx.Client.Create(ctx, metadataConfigMap)).To(Succeed())

			// For ContentSourceBindings Condition tests, we need to add an OwnerRef to the VM image to point to the ContentLibraryProvider.
			// The controller uses this Ref to know which content library this image is part of.
			vmImage.OwnerReferences = []metav1.OwnerReference{{
				Name:       contentLibraryProvider.Name,
				Kind:       "ContentLibraryProvider",
				APIVersion: "vmoperator.vmware.com/v1alpha1",
				UID:        contentLibraryProvider.ObjectMeta.UID,
			}}
			Expect(ctx.Client.Update(ctx, vmImage)).To(Succeed())

			contentLibraryProvider.OwnerReferences = []metav1.OwnerReference{{
				Name:       contentsource.Name,
				Kind:       "ContentSource",
				APIVersion: "vmoperator.vmware.com/v1alpha1",
				UID:        contentsource.ObjectMeta.UID,
			}}
			Expect(ctx.Client.Update(ctx, contentLibraryProvider)).To(Succeed())
		})

		AfterEach(func() {
			By("Delete VirtualMachine", func() {
				if err := ctx.Client.Delete(ctx, vm); err == nil {
					vm := &vmopv1alpha1.VirtualMachine{}
					// If VM is still around because of finalizer, try to cleanup for next test.
					if err := ctx.Client.Get(ctx, vmKey, vm); err == nil && len(vm.Finalizers) > 0 {
						vm.Finalizers = nil
						_ = ctx.Client.Update(ctx, vm)
					}
				} else {
					Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}
			})

			By("Delete cluster scoped resources", func() {
				err := ctx.Client.Delete(ctx, vmClass)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = ctx.Client.Delete(ctx, vmImage)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = ctx.Client.Delete(ctx, storageClass)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = ctx.Client.Delete(ctx, contentLibraryProvider)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = ctx.Client.Delete(ctx, contentsource)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})
		})

		When("the pause annotation is set", func() {
			It("Reconcile returns early and the finalizer never gets added", func() {
				// Set the Pause annotation on the VM
				vm.Annotations = map[string]string{
					vmopv1alpha1.PauseAnnotation: "",
				}

				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				Consistently(func() []string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizer), "waiting for VirtualMachine finalizer")
			})
		})

		It("Reconciles after VirtualMachine creation", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			By("VirtualMachine should have finalizer added", func() {
				waitForVirtualMachineFinalizer(ctx, vmKey)
			})

			By("VirtualMachine should exist in Fake Provider", func() {
				Eventually(func() bool {
					exists, err := intgFakeVMProvider.DoesVirtualMachineExist(ctx, vm)
					if err != nil {
						return false
					}
					return exists
				}).Should(BeTrue())
			})

			By("VirtualMachine should reflect VMProvider updates", func() {
				Eventually(func() string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Status.BiosUUID
					}
					return ""
				}).Should(Equal(dummyBiosUUID), "waiting for expected BiosUUID")

				Eventually(func() string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Status.InstanceUUID
					}
					return ""
				}).Should(Equal(dummyInstanceUUID), "waiting for expected InstanceUUID")

				Eventually(func() vmopv1alpha1.VMStatusPhase {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Status.Phase
					}
					return ""
				}).Should(Equal(vmopv1alpha1.Created), "waiting for expected VM Phase")
			})

			By("VirtualMachine should not be updated in steady-state", func() {
				vm := getVirtualMachine(ctx, vmKey)
				Expect(vm).ToNot(BeNil())
				rv := vm.GetResourceVersion()
				Expect(rv).ToNot(BeEmpty())
				expected := fmt.Sprintf("%s :: %d", rv, vm.GetGeneration())
				// The resync period is 1 second, so balance between giving enough time vs a slow test.
				// Note: the kube-apiserver we test against (obtained from kubebuilder) is old and
				// appears to behavior differently than newer versions (like used in the SV) in that noop
				// Status subresource updates don't increment the ResourceVersion.
				Consistently(func() string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return fmt.Sprintf("%s :: %d", vm.GetResourceVersion(), vm.GetGeneration())
					}
					return ""
				}, 4*time.Second).Should(Equal(expected))
			})
		})

		When("InstanceStorage FSS is Enabled", func() {
			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.PlaceVirtualMachineFn = func(_ context.Context, vm *vmopv1alpha1.VirtualMachine, _ vmprovider.VMConfigArgs) error {
					if vm.Annotations == nil {
						vm.Annotations = map[string]string{}
					}
					vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = "host-42.vmware.com"
					vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = "host-42"
					return nil
				}
				intgFakeVMProvider.Unlock()
			})

			JustBeforeEach(func() {
				atomic.StoreUint32(&instanceStorageFSS, 1)
			})

			It("Reconciles after VirtualMachine creation", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				By("VirtualMachine should have finalizer added", func() {
					waitForVirtualMachineFinalizer(ctx, vmKey)
				})

				By("VirtualMachine should have instance storage configured", func() {
					waitForVirtualMachineInstanceStorage(ctx, vmKey)
				})

				By("VirtualMachine should have selected node annotations", func() {
					waitForVirtualMachineInstanceStorageSelectedNode(ctx, vmKey)
				})

				By("VirtualMachine should not be created in Fake Provider", func() {
					exists, err := intgFakeVMProvider.DoesVirtualMachineExist(ctx, vm)
					Expect(err).ToNot(HaveOccurred())
					Expect(exists).To(BeFalse())
				})

				By("set pvcs-bound annotation", func() {
					markVMForInstanceStoragePVCBound(vmKey)
				})

				By("VirtualMachine should exist in Fake Provider", func() {
					Eventually(func() bool {
						exists, err := intgFakeVMProvider.DoesVirtualMachineExist(ctx, vm)
						if err != nil {
							return false
						}
						return exists
					}).Should(BeTrue())
				})
			})
		})

		When("VMService FSS is Enabled", func() {
			var (
				vmClassBinding       *vmopv1alpha1.VirtualMachineClassBinding
				contentSourceBinding *vmopv1alpha1.ContentSourceBinding
			)

			BeforeEach(func() {
				vmClassBinding = &vmopv1alpha1.VirtualMachineClassBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake-class-binding",
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
						Name:       contentsource.Name,
						Kind:       reflect.TypeOf(contentsource).Elem().Name(),
					},
				}
			})

			JustBeforeEach(func() {
				atomic.StoreUint32(&vmServiceFSS, 1)
			})

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

				EventuallyWithOffset(1, func() []vmopv1alpha1.Condition {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Status.Conditions
					}
					return nil
				}).Should(conditions.MatchConditions(expectedCondition))
			}

			validateNoContentSourceBindingCondition := func(vm *vmopv1alpha1.VirtualMachine) {
				msg := fmt.Sprintf("Namespace does not have access to VirtualMachineImage. imageName: %v, contentLibraryUUID: %v, namespace: %v",
					vm.Spec.ImageName, contentLibraryProvider.Spec.UUID, vm.Namespace)

				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(
						vmopv1alpha1.VirtualMachinePrereqReadyCondition,
						vmopv1alpha1.ContentSourceBindingNotFoundReason,
						vmopv1alpha1.ConditionSeverityError,
						msg),
				}

				EventuallyWithOffset(1, func() []vmopv1alpha1.Condition {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Status.Conditions
					}
					return nil
				}).Should(conditions.MatchConditions(expectedCondition))
			}

			validatePreReqTrueCondition := func(objKey types.NamespacedName) {
				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.VirtualMachinePrereqReadyCondition),
				}
				EventuallyWithOffset(1, func() []vmopv1alpha1.Condition {
					if vm := getVirtualMachine(ctx, objKey); vm != nil {
						return vm.Status.Conditions
					}
					return nil
				}).Should(conditions.MatchConditions(expectedCondition))
			}

			// nolint: dupl
			Context("VMClassBinding Conditions", func() {
				BeforeEach(func() {
					Expect(ctx.Client.Create(ctx, contentSourceBinding)).To(Succeed())
				})
				AfterEach(func() {
					Expect(ctx.Client.Delete(ctx, contentSourceBinding)).To(Succeed())
				})

				It("Reconciles VirtualMachine after VirtualMachineClassBinding created", func() {
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

					By("VirtualMachine should have finalizer added", func() {
						waitForVirtualMachineFinalizer(ctx, vmKey)
					})

					By("validating that the VM should have the Condition Reason for missing VirtualMachineClassBindings", func() {
						validateNoVMClassBindingCondition(vm)
					})

					By("VirtualMachineClassBinding is added to the namespace", func() {
						Expect(ctx.Client.Create(ctx, vmClassBinding)).To(Succeed())
					})

					By("validating that the VirtualMachinePreReq condition is marked as True", func() {
						validatePreReqTrueCondition(vmKey)
					})

					By("deleting the VirtualMachineClassBinding", func() {
						Expect(ctx.Client.Delete(ctx, vmClassBinding)).To(Succeed())
					})

					By("PreReq condition should be False with VirtualMachineClassBindingNotFound reason set", func() {
						validateNoVMClassBindingCondition(vm)
					})
				})
			})

			// nolint: dupl
			Context("ContentSourceBinding Conditions", func() {
				BeforeEach(func() {
					Expect(ctx.Client.Create(ctx, vmClassBinding)).To(Succeed())
				})
				AfterEach(func() {
					Expect(ctx.Client.Delete(ctx, vmClassBinding)).To(Succeed())
				})

				It("Reconciles VirtualMachine after VirtualMachineClassBinding created", func() {
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

					By("VirtualMachine should have finalizer added", func() {
						waitForVirtualMachineFinalizer(ctx, vmKey)
					})

					By("validating that the VM should have the Condition Reason for missing ContentSourceBindings", func() {
						validateNoContentSourceBindingCondition(vm)
					})

					By("ContentSource is added to the namespace", func() {
						Expect(ctx.Client.Create(ctx, contentSourceBinding)).To(Succeed())
					})

					By("validating that the VirtualMachinePreReq condition is marked as True", func() {
						validatePreReqTrueCondition(vmKey)
					})

					By("deleting the ContentSourceBindings", func() {
						Expect(ctx.Client.Delete(ctx, contentSourceBinding)).To(Succeed())
					})

					By("PreReq condition should be False with ContentSourceBindingNotFound reason set", func() {
						validateNoContentSourceBindingCondition(vm)
					})
				})
			})
		})

		When("Provider CreateVM returns an error", func() {
			errMsg := "create error"

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) error {
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine is in Creating Phase", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)

				By("Phase should be Creating", func() {
					Eventually(func() vmopv1alpha1.VMStatusPhase {
						if vm := getVirtualMachine(ctx, vmKey); vm != nil {
							return vm.Status.Phase
						}
						return ""
					}).Should(Equal(vmopv1alpha1.Creating))
				})
			})
		})

		It("Reconciles after VirtualMachine deletion", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())
			By("Finalizer should be removed after deletion", func() {
				Eventually(func() []string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizer))
			})

			/*
				By("VM should be in Deleted Phase", func() {
					vm := getVirtualMachine(ctx, vmKey)
					Expect(vm).ToNot(BeNil())
					Expect(vm.Status.Phase).To(Equal(vmopv1alpha1.Deleted))
				})
			*/

			By("VirtualMachine should not exist in Fake Provider", func() {
				exists, err := intgFakeVMProvider.DoesVirtualMachineExist(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})

		When("Provider DeleteVM returns an error", func() {
			errMsg := "delete error"

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine) error {
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine is in Deleting Phase", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)

				Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())
				By("Phase should be Deleting", func() {
					Eventually(func() vmopv1alpha1.VMStatusPhase {
						if vm := getVirtualMachine(ctx, vmKey); vm != nil {
							return vm.Status.Phase
						}
						return ""
					}).Should(Equal(vmopv1alpha1.Deleting))
				})

				By("Finalizer should still be present", func() {
					vm := getVirtualMachine(ctx, vmKey)
					Expect(vm).ToNot(BeNil())
					Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
				})
			})
		})
	})
}
