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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
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

		// vacEncrypted refers to an encrypted storage policy and vacPlain to a
		// storage policy that is not encrypted.
		vacEncrypted *storagev1.VolumeAttributesClass
		vacPlain     *storagev1.VolumeAttributesClass
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

		var (
			vacEncryptedProfileID = uuid.NewString()
			vacPlainProfileID     = uuid.NewString()
		)
		vacEncrypted = &storagev1.VolumeAttributesClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-vac-encrypted",
				UID:  types.UID(uuid.NewString()),
			},
			Parameters: map[string]string{
				"storagePolicyID": vacEncryptedProfileID,
			},
		}
		vacPlain = &storagev1.VolumeAttributesClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-vac-plain",
				UID:  types.UID(uuid.NewString()),
			},
			Parameters: map[string]string{
				"storagePolicyID": vacPlainProfileID,
			},
		}

		// A StoragePolicy must exist for every profile ID in order to determine
		// whether or not it is encrypted.
		vacEncryptedPolicy := &infrav1.StoragePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: pkgcfg.FromContext(ctx).PodNamespace,
				Name:      kubeutil.GetStoragePolicyObjectName(vacEncryptedProfileID),
			},
			Status: infrav1.StoragePolicyStatus{Encrypted: true},
		}
		vacPlainPolicy := &infrav1.StoragePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: pkgcfg.FromContext(ctx).PodNamespace,
				Name:      kubeutil.GetStoragePolicyObjectName(vacPlainProfileID),
			},
			Status: infrav1.StoragePolicyStatus{Encrypted: false},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{
			encClass,
			storageClass1,
			storageClass2,
			vacEncrypted,
			vacPlain,
			vacEncryptedPolicy,
			vacPlainPolicy,
			vm,
		}
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

					When("the vm uses a VolumeAttributesClass", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.StoragePolicyMutability = true
							})
							configSpec.DeviceChange = nil
						})

						When("the VolumeAttributesClass uses an encrypted storage policy but the StorageClass does not", func() {
							BeforeEach(func() {
								vm.Spec.StorageClass = storageClass1.Name
								vm.Spec.VolumeAttributesClassName = vacEncrypted.Name
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

						When("the VolumeAttributesClass uses a storage policy that is not encrypted but the StorageClass does", func() {
							BeforeEach(func() {
								vm.Spec.StorageClass = storageClass2.Name
								vm.Spec.VolumeAttributesClassName = vacPlain.Name
							})
							It("should deploy an unencrypted vm", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).To(BeNil())
								Expect(configSpec.Crypto).To(BeNil())
							})
						})

						When("the VolumeAttributesClass does not exist", func() {
							BeforeEach(func() {
								vm.Spec.StorageClass = storageClass2.Name
								vm.Spec.VolumeAttributesClassName = vacEncrypted.Name + fakeString
							})
							It("should return an error", func() {
								Expect(err).To(HaveOccurred())
								Expect(apierrors.IsNotFound(err)).To(BeTrue())
								Expect(err).To(MatchError(ContainSubstring(vm.Spec.VolumeAttributesClassName)))
								Expect(configSpec.Crypto).To(BeNil())
							})
						})

						When("the StoragePolicyMutability feature is disabled", func() {
							BeforeEach(func() {
								pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
									config.Features.StoragePolicyMutability = false
								})
								vm.Spec.StorageClass = storageClass1.Name
								vm.Spec.VolumeAttributesClassName = vacEncrypted.Name
							})
							It("should return an error", func() {
								Expect(err).To(MatchError(ContainSubstring("capability is not enabled")))
								Expect(configSpec.Crypto).To(BeNil())
							})
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
							// The storage policy of a VM that is under a VolumeAttributesClass is the
							// VolumeAttributesClass's. So a VM that the VMIM encrypted is in sync when
							// its StorageClass is not encrypted and it has no vTPM, and it does not get
							// an InvalidState.
							When("the vm is under a VolumeAttributesClass whose storage policy is encrypted but the StorageClass's is not", func() {
								BeforeEach(func() {
									pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
										config.Features.StoragePolicyMutability = true
									})
									vm.Spec.StorageClass = storageClass1.Name
									vm.Spec.VolumeAttributesClassName = vacEncrypted.Name
								})
								It("should set EncryptionSynced=true", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
									Expect(configSpec.Crypto).To(BeNil())
								})
							})

							// Encrypting the vm's disks writes the storage profile of the vm's storage
							// policy on them. The VMIM does that for a vm that is under a
							// VolumeAttributesClass, so this reconciler must not.
							When("fast deploy is enabled and the vm has a disk that is not encrypted", func() {
								BeforeEach(func() {
									pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
										config.Features.FastDeploy = true
									})
									moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
										&vimtypes.VirtualDisk{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 2000,
												Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
													VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
														FileName: "[datastore1] my-vm/my-vm.vmdk",
													},
												},
											},
										},
									}
								})

								When("the vm is not under a VolumeAttributesClass and its StorageClass is encrypted", func() {
									It("should encrypt the disk with the storage profile", func() {
										Expect(err).ToNot(HaveOccurred())
										Expect(configSpec.DeviceChange).To(HaveLen(1))
										devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
										Expect(devSpec).ToNot(BeNil())
										cryptoSpec, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecEncrypt)
										Expect(ok).To(BeTrue())
										Expect(cryptoSpec.CryptoKeyId.KeyId).To(Equal(provider1Key2ID))
										Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
										Expect(devSpec.Profile).To(HaveLen(1))
										profileSpec, ok := devSpec.Profile[0].(*vimtypes.VirtualMachineDefinedProfileSpec)
										Expect(ok).To(BeTrue())
										Expect(profileSpec.ProfileId).To(Equal(simulator.DefaultEncryptionProfileID))
									})
								})

								When("the vm is under a VolumeAttributesClass whose storage policy is encrypted but the StorageClass's is not", func() {
									BeforeEach(func() {
										pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
											config.Features.StoragePolicyMutability = true
										})
										vm.Spec.StorageClass = storageClass1.Name
										vm.Spec.VolumeAttributesClassName = vacEncrypted.Name
									})
									It("should not encrypt the disk or write a storage profile", func() {
										Expect(err).ToNot(HaveOccurred())
										Expect(configSpec.DeviceChange).To(BeEmpty())
									})
								})
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
						When("the vm uses a VolumeAttributesClass", func() {
							BeforeEach(func() {
								pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
									config.Features.StoragePolicyMutability = true
								})
							})

							// The VMIM, not this reconciler, encrypts an existing VM that is under a
							// VolumeAttributesClass whose storage policy is encrypted, unless the VM has
							// a vTPM. See reconcileArgs.underVAC.
							When("the VolumeAttributesClass uses an encrypted storage policy but the StorageClass does not", func() {
								BeforeEach(func() {
									vm.Spec.VolumeAttributesClassName = vacEncrypted.Name
								})
								It("should not encrypt the vm", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeNil())
									Expect(configSpec.Crypto).To(BeNil())
									Expect(configSpec.DeviceChange).To(BeEmpty())
								})

								// This reconciler does not encrypt the vm, so there is nothing for the
								// power state to hold, and no InvalidState.
								When("the vm is powered on", func() {
									BeforeEach(func() {
										moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
									})
									It("should not set EncryptionSynced", func() {
										Expect(err).ToNot(HaveOccurred())
										Expect(conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeNil())
										Expect(configSpec.Crypto).To(BeNil())
									})
								})

								// A vTPM encrypts the vm as it does when the vm is not under a
								// VolumeAttributesClass.
								When("the vm has a vtpm", func() {
									BeforeEach(func() {
										moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
											&vimtypes.VirtualTPM{},
										}
									})
									It("should encrypt the vm with the default key provider", func() {
										Expect(err).ToNot(HaveOccurred())
										Expect(conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeNil())
										cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
										Expect(ok).To(BeTrue())
										Expect(cryptoSpec.CryptoKeyId.KeyId).To(BeEmpty())
										Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
									})
								})

								When("the vm is adding a vtpm", func() {
									BeforeEach(func() {
										configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
											&vimtypes.VirtualDeviceConfigSpec{
												Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
												Device:    &vimtypes.VirtualTPM{},
											},
										}
									})
									It("should encrypt the vm with the default key provider", func() {
										Expect(err).ToNot(HaveOccurred())
										Expect(conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeNil())
										cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
										Expect(ok).To(BeTrue())
										Expect(cryptoSpec.CryptoKeyId.KeyId).To(BeEmpty())
										Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
									})
								})
							})

							When("the VolumeAttributesClass uses a storage policy that is not encrypted but the StorageClass does", func() {
								BeforeEach(func() {
									vm.Spec.StorageClass = storageClass2.Name
									vm.Spec.VolumeAttributesClassName = vacPlain.Name
								})
								It("should not encrypt the vm", func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).To(BeNil())
									Expect(configSpec.Crypto).To(BeNil())
								})
							})

							When("the VolumeAttributesClass does not exist", func() {
								BeforeEach(func() {
									vm.Spec.VolumeAttributesClassName = vacEncrypted.Name + fakeString
								})
								It("should return an error", func() {
									Expect(err).To(HaveOccurred())
									Expect(apierrors.IsNotFound(err)).To(BeTrue())
									Expect(err).To(MatchError(ContainSubstring(vm.Spec.VolumeAttributesClassName)))
									Expect(configSpec.Crypto).To(BeNil())
								})
							})

							When("the StoragePolicyMutability feature is disabled", func() {
								BeforeEach(func() {
									pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
										config.Features.StoragePolicyMutability = false
									})
									vm.Spec.VolumeAttributesClassName = vacEncrypted.Name
								})
								It("should return an error", func() {
									Expect(err).To(MatchError(ContainSubstring("capability is not enabled")))
									Expect(configSpec.Crypto).To(BeNil())
								})
							})
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

						// The VMIM, not this reconciler, encrypts an existing VM that is under a
						// VolumeAttributesClass whose storage policy is encrypted, with the resolved
						// key, unless the VM has a vTPM. See reconcileArgs.underVAC.
						When("the vm is under a VolumeAttributesClass whose storage policy is encrypted but the StorageClass's is not", func() {
							BeforeEach(func() {
								pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
									config.Features.StoragePolicyMutability = true
								})
								vm.Spec.StorageClass = storageClass1.Name
								vm.Spec.VolumeAttributesClassName = vacEncrypted.Name
							})
							It("should not encrypt the vm", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeNil())
								Expect(configSpec.Crypto).To(BeNil())
							})

							When("the vm has a vtpm", func() {
								BeforeEach(func() {
									moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
										&vimtypes.VirtualTPM{},
									}
								})
								It("should encrypt the vm with the key of the EncryptionClass", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeNil())
									cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
									Expect(ok).To(BeTrue())
									Expect(cryptoSpec.CryptoKeyId.KeyId).To(Equal(provider1Key1ID))
									Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider1ID))
								})
							})
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

						When("adding encrypted disk with missing policy", func() {
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
												ProfileId: "missing-policy",
											},
										},
									},
								}
							})
							It("should return a not found error", func() {
								Expect(err).To(HaveOccurred())
								Expect(err.Error()).To(ContainSubstring("storage class with profile ID missing-policy not found"))
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
								withObjs = append(withObjs, &storagev1.StorageClass{
									ObjectMeta: metav1.ObjectMeta{
										Name: "fake-storage-class",
									},
									Parameters: map[string]string{
										"storagePolicyID": fakeString,
									},
								})
								withObjs = append(withObjs, &infrav1.StoragePolicy{
									ObjectMeta: metav1.ObjectMeta{
										Namespace: pkgcfg.FromContext(ctx).PodNamespace,
										Name:      kubeutil.GetStoragePolicyObjectName(fakeString),
									},
									Status: infrav1.StoragePolicyStatus{
										Encrypted: false,
									},
								})
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

			// A VM that is under a VolumeAttributesClass is recrypted to the key this
			// reconciler resolves, as a VM that is not, and no storage profile is
			// written. A VMIM that moves the VM to another datastore keeps the VM's
			// current key, which can leave the VM with a key other than the resolved
			// one. The recrypt waits until the VMIM is done, i.e. until
			// status.volumeAttributesClassName is the VolumeAttributesClass in
			// spec.volumeAttributesClassName. See reconcileArgs.vacChangeInProgress.
			When("the vm is under a VolumeAttributesClass and has a key other than the resolved key", func() {

				// expectRecrypt asserts that the vm and its disk are recrypted with
				// the given key, and that no storage profile is written.
				expectRecrypt := func(providerID, keyID string) {
					GinkgoHelper()
					Expect(err).ToNot(HaveOccurred())
					cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
					Expect(ok).To(BeTrue())
					Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(providerID))
					Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(keyID))
					Expect(configSpec.DeviceChange).To(HaveLen(1))
					devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
					Expect(devSpec).ToNot(BeNil())
					Expect(devSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
					Expect(devSpec.Backing).ToNot(BeNil())
					Expect(devSpec.Backing.Crypto).To(Equal(configSpec.Crypto))
					Expect(devSpec.Profile).To(BeEmpty())
				}

				// expectNoRecrypt asserts that neither the vm nor its disk is
				// recrypted.
				expectNoRecrypt := func() {
					GinkgoHelper()
					Expect(err).ToNot(HaveOccurred())
					Expect(configSpec.Crypto).To(BeNil())
					Expect(configSpec.DeviceChange).To(BeEmpty())
				}

				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.StoragePolicyMutability = true
					})

					// The VolumeAttributesClass's storage policy is encrypted and the
					// StorageClass's is not, so the VM's storage policy is the
					// VolumeAttributesClass's.
					vm.Spec.StorageClass = storageClass1.Name
					vm.Spec.VolumeAttributesClassName = vacEncrypted.Name

					// The VM and its disk have the key of another provider than the
					// one of the VM's EncryptionClass, as they would after a VMIM
					// moved the VM to another datastore and kept the key.
					moVM.Config.KeyId = &vimtypes.CryptoKeyId{
						KeyId: provider2Key1ID,
						ProviderId: &vimtypes.KeyProviderId{
							Id: provider2ID,
						},
					}
					moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 2000,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
										FileName: "[datastore1] my-vm/my-vm.vmdk",
									},
									KeyId: &vimtypes.CryptoKeyId{
										KeyId: provider2Key1ID,
										ProviderId: &vimtypes.KeyProviderId{
											Id: provider2ID,
										},
									},
								},
							},
						},
					}
				})

				When("the VolumeAttributesClass in status is the one in the spec", func() {
					BeforeEach(func() {
						vm.Status.VolumeAttributesClassName = vacEncrypted.Name
					})
					It("should recrypt the vm with the key of the EncryptionClass", func() {
						expectRecrypt(provider1ID, provider1Key1ID)
					})
				})

				// The VolumeAttributesClass in status is only recorded after a VMIM is
				// done, so it is empty for a VM that was never moved.
				When("the VolumeAttributesClass in status is empty", func() {
					BeforeEach(func() {
						vm.Status.VolumeAttributesClassName = ""
					})
					It("should recrypt the vm with the key of the EncryptionClass", func() {
						expectRecrypt(provider1ID, provider1Key1ID)
					})
				})

				When("the VolumeAttributesClass in status is not the one in the spec", func() {
					BeforeEach(func() {
						vm.Status.VolumeAttributesClassName = vacPlain.Name
					})
					It("should not recrypt the vm or its disk", func() {
						expectNoRecrypt()
					})

					// The recrypt is only held back, not given up, so the vm is recrypted
					// as soon as the VMIM is done and the VolumeAttributesClass in status
					// is the one in the spec.
					When("the change is then done", func() {
						It("should recrypt the vm with the key of the EncryptionClass", func() {
							expectNoRecrypt()

							vm.Status.VolumeAttributesClassName = vacEncrypted.Name
							configSpec = &vimtypes.VirtualMachineConfigSpec{}
							err = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)

							expectRecrypt(provider1ID, provider1Key1ID)
						})
					})
				})

				// A VM that is not under a VolumeAttributesClass has no change to wait
				// for, whatever its status says.
				When("the vm is not under a VolumeAttributesClass", func() {
					BeforeEach(func() {
						vm.Spec.StorageClass = storageClass2.Name
						vm.Spec.VolumeAttributesClassName = ""
						vm.Status.VolumeAttributesClassName = vacPlain.Name
					})
					It("should recrypt the vm with the key of the EncryptionClass", func() {
						expectRecrypt(provider1ID, provider1Key1ID)
					})
				})

				When("the key of the EncryptionClass is of the same provider but another key", func() {
					BeforeEach(func() {
						moVM.Config.KeyId = &vimtypes.CryptoKeyId{
							KeyId: provider1Key2ID,
							ProviderId: &vimtypes.KeyProviderId{
								Id: provider1ID,
							},
						}
						moVM.Config.Hardware.Device[0].(*vimtypes.VirtualDisk).
							Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).KeyId = moVM.Config.KeyId
					})

					When("the VolumeAttributesClass in status is the one in the spec", func() {
						BeforeEach(func() {
							vm.Status.VolumeAttributesClassName = vacEncrypted.Name
						})
						It("should recrypt the vm with the key of the EncryptionClass", func() {
							expectRecrypt(provider1ID, provider1Key1ID)
						})
					})

					When("the VolumeAttributesClass in status is not the one in the spec", func() {
						BeforeEach(func() {
							vm.Status.VolumeAttributesClassName = vacPlain.Name
						})
						It("should not recrypt the vm or its disk", func() {
							expectNoRecrypt()
						})
					})
				})

				When("there is no EncryptionClass and the default key provider is another provider", func() {
					BeforeEach(func() {
						vm.Spec.Crypto = nil
						Expect(cryptoManager.MarkDefault(ctx, provider1ID)).To(Succeed())
					})

					When("the VolumeAttributesClass in status is the one in the spec", func() {
						BeforeEach(func() {
							vm.Status.VolumeAttributesClassName = vacEncrypted.Name
						})
						It("should recrypt the vm with the default key provider", func() {
							// The key is generated on demand when the provider is the default.
							expectRecrypt(provider1ID, "")
						})
					})

					When("the VolumeAttributesClass in status is not the one in the spec", func() {
						BeforeEach(func() {
							vm.Status.VolumeAttributesClassName = vacPlain.Name
						})
						It("should not recrypt the vm or its disk", func() {
							expectNoRecrypt()
						})
					})
				})

				// A vTPM encrypts the VM whatever its storage policy is, and the vm is
				// recrypted, and held back from it, in the same way.
				When("the VolumeAttributesClass's storage policy is not encrypted and the vm has a vtpm", func() {
					BeforeEach(func() {
						vm.Spec.VolumeAttributesClassName = vacPlain.Name
						moVM.Config.Hardware.Device = append(
							moVM.Config.Hardware.Device,
							&vimtypes.VirtualTPM{})
					})

					When("the VolumeAttributesClass in status is the one in the spec", func() {
						BeforeEach(func() {
							vm.Status.VolumeAttributesClassName = vacPlain.Name
						})
						It("should recrypt the vm with the key of the EncryptionClass", func() {
							expectRecrypt(provider1ID, provider1Key1ID)
						})
					})

					When("the VolumeAttributesClass in status is not the one in the spec", func() {
						BeforeEach(func() {
							vm.Status.VolumeAttributesClassName = vacEncrypted.Name
						})
						It("should not recrypt the vm or its disk", func() {
							expectNoRecrypt()
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

				// The PVC's own storage policy decides whether or not to encrypt
				// the FCD. It comes from the PVC's VolumeAttributesClass, which
				// takes precedence over the PVC's StorageClass, when the PVC
				// specifies one and the StoragePolicyMutability capability is
				// enabled.
				When("PVC specifies a VolumeAttributesClass", func() {
					BeforeEach(func() {
						pvc.Annotations = map[string]string{
							pkgconst.PVCEncryptionClassNameAnnotation: fcdEncClass.Name,
						}
					})

					When("the StoragePolicyMutability feature is enabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.StoragePolicyMutability = true
							})
						})

						When("the VolumeAttributesClass's storage policy is encrypted and the StorageClass's is not", func() {
							BeforeEach(func() {
								pvc.Spec.StorageClassName = ptr.To(storageClass1.Name)
								pvc.Spec.VolumeAttributesClassName = ptr.To(vacEncrypted.Name)
							})
							// The VMIM, not this reconciler, encrypts the FCD of a PVC that is under a
							// VolumeAttributesClass and applies the PVC's storage policy, so no key is
							// applied and no storage profile is written.
							It("should not encrypt the FCD or write a storage profile", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(BeEmpty())
							})

							// The FCD is skipped before its key is resolved, since there is no key to
							// apply, so a key that cannot be resolved is not an error.
							When("the PVC's EncryptionClass does not exist", func() {
								BeforeEach(func() {
									pvc.Annotations = map[string]string{
										pkgconst.PVCEncryptionClassNameAnnotation: "non-existent-enc-class",
									}
								})
								It("should not return an error", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(configSpec.DeviceChange).To(BeEmpty())
								})
							})

							When("there is no default key provider and the PVC has no EncryptionClass", func() {
								BeforeEach(func() {
									pvc.Annotations = nil
								})
								It("should not return an error", func() {
									Expect(err).ToNot(HaveOccurred())
									Expect(configSpec.DeviceChange).To(BeEmpty())
								})
							})
						})

						When("the VolumeAttributesClass's storage policy is not encrypted and the StorageClass's is", func() {
							BeforeEach(func() {
								pvc.Spec.StorageClassName = ptr.To(storageClass2.Name)
								pvc.Spec.VolumeAttributesClassName = ptr.To(vacPlain.Name)
							})
							It("should not encrypt the FCD", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(BeEmpty())
							})
						})

						When("the PVC does not specify a StorageClass", func() {
							BeforeEach(func() {
								pvc.Spec.StorageClassName = nil
								pvc.Spec.VolumeAttributesClassName = ptr.To(vacEncrypted.Name)
							})
							It("should not encrypt the FCD or write a storage profile", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(BeEmpty())
							})
						})

						When("the FCD is encrypted with a different key", func() {
							BeforeEach(func() {
								pvc.Spec.StorageClassName = ptr.To(storageClass1.Name)
								pvc.Spec.VolumeAttributesClassName = ptr.To(vacEncrypted.Name)

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
								cryptoSpec, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
								Expect(ok).To(BeTrue())
								Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(provider2Key1ID))
								Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider2ID))
							})
						})

						When("the VolumeAttributesClass does not exist", func() {
							BeforeEach(func() {
								pvc.Spec.StorageClassName = ptr.To(storageClass2.Name)
								pvc.Spec.VolumeAttributesClassName = ptr.To(vacEncrypted.Name + fakeString)
							})
							It("should return an error", func() {
								Expect(err).To(HaveOccurred())
								Expect(apierrors.IsNotFound(err)).To(BeTrue())
								Expect(err).To(MatchError(ContainSubstring(*pvc.Spec.VolumeAttributesClassName)))
								Expect(configSpec.DeviceChange).To(BeEmpty())
							})
						})
					})

					// The VolumeAttributesClass of a PVC is not considered when
					// the capability is not enabled, so the PVC's StorageClass
					// decides.
					When("the StoragePolicyMutability feature is disabled", func() {
						When("only the VolumeAttributesClass's storage policy is encrypted", func() {
							BeforeEach(func() {
								pvc.Spec.StorageClassName = ptr.To(storageClass1.Name)
								pvc.Spec.VolumeAttributesClassName = ptr.To(vacEncrypted.Name)
							})
							It("should not encrypt the FCD", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(BeEmpty())
							})
						})

						When("only the StorageClass's storage policy is encrypted", func() {
							BeforeEach(func() {
								pvc.Spec.StorageClassName = ptr.To(storageClass2.Name)
								pvc.Spec.VolumeAttributesClassName = ptr.To(vacPlain.Name)
							})
							It("should encrypt the FCD with the StorageClass's storage policy", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(HaveLen(1))
								devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
								Expect(devSpec).ToNot(BeNil())
								_, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecEncrypt)
								Expect(ok).To(BeTrue())
								Expect(devSpec.Profile).To(HaveLen(1))
								profileSpec, ok := devSpec.Profile[0].(*vimtypes.VirtualMachineDefinedProfileSpec)
								Expect(ok).To(BeTrue())
								Expect(profileSpec.ProfileId).To(Equal(simulator.DefaultEncryptionProfileID))
							})
						})

						When("the VolumeAttributesClass does not exist", func() {
							BeforeEach(func() {
								pvc.Spec.StorageClassName = ptr.To(storageClass2.Name)
								pvc.Spec.VolumeAttributesClassName = ptr.To(vacEncrypted.Name + fakeString)
							})
							It("should not return an error", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(configSpec.DeviceChange).To(HaveLen(1))
							})
						})
					})
				})

				// An FCD of a PVC that is under a VolumeAttributesClass is recrypted to
				// the PVC's key, as an FCD of a PVC that is not, and no storage profile
				// is written. The recrypt waits until the PVC's change to the
				// VolumeAttributesClass is done, i.e. until
				// status.currentVolumeAttributesClassName is the VolumeAttributesClass in
				// spec.volumeAttributesClassName. See isPVCVACChangeInProgress.
				When("PVC is under a VolumeAttributesClass and its FCD has a key other than the PVC's key", func() {

					// expectFCDRecrypt asserts that the FCD is recrypted with the given
					// key, and that no storage profile is written.
					expectFCDRecrypt := func(providerID, keyID string) {
						GinkgoHelper()
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.DeviceChange).To(HaveLen(1))
						devSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
						Expect(devSpec).ToNot(BeNil())
						Expect(devSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
						Expect(devSpec.Device.GetVirtualDevice().Key).To(Equal(fcdDiskKey))
						Expect(devSpec.Backing).ToNot(BeNil())
						cryptoSpec, ok := devSpec.Backing.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
						Expect(ok).To(BeTrue())
						Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(providerID))
						Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(keyID))
						Expect(devSpec.Profile).To(BeEmpty())
					}

					expectNoFCDRecrypt := func() {
						GinkgoHelper()
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.DeviceChange).To(BeEmpty())
					}

					BeforeEach(func() {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.StoragePolicyMutability = true
						})

						// The VolumeAttributesClass's storage policy is encrypted and the
						// StorageClass's is not, so the PVC's storage policy is the
						// VolumeAttributesClass's.
						pvc.Spec.StorageClassName = ptr.To(storageClass1.Name)
						pvc.Spec.VolumeAttributesClassName = ptr.To(vacEncrypted.Name)
						pvc.Annotations = map[string]string{
							pkgconst.PVCEncryptionClassNameAnnotation: fcdEncClass.Name,
						}

						// The FCD has the key of another provider than the one of the
						// PVC's EncryptionClass, as it would after a VMIM moved the VM to
						// another datastore and kept the key.
						disk := moVM.Config.Hardware.Device[1].(*vimtypes.VirtualDisk)
						disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).KeyId = &vimtypes.CryptoKeyId{
							KeyId: provider1Key1ID,
							ProviderId: &vimtypes.KeyProviderId{
								Id: provider1ID,
							},
						}
					})

					When("the PVC's current VolumeAttributesClass is the one in its spec", func() {
						BeforeEach(func() {
							pvc.Status.CurrentVolumeAttributesClassName = ptr.To(vacEncrypted.Name)
						})
						It("should recrypt the FCD with the key of the PVC's EncryptionClass", func() {
							expectFCDRecrypt(provider2ID, provider2Key1ID)
						})
					})

					// The current VolumeAttributesClass is not set for a PVC that nothing
					// has moved, so it is treated as in sync.
					When("the PVC has no current VolumeAttributesClass", func() {
						BeforeEach(func() {
							pvc.Status.CurrentVolumeAttributesClassName = nil
						})
						It("should recrypt the FCD with the key of the PVC's EncryptionClass", func() {
							expectFCDRecrypt(provider2ID, provider2Key1ID)
						})
					})

					When("the PVC's current VolumeAttributesClass is not the one in its spec", func() {
						BeforeEach(func() {
							pvc.Status.CurrentVolumeAttributesClassName = ptr.To(vacPlain.Name)
						})
						It("should not recrypt the FCD", func() {
							expectNoFCDRecrypt()
						})
					})

					When("the PVC has no EncryptionClass and there is a default key provider", func() {
						BeforeEach(func() {
							pvc.Annotations = nil
							Expect(cryptoManager.MarkDefault(ctx, provider3ID)).To(Succeed())
						})

						When("the PVC's current VolumeAttributesClass is the one in its spec", func() {
							BeforeEach(func() {
								pvc.Status.CurrentVolumeAttributesClassName = ptr.To(vacEncrypted.Name)
							})
							It("should recrypt the FCD with the default key provider", func() {
								// The key is generated on demand when the provider is the default.
								expectFCDRecrypt(provider3ID, "")
							})
						})

						When("the PVC's current VolumeAttributesClass is not the one in its spec", func() {
							BeforeEach(func() {
								pvc.Status.CurrentVolumeAttributesClassName = ptr.To(vacPlain.Name)
							})
							It("should not recrypt the FCD", func() {
								expectNoFCDRecrypt()
							})
						})
					})

					// Another VolumeAttributesClass in status is only a change in progress
					// for a PVC that is under a VolumeAttributesClass, so the PVC's
					// StorageClass decides when the capability is not enabled.
					When("the StoragePolicyMutability feature is disabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.StoragePolicyMutability = false
							})
							pvc.Spec.StorageClassName = ptr.To(storageClass2.Name)
							pvc.Status.CurrentVolumeAttributesClassName = ptr.To(vacPlain.Name)
						})
						It("should recrypt the FCD with the key of the PVC's EncryptionClass", func() {
							expectFCDRecrypt(provider2ID, provider2Key1ID)
						})
					})

					When("the PVC does not specify a VolumeAttributesClass", func() {
						BeforeEach(func() {
							pvc.Spec.StorageClassName = ptr.To(storageClass2.Name)
							pvc.Spec.VolumeAttributesClassName = nil
							pvc.Status.CurrentVolumeAttributesClassName = ptr.To(vacPlain.Name)
						})
						It("should recrypt the FCD with the key of the PVC's EncryptionClass", func() {
							expectFCDRecrypt(provider2ID, provider2Key1ID)
						})
					})

					// The vm and the PVC each wait for their own change, since each is
					// moved to its VolumeAttributesClass on its own.
					When("the vm is under a VolumeAttributesClass and has a key other than the resolved key", func() {
						BeforeEach(func() {
							vm.Spec.StorageClass = storageClass1.Name
							vm.Spec.VolumeAttributesClassName = vacEncrypted.Name
							moVM.Config.KeyId = &vimtypes.CryptoKeyId{
								KeyId: provider2Key1ID,
								ProviderId: &vimtypes.KeyProviderId{
									Id: provider2ID,
								},
							}
						})

						When("the vm's change is in progress and the PVC's is done", func() {
							BeforeEach(func() {
								vm.Status.VolumeAttributesClassName = vacPlain.Name
								pvc.Status.CurrentVolumeAttributesClassName = ptr.To(vacEncrypted.Name)
							})
							It("should recrypt the FCD but not the vm", func() {
								Expect(configSpec.Crypto).To(BeNil())
								expectFCDRecrypt(provider2ID, provider2Key1ID)
							})
						})

						When("the vm's change is done and the PVC's is in progress", func() {
							BeforeEach(func() {
								vm.Status.VolumeAttributesClassName = vacEncrypted.Name
								pvc.Status.CurrentVolumeAttributesClassName = ptr.To(vacPlain.Name)
							})
							It("should recrypt the vm but not the FCD", func() {
								expectNoFCDRecrypt()
								cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecShallowRecrypt)
								Expect(ok).To(BeTrue())
								Expect(cryptoSpec.NewKeyId.ProviderId.Id).To(Equal(provider1ID))
								Expect(cryptoSpec.NewKeyId.KeyId).To(Equal(provider1Key1ID))
							})
						})
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
