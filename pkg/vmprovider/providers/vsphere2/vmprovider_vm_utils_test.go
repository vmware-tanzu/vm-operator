// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	goctx "context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	vsphere "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/instancestorage"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmUtilTests() {

	var (
		k8sClient   client.Client
		initObjects []client.Object

		vmCtx context.VirtualMachineContextA2
	)

	BeforeEach(func() {
		vm := builder.DummyBasicVirtualMachineA2("test-vm", "dummy-ns")

		vmCtx = context.VirtualMachineContextA2{
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

		// NOTE: As we currently have it, v1a2 must have this enabled.
		When("WCP_Namespaced_VM_Class FSS is enabled", func() {
			var (
				vmClass *vmopv1.VirtualMachineClass
			)

			BeforeEach(func() {
				vmClass = builder.DummyVirtualMachineClass2A2("dummy-vm-class")
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
					expectedErrMsg := fmt.Sprintf("virtualmachineclasses.vmoperator.vmware.com %q not found", vmCtx.VM.Spec.ClassName)

					_, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := []metav1.Condition{
						*conditions.FalseCondition(vmopv1.VirtualMachineConditionClassReady, "NotFound", expectedErrMsg),
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

	Context("GetVirtualMachineImageSpecAndStatus", func() {

		// NOTE: As we currently have it, v1a2 must have this enabled.
		When("WCPVMImageRegistry FSS is enabled", func() {

			var (
				nsVMImage      *vmopv1.VirtualMachineImage
				clusterVMImage *vmopv1.ClusterVirtualMachineImage
			)

			BeforeEach(func() {
				nsVMImage = builder.DummyVirtualMachineImageA2("dummy-ns-vm-image")
				nsVMImage.Namespace = vmCtx.VM.Namespace
				conditions.MarkTrue(nsVMImage, vmopv1.ReadyConditionType)
				clusterVMImage = builder.DummyClusterVirtualMachineImageA2("dummy-cluster-vm-image")
				conditions.MarkTrue(clusterVMImage, vmopv1.ReadyConditionType)

				lib.IsWCPVMImageRegistryEnabled = func() bool {
					return true
				}
			})

			When("Neither cluster or namespace scoped VM image exists", func() {

				It("returns error and sets condition", func() {
					_, _, _, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					expectedErrMsg := fmt.Sprintf("Failed to get the VM's image: %s", vmCtx.VM.Spec.ImageName)
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := []metav1.Condition{
						*conditions.FalseCondition(vmopv1.VirtualMachineConditionImageReady, "NotFound", expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("VM image exists but the image is not ready", func() {

				const expectedErrMsg = "VirtualMachineImage is not ready"

				Context("VM image has the Ready condition set to False", func() {
					reason := vmopv1.VirtualMachineImageProviderNotReadyReason
					errMsg := "Provider item is not in ready condition"

					BeforeEach(func() {
						conditions.MarkFalse(nsVMImage,
							vmopv1.ReadyConditionType,
							reason,
							errMsg)
						initObjects = append(initObjects, nsVMImage)
						vmCtx.VM.Spec.ImageName = nsVMImage.Name
					})

					It("returns error and sets VM condition with reason and message from the image", func() {
						_, _, _, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())

						Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

						expectedCondition := []metav1.Condition{
							*conditions.FalseCondition(
								vmopv1.VirtualMachineConditionImageReady, reason, errMsg),
						}
						Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
					})
				})

				Context("VM image does not have the Ready condition", func() {
					reason := "NotReady"

					BeforeEach(func() {
						conditions.Delete(nsVMImage, vmopv1.ReadyConditionType)
						initObjects = append(initObjects, nsVMImage)
						vmCtx.VM.Spec.ImageName = nsVMImage.Name
					})

					It("returns error and sets VM condition with reason and message from the image", func() {
						_, _, _, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

						expectedCondition := []metav1.Condition{
							*conditions.FalseCondition(
								vmopv1.VirtualMachineConditionImageReady, reason, expectedErrMsg),
						}
						Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
					})
				})

			})

			When("Namespace scoped VirtualMachineImage exists and ready", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, nsVMImage)
					vmCtx.VM.Spec.ImageName = nsVMImage.Name
				})

				It("returns success", func() {
					imgObj, spec, status, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(imgObj).ToNot(BeNil())
					Expect(imgObj.GetObjectKind().GroupVersionKind().Kind).To(Equal("VirtualMachineImage"))
					Expect(spec).ToNot(BeNil())
					Expect(status).ToNot(BeNil())
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
				})
			})

			When("ClusterVirtualMachineImage exists and ready", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, clusterVMImage)
					vmCtx.VM.Spec.ImageName = clusterVMImage.Name
				})

				It("returns success", func() {
					imgObj, spec, status, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(imgObj).ToNot(BeNil())
					Expect(imgObj.GetObjectKind().GroupVersionKind().Kind).To(Equal("ClusterVirtualMachineImage"))
					Expect(spec).ToNot(BeNil())
					Expect(status).ToNot(BeNil())
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
				})
			})
		})
	})

	Context("GetVirtualMachineBootstrap", func() {
		const dataName = "dummy-vm-bootstrap-data"
		const vAppDataName = "dummy-vm-bootstrap-vapp-data"

		var (
			bootstrapCM     *corev1.ConfigMap
			bootstrapSecret *corev1.Secret
			bootstrapVAppCM *corev1.ConfigMap
		)

		BeforeEach(func() {
			bootstrapCM = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataName,
					Namespace: vmCtx.VM.Namespace,
				},
				Data: map[string]string{
					"foo": "bar",
				},
			}

			bootstrapSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataName,
					Namespace: vmCtx.VM.Namespace,
				},
				Data: map[string][]byte{
					"foo1": []byte("bar1"),
				},
			}

			bootstrapVAppCM = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vAppDataName,
					Namespace: vmCtx.VM.Namespace,
				},
				Data: map[string]string{
					"foo-vapp": "bar-vapp",
				},
			}
		})

		When("Bootstrap via CloudInit", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				}
				vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig = &corev1.SecretKeySelector{}
				vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Name = dataName
			})

			It("return an error when resources does not exist", func() {
				_, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
			})

			When("ConfigMap exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM)
				})

				It("returns success", func() {
					data, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(data).To(HaveKeyWithValue("foo", "bar"))
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
				})
			})

			When("Secret exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM, bootstrapSecret)
				})

				When("Prefers Secret over ConfigMap", func() {
					It("returns success", func() {
						data, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						// Prefer Secret over ConfigMap.
						Expect(data).To(HaveKeyWithValue("foo1", "bar1"))
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					})
				})
			})

			When("Optional is set", func() {
				Context("Secret does not exist", func() {
					Context("Optional is true", func() {
						BeforeEach(func() {
							vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Optional = pointer.Bool(true)
						})

						It("returns success", func() {
							data, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
							Expect(err).ToNot(HaveOccurred())
							Expect(data).To(BeEmpty())
							Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
						})
					})

					Context("Optional is false", func() {
						BeforeEach(func() {
							vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Optional = pointer.Bool(false)
						})

						It("returns error", func() {
							_, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(Equal(`secrets "dummy-vm-bootstrap-data" not found`))
							Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
						})
					})
				})

				Context("Key in Secret does not exist", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, bootstrapSecret)

						vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Key = "secret-key-that-does-not-exist"
					})

					Context("Optional is true", func() {
						BeforeEach(func() {
							vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Optional = pointer.Bool(true)
						})

						It("returns success", func() {
							data, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
							Expect(err).ToNot(HaveOccurred())
							Expect(data).ToNot(BeEmpty())
							Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
						})
					})

					Context("Optional is false", func() {
						BeforeEach(func() {
							vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Optional = pointer.Bool(false)
						})

						It("returns an error", func() {
							_, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(Equal(`required key "secret-key-that-does-not-exist" not found in Secret dummy-vm-bootstrap-data`))
							Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
						})
					})

				})
			})
		})

		When("Bootstrap via Sysprep", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
				}
				vmCtx.VM.Spec.Bootstrap.Sysprep.RawSysprep = &corev1.SecretKeySelector{}
				vmCtx.VM.Spec.Bootstrap.Sysprep.RawSysprep.Name = dataName
			})

			It("return an error when resource does not exist", func() {
				_, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
			})

			When("ConfigMap exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM)
				})

				It("returns success", func() {
					data, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(data).To(HaveKeyWithValue("foo", "bar"))
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
				})
			})

			When("Secret exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM, bootstrapSecret)
				})

				When("Prefers Secret over ConfigMap", func() {
					It("returns success", func() {
						data, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(HaveKeyWithValue("foo1", "bar1"))
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					})
				})
			})
		})

		When("Bootstrap with vAppConfig", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
				}
				vmCtx.VM.Spec.Bootstrap.VAppConfig.RawProperties = vAppDataName
			})

			It("return an error when resource does not exist", func() {
				_, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
			})

			When("ConfigMap exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapVAppCM)
				})

				It("returns success", func() {
					_, data, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(data).To(HaveKeyWithValue("foo-vapp", "bar-vapp"))
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
				})
			})

			When("vAppConfig with properties", func() {
				BeforeEach(func() {
					vmCtx.VM.Spec.Bootstrap.VAppConfig.Properties = []common.KeyValueOrSecretKeySelectorPair{
						{
							Value: common.ValueOrSecretKeySelector{
								From: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: vAppDataName,
									},
									Key: "foo-vapp",
								},
							},
						},
					}

					It("returns success", func() {
						_, _, exData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(exData).To(HaveKey(vAppDataName))
						data := exData[vAppDataName]
						Expect(data).To(HaveKeyWithValue("foo-vapp", "bar-vapp"))
					})

					Context("Optional is set", func() {
						Context("Secret does not exist", func() {
							BeforeEach(func() {
								vmCtx.VM.Spec.Bootstrap.VAppConfig.Properties[0].Value.From.Name = "secret-does-exist"
								vmCtx.VM.Spec.Bootstrap.VAppConfig.Properties[0].Value.From.Optional = pointer.Bool(true)
							})

							It("returns success when optional is true", func() {
								_, _, exData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
								Expect(err).ToNot(HaveOccurred())
								Expect(exData).To(BeEmpty())
								Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
							})
						})

						Context("Key in Secret does not exist", func() {
							BeforeEach(func() {
								vmCtx.VM.Spec.Bootstrap.VAppConfig.Properties[0].Value.From.Key = "bogus-vapp-prop-key"
								vmCtx.VM.Spec.Bootstrap.VAppConfig.Properties[0].Value.From.Optional = pointer.Bool(true)
							})

							It("returns error when Optional is false", func() {
								_, _, _, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
								Expect(err).To(HaveOccurred())
								Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
							})
						})
					})
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
					Folder:       "fooFolder",
				},
			}
		})

		It("returns success when VM does not have SetResourcePolicy", func() {
			if vmCtx.VM.Spec.Reserved == nil {
				vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
			}
			vmCtx.VM.Spec.Reserved.ResourcePolicyName = ""
			rp, err := vsphere.GetVMSetResourcePolicy(vmCtx, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(rp).To(BeNil())
		})

		It("VM SetResourcePolicy does not exist", func() {
			if vmCtx.VM.Spec.Reserved == nil {
				vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
			}
			vmCtx.VM.Spec.Reserved.ResourcePolicyName = "bogus"
			rp, err := vsphere.GetVMSetResourcePolicy(vmCtx, k8sClient)
			Expect(err).To(HaveOccurred())
			Expect(rp).To(BeNil())
		})

		When("VM SetResourcePolicy exists", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vmResourcePolicy)
				if vmCtx.VM.Spec.Reserved == nil {
					vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
				}
				vmCtx.VM.Spec.Reserved.ResourcePolicyName = vmResourcePolicy.Name
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
			vmClass *vmopv1.VirtualMachineClass
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
			vmClass = builder.DummyVirtualMachineClassA2()
		})

		When("InstanceStorage FFS is enabled", func() {

			It("VM Class does not contain instance storage volumes", func() {
				is := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
				Expect(is).To(BeFalse())
				Expect(instancestorage.FilterVolumes(vmCtx.VM)).To(BeEmpty())
			})

			When("Instance Volume is added in VM Class", func() {
				BeforeEach(func() {
					vmClass.Spec.Hardware.InstanceStorage = builder.DummyInstanceStorageA2()
				})

				It("Instance Volumes should be added", func() {
					is := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(is).To(BeTrue())
					expectInstanceStorageVolumes(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)
				})

				It("Instance Storage is already added to VM Spec.Volumes", func() {
					is := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(is).To(BeTrue())

					isVolumesBefore := instancestorage.FilterVolumes(vmCtx.VM)
					expectInstanceStorageVolumes(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)

					// Instance Storage is already configured, should not patch again
					is = vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(is).To(BeTrue())
					isVolumesAfter := instancestorage.FilterVolumes(vmCtx.VM)
					Expect(isVolumesAfter).To(HaveLen(len(isVolumesBefore)))
					Expect(isVolumesAfter).To(Equal(isVolumesBefore))
				})
			})
		})
	})

	Context("HasPVC", func() {

		Context("Spec has no PVC", func() {
			It("will return false", func() {
				spec := vmopv1.VirtualMachineSpec{}
				Expect(vsphere.HasPVC(spec)).To(BeFalse())
			})
		})

		Context("Spec has PVCs", func() {
			It("will return true", func() {
				spec := vmopv1.VirtualMachineSpec{
					Volumes: []vmopv1.VirtualMachineVolume{
						{
							Name: "dummy-vol",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "pvc-claim-1",
									},
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

	Context("GetAttachedDiskUuidToPVC", func() {
		const (
			attachedDiskUUID = "dummy-uuid"
		)
		var (
			attachedPVC *corev1.PersistentVolumeClaim
		)

		BeforeEach(func() {
			// Create multiple PVCs to verity only the expected one is returned.
			unusedPVC := builder.DummyPersistentVolumeClaim()
			unusedPVC.Name = "unused-pvc"
			unusedPVC.Namespace = vmCtx.VM.Namespace
			unattachedPVC := builder.DummyPersistentVolumeClaim()
			unattachedPVC.Name = "unattached-pvc"
			unattachedPVC.Namespace = vmCtx.VM.Namespace
			attachedPVC = builder.DummyPersistentVolumeClaim()
			attachedPVC.Name = "attached-pvc"
			attachedPVC.Namespace = vmCtx.VM.Namespace
			initObjects = append(initObjects, unusedPVC, unattachedPVC, attachedPVC)

			vmCtx.VM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "unattached-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: unattachedPVC.Name,
							},
						},
					},
				},
				{
					Name: "attached-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: attachedPVC.Name,
							},
						},
					},
				},
			}

			vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:     "unattached-vol",
					Attached: false,
					DiskUUID: "unattached-disk-uuid",
				},
				{
					Name:     "attached-vol",
					Attached: true,
					DiskUUID: attachedDiskUUID,
				},
			}
		})

		It("Should return a map of disk uuid to PVCs that are attached to the VM", func() {
			diskUUIDToPVCMap, err := vsphere.GetAttachedDiskUUIDToPVC(vmCtx, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(diskUUIDToPVCMap).To(HaveLen(1))
			Expect(diskUUIDToPVCMap).To(HaveKey(attachedDiskUUID))
			pvc := diskUUIDToPVCMap[attachedDiskUUID]
			Expect(pvc.Name).To(Equal(attachedPVC.Name))
			Expect(pvc.Namespace).To(Equal(attachedPVC.Namespace))
		})
	})
}
