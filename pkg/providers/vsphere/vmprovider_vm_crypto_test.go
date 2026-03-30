// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vimcrypto "github.com/vmware/govmomi/crypto"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmCryptoTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass

		zoneName string
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
		})
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vmClass = builder.DummyVirtualMachineClassGenName()
		vm = builder.DummyBasicVirtualMachine("test-vm", "")

		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true

		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.Features.BringYourOwnEncryptionKey = true
		})
		parentCtx = vmconfig.WithContext(parentCtx)
		parentCtx = vmconfig.Register(parentCtx, crypto.New())

		vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(
			parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(
			ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		clusterVMI1 := &vmopv1.ClusterVirtualMachineImage{}

		if testConfig.WithContentLibrary {
			Expect(ctx.Client.Get(
				ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
				clusterVMI1)).To(Succeed())
		} else {
			vsphere.SkipVMImageCLProviderCheck = true
			clusterVMI1 = builder.DummyClusterVirtualMachineImage("DC0_C0_RP0_VM0")
			Expect(ctx.Client.Create(ctx, clusterVMI1)).To(Succeed())
			conditions.MarkTrue(clusterVMI1, vmopv1.ReadyConditionType)
			Expect(ctx.Client.Status().Update(ctx, clusterVMI1)).To(Succeed())
		}

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMI1.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMI1.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		zoneName = ctx.GetFirstZoneName()
		vm.Labels[corev1.LabelTopologyZone] = zoneName
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		var storageClass storagev1.StorageClass
		Expect(ctx.Client.Get(
			ctx,
			client.ObjectKey{Name: ctx.EncryptedStorageClassName},
			&storageClass)).To(Succeed())
		Expect(kubeutil.MarkEncryptedStorageClass(
			ctx,
			ctx.Client,
			storageClass,
			true)).To(Succeed())
	})

	AfterEach(func() {
		vsphere.SkipVMImageCLProviderCheck = false

		if vm != nil &&
			!pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
			By("Assert vm.Status.Crypto is nil when BYOK is disabled", func() {
				Expect(vm.Status.Crypto).To(BeNil())
			})
		}

		vmClass = nil
		vm = nil

		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	useExistingVM := func(
		cryptoSpec vimtypes.BaseCryptoSpec, vTPM bool) {

		vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, vmList).ToNot(BeEmpty())

		vcVM := vmList[0]
		var o mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
		vm.Spec.InstanceUUID = o.Config.InstanceUuid

		powerState, err := vcVM.PowerState(ctx)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		if powerState == vimtypes.VirtualMachinePowerStatePoweredOn {
			tsk, err := vcVM.PowerOff(ctx)
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, tsk.Wait(ctx)).To(Succeed())
		}

		if cryptoSpec != nil || vTPM {
			configSpec := vimtypes.VirtualMachineConfigSpec{
				Crypto: cryptoSpec,
			}
			if vTPM {
				configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Device: &vimtypes.VirtualTPM{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           -1000,
								ControllerKey: 100,
							},
						},
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					},
				}
			}
			tsk, err := vcVM.Reconfigure(ctx, configSpec)
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, tsk.Wait(ctx)).To(Succeed())
		}
	}

	When("deploying an encrypted vm", func() {
		JustBeforeEach(func() {
			vm.Spec.StorageClass = ctx.EncryptedStorageClassName
			Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
		})

		When("using a default provider", func() {

			When("default provider is native key provider", func() {
				JustBeforeEach(func() {
					m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
					Expect(m.MarkDefault(ctx, ctx.NativeKeyProviderID)).To(Succeed())
				})

				When("using sync create", func() {
					BeforeEach(func() {
						pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
							config.AsyncCreateEnabled = false
							config.AsyncSignalEnabled = true
						})
					})
					It("should succeed", func() {
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
						Expect(vm.Status.Crypto).ToNot(BeNil())
						Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
							[]vmopv1.VirtualMachineEncryptionType{
								vmopv1.VirtualMachineEncryptionTypeConfig,
							}))
						Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
						Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
					})
				})

				When("using async create", func() {
					BeforeEach(func() {
						pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
							config.AsyncCreateEnabled = true
							config.AsyncSignalEnabled = true
						})
					})
					It("should succeed", func() {
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
						Expect(vm.Status.Crypto).ToNot(BeNil())
						Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
							[]vmopv1.VirtualMachineEncryptionType{
								vmopv1.VirtualMachineEncryptionTypeConfig,
							}))
						Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
						Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
					})

					// Please note this test uses FlakeAttempts(5) due to the
					// validation of some predictable-over-time behavior.
					When("there is a duplicate create", FlakeAttempts(5), func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.MaxDeployThreadsOnProvider = 16
							})
						})
						It("should return ErrReconcileInProgress", func() {
							var (
								errs   []error
								errsMu sync.Mutex
								done   sync.WaitGroup
								start  = make(chan struct{})
							)

							// Set up five goroutines that race to
							// create the VM first.
							for i := 0; i < 5; i++ {
								done.Add(1)
								go func(copyOfVM *vmopv1.VirtualMachine) {
									defer done.Done()
									<-start
									err := createOrUpdateVM(ctx, vmProvider, copyOfVM)
									if err != nil {
										errsMu.Lock()
										errs = append(errs, err)
										errsMu.Unlock()
									} else {
										vm = copyOfVM
									}
								}(vm.DeepCopy())
							}

							close(start)

							done.Wait()

							Expect(errs).To(HaveLen(4))

							Expect(errs).Should(ConsistOf(
								providers.ErrReconcileInProgress,
								providers.ErrReconcileInProgress,
								providers.ErrReconcileInProgress,
								providers.ErrReconcileInProgress,
							))

							Expect(vm.Status.Crypto).ToNot(BeNil())
							Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
								[]vmopv1.VirtualMachineEncryptionType{
									vmopv1.VirtualMachineEncryptionTypeConfig,
								}))
							Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
							Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
						})
					})
				})
			})

			When("default provider is not native key provider", func() {
				JustBeforeEach(func() {
					m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
					Expect(m.MarkDefault(ctx, ctx.EncryptionClass1ProviderID)).To(Succeed())
				})

				It("should succeed", func() {
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.Crypto).ToNot(BeNil())
					Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
						[]vmopv1.VirtualMachineEncryptionType{
							vmopv1.VirtualMachineEncryptionTypeConfig,
						}))
					Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass1ProviderID))
					Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
				})
			})
		})

		Context("using an encryption class", func() {

			JustBeforeEach(func() {
				vm.Spec.Crypto.EncryptionClassName = ctx.EncryptionClass1Name
			})

			It("should succeed", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(vm.Status.Crypto).ToNot(BeNil())
				Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
					[]vmopv1.VirtualMachineEncryptionType{
						vmopv1.VirtualMachineEncryptionTypeConfig,
					}))
				Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass1ProviderID))
				Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass1KeyID))
				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
			})
		})
	})

	When("encrypting an existing vm", func() {
		var (
			hasVTPM bool
		)

		BeforeEach(func() {
			hasVTPM = false
		})

		JustBeforeEach(func() {
			useExistingVM(nil, hasVTPM)
			vm.Spec.StorageClass = ctx.EncryptedStorageClassName
		})

		When("using a default provider", func() {

			When("default provider is native key provider", func() {
				JustBeforeEach(func() {
					m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
					Expect(m.MarkDefault(ctx, ctx.NativeKeyProviderID)).To(Succeed())
				})

				It("should succeed", func() {
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.Crypto).ToNot(BeNil())

					Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
						[]vmopv1.VirtualMachineEncryptionType{
							vmopv1.VirtualMachineEncryptionTypeConfig,
						}))
					Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
					Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
				})
			})

			When("default provider is not native key provider", func() {
				JustBeforeEach(func() {
					m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
					Expect(m.MarkDefault(ctx, ctx.EncryptionClass1ProviderID)).To(Succeed())
				})

				It("should succeed", func() {
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.Crypto).ToNot(BeNil())

					Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
						[]vmopv1.VirtualMachineEncryptionType{
							vmopv1.VirtualMachineEncryptionTypeConfig,
						}))
					Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass1ProviderID))
					Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
				})
			})
		})

		Context("using an encryption class", func() {

			JustBeforeEach(func() {
				vm.Spec.Crypto.EncryptionClassName = ctx.EncryptionClass2Name
			})

			It("should succeed", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(vm.Status.Crypto).ToNot(BeNil())

				Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
					[]vmopv1.VirtualMachineEncryptionType{
						vmopv1.VirtualMachineEncryptionTypeConfig,
					}))
				Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
				Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass2KeyID))
				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
			})

			When("using a non-encryption storage class", func() {
				JustBeforeEach(func() {
					vm.Spec.StorageClass = ctx.StorageClassName
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				})

				When("there is no vTPM", func() {
					It("should not error, but have condition", func() {
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
						Expect(vm.Status.Crypto).To(BeNil())
						c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal("InvalidState"))
						Expect(c.Message).To(Equal("Must use encryption storage class or have vTPM when encrypting vm"))
					})
				})

				When("there is a vTPM", func() {
					BeforeEach(func() {
						hasVTPM = true
					})
					It("should succeed", func() {
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
						Expect(vm.Status.Crypto).ToNot(BeNil())

						Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
							[]vmopv1.VirtualMachineEncryptionType{
								vmopv1.VirtualMachineEncryptionTypeConfig,
							}))
						Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
						Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass2KeyID))
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
					})
				})
			})
		})
	})

	When("recrypting a vm", func() {
		var (
			hasVTPM bool
		)

		BeforeEach(func() {
			hasVTPM = false
		})

		JustBeforeEach(func() {
			useExistingVM(&vimtypes.CryptoSpecEncrypt{
				CryptoKeyId: vimtypes.CryptoKeyId{
					KeyId: nsInfo.EncryptionClass1KeyID,
					ProviderId: &vimtypes.KeyProviderId{
						Id: ctx.EncryptionClass1ProviderID,
					},
				},
			}, hasVTPM)
			vm.Spec.StorageClass = ctx.EncryptedStorageClassName
		})

		When("using a default provider", func() {

			When("default provider is native key provider", func() {
				JustBeforeEach(func() {
					m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
					Expect(m.MarkDefault(ctx, ctx.NativeKeyProviderID)).To(Succeed())
				})

				It("should succeed", func() {
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.Crypto).ToNot(BeNil())

					Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
						[]vmopv1.VirtualMachineEncryptionType{
							vmopv1.VirtualMachineEncryptionTypeConfig,
						}))
					Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
					Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
					Expect(vm.Status.Crypto.KeyID).ToNot(Equal(nsInfo.EncryptionClass1KeyID))
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
				})
			})

			When("default provider is not native key provider", func() {
				JustBeforeEach(func() {
					m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
					Expect(m.MarkDefault(ctx, ctx.EncryptionClass2ProviderID)).To(Succeed())
				})

				It("should succeed", func() {
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.Crypto).ToNot(BeNil())

					Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
						[]vmopv1.VirtualMachineEncryptionType{
							vmopv1.VirtualMachineEncryptionTypeConfig,
						}))
					Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
					Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
					Expect(vm.Status.Crypto.KeyID).ToNot(Equal(nsInfo.EncryptionClass1KeyID))
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
				})
			})
		})

		Context("using an encryption class", func() {

			JustBeforeEach(func() {
				vm.Spec.Crypto.EncryptionClassName = ctx.EncryptionClass2Name
			})

			It("should succeed", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(vm.Status.Crypto).ToNot(BeNil())

				Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
					[]vmopv1.VirtualMachineEncryptionType{
						vmopv1.VirtualMachineEncryptionTypeConfig,
					}))
				Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
				Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass2KeyID))
				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
			})

			When("using a non-encryption storage class with a vTPM", func() {
				BeforeEach(func() {
					hasVTPM = true
				})

				JustBeforeEach(func() {
					vm.Spec.StorageClass = ctx.StorageClassName
				})

				It("should succeed", func() {
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.Crypto).ToNot(BeNil())

					Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
						[]vmopv1.VirtualMachineEncryptionType{
							vmopv1.VirtualMachineEncryptionTypeConfig,
						}))
					Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
					Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass2KeyID))
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
				})
			})
		})
	})
}
