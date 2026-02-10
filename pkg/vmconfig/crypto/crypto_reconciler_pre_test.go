// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crypto_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/crypto"
	"github.com/vmware/govmomi/pbm/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	pkgcrypto "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("Reconcile", Label(testlabels.Crypto), func() {
	var (
		r             vmconfig.ReconcilerWithContext
		ctx           context.Context
		k8sClient     ctrlclient.Client
		vimClient     *vim25.Client
		cryptoManager *crypto.ManagerKmip
		moVM          mo.VirtualMachine
		vm            *vmopv1.VirtualMachine
		encClass      *byokv1.EncryptionClass
		withObjs      []ctrlclient.Object
		withFuncs     interceptor.Funcs
		configSpec    *vimtypes.VirtualMachineConfigSpec

		provider1ID     string
		provider1Key1ID string
		provider1Key2ID string

		provider2ID     string
		provider2Key1ID string

		provider3ID string

		storageClass1 *storagev1.StorageClass
		storageClass2 *storagev1.StorageClass
	)

	BeforeEach(func() {
		r = pkgcrypto.New()

		vcsimCtx := builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()), builder.VCSimTestConfig{})
		ctx = vcsimCtx
		ctx = r.WithContext(ctx)
		ctx = vmconfig.WithContext(ctx)

		ctx = pkgctx.WithVMRecentTasks(ctx, []vimtypes.TaskInfo{
			{
				State:         vimtypes.TaskInfoStateSuccess,
				DescriptionId: "fake.task.0",
			},
		})

		vimClient = vcsimCtx.VCClient.Client
		cryptoManager = crypto.NewManagerKmip(vimClient)

		provider1ID = uuid.NewString()
		Expect(cryptoManager.RegisterKmsCluster(
			ctx,
			provider1ID,
			vimtypes.KmipClusterInfoKmsManagementTypeTrustAuthority,
		)).To(Succeed())
		{
			var err error
			provider1Key1ID, err = cryptoManager.GenerateKey(
				ctx,
				provider1ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(provider1Key1ID).ToNot(BeEmpty())
		}
		{
			var err error
			provider1Key2ID, err = cryptoManager.GenerateKey(
				ctx,
				provider1ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(provider1Key2ID).ToNot(BeEmpty())
		}

		provider2ID = uuid.NewString()
		Expect(cryptoManager.RegisterKmsCluster(
			ctx,
			provider2ID,
			vimtypes.KmipClusterInfoKmsManagementTypeTrustAuthority,
		)).To(Succeed())
		{
			var err error
			provider2Key1ID, err = cryptoManager.GenerateKey(
				ctx,
				provider2ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(provider2Key1ID).ToNot(BeEmpty())
		}

		provider3ID = uuid.NewString()
		Expect(cryptoManager.RegisterKmsCluster(
			ctx,
			provider3ID,
			vimtypes.KmipClusterInfoKmsManagementTypeNativeProvider,
		)).To(Succeed())

		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				KeyId: &vimtypes.CryptoKeyId{
					ProviderId: &vimtypes.KeyProviderId{
						Id: provider1ID,
					},
					KeyId: provider1Key1ID,
				},
			},
			Summary: vimtypes.VirtualMachineSummary{
				Runtime: vimtypes.VirtualMachineRuntimeInfo{
					PowerState: vimtypes.VirtualMachinePowerStatePoweredOff,
				},
			},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "my-namespace",
				Name:      "my-vm",
			},
			Spec: vmopv1.VirtualMachineSpec{
				StorageClass: "my-storage-class-2",
				Crypto: &vmopv1.VirtualMachineCryptoSpec{
					EncryptionClassName:   "my-encryption-class",
					UseDefaultKeyProvider: ptr.To(true),
				},
			},
		}

		encClass = &byokv1.EncryptionClass{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "my-namespace",
				Name:      "my-encryption-class",
			},
			Spec: byokv1.EncryptionClassSpec{
				KeyProvider: provider1ID,
				KeyID:       provider1Key1ID,
			},
		}

		storageClass1 = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-storage-class-1",
				UID:  types.UID(uuid.NewString()),
			},
		}
		storageClass2 = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-storage-class-2",
				UID:  types.UID(uuid.NewString()),
			},
			Parameters: map[string]string{
				"storagePolicyID": simulator.DefaultEncryptionProfileID,
			},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{encClass, storageClass1, storageClass2, vm}
	})
	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)

		if ctx != nil && k8sClient != nil {
			Expect(kubeutil.MarkEncryptedStorageClass(
				ctx,
				k8sClient,
				*storageClass2,
				true)).To(Succeed())
		}
	})

	When("it should panic", func() {
		When("ctx is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})

		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})

		When("vimClient is nil", func() {
			JustBeforeEach(func() {
				vimClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vimClient is nil"))
			})
		})

		When("vm is nil", func() {
			BeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})

		When("configSpec is nil", func() {
			BeforeEach(func() {
				configSpec = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("configSpec is nil"))
			})
		})
	})

	When("it should not panic", func() {
		var (
			err error
		)

		JustBeforeEach(func() {
			err = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
		})

		When("creating a new vm", func() {

			BeforeEach(func() {
				moVM.Config = nil
			})

			When("spec.crypto.encryptionClassName is non-empty", func() {
				When("the EncryptionClass does not exit", func() {
					BeforeEach(func() {
						vm.Spec.Crypto.EncryptionClassName += fakeString
					})
					It("should return an error", func() {
						Expect(err).To(HaveOccurred())
						Expect(apierrors.IsNotFound(err)).To(BeTrue())
						c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassNotFound.String()))
					})
				})

				When("there is a non-404 error getting the encryption class", func() {
					BeforeEach(func() {
						withFuncs.Get = func(
							ctx context.Context,
							client ctrlclient.WithWatch,
							key ctrlclient.ObjectKey,
							obj ctrlclient.Object,
							opts ...ctrlclient.GetOption) error {

							if _, ok := obj.(*byokv1.EncryptionClass); ok {
								return errors.New(fakeString)
							}
							return client.Get(ctx, key, obj, opts...)
						}
					})
					It("should return an error", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fakeString))
						c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(pkgcrypto.ReasonInternalError.String()))
						Expect(c.Message).To(Equal(fakeString))
					})
				})

				When("the EncryptionClass specifies an invalid provider", func() {
					BeforeEach(func() {
						encClass.Spec.KeyProvider = fakeString
					})
					It("should return an error", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(pkgcrypto.ErrInvalidKeyProvider))
						c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassInvalid.String()))
						Expect(c.Message).To(Equal(pkgcrypto.ErrInvalidKeyProvider.Error()))
					})
				})

				When("the EncryptionClass specifies an invalid key id", func() {
					BeforeEach(func() {
						encClass.Spec.KeyProvider = provider1ID
						encClass.Spec.KeyID = "invalid"
					})
					It("should return an error", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(pkgcrypto.ErrInvalidKeyID))
						c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassInvalid.String()))
						Expect(c.Message).To(Equal(pkgcrypto.ErrInvalidKeyID.Error()))
					})
				})

				When("the EncryptionClass exists", func() {
					When("the vm is being created with a vtpm", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
								&vimtypes.VirtualDeviceConfigSpec{
									Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
									Device:    &vimtypes.VirtualTPM{},
								},
							}
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should deploy an encrypted vm", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							Expect(configSpec.Crypto).ToNot(BeNil())
							cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
							Expect(ok).To(BeTrue())
							Expect(cryptoSpec.CryptoKeyId.KeyId).To(Equal(provider1Key1ID))
							Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
						})
					})

					When("the vm uses an encrypted StorageClass", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = nil
							vm.Spec.StorageClass = storageClass2.Name
						})
						It("should deploy an encrypted vm", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
							Expect(ok).To(BeTrue())
							Expect(cryptoSpec.CryptoKeyId.KeyId).To(Equal(provider1Key1ID))
							Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
						})
					})

					When("the vm is not being created with a vtpm or use an encrypted StorageClass", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = nil
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should return an error", func() {
							Expect(err).To(MatchError(pkgcrypto.ErrMustUseVTPMOrEncryptedStorageClass))
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
							Expect(c.Message).To(Equal(pkgcrypto.ErrMustUseVTPMOrEncryptedStorageClass.Error()))
						})
					})
				})
			})

			When("spec.crypto.encryptionClassName is empty", func() {

				BeforeEach(func() {
					vm.Spec.Crypto = nil
				})

				When("there is a default key provider", func() {
					BeforeEach(func() {
						Expect(cryptoManager.MarkDefault(ctx, provider1ID)).To(Succeed())
					})

					When("the vm is being created with a vtpm", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
								&vimtypes.VirtualDeviceConfigSpec{
									Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
									Device:    &vimtypes.VirtualTPM{},
								},
							}
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should deploy an encrypted vm", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
							Expect(ok).To(BeTrue())
							Expect(cryptoSpec.CryptoKeyId.KeyId).To(BeEmpty())
							Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
						})
					})

					When("the vm uses an encrypted StorageClass", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = nil
							vm.Spec.StorageClass = storageClass2.Name
						})
						It("should deploy an encrypted vm", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
							Expect(ok).To(BeTrue())
							Expect(cryptoSpec.CryptoKeyId.KeyId).To(BeEmpty())
							Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
						})
					})

					When("the vm is not being created with a vtpm or use an encrypted StorageClass", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = nil
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should deploy an unencrypted vm", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							Expect(configSpec.Crypto).To(BeNil())
						})
					})
				})

				When("there is not a default key provider", func() {

					When("the vm is being created with a vtpm", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
								&vimtypes.VirtualDeviceConfigSpec{
									Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
									Device:    &vimtypes.VirtualTPM{},
								},
							}
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should return an error", func() {
							Expect(err).To(MatchError(pkgcrypto.ErrMustNotUseVTPMOrEncryptedStorageClass))
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(pkgcrypto.ReasonNoDefaultKeyProvider.String()))
							Expect(c.Message).To(Equal(pkgcrypto.ErrMustNotUseVTPMOrEncryptedStorageClass.Error()))
						})
					})

					When("the vm uses an encrypted StorageClass", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = nil
							vm.Spec.StorageClass = storageClass2.Name
						})
						It("should return an error", func() {
							Expect(err).To(MatchError(pkgcrypto.ErrMustNotUseVTPMOrEncryptedStorageClass))
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(pkgcrypto.ReasonNoDefaultKeyProvider.String()))
							Expect(c.Message).To(Equal(pkgcrypto.ErrMustNotUseVTPMOrEncryptedStorageClass.Error()))
						})
					})

					When("the vm is not being created with a vtpm or use an encrypted StorageClass", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = nil
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should deploy an unencrypted vm", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							Expect(configSpec.Crypto).To(BeNil())
						})
					})
				})
			})
		})

		When("updating existing vm", func() {

			When("spec.crypto.encryptionClassName is empty", func() {

				BeforeEach(func() {
					vm.Spec.Crypto = nil
				})

				When("there is a default key provider", func() {
					BeforeEach(func() {
						Expect(cryptoManager.MarkDefault(ctx, provider1ID)).To(Succeed())
					})
					When("the vm is already encrypted", func() {
						BeforeEach(func() {
							moVM.Config.KeyId = &vimtypes.CryptoKeyId{
								KeyId: provider1Key2ID,
								ProviderId: &vimtypes.KeyProviderId{
									Id: provider1ID,
								},
							}
						})
						When("the providers are the same", func() {
							It("should set EncryptionSynced=true", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
							})
							When("the vm does not have an encrypted storage class", func() {
								BeforeEach(func() {
									vm.Spec.StorageClass = storageClass1.Name
								})

								It("should set EncryptionSynced=false with InvalidState", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "use encryption storage class or have vTPM")))
								})

								When("the vm has a vtpm", func() {
									BeforeEach(func() {
										moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
											&vimtypes.VirtualTPM{},
										}
									})
									It("should set EncryptionSynced=true", func() {
										Expect(err).ToNot(HaveOccurred())
										Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
									})

									When("the vtpm is being removed", func() {
										BeforeEach(func() {
											configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
												&vimtypes.VirtualDeviceConfigSpec{
													Device:    &vimtypes.VirtualTPM{},
													Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
												},
											}
										})
										It("should set EncryptionSynced=false with InvalidChanges", func() {
											Expect(err).ToNot(HaveOccurred())
											c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
											Expect(c).ToNot(BeNil())
											Expect(c.Status).To(Equal(metav1.ConditionFalse))
											Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidChanges.String()))
											Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "not remove vTPM")))
										})
									})
								})
							})
						})
						When("the new provider is different than the current provider", func() {
							BeforeEach(func() {
								Expect(cryptoManager.MarkDefault(ctx, provider2ID)).To(Succeed())
							})
							It("should recrypt the vm", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).To(BeNil())
								cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
								Expect(ok).To(BeTrue())
								Expect(cryptoSpec.NewKeyId.KeyId).To(BeEmpty())
								Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider2ID))
							})
						})
					})

					When("the vm is not already encrypted", func() {
						BeforeEach(func() {
							moVM.Config.KeyId = nil
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should not encrypt the vm", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							Expect(configSpec.Crypto).To(BeNil())
						})
						When("the vm has a vtpm but not encrypted storage class", func() {
							BeforeEach(func() {
								moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualTPM{},
								}
								vm.Spec.StorageClass = storageClass1.Name
							})
							It("should encrypt the vm", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).To(BeNil())
								cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
								Expect(ok).To(BeTrue())
								Expect(cryptoSpec.CryptoKeyId.KeyId).To(BeEmpty())
								Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
							})

							When("the vtpm is being removed", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Device:    &vimtypes.VirtualTPM{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
										},
									}
								})
								It("should return an error", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidChanges.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("encrypting", "not remove vTPM")))
								})
							})
						})
					})
				})

				When("there is not a default key provider", func() {
					BeforeEach(func() {
						Expect(cryptoManager.SetDefaultKmsClusterId(ctx, "", nil)).To(Succeed())
					})

					When("the vm is encrypted", func() {
						It("should return an error", func() {
							Expect(err).To(HaveOccurred())
							Expect(err).To(MatchError(pkgcrypto.ErrNoDefaultKeyProvider))
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(pkgcrypto.ReasonNoDefaultKeyProvider.String()))
						})

						When("vm has running task", func() {
							BeforeEach(func() {
								moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
												KeyId: &vimtypes.CryptoKeyId{},
											},
										},
									},
								}
								ctx = pkgctx.WithVMRecentTasks(ctx, []vimtypes.TaskInfo{
									{
										State:         vimtypes.TaskInfoStateRunning,
										DescriptionId: "fake.task.1",
									},
								})
							})

							It("should update the status without returning an error", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(vm.Status.Crypto).ToNot(BeNil())
								Expect(vm.Status.Crypto).To(Equal(&vmopv1.VirtualMachineCryptoStatus{
									Encrypted: []vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
										vmopv1.VirtualMachineEncryptionTypeDisks,
									},
									ProviderID: provider1ID,
									KeyID:      provider1Key1ID,
								}))
							})
						})

						When("vm is paused", func() {
							Context("by admin", func() {
								BeforeEach(func() {
									moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
													KeyId: &vimtypes.CryptoKeyId{},
												},
											},
										},
									}
									moVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
										&vimtypes.OptionValue{
											Key:   vmopv1.PauseVMExtraConfigKey,
											Value: "true",
										},
									}
								})
								It("should update the status without returning an error", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(vm.Status.Crypto).ToNot(BeNil())
									Expect(vm.Status.Crypto).To(Equal(&vmopv1.VirtualMachineCryptoStatus{
										Encrypted: []vmopv1.VirtualMachineEncryptionType{
											vmopv1.VirtualMachineEncryptionTypeConfig,
											vmopv1.VirtualMachineEncryptionTypeDisks,
										},
										ProviderID: provider1ID,
										KeyID:      provider1Key1ID,
									}))
								})
							})

							Context("by devops", func() {
								BeforeEach(func() {
									moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
													KeyId: &vimtypes.CryptoKeyId{},
												},
											},
										},
									}
									vm.Annotations = map[string]string{
										vmopv1.PauseAnnotation: "",
									}
								})
								It("should update the status without returning an error", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(vm.Status.Crypto).ToNot(BeNil())
									Expect(vm.Status.Crypto).To(Equal(&vmopv1.VirtualMachineCryptoStatus{
										Encrypted: []vmopv1.VirtualMachineEncryptionType{
											vmopv1.VirtualMachineEncryptionTypeConfig,
											vmopv1.VirtualMachineEncryptionTypeDisks,
										},
										ProviderID: provider1ID,
										KeyID:      provider1Key1ID,
									}))
								})
							})

							Context("by admin and devops", func() {
								BeforeEach(func() {
									moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{
													KeyId: &vimtypes.CryptoKeyId{},
												},
											},
										},
									}
									moVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
										&vimtypes.OptionValue{
											Key:   vmopv1.PauseVMExtraConfigKey,
											Value: "true",
										},
									}
									vm.Annotations = map[string]string{
										vmopv1.PauseAnnotation: "",
									}
								})
								It("should update the status without returning an error", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(vm.Status.Crypto).ToNot(BeNil())
									Expect(vm.Status.Crypto).To(Equal(&vmopv1.VirtualMachineCryptoStatus{
										Encrypted: []vmopv1.VirtualMachineEncryptionType{
											vmopv1.VirtualMachineEncryptionTypeConfig,
											vmopv1.VirtualMachineEncryptionTypeDisks,
										},
										ProviderID: provider1ID,
										KeyID:      provider1Key1ID,
									}))
								})
							})
						})
					})

					When("the vm is not encrypted", func() {
						BeforeEach(func() {
							moVM.Config.KeyId = nil
							moVM.Config.Hardware.Device = nil
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should be a no-op", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							Expect(configSpec.Crypto).To(BeNil())
						})
					})
				})
			})

			When("spec.crypto.encryptionClassName is non-empty", func() {
				When("the EncryptionClass does not exit", func() {
					BeforeEach(func() {
						vm.Spec.Crypto.EncryptionClassName += fakeString
					})
					It("should return an error", func() {
						Expect(err).To(HaveOccurred())
						Expect(apierrors.IsNotFound(err)).To(BeTrue())
						c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassNotFound.String()))
					})
				})

				When("the EncryptionClass exists", func() {

					When("the vm is not already encrypted", func() {
						BeforeEach(func() {
							moVM.Config.KeyId = nil
						})

						It("should encrypt the vm", func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).To(BeNil())
							cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
							Expect(ok).To(BeTrue())
							Expect(cryptoSpec.CryptoKeyId.KeyId).To(Equal(provider1Key1ID))
							Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
						})

						When("the vm is powered on", func() {
							BeforeEach(func() {
								moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
							})
							It("should set EncryptionSynced=false with InvalidState", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).ToNot(BeNil())
								Expect(c.Status).To(Equal(metav1.ConditionFalse))
								Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
								Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("encrypting", "be powered off")))
							})
						})

						When("vm has snapshots", func() {
							BeforeEach(func() {
								moVM.Snapshot = getSnapshotInfoWithLinearChain()
							})
							It(shouldSetEncryptionSyncedWithInvalidState, func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).ToNot(BeNil())
								Expect(c.Status).To(Equal(metav1.ConditionFalse))
								Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
								Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("encrypting", "not have snapshots")))
							})
						})

						When("adding encrypted disk sans policy", func() {
							BeforeEach(func() {
								configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
									&vimtypes.VirtualDeviceConfigSpec{
										Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
											Crypto: &vimtypes.CryptoSpecEncrypt{},
										},
										Device:    &vimtypes.VirtualDisk{},
										Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
										Profile: []vimtypes.BaseVirtualMachineProfileSpec{
											&vimtypes.VirtualMachineDefinedProfileSpec{
												ProfileId: fakeString,
											},
										},
									},
								}
							})
							It(shouldSetEncryptionSyncedWithInvalidChanges, func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).ToNot(BeNil())
								Expect(c.Status).To(Equal(metav1.ConditionFalse))
								Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges).String()))
								Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("encrypting", "specify policy when encrypting devices")))
							})
						})

						When("adding encrypted disk with policy", func() {
							BeforeEach(func() {
								configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
									&vimtypes.VirtualDeviceConfigSpec{
										Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
											Crypto: &vimtypes.CryptoSpecEncrypt{},
										},
										Device:    &vimtypes.VirtualDisk{},
										Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
										Profile: []vimtypes.BaseVirtualMachineProfileSpec{
											&vimtypes.VirtualMachineDefinedProfileSpec{
												ProfileId: simulator.DefaultEncryptionProfileID,
											},
										},
									},
								}
							})
							It("should encrypt the vm", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).To(BeNil())
								cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
								Expect(ok).To(BeTrue())
								Expect(cryptoSpec.CryptoKeyId.KeyId).To(Equal(provider1Key1ID))
								Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
							})
						})
					})

					When("the vm is already encrypted", func() {
						When("the new provider is different than the current provider", func() {
							BeforeEach(func() {
								moVM.Config.KeyId = &vimtypes.CryptoKeyId{
									KeyId: provider2Key1ID,
									ProviderId: &vimtypes.KeyProviderId{
										Id: provider2ID,
									},
								}
							})
							It("should recrypt the vm", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).To(BeNil())
								cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
								Expect(ok).To(BeTrue())
								Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(provider1Key1ID))
								Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider1ID))
							})

							When("the vm does not have an encrypted storage class", func() {
								BeforeEach(func() {
									vm.Spec.StorageClass = storageClass1.Name
								})

								It("should set EncryptionSynced=false with InvalidState", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "use encryption storage class or have vTPM")))
								})
							})
						})

						When("the providers and keys are the same", func() {
							It("should set EncryptionSynced=true", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
							})

							When("adding encrypted disk with policy", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecEncrypt{},
											},
											Device:    &vimtypes.VirtualDisk{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
											Profile: []vimtypes.BaseVirtualMachineProfileSpec{
												&vimtypes.VirtualMachineDefinedProfileSpec{
													ProfileId: simulator.DefaultEncryptionProfileID,
												},
											},
										},
									}
								})
								It("should set EncryptionSynced=true", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
								})
							})

							When("adding encrypted devices sans policy", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecEncrypt{},
											},
											Device:    &vimtypes.VirtualDisk{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
										},
									}
								})
								It("should set EncryptionSynced=false with InvalidChanges", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidChanges.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "specify policy when encrypting devices")))
								})
							})

							When("editing encrypted devices sans policy", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecEncrypt{},
											},
											Device:    &vimtypes.VirtualDisk{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
										},
									}
								})
								It("should set EncryptionSynced=false with InvalidChanges", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges).String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "specify policy when encrypting devices")))
								})
							})

							When("encrypting raw disks", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecEncrypt{},
											},
											Device: &vimtypes.VirtualDisk{
												VirtualDevice: vimtypes.VirtualDevice{
													Backing: &vimtypes.VirtualDiskRawDiskVer2BackingInfo{},
												},
											},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
											Profile: []vimtypes.BaseVirtualMachineProfileSpec{
												&vimtypes.VirtualMachineDefinedProfileSpec{
													ProfileId: simulator.DefaultEncryptionProfileID,
												},
											},
										},
									}
								})
								It("should set EncryptionSynced=false with InvalidChanges", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges).String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "not encrypt raw disks")))
								})
							})

							When("encrypting non-disk devices", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecEncrypt{},
											},
											Device:    &vimtypes.VirtualAHCIController{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
											Profile: []vimtypes.BaseVirtualMachineProfileSpec{
												&vimtypes.VirtualMachineDefinedProfileSpec{
													ProfileId: simulator.DefaultEncryptionProfileID,
												},
											},
										},
									}
								})
								It("should set EncryptionSynced=false with InvalidChanges", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges).String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "not encrypt non-disk devices")))
								})
							})

							When("adding encrypted disk with policy and multiple backings", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecEncrypt{},
												Parent: &vimtypes.VirtualDeviceConfigSpecBackingSpec{},
											},
											Device:    &vimtypes.VirtualDisk{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
											Profile: []vimtypes.BaseVirtualMachineProfileSpec{
												&vimtypes.VirtualMachineDefinedProfileSpec{
													ProfileId: simulator.DefaultEncryptionProfileID,
												},
											},
										},
									}
								})
								It("should set EncryptionSynced=false with InvalidChanges", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges).String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "not encrypt devices with multiple backings")))
								})
							})
						})

						When("the secret keys are modified", func() {
							DescribeTable(
								"should set EncryptionSynced=false with InvalidChanges",
								func(key string) {
									Expect(r.Reconcile(
										ctx,
										k8sClient,
										vimClient,
										vm,
										moVM,
										&vimtypes.VirtualMachineConfigSpec{
											ExtraConfig: []vimtypes.BaseOptionValue{
												&vimtypes.OptionValue{
													Key:   key,
													Value: "",
												},
											},
										}),
									).To(Succeed())

									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidChanges.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "not add/remove/modify secret key")))
								},
								func(key string) string {
									return fmt.Sprintf("when key=%s", key)
								},
								Entry(nil, "ancestordatafilekeys"),
								Entry(nil, "cryptostate"),
								Entry(nil, "datafilekey"),
								Entry(nil, "encryption.required"),
								Entry(nil, "encryption.required.vtpm"),
								Entry(nil, "encryption.unspecified.default"),
							)
						})

						When("the new key is different than the current key", func() {
							BeforeEach(func() {
								moVM.Config.KeyId = &vimtypes.CryptoKeyId{
									KeyId: provider1Key2ID,
									ProviderId: &vimtypes.KeyProviderId{
										Id: provider1ID,
									},
								}
							})
							It("should recrypt the vm", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).To(BeNil())
								cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
								Expect(ok).To(BeTrue())
								Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(provider1Key1ID))
								Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider1ID))
							})

							When("vm has snapshot tree", func() {
								BeforeEach(func() {
									moVM.Snapshot = getSnapshotInfoWithTree()
								})
								It("should set EncryptionSynced=false with InvalidState", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "not have snapshot tree")))
								})
							})

							When("there are encrypted disks", func() {
								BeforeEach(func() {
									moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 1,
												Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
													KeyId: &vimtypes.CryptoKeyId{},
												},
											},
										},
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 2,
												Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
													KeyId: &vimtypes.CryptoKeyId{},
												},
											},
										},
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 3,
												Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{
													KeyId: &vimtypes.CryptoKeyId{},
												},
											},
											VDiskId: &vimtypes.ID{}, // FCD
										},
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 4,
												Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{
													KeyId: &vimtypes.CryptoKeyId{},
												},
											},
										},
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 5,
												Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{
													KeyId: &vimtypes.CryptoKeyId{},
												},
											},
										},
									}
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Device: &vimtypes.VirtualDisk{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 2,
													Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
														KeyId: &vimtypes.CryptoKeyId{},
													},
												},
												CapacityInBytes: 100,
											},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
										},
										&vimtypes.VirtualDeviceConfigSpec{
											Device: &vimtypes.VirtualDisk{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 5,
													Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
														KeyId: &vimtypes.CryptoKeyId{},
													},
												},
											},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
										},
									}
								})
								It("should recrypt the vm and the disks", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).To(BeNil())
									cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
									Expect(ok).To(BeTrue())
									Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(provider1Key1ID))
									Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider1ID))
									Expect(configSpec.DeviceChange).To(HaveLen(4))

									Expect(configSpec.DeviceChange).To(ConsistOf(
										&vimtypes.VirtualDeviceConfigSpec{
											Device: &vimtypes.VirtualDisk{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 1,
													Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
														KeyId: &vimtypes.CryptoKeyId{},
													},
												},
											},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: configSpec.Crypto,
											},
										},
										&vimtypes.VirtualDeviceConfigSpec{
											Device: &vimtypes.VirtualDisk{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 2,
													Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
														KeyId: &vimtypes.CryptoKeyId{},
													},
												},
												CapacityInBytes: 100,
											},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: configSpec.Crypto,
											},
										},
										&vimtypes.VirtualDeviceConfigSpec{
											Device: &vimtypes.VirtualDisk{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 4,
													Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{
														KeyId: &vimtypes.CryptoKeyId{},
													},
												},
											},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: configSpec.Crypto,
											},
										},
										&vimtypes.VirtualDeviceConfigSpec{
											Device: &vimtypes.VirtualDisk{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 5,
													Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
														KeyId: &vimtypes.CryptoKeyId{},
													},
												},
											},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
										},
									))
								})
							})

							When("shallow recrypting devices sans policy", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecShallowRecrypt{},
											},
											Device:    &vimtypes.VirtualDisk{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
										},
									}
								})
								It("should recrypt the vm", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).To(BeNil())
									cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
									Expect(ok).To(BeTrue())
									Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(provider1Key1ID))
									Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider1ID))
								})
							})

							When("deep recrypting devices sans policy", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecDeepRecrypt{},
											},
											Device:    &vimtypes.VirtualDisk{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
										},
									}
								})
								It("should set EncryptionSynced=false with InvalidChanges", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidChanges.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "specify policy when encrypting devices")))
								})
							})

							When("adding encrypted devices sans policy", func() {
								BeforeEach(func() {
									configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
										&vimtypes.VirtualDeviceConfigSpec{
											Backing: &vimtypes.VirtualDeviceConfigSpecBackingSpec{
												Crypto: &vimtypes.CryptoSpecEncrypt{},
											},
											Device:    &vimtypes.VirtualDisk{},
											Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
										},
									}
								})
								It("should set EncryptionSynced=false with InvalidChanges", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidChanges.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "specify policy when encrypting devices")))
								})
							})
						})
					})

				})
			})

			When("reconciling FCDs", func() {
				var (
					pvc           *corev1.PersistentVolumeClaim
					fcdEncClass   *byokv1.EncryptionClass
					scsiCtrlKey   int32 = 100
					fcdDiskKey    int32 = 2000
					fcdUnitNumber int32 = 1
				)

				BeforeEach(func() {
					// Set up a VM with an FCD disk
					moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
						&vimtypes.ParaVirtualSCSIController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: scsiCtrlKey,
									},
									BusNumber: 0,
								},
							},
						},
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           fcdDiskKey,
								ControllerKey: scsiCtrlKey,
								UnitNumber:    &fcdUnitNumber,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
										FileName: "[datastore1] my-vm/my-fcd.vmdk",
									},
									Uuid: "fcd-uuid-1",
								},
							},
							VDiskId: &vimtypes.ID{Id: "fcd-id-1"}, // This makes it an FCD
						},
					}

					// Set up the VM spec with a volume that references the FCD
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "my-fcd-volume",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-fcd-pvc",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To[int32](0),
							UnitNumber:          ptr.To(fcdUnitNumber),
						},
					}

					// Create the PVC with encrypted storage class
					pvc = &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: vm.Namespace,
							Name:      "my-fcd-pvc",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							StorageClassName: ptr.To(storageClass2.Name),
						},
					}

					// Create a separate encryption class for FCDs
					fcdEncClass = &byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: vm.Namespace,
							Name:      "my-fcd-encryption-class",
						},
						Spec: byokv1.EncryptionClassSpec{
							KeyProvider: provider2ID,
							KeyID:       provider2Key1ID,
						},
					}

					withObjs = append(withObjs, pvc, fcdEncClass)
				})

				When("FCD has encryption class annotation", func() {
					BeforeEach(func() {
						pvc.Annotations = map[string]string{
							pkgconst.PVCEncryptionClassNameAnnotation: fcdEncClass.Name,
						}
					})

					When("FCD is not encrypted", func() {
						It("should encrypt the FCD", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(configSpec.DeviceChange).To(HaveLen(1))
							devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
							Expect(devSpec).ToNot(BeNil())
							Expect(devSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
							Expect(devSpec.Backing).ToNot(BeNil())
							cryptoSpec, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecEncrypt)
							Expect(ok).To(BeTrue())
							Expect(cryptoSpec.CryptoKeyId.KeyId).To(Equal(provider2Key1ID))
							Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider2ID))
						})
					})

					When("FCD is already encrypted with same key", func() {
						BeforeEach(func() {
							// Set the FCD as already encrypted with the same key
							disk := moVM.Config.Hardware.Device[1].(*vimtypes.VirtualDisk)
							disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).KeyId = &vimtypes.CryptoKeyId{
								KeyId: provider2Key1ID,
								ProviderId: &vimtypes.KeyProviderId{
									Id: provider2ID,
								},
							}
						})

						It("should not add device changes", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(configSpec.DeviceChange).To(BeEmpty())
						})
					})

					When("FCD is encrypted with different key", func() {
						BeforeEach(func() {
							// Set the FCD as encrypted with a different key
							disk := moVM.Config.Hardware.Device[1].(*vimtypes.VirtualDisk)
							disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).KeyId = &vimtypes.CryptoKeyId{
								KeyId: provider1Key1ID,
								ProviderId: &vimtypes.KeyProviderId{
									Id: provider1ID,
								},
							}
						})

						It("should recrypt the FCD", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(configSpec.DeviceChange).To(HaveLen(1))
							devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
							Expect(devSpec).ToNot(BeNil())
							Expect(devSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
							Expect(devSpec.Backing).ToNot(BeNil())
							cryptoSpec, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
							Expect(ok).To(BeTrue())
							Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(provider2Key1ID))
							Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider2ID))
						})
					})
				})

				When("FCD has no encryption class annotation", func() {
					When("there is a default key provider", func() {
						BeforeEach(func() {
							Expect(cryptoManager.MarkDefault(ctx, provider3ID)).To(Succeed())
						})

						When("FCD is not encrypted", func() {
							It("should encrypt the FCD with default provider", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(HaveLen(1))
								devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
								Expect(devSpec).ToNot(BeNil())
								Expect(devSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
								Expect(devSpec.Backing).ToNot(BeNil())
								cryptoSpec, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecEncrypt)
								Expect(ok).To(BeTrue())
								Expect(cryptoSpec.CryptoKeyId.KeyId).To(BeEmpty()) // Default provider generates keys on-demand
								Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider3ID))
							})
						})

						When("FCD is already encrypted with default provider", func() {
							BeforeEach(func() {
								// Set the FCD as already encrypted with the default provider
								disk := moVM.Config.Hardware.Device[1].(*vimtypes.VirtualDisk)
								disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).KeyId = &vimtypes.CryptoKeyId{
									KeyId: "some-generated-key",
									ProviderId: &vimtypes.KeyProviderId{
										Id: provider3ID,
									},
								}
							})

							It("should not add device changes", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(BeEmpty())
							})
						})
					})

					When("there is no default key provider", func() {
						It("should return an error", func() {
							Expect(err).To(MatchError(pkgcrypto.ErrNoDefaultKeyProvider))
						})
					})
				})

				When("FCD storage class does not support encryption", func() {
					BeforeEach(func() {
						pvc.Spec.StorageClassName = ptr.To(storageClass1.Name)
						pvc.Annotations = map[string]string{
							"encryption.vmware.com/encryption-class-name": fcdEncClass.Name,
						}
					})

					It("should not encrypt the FCD", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.DeviceChange).To(BeEmpty())
					})
				})

				When("PVC has no storage class name", func() {
					BeforeEach(func() {
						pvc.Spec.StorageClassName = nil
						pvc.Annotations = map[string]string{
							pkgconst.PVCEncryptionClassNameAnnotation: fcdEncClass.Name,
						}
					})

					It("should skip the FCD", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.DeviceChange).To(BeEmpty())
					})
				})

				When("PVC has empty storage class name", func() {
					BeforeEach(func() {
						pvc.Spec.StorageClassName = ptr.To("")
						pvc.Annotations = map[string]string{
							pkgconst.PVCEncryptionClassNameAnnotation: fcdEncClass.Name,
						}
					})

					It("should skip the FCD", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.DeviceChange).To(BeEmpty())
					})
				})

				When("PVC is not found", func() {
					BeforeEach(func() {
						// Remove the PVC from withObjs so it won't be created.
						// We need to rebuild withObjs without the pvc.
						newObjs := make([]ctrlclient.Object, 0)
						for _, o := range withObjs {
							if o != pvc {
								newObjs = append(newObjs, o)
							}
						}
						withObjs = newObjs
					})

					It("should return ErrPVCNotFound", func() {
						Expect(err).To(MatchError(pkgcrypto.ErrPVCNotFound))
					})
				})

				When("FCD encryption class annotation refers to non-existent EncryptionClass", func() {
					BeforeEach(func() {
						pvc.Annotations = map[string]string{
							pkgconst.PVCEncryptionClassNameAnnotation: "non-existent-enc-class",
						}
					})

					It("should return a not-found error", func() {
						Expect(err).To(HaveOccurred())
						Expect(apierrors.IsNotFound(err)).To(BeTrue())
					})
				})

				When("volume has no PersistentVolumeClaim", func() {
					BeforeEach(func() {
						// Replace the VM's volume with one that has no PVC
						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name:                       "my-fcd-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									// No PersistentVolumeClaim set
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To[int32](0),
								UnitNumber:          ptr.To(fcdUnitNumber),
							},
						}
					})

					It("should skip the FCD without error", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.DeviceChange).To(BeEmpty())
					})
				})

				When("there are multiple FCDs with mixed encryption states", func() {
					var (
						pvc2           *corev1.PersistentVolumeClaim
						fcdDiskKey2    int32 = 2001
						fcdUnitNumber2 int32 = 2
					)

					BeforeEach(func() {
						// Add a second FCD disk
						moVM.Config.Hardware.Device = append(
							moVM.Config.Hardware.Device,
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Key:           fcdDiskKey2,
									ControllerKey: scsiCtrlKey,
									UnitNumber:    &fcdUnitNumber2,
									Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: "[datastore1] my-vm/my-fcd-2.vmdk",
										},
										Uuid: "fcd-uuid-2",
									},
								},
								VDiskId: &vimtypes.ID{Id: "fcd-id-2"},
							},
						)

						// Add a second volume
						vm.Spec.Volumes = append(vm.Spec.Volumes,
							vmopv1.VirtualMachineVolume{
								Name: "my-fcd-volume-2",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "my-fcd-pvc-2",
										},
									},
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To[int32](0),
								UnitNumber:          ptr.To(fcdUnitNumber2),
							},
						)

						// First PVC has encryption class annotation
						pvc.Annotations = map[string]string{
							pkgconst.PVCEncryptionClassNameAnnotation: fcdEncClass.Name,
						}

						// Second PVC also has encrypted storage class but no encryption
						// class annotation and no default provider - should trigger error
						pvc2 = &corev1.PersistentVolumeClaim{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: vm.Namespace,
								Name:      "my-fcd-pvc-2",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								StorageClassName: ptr.To(storageClass2.Name),
							},
						}
						withObjs = append(withObjs, pvc2)
					})

					When("second FCD has no enc class and no default provider", func() {
						It("should return ErrNoDefaultKeyProvider", func() {
							Expect(err).To(MatchError(pkgcrypto.ErrNoDefaultKeyProvider))
						})
					})

					When("second FCD has encryption class annotation too", func() {
						BeforeEach(func() {
							pvc2.Annotations = map[string]string{
								pkgconst.PVCEncryptionClassNameAnnotation: fcdEncClass.Name,
							}
						})

						When("both FCDs are not encrypted", func() {
							It("should encrypt both FCDs", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(HaveLen(2))
								for _, dc := range configSpec.DeviceChange {
									devSpec := dc.GetVirtualDeviceConfigSpec()
									Expect(devSpec).ToNot(BeNil())
									Expect(devSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
									Expect(devSpec.Backing).ToNot(BeNil())
									cryptoSpec, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecEncrypt)
									Expect(ok).To(BeTrue())
									Expect(cryptoSpec.CryptoKeyId.KeyId).To(Equal(provider2Key1ID))
									Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider2ID))
								}
							})
						})

						When("first FCD is already encrypted, second is not", func() {
							BeforeEach(func() {
								disk := moVM.Config.Hardware.Device[1].(*vimtypes.VirtualDisk)
								disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).KeyId = &vimtypes.CryptoKeyId{
									KeyId: provider2Key1ID,
									ProviderId: &vimtypes.KeyProviderId{
										Id: provider2ID,
									},
								}
							})

							It("should only encrypt the second FCD", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(HaveLen(1))
								devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
								Expect(devSpec).ToNot(BeNil())
								Expect(devSpec.Device.GetVirtualDevice().Key).To(Equal(fcdDiskKey2))
							})
						})
					})
				})

				When("FCD is encrypted with same provider but different key", func() {
					BeforeEach(func() {
						pvc.Annotations = map[string]string{
							pkgconst.PVCEncryptionClassNameAnnotation: fcdEncClass.Name,
						}
						// Set the FCD as encrypted with the same provider but different key
						disk := moVM.Config.Hardware.Device[1].(*vimtypes.VirtualDisk)
						disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).KeyId = &vimtypes.CryptoKeyId{
							KeyId: "some-other-key-id",
							ProviderId: &vimtypes.KeyProviderId{
								Id: provider2ID, // Same provider as fcdEncClass
							},
						}
					})

					It("should recrypt the FCD with the correct key", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.DeviceChange).To(HaveLen(1))
						devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
						Expect(devSpec).ToNot(BeNil())
						Expect(devSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
						Expect(devSpec.Backing).ToNot(BeNil())
						cryptoSpec, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
						Expect(ok).To(BeTrue())
						Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(provider2Key1ID))
						Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider2ID))
					})
				})
			})
		})

	})

})

const (
	shouldSetEncryptionSyncedToFalse            = "should set EncryptionSynced to false"
	shouldSetEncryptionSyncedWithInvalidState   = "should set EncryptionSynced to false w InvalidState"
	shouldSetEncryptionSyncedWithInvalidChanges = "should set EncryptionSynced to false w InvalidChanges"
)

func getSnapshotInfoWithTree() *vimtypes.VirtualMachineSnapshotInfo {
	return &vimtypes.VirtualMachineSnapshotInfo{
		CurrentSnapshot: &vimtypes.ManagedObjectReference{},
		RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
			{
				Name: "1",
				ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
					{
						Name: "2a",
						ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
							{
								Name: "2ai",
							},
						},
					},
					{
						Name: "2b",
						ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
							{
								Name: "2bi",
							},
							{
								Name: "2bii",
							},
						},
					},
				},
			},
		},
	}
}

func getSnapshotInfoWithLinearChain() *vimtypes.VirtualMachineSnapshotInfo {
	return &vimtypes.VirtualMachineSnapshotInfo{
		CurrentSnapshot: &vimtypes.ManagedObjectReference{},
		RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
			{
				Name: "1",
				ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
					{
						Name: "1a",
					},
				},
			},
		},
	}
}
