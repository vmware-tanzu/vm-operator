// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmUtilTests() {

	var (
		k8sClient   client.Client
		initObjects []client.Object

		vmCtx pkgctx.VirtualMachineContext
	)

	BeforeEach(func() {
		vm := builder.DummyBasicVirtualMachine("test-vm", "dummy-ns")

		vmCtx = pkgctx.VirtualMachineContext{
			Context: pkgcfg.WithConfig(pkgcfg.Config{MaxDeployThreadsOnProvider: 16}),
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
		var (
			vmClass *vmopv1.VirtualMachineClass
		)

		BeforeEach(func() {
			vmClass = builder.DummyVirtualMachineClass("dummy-vm-class")
			vmClass.Namespace = vmCtx.VM.Namespace
			vmCtx.VM.Spec.ClassName = vmClass.Name
		})

		assertClassExists := func() {
			obj, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, obj).ToNot(BeNil())
		}

		assertClassNotFound := func() {
			obj, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
			ExpectWithOffset(1, err).To(HaveOccurred())
			expectedErrMsg := fmt.Sprintf(
				"virtualmachineclasses.vmoperator.vmware.com %q not found",
				vmCtx.VM.Spec.ClassName)
			ExpectWithOffset(1, err).To(MatchError(expectedErrMsg))
			expectedCondition := []metav1.Condition{
				*conditions.FalseCondition(
					vmopv1.VirtualMachineConditionClassReady,
					"NotFound",
					"%s", expectedErrMsg),
			}
			ExpectWithOffset(1, vmCtx.VM.Status.Conditions).To(
				conditions.MatchConditions(expectedCondition))
			ExpectWithOffset(1, obj).To(Equal(vmopv1.VirtualMachineClass{}))
		}

		When("spec.className is valid", func() {
			When("class exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClass)
				})
				Context("FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET", func() {
					When("fss disabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
								config.Features.VMImportNewNet = false
							})
						})
						It("should return the class", func() {
							assertClassExists()
						})
					})
					When("fss enabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
								config.Features.VMImportNewNet = true
							})
						})
						It("should return the class", func() {
							assertClassExists()
						})
					})
				})
			})
			When("class does not exist", func() {
				Context("FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET", func() {
					When("fss disabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
								config.Features.VMImportNewNet = false
							})
						})
						It("should return not found", func() {
							assertClassNotFound()
						})
					})
					When("fss enabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
								config.Features.VMImportNewNet = true
							})
						})
						It("should return not found", func() {
							assertClassNotFound()
						})
					})
				})
			})
		})
		When("spec.className is empty", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.ClassName = ""
			})
			Context("FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET", func() {
				When("fss disabled", func() {
					BeforeEach(func() {
						pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
							config.Features.VMImportNewNet = false
						})
					})
					It("should return not found", func() {
						assertClassNotFound()
					})
				})
				When("fss enabled", func() {
					BeforeEach(func() {
						pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
							config.Features.VMImportNewNet = true
						})
					})
					When("underlying vm exists", func() {
						BeforeEach(func() {
							vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
								Hardware: vimtypes.VirtualHardware{
									NumCPU:   2,
									MemoryMB: 1024,
								},
							}
						})
						It("should return the class", func() {
							obj, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
							Expect(err).ToNot(HaveOccurred())
							Expect(obj).ToNot(BeNil())
							Expect(obj.Spec.Hardware.Cpus).To(Equal(int64(2)))
							Expect(obj.Spec.Hardware.Memory.String()).To(Equal("1Gi"))
							configSpec, err := util.UnmarshalConfigSpecFromJSON(obj.Spec.ConfigSpec)
							Expect(err).ToNot(HaveOccurred())
							Expect(configSpec.NumCPUs).To(Equal(int32(2)))
							Expect(configSpec.MemoryMB).To(Equal(int64(1024)))
							Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionClassReady)).To(BeFalse())
						})
					})
					When("underlying vm does not exist", func() {
						It("should return an error", func() {
							obj, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
							Expect(err).To(HaveOccurred())
							Expect(err).To(MatchError("cannot synthesize class from nil ConfigInfo"))
							Expect(obj).To(Equal(vmopv1.VirtualMachineClass{}))
						})
					})
				})
			})
		})
	})

	Context("GetVirtualMachineImageSpecAndStatus", func() {

		var (
			nsVMImage      *vmopv1.VirtualMachineImage
			clusterVMImage *vmopv1.ClusterVirtualMachineImage
		)

		BeforeEach(func() {
			nsVMImage = builder.DummyVirtualMachineImage(builder.DummyVMIName)
			nsVMImage.Namespace = vmCtx.VM.Namespace
			conditions.MarkTrue(nsVMImage, vmopv1.ReadyConditionType)
			clusterVMImage = builder.DummyClusterVirtualMachineImage(builder.DummyCVMIName)
			conditions.MarkTrue(clusterVMImage, vmopv1.ReadyConditionType)
		})

		When("spec.image is nil", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Image = nil
			})

			It("returns error and sets condition", func() {
				_, _, _, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				expectedReason := "NotSet"
				Expect(err.Error()).To(ContainSubstring(expectedReason))

				expectedCondition := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.VirtualMachineConditionImageReady, expectedReason, ""),
				}
				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			})
		})

		When("spec.image.kind is empty", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Image.Kind = ""
			})

			When("Neither cluster or namespace scoped VM image exists", func() {

				It("returns error and sets condition", func() {
					_, _, _, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					expectedErrMsg := fmt.Sprintf("no VM image exists for %q in namespace or cluster scope", vmCtx.VM.Spec.Image.Name)
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := []metav1.Condition{
						*conditions.FalseCondition(vmopv1.VirtualMachineConditionImageReady, "NotFound", "%s", expectedErrMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			When("Namespace scoped VirtualMachineImage exists and ready", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, nsVMImage)
					vmCtx.VM.Spec.Image.Name = nsVMImage.Name
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
					vmCtx.VM.Spec.Image.Kind = "ClusterVirtualMachineImage"
					vmCtx.VM.Spec.Image.Name = clusterVMImage.Name
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

		When("spec.image.kind is invalid", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Image.Kind = "invalid"
			})

			It("returns error and sets condition", func() {
				_, _, _, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				expectedReason := "UnknownKind"
				expectedErrMsg := fmt.Sprintf("%s: %s", expectedReason, vmCtx.VM.Spec.Image.Kind)
				Expect(err.Error()).To(ContainSubstring(expectedErrMsg))
				expectedCondition := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.VirtualMachineConditionImageReady, expectedReason, "%s", vmCtx.VM.Spec.Image.Kind),
				}
				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			})
		})

		When("Neither cluster or namespace scoped VM image exists", func() {

			It("returns error and sets condition", func() {
				_, _, _, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				expectedErrMsg := fmt.Sprintf("virtualmachineimages.vmoperator.vmware.com %q not found", vmCtx.VM.Spec.Image.Name)
				Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

				expectedCondition := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.VirtualMachineConditionImageReady, "NotFound", "%s", expectedErrMsg),
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
						"%s", errMsg)
					initObjects = append(initObjects, nsVMImage)
					vmCtx.VM.Spec.Image.Name = nsVMImage.Name
				})

				It("returns error and sets VM condition with reason and message from the image", func() {
					_, _, _, err := vsphere.GetVirtualMachineImageSpecAndStatus(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())

					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					expectedCondition := []metav1.Condition{
						*conditions.FalseCondition(
							vmopv1.VirtualMachineConditionImageReady, reason, "%s", errMsg),
					}
					Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
				})
			})

			Context("VM image does not have the Ready condition", func() {
				reason := "NotReady"

				BeforeEach(func() {
					conditions.Delete(nsVMImage, vmopv1.ReadyConditionType)
					initObjects = append(initObjects, nsVMImage)
					vmCtx.VM.Spec.Image.Name = nsVMImage.Name
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
				vmCtx.VM.Spec.Image.Name = nsVMImage.Name
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
				vmCtx.VM.Spec.Image.Kind = "ClusterVirtualMachineImage"
				vmCtx.VM.Spec.Image.Name = clusterVMImage.Name
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
				vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig = &common.SecretKeySelector{}
				vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Name = dataName
			})

			It("return an error when resources does not exist", func() {
				_, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
			})

			When("Utilize ConfigMap when V1alpha1ConfigMapTransportAnnotation is set", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM, bootstrapSecret)
					vmCtx.VM.Annotations = map[string]string{vmopv1.V1alpha1ConfigMapTransportAnnotation: "true"}
				})

				It("returns success", func() {
					bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(bsData.Data).To(HaveKeyWithValue("foo", "bar"))
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
				})
			})

			When("Secret exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM, bootstrapSecret)
				})

				When("Prefers Secret over ConfigMap", func() {
					It("returns success", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						// Prefer Secret over ConfigMap.
						Expect(bsData.Data).To(HaveKeyWithValue("foo1", "bar1"))
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					})
				})

				When("v1a1 compat when there is no userdata but ssh-public-keys", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Key = "user-data"
						bootstrapSecret.Data["ssh-public-keys"] = []byte("")
					})

					It("returns success", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						// Prefer Secret over ConfigMap.
						Expect(bsData.Data).To(HaveKeyWithValue("foo1", "bar1"))
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					})
				})
			})

			When("Secret key fallback for CloudInit", func() {
				const value = "should-fallback-to-this-key"

				BeforeEach(func() {
					cloudInitSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dataName,
							Namespace: vmCtx.VM.Namespace,
						},
						Data: map[string][]byte{
							"value": []byte(value),
						},
					}

					initObjects = append(initObjects, bootstrapCM, cloudInitSecret)

					vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig = &common.SecretKeySelector{}
					vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Name = dataName
					vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Key = "user-data"
				})

				It("returns success", func() {
					bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(bsData.Data).To(HaveKeyWithValue("value", value))
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
				})
			})
		})

		When("Bootstrap via RawSysprep", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
				}
				vmCtx.VM.Spec.Bootstrap.Sysprep.RawSysprep = &common.SecretKeySelector{}
				vmCtx.VM.Spec.Bootstrap.Sysprep.RawSysprep.Name = dataName
			})

			It("return an error when resource does not exist", func() {
				_, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
			})

			When("Utilize ConfigMap when V1alpha1ConfigMapTransportAnnotation is set", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM, bootstrapSecret)
					vmCtx.VM.Annotations = map[string]string{vmopv1.V1alpha1ConfigMapTransportAnnotation: "true"}

				})

				It("returns success", func() {
					bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(bsData.Data).To(HaveKeyWithValue("foo", "bar"))
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
				})
			})

			When("Secret exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM, bootstrapSecret)
				})

				When("Prefers Secret over ConfigMap", func() {
					It("returns success", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(bsData.Data).To(HaveKeyWithValue("foo1", "bar1"))
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					})
				})
			})
		})

		Context("Bootstrap via inline Sysprep", func() {
			anotherKey := "some_other_key"

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
				}
			})

			Context("for UserData", func() {
				productIDSecretName := "product_id_secret"
				BeforeEach(func() {
					vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep = &sysprep.Sysprep{
						UserData: sysprep.UserData{
							ProductID: &sysprep.ProductIDSecretKeySelector{
								Name: productIDSecretName,
								Key:  "product_id",
							},
						},
					}
				})

				When("secret is present", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      productIDSecretName,
								Namespace: vmCtx.VM.Namespace,
							},
							Data: map[string][]byte{
								"product_id": []byte("foo_product_id"),
							},
						})
					})

					It("returns success", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(bsData.Sysprep.ProductID).To(Equal("foo_product_id"))
					})

					When("key from selector is not present", func() {
						BeforeEach(func() {
							vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep.UserData.ProductID.Key = anotherKey
						})

						It("returns an error", func() {
							_, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
							Expect(err).To(HaveOccurred())
							Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
						})
					})
				})

				When("secret is not present", func() {
					It("returns an error", func() {
						_, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())
						Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
					})
				})

				When("secret selector is absent", func() {

					BeforeEach(func() {
						vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep.UserData.FullName = "foo"
						vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep.UserData.ProductID = nil
					})

					It("does not return an error", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(bsData.Sysprep.ProductID).To(Equal(""))
					})
				})
			})

			Context("for GUIUnattended", func() {
				pwdSecretName := "password_secret"

				BeforeEach(func() {
					vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep = &sysprep.Sysprep{
						GUIUnattended: &sysprep.GUIUnattended{
							AutoLogon: true,
							Password: &sysprep.PasswordSecretKeySelector{
								Name: pwdSecretName,
								Key:  "password",
							},
						},
					}
				})

				When("secret is present", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      pwdSecretName,
								Namespace: vmCtx.VM.Namespace,
							},
							Data: map[string][]byte{
								"password": []byte("foo_bar123"),
							},
						})
					})

					It("returns success", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(bsData.Sysprep.Password).To(Equal("foo_bar123"))
					})

					When("key from selector is not present", func() {
						BeforeEach(func() {
							vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep.GUIUnattended.Password.Key = anotherKey
						})

						It("returns an error", func() {
							bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
							Expect(err).To(HaveOccurred())
							Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
							Expect(bsData.Sysprep).To(BeNil())
						})
					})
				})

				When("secret is not present", func() {
					It("returns an error", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())
						Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
						Expect(bsData.Sysprep).To(BeNil())
					})
				})
			})

			Context("for Identification", func() {
				pwdSecretName := "domain_password_secret"

				BeforeEach(func() {
					vmCtx.VM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
						DomainName: "foo",
					}
					vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep = &sysprep.Sysprep{
						Identification: &sysprep.Identification{
							DomainAdminPassword: &sysprep.DomainPasswordSecretKeySelector{
								Name: pwdSecretName,
								Key:  "domain_password",
							},
						},
					}
				})

				When("secret is present", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      pwdSecretName,
								Namespace: vmCtx.VM.Namespace,
							},
							Data: map[string][]byte{
								"domain_password": []byte("foo_bar_fizz123"),
							},
						})
					})

					It("returns success", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(bsData.Sysprep.DomainPassword).To(Equal("foo_bar_fizz123"))
					})

					When("key from selector is not present", func() {
						BeforeEach(func() {
							vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep.Identification.DomainAdminPassword.Key = anotherKey
						})

						It("returns an error", func() {
							bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
							Expect(err).To(HaveOccurred())
							Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
							Expect(bsData.Sysprep).To(BeNil())
						})
					})
				})

				When("secret is not present", func() {
					It("returns an error", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).To(HaveOccurred())
						Expect(conditions.IsFalse(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
						Expect(bsData.Sysprep).To(BeNil())
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
				_, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
			})

			When("Utilize ConfigMap when V1alpha1ConfigMapTransportAnnotation is set", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapVAppCM)
					vmCtx.VM.Annotations = map[string]string{vmopv1.V1alpha1ConfigMapTransportAnnotation: "true"}

				})

				It("returns success", func() {
					bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(bsData.VAppData).To(HaveKeyWithValue("foo-vapp", "bar-vapp"))
					Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeTrue())
				})
			})

			When("vAppConfig with properties", func() {
				BeforeEach(func() {
					vmCtx.VM.Spec.Bootstrap.VAppConfig.Properties = []common.KeyValueOrSecretKeySelectorPair{
						{
							Value: common.ValueOrSecretKeySelector{
								From: &common.SecretKeySelector{
									Name: vAppDataName,
									Key:  "foo-vapp",
								},
							},
						},
					}

					It("returns success", func() {
						bsData, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(bsData.VAppExData).To(HaveKey(vAppDataName))
						data := bsData.VAppExData[vAppDataName]
						Expect(data).To(HaveKeyWithValue("foo-vapp", "bar-vapp"))
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
			isVolumes := vmopv1util.FilterInstanceStorageVolumes(vm)
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
			vmClass = builder.DummyVirtualMachineClassGenName()
		})

		When("InstanceStorage FFS is enabled", func() {

			It("VM Class does not contain instance storage volumes", func() {
				is := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass.Spec.Hardware.InstanceStorage)
				Expect(is).To(BeFalse())
				Expect(vmopv1util.FilterInstanceStorageVolumes(vmCtx.VM)).To(BeEmpty())
			})

			When("Instance Volume is added in VM Class", func() {
				BeforeEach(func() {
					vmClass.Spec.Hardware.InstanceStorage = builder.DummyInstanceStorage()
				})

				It("Instance Volumes should be added", func() {
					is := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass.Spec.Hardware.InstanceStorage)
					Expect(is).To(BeTrue())
					expectInstanceStorageVolumes(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)
				})

				It("Instance Storage is already added to VM Spec.Volumes", func() {
					is := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass.Spec.Hardware.InstanceStorage)
					Expect(is).To(BeTrue())

					isVolumesBefore := vmopv1util.FilterInstanceStorageVolumes(vmCtx.VM)
					expectInstanceStorageVolumes(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)

					// Instance Storage is already configured, should not patch again
					is = vsphere.AddInstanceStorageVolumes(vmCtx, vmClass.Spec.Hardware.InstanceStorage)
					Expect(is).To(BeTrue())
					isVolumesAfter := vmopv1util.FilterInstanceStorageVolumes(vmCtx.VM)
					Expect(isVolumesAfter).To(HaveLen(len(isVolumesBefore)))
					Expect(isVolumesAfter).To(Equal(isVolumesBefore))
				})
			})
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
			// Create multiple PVCs to verify only the expected one is returned.
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

	Context("GetAttachedClassicDiskUUIDs", func() {

		var (
			attachedClassicVolStatus = vmopv1.VirtualMachineVolumeStatus{
				Name:     "attached-vol",
				DiskUUID: "attached-classic-disk-uuid",
				Attached: true,
				Type:     vmopv1.VirtualMachineStorageDiskTypeClassic,
			}
			unattachedClassicVolStatus = vmopv1.VirtualMachineVolumeStatus{
				Name:     "unattached-vol",
				DiskUUID: "unattached-classic-disk-uuid",
				Attached: false,
				Type:     vmopv1.VirtualMachineStorageDiskTypeClassic,
			}
			pvcVolStatus = vmopv1.VirtualMachineVolumeStatus{
				Name:     "attached-pvc-vol",
				DiskUUID: "attached-pvc-disk-uuid",
				Attached: true,
				Type:     vmopv1.VirtualMachineStorageDiskTypeManaged,
			}
		)

		BeforeEach(func() {
			vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				attachedClassicVolStatus,
				unattachedClassicVolStatus,
				pvcVolStatus,
			}
		})

		It("Should return only the attached classic disk UUID", func() {
			diskUUIDs := vsphere.GetAttachedClassicDiskUUIDs(vmCtx)
			Expect(diskUUIDs).To(HaveLen(1))
			Expect(diskUUIDs).To(HaveKey(attachedClassicVolStatus.DiskUUID))
		})
	})

	Context("GetAdditionalResourcesForBackup", func() {

		When("VM spec has no bootstrap data set", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = nil
			})

			It("Should not return any additional resources for backup", func() {
				resources, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(resources).To(BeEmpty())
			})
		})

		When("VM spec has bootstrap in CloudConfig referencing a Secret object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						CloudConfig: &cloudinit.CloudConfig{
							Users: []cloudinit.User{
								{
									HashedPasswd: &common.SecretKeySelector{
										Name: "dummy-cloud-config-secret",
									},
								},
							},
						},
					},
				}
				initObjects = append(initObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-cloud-config-secret",
					},
				})
			})

			It("Should return the Secret object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-cloud-config-secret"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("Secret")))
			})
		})

		When("VM spec has bootstrap in RawCloudConfig referencing a Secret object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &common.SecretKeySelector{
							Name: "dummy-raw-cloud-secret",
						},
					},
				}
				initObjects = append(initObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-raw-cloud-secret",
					},
				})
			})

			It("Should return the Secret object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-raw-cloud-secret"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("Secret")))
			})
		})

		When("VM spec has bootstrap in RawCloudConfig referencing a ConfigMap object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &common.SecretKeySelector{
							Name: "dummy-raw-cloud-config-map",
						},
					},
				}
				initObjects = append(initObjects, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-raw-cloud-config-map",
					},
				})
			})

			It("Should return the ConfigMap object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-raw-cloud-config-map"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("ConfigMap")))
			})
		})

		When("VM spec has bootstrap in Sysprep referencing a Secret object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
						Sysprep: &sysprep.Sysprep{
							UserData: sysprep.UserData{
								ProductID: &sysprep.ProductIDSecretKeySelector{
									Name: "dummy-sysprep-secret",
								},
							},
						},
					},
				}
				initObjects = append(initObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-sysprep-secret",
					},
				})
			})

			It("Should return the Secret object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-sysprep-secret"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("Secret")))
			})
		})

		When("VM spec has bootstrap in RawSysprep referencing a Secret object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
						RawSysprep: &common.SecretKeySelector{
							Name: "dummy-raw-sysprep-secret",
						},
					},
				}
				initObjects = append(initObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-raw-sysprep-secret",
					},
				})
			})

			It("Should return the Secret object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-raw-sysprep-secret"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("Secret")))
			})
		})

		When("VM spec has bootstrap in RawSysprep referencing a ConfigMap object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
						RawSysprep: &common.SecretKeySelector{
							Name: "dummy-raw-sysprep-config-map",
						},
					},
				}
				initObjects = append(initObjects, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-raw-sysprep-config-map",
					},
				})
			})

			It("Should return the ConfigMap object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-raw-sysprep-config-map"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("ConfigMap")))
			})
		})

		When("VM spec has bootstrap in VAppConfig Properties referencing a Secret object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
						Properties: []common.KeyValueOrSecretKeySelectorPair{
							{
								Value: common.ValueOrSecretKeySelector{
									From: &common.SecretKeySelector{
										Name: "dummy-vapp-config-property-secret",
										Key:  "foo",
									},
								},
							},
							{
								Value: common.ValueOrSecretKeySelector{
									From: &common.SecretKeySelector{
										Name: "dummy-vapp-config-property-secret",
										Key:  "bar",
									},
								},
							},
						},
					},
				}
				initObjects = append(initObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-vapp-config-property-secret",
					},
				})
			})

			It("Should return the Secret object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-vapp-config-property-secret"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("Secret")))
			})
		})

		When("VM spec has bootstrap in VAppConfig RawProperties referencing a Secret object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
						RawProperties: "dummy-raw-vapp-config-secret",
					},
				}
				initObjects = append(initObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-raw-vapp-config-secret",
					},
				})
			})

			It("Should return the Secret object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-raw-vapp-config-secret"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("Secret")))
			})
		})

		When("VM spec has bootstrap in VAppConfig RawProperties referencing a ConfigMap object", func() {

			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
						RawProperties: "dummy-raw-vapp-config-config-map",
					},
				}
				initObjects = append(initObjects, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmCtx.VM.Namespace,
						Name:      "dummy-raw-vapp-config-config-map",
					},
				})
			})

			It("Should return the Secret object as additional resource for backup", func() {
				objects, err := vsphere.GetAdditionalResourcesForBackup(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(objects).To(HaveLen(1))
				Expect(objects[0].GetName()).To(Equal("dummy-raw-vapp-config-config-map"))
				Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("ConfigMap")))
			})
		})
	})

	Context("PatchSnapshotStatus", func() {
		var (
			vmSnapshot *vmopv1.VirtualMachineSnapshot
			snapMoRef  *vimtypes.ManagedObjectReference
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
					VMRef: &common.LocalObjectRef{
						APIVersion: vmCtx.VM.APIVersion,
						Kind:       vmCtx.VM.Kind,
						Name:       vmCtx.VM.Name,
					},
					Quiesce: &vmopv1.QuiesceSpec{
						Timeout: &metav1.Duration{Duration: timeout},
					},
				},
			}
		})

		When("snapshot patched with vm info and ready condition", func() {
			BeforeEach(func() {
				vmCtx.VM.Status = vmopv1.VirtualMachineStatus{
					UniqueID:   "dummyID",
					PowerState: vmopv1.VirtualMachinePowerStateOn,
				}

				snapMoRef = &vimtypes.ManagedObjectReference{
					Value: "snap-103",
				}

				initObjects = append(initObjects, vmSnapshot)
			})

			It("succeeds", func() {
				err := vsphere.PatchSnapshotSuccessStatus(vmCtx, k8sClient, vmSnapshot, snapMoRef)
				Expect(err).ToNot(HaveOccurred())

				snapObj := &vmopv1.VirtualMachineSnapshot{}
				err = k8sClient.Get(vmCtx, client.ObjectKey{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, snapObj)
				Expect(err).To(BeNil())
				Expect(snapObj.Status.UniqueID).To(Equal(snapMoRef.Value))
				Expect(snapObj.Status.Quiesced).To(BeTrue())
				Expect(snapObj.Status.PowerState).To(Equal(vmCtx.VM.Status.PowerState))
				Expect(snapObj.Status.Conditions).To(HaveLen(1))
				Expect(snapObj.Status.Conditions[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReadyCondition))
				Expect(snapObj.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			})
		})
	})
}
