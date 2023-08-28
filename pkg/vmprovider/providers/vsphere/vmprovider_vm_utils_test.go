// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	goctx "context"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/vim25/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmUtilTests() {

	var (
		k8sClient   client.Client
		initObjects []client.Object

		vmCtx context.VirtualMachineContext
	)

	BeforeEach(func() {
		vm := builder.DummyBasicVirtualMachine("test-vm", "dummy-ns")

		vmCtx = context.VirtualMachineContext{
			Context: goctx.WithValue(goctx.Background(), context.MaxDeployThreadsContextKey, 16),
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
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

	Context("GetVirtualMachineClass", func() {
		oldNamespacedVMClassFSSEnabledFunc := lib.IsNamespacedVMClassFSSEnabled

		When("WCP_Namespaced_VM_Class FSS is disabled", func() {
			var (
				vmClass        *vmopv1.VirtualMachineClass
				vmClassBinding *vmopv1.VirtualMachineClassBinding
			)

			BeforeEach(func() {
				vmClass, vmClassBinding = builder.DummyVirtualMachineClassAndBinding("dummy-vm-class", vmCtx.VM.Namespace)
				vmCtx.VM.Spec.ClassName = vmClass.Name

				lib.IsNamespacedVMClassFSSEnabled = func() bool {
					return false
				}
			})

			AfterEach(func() {
				lib.IsNamespacedVMClassFSSEnabled = oldNamespacedVMClassFSSEnabledFunc
			})

			Context("VirtualMachineClass custom resource doesn't exist", func() {
				It("Returns error and sets condition when VM Class does not exist", func() {
					expectedErrMsg := fmt.Sprintf("Failed to get VirtualMachineClass: %s", vmCtx.VM.Spec.ClassName)

					_, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := vmopv1.Conditions{
						*conditions.FalseCondition(
							vmopv1.VirtualMachinePrereqReadyCondition,
							vmopv1.VirtualMachineClassNotFoundReason,
							vmopv1.ConditionSeverityError,
							expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			validateNoVMClassBindingCondition := func(vm *vmopv1.VirtualMachine) {
				msg := fmt.Sprintf("Namespace %s does not have access to VirtualMachineClass %s", vm.Namespace, vm.Spec.ClassName)

				expectedCondition := vmopv1.Conditions{
					*conditions.FalseCondition(
						vmopv1.VirtualMachinePrereqReadyCondition,
						vmopv1.VirtualMachineClassBindingNotFoundReason,
						vmopv1.ConditionSeverityError,
						msg),
				}
				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			}

			Context("VirtualMachineClass custom resource exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClass)
				})

				Context("No VirtualMachineClassBinding exists in namespace", func() {
					It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
						expectedErr := fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vmCtx.VM.Spec.ClassName, vmCtx.VM.Namespace)

						_, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(expectedErr))
						validateNoVMClassBindingCondition(vmCtx.VM)
					})
				})

				Context("VirtualMachineBinding is not present for VM Class", func() {
					BeforeEach(func() {
						vmClassBinding.ClassRef.Name = "blah-blah-binding"
						initObjects = append(initObjects, vmClassBinding)
					})

					It("returns an error and sets the VirtualMachinePrereqReady Condition to false", func() {
						expectedErr := fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vmCtx.VM.Spec.ClassName, vmCtx.VM.Namespace)

						_, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(expectedErr))
						validateNoVMClassBindingCondition(vmCtx.VM)
					})
				})

				Context("VirtualMachineBinding is present for VM Class", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, vmClassBinding)
					})

					It("returns success", func() {
						class, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(class).ToNot(BeNil())
					})
				})
			})
		})

		When("WCP_Namespaced_VM_Class FSS is enabled", func() {
			var (
				vmClass *vmopv1.VirtualMachineClass
			)

			BeforeEach(func() {
				vmClass = builder.DummyVirtualMachineClass()
				vmClass.Namespace = vmCtx.VM.Namespace
				vmCtx.VM.Spec.ClassName = vmClass.Name

				lib.IsNamespacedVMClassFSSEnabled = func() bool {
					return true
				}
			})

			AfterEach(func() {
				lib.IsNamespacedVMClassFSSEnabled = oldNamespacedVMClassFSSEnabledFunc
			})

			Context("VirtualMachineClass custom resource doesn't exist", func() {
				It("Returns error and sets condition when VM Class does not exist", func() {
					expectedErrMsg := fmt.Sprintf("Failed to get VirtualMachineClass: %s", vmCtx.VM.Spec.ClassName)

					_, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := vmopv1.Conditions{
						*conditions.FalseCondition(
							vmopv1.VirtualMachinePrereqReadyCondition,
							vmopv1.VirtualMachineClassNotFoundReason,
							vmopv1.ConditionSeverityError,
							expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			Context("VirtualMachineClass custom resource exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClass)
				})

				It("returns success", func() {
					class, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(class).ToNot(BeNil())
				})
			})
		})

	})

	Context("GetVMImageAndContentLibraryUUID", func() {

		When("WCPVMImageRegistry FSS is disabled", func() {

			var (
				contentSource        *vmopv1.ContentSource
				clProvider           *vmopv1.ContentLibraryProvider
				contentSourceBinding *vmopv1.ContentSourceBinding
				vmImage              *vmopv1.VirtualMachineImage
			)

			BeforeEach(func() {
				contentSource, clProvider, contentSourceBinding = builder.DummyContentSourceProviderAndBinding("dummy-cl-uuid", vmCtx.VM.Namespace)
				vmImage = &vmopv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-image",
						OwnerReferences: []metav1.OwnerReference{{
							Name: clProvider.Name,
							Kind: "ContentLibraryProvider",
						}},
					},
				}

				vmCtx.VM.Spec.ImageName = vmImage.Name

				lib.IsWCPVMImageRegistryEnabled = func() bool {
					return false
				}
			})

			When("VirtualMachineImage does not exist", func() {
				It("returns error and sets condition", func() {
					expectedErrMsg := fmt.Sprintf("Failed to get VirtualMachineImage: %s", vmCtx.VM.Spec.ImageName)

					_, _, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := vmopv1.Conditions{
						*conditions.FalseCondition(
							vmopv1.VirtualMachinePrereqReadyCondition,
							vmopv1.VirtualMachineImageNotFoundReason,
							vmopv1.ConditionSeverityError,
							expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("ContentLibraryProvider does not exist", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmImage)
				})

				It("returns error and sets condition", func() {
					expectedErrMsg := fmt.Sprintf("Failed to get ContentLibraryProvider: %s", clProvider.Name)

					_, _, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := vmopv1.Conditions{
						*conditions.FalseCondition(
							vmopv1.VirtualMachinePrereqReadyCondition,
							vmopv1.ContentLibraryProviderNotFoundReason,
							vmopv1.ConditionSeverityError,
							expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			validateNoContentSourceBindingCondition := func(vm *vmopv1.VirtualMachine, clUUID string) {
				msg := fmt.Sprintf("Namespace %s does not have access to ContentSource %s for VirtualMachineImage %s",
					vm.Namespace, clUUID, vm.Spec.ImageName)

				expectedCondition := vmopv1.Conditions{
					*conditions.FalseCondition(
						vmopv1.VirtualMachinePrereqReadyCondition,
						vmopv1.ContentSourceBindingNotFoundReason,
						vmopv1.ConditionSeverityError,
						msg),
				}

				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			}

			Context("VirtualMachineImage and ContentLibraryProvider exist", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, clProvider, vmImage)
				})

				When("No ContentSourceBindings exist in the namespace", func() {
					It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
						expectedErrMsg := fmt.Sprintf("Namespace %s does not have access to ContentSource %s for VirtualMachineImage %s",
							vmCtx.VM.Namespace, clProvider.Spec.UUID, vmCtx.VM.Spec.ImageName)

						_, _, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

						validateNoContentSourceBindingCondition(vmCtx.VM, clProvider.Spec.UUID)
					})
				})

				When("ContentSourceBinding is not present for the content library corresponding to the VM image", func() {
					BeforeEach(func() {
						contentSourceBinding.ContentSourceRef.Name = "blah-blah-binding"
						initObjects = append(initObjects, contentSourceBinding)
					})

					It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
						expectedErrMsg := fmt.Sprintf("Namespace %s does not have access to ContentSource %s for VirtualMachineImage %s",
							vmCtx.VM.Namespace, clProvider.Spec.UUID, vmCtx.VM.Spec.ImageName)

						_, _, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

						validateNoContentSourceBindingCondition(vmCtx.VM, clProvider.Spec.UUID)
					})
				})

				When("ContentSourceBinding present for ContentSource", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, contentSource, contentSourceBinding)
					})

					It("returns success", func() {
						_, image, uuid, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(image).ToNot(BeNil())
						Expect(uuid).ToNot(BeEmpty())
						Expect(uuid).To(Equal(clProvider.Spec.UUID))
					})
				})
			})
		})

		When("WCPVMImageRegistry FSS is enabled", func() {

			var (
				cl             *imgregv1a1.ContentLibrary
				nsVMImage      *vmopv1.VirtualMachineImage
				clusterCL      *imgregv1a1.ClusterContentLibrary
				clusterVMImage *vmopv1.ClusterVirtualMachineImage
			)

			BeforeEach(func() {
				cl = builder.DummyContentLibrary("dummy-cl", vmCtx.VM.Namespace, "dummy-cl-uuid")
				nsVMImage = builder.DummyVirtualMachineImage("dummy-ns-vm-image")
				nsVMImage.Namespace = vmCtx.VM.Namespace
				nsVMImage.Status.ContentLibraryRef = &corev1.TypedLocalObjectReference{
					Kind: "ContentLibrary",
					Name: cl.Name,
				}
				clusterCL = builder.DummyClusterContentLibrary("dummy-cluster-cl", "dummy-ccl-uuid")
				clusterVMImage = builder.DummyClusterVirtualMachineImage("dummy-cluster-vm-image")
				clusterVMImage.Status.ContentLibraryRef = &corev1.TypedLocalObjectReference{
					Kind: "ClusterContentLibrary",
					Name: clusterCL.Name,
				}

				lib.IsWCPVMImageRegistryEnabled = func() bool {
					return true
				}
			})

			When("Neither cluster or namespace scoped VM image exists", func() {

				It("returns error and sets condition", func() {
					_, _, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					expectedErrMsg := fmt.Sprintf("Failed to get the VM's image: %s", vmCtx.VM.Spec.ImageName)
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := vmopv1.Conditions{
						*conditions.FalseCondition(
							vmopv1.VirtualMachinePrereqReadyCondition,
							vmopv1.VirtualMachineImageNotFoundReason,
							vmopv1.ConditionSeverityError,
							expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("VM image exists but the provider is not ready", func() {

				BeforeEach(func() {
					conditions.MarkFalse(nsVMImage, vmopv1.VirtualMachineImageProviderReadyCondition, "", "", "")
					// Set other conditions true to verify the specific message in the MatchConditions function.
					conditions.MarkTrue(nsVMImage, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
					conditions.MarkTrue(nsVMImage, vmopv1.VirtualMachineImageSyncedCondition)
					initObjects = append(initObjects, cl, nsVMImage)
					vmCtx.VM.Spec.ImageName = nsVMImage.Name
				})

				It("returns error and sets VM condition", func() {
					_, _, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					expectedErrMsg := fmt.Sprintf("VM's image provider is not ready: %s", vmCtx.VM.Spec.ImageName)
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := vmopv1.Conditions{
						*conditions.FalseCondition(
							vmopv1.VirtualMachinePrereqReadyCondition,
							vmopv1.VirtualMachineImageNotReadyReason,
							vmopv1.ConditionSeverityError,
							expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("VM image exists but the provider is not security complaint", func() {

				BeforeEach(func() {
					conditions.MarkFalse(nsVMImage, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition, "", "", "")
					// Set other conditions true to verify the specific message in the MatchConditions function.
					conditions.MarkTrue(nsVMImage, vmopv1.VirtualMachineImageProviderReadyCondition)
					conditions.MarkTrue(nsVMImage, vmopv1.VirtualMachineImageSyncedCondition)
					initObjects = append(initObjects, cl, nsVMImage)
					vmCtx.VM.Spec.ImageName = nsVMImage.Name
				})

				It("returns error and sets VM condition", func() {
					_, _, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					expectedErrMsg := fmt.Sprintf("VM's image provider is not security compliant: %s", vmCtx.VM.Spec.ImageName)
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := vmopv1.Conditions{
						*conditions.FalseCondition(
							vmopv1.VirtualMachinePrereqReadyCondition,
							vmopv1.VirtualMachineImageNotReadyReason,
							vmopv1.ConditionSeverityError,
							expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("VM image exists but the content is not synced", func() {

				BeforeEach(func() {
					// Switch to cluster scoped VM image as we used namespace scoped VM image in the previous test.
					// So that we don't have to write additional test cases to cover both namespace and cluster images.
					conditions.MarkFalse(clusterVMImage, vmopv1.VirtualMachineImageSyncedCondition, "", "", "")
					// Set other conditions true to verify the specific message in the MatchConditions function.
					conditions.MarkTrue(clusterVMImage, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
					conditions.MarkTrue(clusterVMImage, vmopv1.VirtualMachineImageProviderReadyCondition)
					initObjects = append(initObjects, clusterCL, clusterVMImage)
					vmCtx.VM.Spec.ImageName = clusterVMImage.Name
				})

				It("returns error and sets VM condition", func() {
					_, _, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					expectedErrMsg := fmt.Sprintf("VM's image content version is not synced: %s", vmCtx.VM.Spec.ImageName)
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := vmopv1.Conditions{
						*conditions.FalseCondition(
							vmopv1.VirtualMachinePrereqReadyCondition,
							vmopv1.VirtualMachineImageNotReadyReason,
							vmopv1.ConditionSeverityError,
							expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("Namespace scoped VirtualMachineImage exists and ready", func() {
				BeforeEach(func() {
					conditions.MarkTrue(nsVMImage, vmopv1.VirtualMachineImageProviderReadyCondition)
					conditions.MarkTrue(nsVMImage, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
					conditions.MarkTrue(nsVMImage, vmopv1.VirtualMachineImageSyncedCondition)
					initObjects = append(initObjects, cl, nsVMImage)
					vmCtx.VM.Spec.ImageName = nsVMImage.Name
				})

				It("returns success", func() {
					_, imageStatus, uuid, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(imageStatus).ToNot(BeNil())
					Expect(uuid).To(Equal(string(cl.Spec.UUID)))
				})
			})

			When("ClusterVirtualMachineImage exists and ready", func() {
				BeforeEach(func() {
					conditions.MarkTrue(clusterVMImage, vmopv1.VirtualMachineImageProviderReadyCondition)
					conditions.MarkTrue(clusterVMImage, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
					conditions.MarkTrue(clusterVMImage, vmopv1.VirtualMachineImageSyncedCondition)
					initObjects = append(initObjects, clusterCL, clusterVMImage)
					vmCtx.VM.Spec.ImageName = clusterVMImage.Name
				})

				It("returns success", func() {
					_, imageStatus, uuid, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(imageStatus).ToNot(BeNil())
					Expect(uuid).To(Equal(string(clusterCL.Spec.UUID)))
				})
			})
		})
	})

	Context("GetVMMetadata", func() {

		var (
			vmMetaDataConfigMap *corev1.ConfigMap
			vmMetaDataSecret    *corev1.Secret
		)

		BeforeEach(func() {
			vmMetaDataConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm-metadata",
					Namespace: vmCtx.VM.Namespace,
				},
				Data: map[string]string{
					"foo": "bar",
				},
			}

			vmMetaDataSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm-metadata",
					Namespace: vmCtx.VM.Namespace,
				},
				Data: map[string][]byte{
					"foo": []byte("bar"),
				},
			}
		})

		When("both ConfigMap and Secret are specified", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
					ConfigMapName: vmMetaDataConfigMap.Name,
					SecretName:    vmMetaDataSecret.Name,
					Transport:     "transport",
				}
			})

			It("returns an error", func() {
				_, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid VM Metadata"))
			})
		})

		When("neither ConfigMap nor Secret is specified", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
					Transport: vmopv1.VirtualMachineMetadataCloudInitTransport,
				}
			})
			It("returns metadata transport", func() {
				md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(md.Transport).To(Equal(vmopv1.VirtualMachineMetadataCloudInitTransport))
			})
		})

		When("VM Metadata is specified via a ConfigMap", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
					ConfigMapName: vmMetaDataConfigMap.Name,
					Transport:     vmopv1.VirtualMachineMetadataCloudInitTransport,
				}
			})

			It("return an error when ConfigMap does not exist", func() {
				md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(md.Data).To(BeEmpty())
				Expect(md.Transport).To(Equal(vmopv1.VirtualMachineMetadataCloudInitTransport))
			})

			When("ConfigMap exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmMetaDataConfigMap)
				})

				It("returns success", func() {
					md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(md.Data).To(Equal(vmMetaDataConfigMap.Data))
					Expect(md.Transport).To(Equal(vmopv1.VirtualMachineMetadataCloudInitTransport))
				})
			})
		})

		When("VM Metadata is specified via a Secret", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
					SecretName: vmMetaDataSecret.Name,
					Transport:  vmopv1.VirtualMachineMetadataCloudInitTransport,
				}
			})

			It("returns an error when Secret does not exist", func() {
				md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(md.Data).To(BeEmpty())
				Expect(md.Transport).To(Equal(vmopv1.VirtualMachineMetadataCloudInitTransport))
			})

			When("Secret exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmMetaDataSecret)
				})

				It("returns success", func() {
					md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(md.Data).ToNot(BeEmpty())
					Expect(md.Transport).To(Equal(vmopv1.VirtualMachineMetadataCloudInitTransport))
				})
			})
		})
	})

	Context("GetVMSetResourcePolicy", func() {

		var (
			vmResourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
		)

		BeforeEach(func() {
			vmResourcePolicy = &vmopv1.VirtualMachineSetResourcePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm-rp",
					Namespace: vmCtx.VM.Namespace,
				},
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
					ResourcePool: vmopv1.ResourcePoolSpec{Name: "fooRP"},
					Folder:       vmopv1.FolderSpec{Name: "fooFolder"},
				},
			}
		})

		It("returns success when VM does not have SetResourcePolicy", func() {
			vmCtx.VM.Spec.ResourcePolicyName = ""
			rp, err := vsphere.GetVMSetResourcePolicy(vmCtx, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(rp).To(BeNil())
		})

		It("VM SetResourcePolicy does not exist", func() {
			vmCtx.VM.Spec.ResourcePolicyName = "bogus"
			rp, err := vsphere.GetVMSetResourcePolicy(vmCtx, k8sClient)
			Expect(err).To(HaveOccurred())
			Expect(rp).To(BeNil())
		})

		When("VM SetResourcePolicy exists", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vmResourcePolicy)
				vmCtx.VM.Spec.ResourcePolicyName = vmResourcePolicy.Name
			})

			It("returns success", func() {
				rp, err := vsphere.GetVMSetResourcePolicy(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(rp).ToNot(BeNil())
			})
		})
	})

	Context("AddInstanceStorageVolumes", func() {

		var (
			vmClass            *vmopv1.VirtualMachineClass
			instanceStorageFSS uint32
		)

		expectInstanceStorageVolumes := func(
			vm *vmopv1.VirtualMachine,
			isStorage vmopv1.InstanceStorage) {

			ExpectWithOffset(1, isStorage.Volumes).ToNot(BeEmpty())
			isVolumes := instancestorage.FilterVolumes(vm)
			ExpectWithOffset(1, isVolumes).To(HaveLen(len(isStorage.Volumes)))

			for _, isVol := range isStorage.Volumes {
				found := false

				for idx, vol := range isVolumes {
					claim := vol.PersistentVolumeClaim.InstanceVolumeClaim
					if claim.StorageClass == isStorage.StorageClass && claim.Size == isVol.Size {
						isVolumes = append(isVolumes[:idx], isVolumes[idx+1:]...)
						found = true
						break
					}
				}

				ExpectWithOffset(1, found).To(BeTrue(), "failed to find instance storage volume for %v", isVol)
			}
		}

		BeforeEach(func() {
			lib.IsInstanceStorageFSSEnabled = func() bool {
				return atomic.LoadUint32(&instanceStorageFSS) != 0
			}

			vmClass = builder.DummyVirtualMachineClass()
		})

		AfterEach(func() {
			atomic.StoreUint32(&instanceStorageFSS, 0)
		})

		It("Instance Storage FSS is disabled", func() {
			atomic.StoreUint32(&instanceStorageFSS, 0)

			err := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
			Expect(err).ToNot(HaveOccurred())
			Expect(instancestorage.FilterVolumes(vmCtx.VM)).To(BeEmpty())
		})

		When("InstanceStorage FFS is enabled", func() {
			BeforeEach(func() {
				atomic.StoreUint32(&instanceStorageFSS, 1)
			})

			It("VM Class does not contain instance storage volumes", func() {
				err := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
				Expect(err).ToNot(HaveOccurred())
				Expect(instancestorage.FilterVolumes(vmCtx.VM)).To(BeEmpty())
			})

			When("Instance Volume is added in VM Class", func() {
				BeforeEach(func() {
					vmClass.Spec.Hardware.InstanceStorage = builder.DummyInstanceStorage()
				})

				It("Instance Volumes should be added", func() {
					err := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(err).ToNot(HaveOccurred())
					expectInstanceStorageVolumes(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)
				})

				It("Instance Storage is already added to VM Spec.Volumes", func() {
					err := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(err).ToNot(HaveOccurred())

					isVolumesBefore := instancestorage.FilterVolumes(vmCtx.VM)
					expectInstanceStorageVolumes(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)

					// Instance Storage is already configured, should not patch again
					err = vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(err).ToNot(HaveOccurred())
					isVolumesAfter := instancestorage.FilterVolumes(vmCtx.VM)
					Expect(isVolumesAfter).To(Equal(isVolumesBefore))
				})
			})
		})
	})

	Context("hasPVC", func() {
		var spec vmopv1.VirtualMachineSpec
		Context("Spec has no PVC", func() {
			It("will return false", func() {
				spec = vmopv1.VirtualMachineSpec{}
				Expect(vsphere.HasPVC(spec)).To(BeFalse())
			})
		})
		Context("Spec has PVCs", func() {
			It("will return true", func() {
				spec = vmopv1.VirtualMachineSpec{
					Volumes: []vmopv1.VirtualMachineVolume{
						{
							Name: "dummy-vol",
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-claim-1",
								},
							},
						},
					},
				}
				Expect(vsphere.HasPVC(spec)).To(BeTrue())
			})
		})
	})

	Context("HardwareVersionForPVCandPCIDevices", func() {
		var (
			configSpec     *types.VirtualMachineConfigSpec
			imageHWVersion int32
		)

		BeforeEach(func() {
			imageHWVersion = 14
			configSpec = &types.VirtualMachineConfigSpec{
				Name: "dummy-VM",
				DeviceChange: []types.BaseVirtualDeviceConfigSpec{
					&types.VirtualDeviceConfigSpec{
						Operation: types.VirtualDeviceConfigSpecOperationAdd,
						Device: &types.VirtualPCIPassthrough{
							VirtualDevice: types.VirtualDevice{
								Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu: "profile-from-configspec",
								},
							},
						},
					},
					&types.VirtualDeviceConfigSpec{
						Operation: types.VirtualDeviceConfigSpecOperationAdd,
						Device: &types.VirtualPCIPassthrough{
							VirtualDevice: types.VirtualDevice{
								Backing: &types.VirtualPCIPassthroughDynamicBackingInfo{
									AllowedDevice: []types.VirtualPCIPassthroughAllowedDevice{
										{
											VendorId: 52,
											DeviceId: 53,
										},
									},
									CustomLabel: "label-from-configspec",
								},
							},
						},
					},
				},
			}
		})

		It("ConfigSpec has PCI devices and VM spec has PVCs", func() {
			Expect(vsphere.HardwareVersionForPVCandPCIDevices(imageHWVersion, configSpec, true)).To(Equal(int32(17)))
		})

		It("ConfigSpec has PCI devices and VM spec has no PVCs", func() {
			Expect(vsphere.HardwareVersionForPVCandPCIDevices(imageHWVersion, configSpec, false)).To(Equal(int32(17)))
		})

		It("ConfigSpec has PCI devices, VM spec has PVCs image hardware version is higher than min supported HW version for PCI devices", func() {
			imageHWVersion = 18
			Expect(vsphere.HardwareVersionForPVCandPCIDevices(imageHWVersion, configSpec, true)).To(Equal(int32(18)))
		})

		It("VM spec has PVCs and config spec has no devices", func() {
			configSpec = &types.VirtualMachineConfigSpec{}
			Expect(vsphere.HardwareVersionForPVCandPCIDevices(imageHWVersion, configSpec, true)).To(Equal(int32(15)))
		})

		It("VM spec has PVCs, config spec has no devices and image hardware version is higher than min supported PVC HW version", func() {
			configSpec = &types.VirtualMachineConfigSpec{}
			imageHWVersion = 16
			Expect(vsphere.HardwareVersionForPVCandPCIDevices(imageHWVersion, configSpec, true)).To(Equal(int32(16)))
		})
	})
}
