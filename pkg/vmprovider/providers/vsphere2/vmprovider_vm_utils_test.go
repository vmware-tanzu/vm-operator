// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
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
			Context: pkgconfig.WithConfig(pkgconfig.Config{MaxDeployThreadsOnProvider: 16}),
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
		// NOTE: As we currently have it, v1a2 must have this enabled.
		When("WCP_Namespaced_VM_Class FSS is enabled", func() {
			var (
				vmClass *vmopv1.VirtualMachineClass
			)

			BeforeEach(func() {
				vmClass = builder.DummyVirtualMachineClass2A2("dummy-vm-class")
				vmClass.Namespace = vmCtx.VM.Namespace
				vmCtx.VM.Spec.ClassName = vmClass.Name
				pkgconfig.SetContext(vmCtx, func(config *pkgconfig.Config) {
					config.Features.NamespacedVMClass = true
				})
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

				pkgconfig.SetContext(vmCtx, func(config *pkgconfig.Config) {
					config.Features.ImageRegistry = true
				})
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
				vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig = &common.SecretKeySelector{}
				vmCtx.VM.Spec.Bootstrap.CloudInit.RawCloudConfig.Name = dataName
			})

			It("return an error when resources does not exist", func() {
				_, err := vsphere.GetVirtualMachineBootstrap(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
			})

			When("ConfigMap exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM)
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

			When("ConfigMap exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapCM)
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
						UserData: &sysprep.UserData{
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
					vmCtx.VM.Spec.Bootstrap.Sysprep.Sysprep = &sysprep.Sysprep{
						Identification: &sysprep.Identification{
							JoinDomain: "foo",
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

			When("ConfigMap exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, bootstrapVAppCM)
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
							UserData: &sysprep.UserData{
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
}
