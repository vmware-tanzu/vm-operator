// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package crypto_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto/internal"
)

var _ = Describe("OnResult", Label(testlabels.Crypto), func() {
	var (
		r           vmconfig.ReconcilerWithContext
		ctx         context.Context
		moVM        mo.VirtualMachine
		vm          *vmopv1.VirtualMachine
		reconfigErr error
	)

	BeforeEach(func() {
		r = crypto.New()
		ctx = r.WithContext(context.Background())
		moVM.Config = &vimtypes.VirtualMachineConfigInfo{}
		vm = &vmopv1.VirtualMachine{}
		reconfigErr = nil
	})

	assertStateNotSynced := func(
		vm *vmopv1.VirtualMachine,
		expectedMessage string) {

		c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
		ExpectWithOffset(1, c).ToNot(BeNil())
		ExpectWithOffset(1, c.Status).To(Equal(metav1.ConditionFalse))
		ExpectWithOffset(1, c.Reason).To(Equal(crypto.ReasonReconfigureError.String()))
		ExpectWithOffset(1, c.Message).To(Equal(expectedMessage))
	}

	When("it should panic", func() {
		When("ctx is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.OnResult(ctx, vm, moVM, reconfigErr)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})

		When("moVM.config is nil", func() {
			BeforeEach(func() {
				moVM.Config = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.OnResult(ctx, vm, moVM, reconfigErr)
				}
				Expect(fn).To(PanicWith("moVM.config is nil"))
			})
		})

		When("vm is nil", func() {
			BeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.OnResult(ctx, vm, moVM, reconfigErr)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})
	})

	When("it should not panic", func() {
		var (
			err error
		)
		JustBeforeEach(func() {
			err = r.OnResult(ctx, vm, moVM, reconfigErr)
		})

		When("spec.crypto is nil", func() {
			BeforeEach(func() {
				vm.Spec.Crypto = nil
			})
			It("should not panic", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("reconfigErr does not include a fault", func() {
			BeforeEach(func() {
				reconfigErr = errors.New("fake")
			})
			It("should not panic", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("succeeded", func() {
			When("crypto was not involved", func() {
				BeforeEach(func() {
					internal.SetOperation(ctx, "updating unencrypted")
				})
				When("vm does not already have crypto synced condition", func() {
					BeforeEach(func() {
						conditions.Delete(vm, vmopv1.VirtualMachineEncryptionSynced)
					})
					It("should leave the condition alone", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
					})
				})
				When("vm does already have crypto synced condition", func() {
					When("existing condition is true", func() {
						BeforeEach(func() {
							conditions.MarkTrue(vm, vmopv1.VirtualMachineEncryptionSynced)
						})
						It("should leave the condition alone", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
						})
					})
					When("existing condition is false", func() {
						BeforeEach(func() {
							conditions.MarkFalse(vm, vmopv1.VirtualMachineEncryptionSynced, "fake", "fake")
						})
						It("should leave the condition alone", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
						})
					})
				})

				When("status.crypto is not nil", func() {
					BeforeEach(func() {
						vm.Status.Crypto = &vmopv1.VirtualMachineCryptoStatus{
							Encrypted: []vmopv1.VirtualMachineEncryptionType{
								"invalid1",
								"invalid2",
							},
							ProviderID: "invalid-provider-id",
							KeyID:      "invalid-key-id",
						}
					})
					When("vm is not encrypted", func() {
						It("should clear status.crypto", func() {
							Expect(vm.Status.Crypto).To(BeNil())
						})
					})
					When("vm was already encrypted using storage class or vTPM", func() {
						BeforeEach(func() {
							moVM.Config.KeyId = &vimtypes.CryptoKeyId{
								KeyId: "123",
								ProviderId: &vimtypes.KeyProviderId{
									Id: "abc",
								},
							}
						})

						When("vm is encrypted using storage class", func() {
							BeforeEach(func() {
								internal.MarkEncryptedStorageClass(ctx)
							})
							It("should update status.crypto", func() {
								Expect(vm.Status.Crypto).ToNot(BeNil())
								Expect(vm.Status.Crypto).To(Equal(&vmopv1.VirtualMachineCryptoStatus{
									Encrypted: []vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
										vmopv1.VirtualMachineEncryptionTypeDisks,
									},
									ProviderID: "abc",
									KeyID:      "123",
								}))
							})
						})
						When("vm is encrypted using vTPM", func() {
							BeforeEach(func() {
								moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualTPM{},
								}
							})
							It("should update status.crypto", func() {
								Expect(vm.Status.Crypto).ToNot(BeNil())
								Expect(vm.Status.Crypto).To(Equal(&vmopv1.VirtualMachineCryptoStatus{
									Encrypted: []vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
									},
									ProviderID: "abc",
									KeyID:      "123",
								}))
							})
						})
					})
				})

			})
			When("crypto was involved", func() {
				BeforeEach(func() {
					internal.SetOperation(ctx, "encrypting")
				})
				When("vm does not already have crypto synced condition", func() {
					BeforeEach(func() {
						conditions.Delete(vm, vmopv1.VirtualMachineEncryptionSynced)
					})
					It("should set the condition to true", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
					})
				})
				When("vm does already have crypto synced condition", func() {
					When("existing condition is true", func() {
						BeforeEach(func() {
							conditions.MarkTrue(vm, vmopv1.VirtualMachineEncryptionSynced)
						})
						It("should set the condition to true", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
						})
					})
					When("existing condition is false", func() {
						BeforeEach(func() {
							conditions.MarkFalse(vm, vmopv1.VirtualMachineEncryptionSynced, "fake", "fake")
						})
						It("should set the condition to true", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
						})
					})
				})
				When("vm was encrypted using storage class or vTPM", func() {
					BeforeEach(func() {
						moVM.Config.KeyId = &vimtypes.CryptoKeyId{
							KeyId: "123",
							ProviderId: &vimtypes.KeyProviderId{
								Id: "abc",
							},
						}
					})

					When("vm was encrypted using storage class", func() {
						BeforeEach(func() {
							internal.MarkEncryptedStorageClass(ctx)
						})
						It("should set status.crypto", func() {
							Expect(vm.Status.Crypto).ToNot(BeNil())
							Expect(vm.Status.Crypto).To(Equal(&vmopv1.VirtualMachineCryptoStatus{
								Encrypted: []vmopv1.VirtualMachineEncryptionType{
									vmopv1.VirtualMachineEncryptionTypeConfig,
									vmopv1.VirtualMachineEncryptionTypeDisks,
								},
								ProviderID: "abc",
								KeyID:      "123",
							}))
						})
					})
					When("vm was encrypted using vTPM", func() {
						BeforeEach(func() {
							moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualTPM{},
							}
						})
						It("should set status.crypto", func() {
							Expect(vm.Status.Crypto).ToNot(BeNil())
							Expect(vm.Status.Crypto).To(Equal(&vmopv1.VirtualMachineCryptoStatus{
								Encrypted: []vmopv1.VirtualMachineEncryptionType{
									vmopv1.VirtualMachineEncryptionTypeConfig,
								},
								ProviderID: "abc",
								KeyID:      "123",
							}))
						})
					})
				})
			})
		})

		When("failed", func() {
			Context("with a soap error", func() {
				BeforeEach(func() {
					reconfigErr = soap.WrapSoapFault(&soap.Fault{
						Detail: struct {
							Fault vimtypes.AnyType "xml:\",any,typeattr\""
						}{
							Fault: nil,
						},
					})
				})
				It("should not return an error or set any conditions", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
				})
			})

			Context("with a task error", func() {
				Context("and is a nil LocalizedMethodFault", func() {
					BeforeEach(func() {
						reconfigErr = task.Error{}
					})
					It("should not return an error or set any conditions", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
					})
				})

				Context("and is an unknown fault", func() {
					BeforeEach(func() {
						reconfigErr = task.Error{
							LocalizedMethodFault: &vimtypes.LocalizedMethodFault{},
						}
					})
					It("should not return an error or set any conditions", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(conditions.Has(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeFalse())
					})
				})

				DescribeTable("and is a single, known fault",
					func(
						currentCryptoState *vimtypes.CryptoKeyId,
						operation string,
						fault vimtypes.BaseMethodFault,
						expectedConditionMessage,
						localizedMessage string,
						msgKeys []string) {

						vm := &vmopv1.VirtualMachine{}

						mf := fault.GetMethodFault()
						for i := range msgKeys {
							mf.FaultMessage = append(
								mf.FaultMessage,
								vimtypes.LocalizableMessage{
									Key: msgKeys[i],
								})
						}

						internal.SetOperation(ctx, operation)

						Expect(r.OnResult(
							ctx,
							vm,
							mo.VirtualMachine{
								Config: &vimtypes.VirtualMachineConfigInfo{
									KeyId: currentCryptoState,
								},
							},
							task.Error{
								LocalizedMethodFault: &vimtypes.LocalizedMethodFault{
									Fault:            fault,
									LocalizedMessage: localizedMessage,
								},
							})).To(Succeed())

						assertStateNotSynced(vm, expectedConditionMessage)
					},

					joinTableEntries(
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.GenericVmConfigFault{} },
							"specify a valid key",
							"",
							"msg.vigor.enc.keyNotFound",
						),

						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.GenericVmConfigFault{} },
							"specify a key that can be located",
							"",
							"msg.keysafe.locator",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.GenericVmConfigFault{} },
							"add vTPM",
							"",
							"msg.vtpm.add.notEncrypted",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.GenericVmConfigFault{} },
							"have vTPM",
							"",
							"msg.vigor.enc.required.vtpm",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.GenericVmConfigFault{} },
							"specify a valid key and specify a key that can be located",
							"",
							"msg.vigor.enc.keyNotFound",
							"msg.keysafe.locator",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.GenericVmConfigFault{} },
							"specify a valid key, specify a key that can be located, and add vTPM",
							"",
							"msg.vigor.enc.keyNotFound",
							"msg.keysafe.locator",
							"msg.vtpm.add.notEncrypted",
						),

						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.SystemError{} },
							"specify a valid key",
							"Error creating disk Key locator",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.SystemError{} },
							"specify a key that can be located",
							"Key locator error",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.SystemError{} },
							"not specify encryption bundle",
							"Key required for encryption.bundle.",
						),

						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.NotSupported{} },
							"not have encryption IO filter",
							"",
							"msg.disk.policyChangeFailure",
						),

						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceOperation{} },
							"not specify encrypted disk",
							"",
							"msg.hostd.deviceSpec.enc.encrypted",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceOperation{} },
							"not specify decrypted disk",
							"",
							"msg.hostd.deviceSpec.enc.notEncrypted",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceOperation{} },
							"not add/remove device sans crypto spec",
							"",
							"fake.msg.id",
						),

						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidArgument{} },
							"not set secret key",
							"",
							"config.extraConfig[\"dataFileKey\"]",
						),

						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceSpec{} },
							"have encryption IO filter",
							"",
							"msg.hostd.deviceSpec.enc.badPolicy",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceSpec{} },
							"not apply only to disk",
							"",
							"msg.hostd.deviceSpec.enc.notDisk",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceSpec{} },
							"not have disk with shared backing",
							"",
							"msg.hostd.deviceSpec.enc.sharedBacking",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceSpec{} },
							"not have raw disk mapping",
							"",
							"msg.hostd.deviceSpec.enc.notFile",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceSpec{} },
							"not add encrypted disk",
							"",
							"msg.hostd.configSpec.enc.mismatch",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidDeviceSpec{} },
							"not add plain disk",
							"",
							"msg.hostd.deviceSpec.add.noencrypt",
						),

						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidVmConfig{} },
							"not have snapshots",
							"",
							"msg.hostd.configSpec.enc.snapshots",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidVmConfig{} },
							"not have only disk snapshots",
							"",
							"msg.hostd.deviceSpec.enc.diskChain",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidVmConfig{} },
							"not be encrypted",
							"",
							"msg.hostd.configSpec.enc.notEncrypted",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidVmConfig{} },
							"be encrypted",
							"",
							"msg.hostd.configSpec.enc.encrypted",
						),
						getMethodFaultTableEntries(
							func() vimtypes.BaseMethodFault { return &vimtypes.InvalidVmConfig{} },
							"have vm and disks with different encryption states",
							"",
							"msg.hostd.configSpec.enc.mismatch",
						),
					),
				)

				Context("and is multiple, known faults", func() {
					BeforeEach(func() {
						internal.SetOperation(ctx, "updating unencrypted")
					})
					When("there are two faults", func() {
						BeforeEach(func() {
							reconfigErr = task.Error{
								LocalizedMethodFault: &vimtypes.LocalizedMethodFault{
									LocalizedMessage: "Key locator error",
									Fault: &vimtypes.SystemError{
										RuntimeFault: vimtypes.RuntimeFault{
											MethodFault: vimtypes.MethodFault{
												FaultCause: &vimtypes.LocalizedMethodFault{
													Fault: &vimtypes.NotSupported{
														RuntimeFault: vimtypes.RuntimeFault{
															MethodFault: vimtypes.MethodFault{
																FaultMessage: []vimtypes.LocalizableMessage{
																	{
																		Key: "msg.disk.policyChangeFailure",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							}
						})
						It("should return nil error and set a condition with the expected message", func() {
							Expect(err).ToNot(HaveOccurred())
							assertStateNotSynced(vm, "Must specify a key that can be located and not have encryption IO filter when updating unencrypted vm")
						})
					})
					When("there are three faults", func() {
						BeforeEach(func() {
							reconfigErr = task.Error{
								LocalizedMethodFault: &vimtypes.LocalizedMethodFault{
									LocalizedMessage: "Key locator error",
									Fault: &vimtypes.SystemError{
										RuntimeFault: vimtypes.RuntimeFault{
											MethodFault: vimtypes.MethodFault{
												FaultCause: &vimtypes.LocalizedMethodFault{
													Fault: &vimtypes.NotSupported{
														RuntimeFault: vimtypes.RuntimeFault{
															MethodFault: vimtypes.MethodFault{
																FaultMessage: []vimtypes.LocalizableMessage{
																	{
																		Key: "msg.disk.policyChangeFailure",
																	},
																},
																FaultCause: &vimtypes.LocalizedMethodFault{
																	Fault: &vimtypes.InvalidPowerState{},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							}
						})
						It("should return nil error and set a condition with the expected message", func() {
							Expect(err).ToNot(HaveOccurred())
							assertStateNotSynced(vm, "Must specify a key that can be located, not have encryption IO filter, and be powered off when updating unencrypted vm")
						})
					})
				})
			})
		})
	})
})

func joinTableEntries(entries ...[]TableEntry) []TableEntry {
	var list []TableEntry
	for i := range entries {
		list = append(list, entries[i]...)
	}
	return list
}

func getMethodFaultTableEntries(
	newFault func() vimtypes.BaseMethodFault,
	conditionMessage,
	localizedMessage string,
	msgKeys ...string) []TableEntry {

	faultName := reflect.ValueOf(newFault()).Elem().Type().Name()

	var w bytes.Buffer
	if localizedMessage != "" {
		fmt.Fprintf(&w, "localizedMessage=%s", localizedMessage)
	}

	if len(msgKeys) > 0 {
		if w.Len() > 0 {
			w.WriteString(", ")
		}
		fmt.Fprintf(&w, "messageKeys=%s", strings.Join(msgKeys, ";"))
	}

	titleSuffix := w.String()

	return []TableEntry{
		Entry(
			fmt.Sprintf("%s, decrypting vm, %s", faultName, titleSuffix),
			&vimtypes.CryptoKeyId{},
			"decrypting",
			newFault(),
			crypto.SprintfStateNotSynced("decrypting", conditionMessage),
			localizedMessage,
			msgKeys,
		),
		Entry(
			fmt.Sprintf("%s, encrypting vm, %s", faultName, titleSuffix),
			nil,
			"encrypting",
			newFault(),
			crypto.SprintfStateNotSynced("encrypting", conditionMessage),
			localizedMessage,
			msgKeys,
		),
		Entry(
			fmt.Sprintf("%s, recrypting vm, %s", faultName, titleSuffix),
			&vimtypes.CryptoKeyId{},
			"recrypting",
			newFault(),
			crypto.SprintfStateNotSynced("recrypting", conditionMessage),
			localizedMessage,
			msgKeys,
		),
		Entry(
			fmt.Sprintf("%s, updating encrypted vm, %s", faultName, titleSuffix),
			&vimtypes.CryptoKeyId{},
			"updating encrypted",
			newFault(),
			crypto.SprintfStateNotSynced("updating encrypted", conditionMessage),
			localizedMessage,
			msgKeys,
		),
		Entry(
			fmt.Sprintf("%s, updating unencrypted vm, %s", faultName, titleSuffix),
			nil,
			"updating unencrypted",
			newFault(),
			crypto.SprintfStateNotSynced("updating unencrypted", conditionMessage),
			localizedMessage,
			msgKeys,
		),
	}
}
