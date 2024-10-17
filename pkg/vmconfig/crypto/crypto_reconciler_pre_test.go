// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
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
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
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
			context.Background(), builder.VCSimTestConfig{})
		ctx = vcsimCtx
		ctx = r.WithContext(ctx)
		ctx = vmconfig.WithContext(ctx)

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

		When("moVM.config is nil", func() {
			BeforeEach(func() {
				moVM.Config = nil
			})
			It("should not panic", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("spec.crypto is nil", func() {
			BeforeEach(func() {
				vm.Spec.Crypto = nil
			})

			It("should not return an error, set a condition, or update the config spec", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
				Expect(configSpec).To(Equal(&vimtypes.VirtualMachineConfigSpec{}))
			})
		})

		When("encryptionClassName is empty", func() {

			BeforeEach(func() {
				vm.Spec.Crypto.EncryptionClassName = ""
			})

			When("there is not default key provider", func() {

				Context("vm is not encrypted", func() {
					BeforeEach(func() {
						moVM.Config.KeyId = nil
						vm.Spec.StorageClass = storageClass1.Name
					})

					When("using encryption storage class", func() {
						BeforeEach(func() {
							vm.Spec.StorageClass = storageClass2.Name
						})
						It(shouldSetEncryptionSyncedWithInvalidState, func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidState).String()))
							Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating unencrypted", "not use encryption storage class")))
						})

						When("have vtpm", func() {
							BeforeEach(func() {
								moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualTPM{},
								}
							})
							It(shouldSetEncryptionSyncedWithInvalidState, func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).ToNot(BeNil())
								Expect(c.Status).To(Equal(metav1.ConditionFalse))
								Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidState).String()))
								Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating unencrypted", "not use encryption storage class and not have vTPM")))
							})
						})

						When("adding vtpm", func() {
							BeforeEach(func() {
								configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
									&vimtypes.VirtualDeviceConfigSpec{
										Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
										Device:    &vimtypes.VirtualTPM{},
									},
								}
							})
							It(shouldSetEncryptionSyncedWithInvalidChanges, func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).ToNot(BeNil())
								Expect(c.Status).To(Equal(metav1.ConditionFalse))
								Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges | pkgcrypto.ReasonInvalidState).String()))
								Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating unencrypted", "not use encryption storage class and not add vTPM")))
							})
						})
					})

					When("have vtpm", func() {
						BeforeEach(func() {
							moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualTPM{},
							}
						})
						It(shouldSetEncryptionSyncedWithInvalidChanges, func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidState).String()))
							Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating unencrypted", "not have vTPM")))
						})
					})

					When("adding vtpm", func() {
						BeforeEach(func() {
							configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
								&vimtypes.VirtualDeviceConfigSpec{
									Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
									Device:    &vimtypes.VirtualTPM{},
								},
							}
						})
						It(shouldSetEncryptionSyncedWithInvalidChanges, func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges).String()))
							Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating unencrypted", "not add vTPM")))
						})
					})

					When("spec.crypto.UseDefaultKeyProvider=nil", func() {
						BeforeEach(func() {
							vm.Spec.Crypto.UseDefaultKeyProvider = nil
						})
						It("should not return an error, set a condition, or update the config spec", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
							Expect(configSpec).To(Equal(&vimtypes.VirtualMachineConfigSpec{}))
						})
					})

					When("spec.crypto.UseDefaultKeyProvider=false", func() {
						BeforeEach(func() {
							*vm.Spec.Crypto.UseDefaultKeyProvider = false
						})
						It("should not return an error, set a condition, or update the config spec", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
							Expect(configSpec).To(Equal(&vimtypes.VirtualMachineConfigSpec{}))
						})
					})

				})

				Context("encrypted vm", func() {
					When("modifying secret key", func() {
						DescribeTable(
							shouldSetEncryptionSyncedWithInvalidChanges,
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
				})
			})

			When("there is default key provider", func() {

				BeforeEach(func() {
					Expect(cryptoManager.MarkDefault(ctx, provider3ID)).To(Succeed())
				})

				When("vm is not encrypted", func() {
					BeforeEach(func() {
						moVM.Config.KeyId = nil
					})

					When("default provider is not native", func() {
						BeforeEach(func() {
							Expect(cryptoManager.MarkDefault(ctx, provider2ID)).To(Succeed())
						})
						It("should encrypt the VM", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
							Expect(configSpec.Crypto).ToNot(BeNil())
							cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
							Expect(ok).To(BeTrue())
							Expect(cryptoSpec.CryptoKeyId.KeyId).To(BeEmpty())
							Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider2ID))
						})
					})

					It("should encrypt the VM", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
						Expect(configSpec.Crypto).ToNot(BeNil())
						cryptoSpec, ok := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)
						Expect(ok).To(BeTrue())
						Expect(cryptoSpec.CryptoKeyId.KeyId).To(BeEmpty())
						Expect(cryptoSpec.CryptoKeyId.ProviderId.Id).To(Equal(provider3ID))
					})

					When("storage class is not encrypted and there is no vTPM", func() {
						BeforeEach(func() {
							vm.Spec.StorageClass = storageClass1.Name
						})
						It("should return an error", func() {
							Expect(err).To(MatchError("encrypting vm requires compatible storage class or vTPM"))
						})
					})
				})

				When("vm is encrypted", func() {
					When("recrypting vm", func() {
						BeforeEach(func() {
							moVM.Config.KeyId = &vimtypes.CryptoKeyId{
								KeyId: "123",
								ProviderId: &vimtypes.KeyProviderId{
									Id: provider2ID,
								},
							}
						})

						When("storage class is not encrypted and there is no vTPM", func() {
							BeforeEach(func() {
								vm.Spec.StorageClass = storageClass1.Name
							})
							It("should return an error", func() {
								Expect(err).To(MatchError("recrypting vm requires compatible storage class or vTPM"))
							})
						})
					})
					When("updating encrypted vm", func() {
						BeforeEach(func() {
							moVM.Config.KeyId = &vimtypes.CryptoKeyId{
								ProviderId: &vimtypes.KeyProviderId{
									Id: provider3ID,
								},
							}
						})
						When("storage class is not encrypted and there is no vTPM", func() {
							BeforeEach(func() {
								vm.Spec.StorageClass = storageClass1.Name
							})
							It("should return an error", func() {
								Expect(err).To(MatchError("updating encrypted vm requires compatible storage class or vTPM"))
							})
						})
					})
				})
			})

		})

		When("encryptionClassName is not empty", func() {

			When("encryption class does not exist", func() {
				BeforeEach(func() {
					withObjs = []ctrlclient.Object{
						storageClass1,
						storageClass2,
						vm,
					}
				})
				It(shouldSetEncryptionSyncedToFalse, func() {
					Expect(err).ToNot(HaveOccurred())
					c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassNotFound.String()))
					Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "specify encryption class that exists")))
				})
			})

			When("encryption class does exist", func() {

				When("there is a non-404 error getting the encryption class", func() {
					BeforeEach(func() {
						withFuncs.Get = func(
							ctx context.Context,
							client ctrlclient.WithWatch,
							key ctrlclient.ObjectKey,
							obj ctrlclient.Object,
							opts ...ctrlclient.GetOption) error {

							if _, ok := obj.(*byokv1.EncryptionClass); ok {
								return errors.New("fake")
							}
							return client.Get(ctx, key, obj, opts...)
						}
					})
					It("should return an error", func() {
						Expect(err).To(MatchError("fake"))
					})
				})

				When("encryption class is not ready", func() {
					When("providerID is empty", func() {
						BeforeEach(func() {
							encClass.Spec.KeyProvider = ""
						})
						It(shouldSetEncryptionSyncedToFalse, func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassInvalid.String()))
							Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "specify encryption class with a non-empty provider")))
						})
					})
					When("providerID is invalid", func() {
						BeforeEach(func() {
							encClass.Spec.KeyProvider = "invalid-provider-id"
						})
						It(shouldSetEncryptionSyncedToFalse, func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassInvalid.String()))
							Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "specify encryption class with a valid provider")))
						})
					})

					When("keyID is invalid", func() {
						BeforeEach(func() {
							encClass.Spec.KeyID = "invalid-key-id"
						})
						It(shouldSetEncryptionSyncedToFalse, func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassInvalid.String()))
							Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "specify encryption class with a valid key")))
						})
					})

					When("providerID and keyID are invalid", func() {
						BeforeEach(func() {
							encClass.Spec.KeyProvider = "invalid-provider-id"
							encClass.Spec.KeyID = "invalid-key-id"
						})
						It(shouldSetEncryptionSyncedToFalse, func() {
							Expect(err).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
							Expect(c).ToNot(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(pkgcrypto.ReasonEncryptionClassInvalid.String()))
							Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "specify encryption class with a valid provider and specify encryption class with a valid key")))
						})
					})
				})

				When("encryption class is ready", func() {

					When("updating encrypted", func() {
						BeforeEach(func() {
							moVM.Config.KeyId = &vimtypes.CryptoKeyId{
								KeyId: encClass.Spec.KeyID,
								ProviderId: &vimtypes.KeyProviderId{
									Id: encClass.Spec.KeyProvider,
								},
							}
						})

						When("storage class is not encrypted and no vTPM", func() {
							BeforeEach(func() {
								vm.Spec.StorageClass = storageClass1.Name
							})
							When("storage class is not encrypted and no vTPM", func() {
								It(shouldSetEncryptionSyncedToFalse, func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "use encryption storage class or have vTPM")))
								})
								When("has vTPM being removed", func() {
									BeforeEach(func() {
										moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
											&vimtypes.VirtualTPM{},
										}
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
										Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
										Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "use encryption storage class or have vTPM")))
									})
								})
							})
						})

						When("no changes", func() {
							It("should not return an error or set condition", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
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
												ProfileData: &vimtypes.VirtualMachineProfileRawData{
													ExtensionKey: "com.vmware.vim.sips",
													ObjectData:   profileWithIOFilters,
												},
											},
										},
									},
								}
							})
							It("should not return an error or set condition", func() {
								Expect(err).ToNot(HaveOccurred())
								Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
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
							It(shouldSetEncryptionSyncedWithInvalidChanges, func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
								Expect(c).ToNot(BeNil())
								Expect(c.Status).To(Equal(metav1.ConditionFalse))
								Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges).String()))
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
							It(shouldSetEncryptionSyncedWithInvalidChanges, func() {
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
												ProfileData: &vimtypes.VirtualMachineProfileRawData{
													ExtensionKey: "com.vmware.vim.sips",
													ObjectData:   profileWithIOFilters,
												},
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
												ProfileData: &vimtypes.VirtualMachineProfileRawData{
													ExtensionKey: "com.vmware.vim.sips",
													ObjectData:   profileWithIOFilters,
												},
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
												ProfileData: &vimtypes.VirtualMachineProfileRawData{
													ExtensionKey: "com.vmware.vim.sips",
													ObjectData:   profileWithIOFilters,
												},
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
								Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("updating encrypted", "not encrypt devices with multiple backings")))
							})
						})
					})

					When("encrypting", func() {

						assertIsEncrypt := func() {
							ExpectWithOffset(1, err).ToNot(HaveOccurred())
							ExpectWithOffset(1, conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeNil())
							ExpectWithOffset(1, configSpec.Crypto).ToNot(BeNil())
							ExpectWithOffset(1, configSpec.Crypto).To(Equal(&vimtypes.CryptoSpecEncrypt{
								CryptoKeyId: vimtypes.CryptoKeyId{
									KeyId: encClass.Spec.KeyID,
									ProviderId: &vimtypes.KeyProviderId{
										Id: encClass.Spec.KeyProvider,
									},
								},
							}))
						}

						BeforeEach(func() {
							moVM.Config.KeyId = nil
						})

						When("storage class is not encrypted and no vTPM", func() {
							BeforeEach(func() {
								vm.Spec.StorageClass = storageClass1.Name
							})
							When("storage class is not encrypted and no vTPM", func() {
								It(shouldSetEncryptionSyncedToFalse, func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("encrypting", "use encryption storage class or have vTPM")))
								})
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

						When("vm is powered on", func() {
							BeforeEach(func() {
								moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
							})
							It(shouldSetEncryptionSyncedWithInvalidState, func() {
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
						When("vm is powered off with no snapshots", func() {
							It("should encrypt the vm", func() {
								assertIsEncrypt()
							})
						})
					})
					When("recrypting", func() {

						When("new providerID", func() {
							BeforeEach(func() {
								encClass.Spec.KeyProvider = provider2ID
							})
							When("vm has snapshot tree", func() {
								BeforeEach(func() {
									moVM.Snapshot = getSnapshotInfoWithTree()
								})
								It(shouldSetEncryptionSyncedWithInvalidState, func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "not have snapshot tree")))
								})
							})
						})
						When("new keyID", func() {
							BeforeEach(func() {
								encClass.Spec.KeyID = provider1Key2ID
							})
							When("vm has snapshot tree", func() {
								BeforeEach(func() {
									moVM.Snapshot = getSnapshotInfoWithTree()
								})
								It(shouldSetEncryptionSyncedWithInvalidState, func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "not have snapshot tree")))
								})
							})
						})
						When("new providerID and new keyID", func() {
							BeforeEach(func() {
								encClass.Spec.KeyProvider = provider2ID
								encClass.Spec.KeyID = provider2Key1ID
							})
							When("vm has snapshot tree", func() {
								BeforeEach(func() {
									moVM.Snapshot = getSnapshotInfoWithTree()
								})
								It(shouldSetEncryptionSyncedWithInvalidState, func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "not have snapshot tree")))
								})
							})

							assertIsRecrypt := func() {
								ExpectWithOffset(1, err).ToNot(HaveOccurred())
								ExpectWithOffset(1, conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeNil())
								ExpectWithOffset(1, configSpec.Crypto).ToNot(BeNil())
								ExpectWithOffset(1, configSpec.Crypto).To(Equal(&vimtypes.CryptoSpecShallowRecrypt{
									NewKeyId: vimtypes.CryptoKeyId{
										KeyId: encClass.Spec.KeyID,
										ProviderId: &vimtypes.KeyProviderId{
											Id: encClass.Spec.KeyProvider,
										},
									},
								}))
							}

							When("storage class is not encrypted and no vTPM", func() {
								BeforeEach(func() {
									vm.Spec.StorageClass = storageClass1.Name
								})
								When("storage class is not encrypted and no vTPM", func() {
									It(shouldSetEncryptionSyncedToFalse, func() {
										Expect(err).ToNot(HaveOccurred())
										c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
										Expect(c).ToNot(BeNil())
										Expect(c.Status).To(Equal(metav1.ConditionFalse))
										Expect(c.Reason).To(Equal(pkgcrypto.ReasonInvalidState.String()))
										Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "use encryption storage class or have vTPM")))
									})
								})
							})

							When("powered off", func() {
								BeforeEach(func() {
									moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOff
								})
								It("should recrypt the vm", func() {
									assertIsRecrypt()
								})
								When("has linear snapshot chain", func() {
									BeforeEach(func() {
										moVM.Snapshot = getSnapshotInfoWithLinearChain()
									})
									It("should recrypt the vm", func() {
										assertIsRecrypt()
									})
								})
							})
							When("powered on", func() {
								BeforeEach(func() {
									moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
								})
								It("should recrypt the vm", func() {
									assertIsRecrypt()
								})
								When("has linear snapshot chain", func() {
									BeforeEach(func() {
										moVM.Snapshot = getSnapshotInfoWithLinearChain()
									})
									It("should recrypt the vm", func() {
										assertIsRecrypt()
									})
								})
							})
							When("suspended", func() {
								BeforeEach(func() {
									moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStateSuspended
								})
								It("should recrypt the vm", func() {
									assertIsRecrypt()
								})
								When("has linear snapshot chain", func() {
									BeforeEach(func() {
										moVM.Snapshot = getSnapshotInfoWithLinearChain()
									})
									It("should recrypt the vm", func() {
										assertIsRecrypt()
									})
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
										},
									}
								})
								It(shouldSetEncryptionSyncedWithInvalidChanges, func() {
									Expect(err).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal((pkgcrypto.ReasonInvalidChanges).String()))
									Expect(c.Message).To(Equal(pkgcrypto.SprintfStateNotSynced("recrypting", "specify policy when encrypting devices")))
								})
							})
						})

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
						Name: "1a",
					},
					{
						Name: "1b",
					},
				},
			},
			{
				Name: "2",
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

const profileWithIOFilters = `<?xml version="1.0" encoding="UTF-8"?>
<storageProfile xsi:type="StorageProfile">
    <constraints>
        <subProfiles>
            <capability>
                <capabilityId>
                    <id>vmwarevmcrypt@encryption</id>
                    <namespace>IOFILTERS</namespace>
                    <constraint></constraint>
                </capabilityId>
            </capability>
            <name>Rule-Set 1: IOFILTERS</name>
        </subProfiles>
    </constraints>
    <createdBy>None</createdBy>
    <creationTime>1970-01-01T00:00:00Z</creationTime>
    <lastUpdatedTime>1970-01-01T00:00:00Z</lastUpdatedTime>
    <generationId>1</generationId>
    <name>None</name>
    <profileId>Phony Profile ID</profileId>
</storageProfile>`
