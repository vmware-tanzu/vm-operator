// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

var _ = Describe(
	"MutateNICUnitNumbersOnUpdate",
	Label(
		testlabels.API,
		testlabels.Create,
		testlabels.Update,
		testlabels.Mutation,
		testlabels.Webhook,
	),
	func() {

		var (
			ctx *builder.UnitTestContextForMutatingWebhook
			vm  *vmopv1.VirtualMachine
		)

		// enableFeature turns the VMNetworkUnitNumbers capability on in the
		// request context. The unit-test config starts empty, so BuildVersion
		// must also be set to a non-empty value for IsObjectUpgraded to pass.
		enableFeature := func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMNetworkUnitNumbers = true
				config.BuildVersion = testBuildVersion
			})
		}

		// stampUpgradeAnnotations adds the schema upgrade annotations to the
		// new VM, as ReconcileSchemaUpgrade does. The mutator only checks if
		// the new VM is schema upgraded.
		stampUpgradeAnnotations := func() {
			if vm.Annotations == nil {
				vm.Annotations = make(map[string]string)
			}
			vm.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey] = pkgcfg.FromContext(ctx).BuildVersion
			vm.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey] = vmopv1.GroupVersion.Version
			vm.Annotations[pkgconst.UpgradedToFeatureVersionAnnotationKey] = vmopv1util.ActivatedFeatureVersion(ctx).String()
		}

		// callMutatorOnUpdate invokes the mutator as an update: oldVM is a
		// copy of vm before mutation.
		callMutatorOnUpdate := func() (bool, error) {
			stampUpgradeAnnotations()
			oldVM := vm.DeepCopy()
			return mutation.MutateNICUnitNumbersOnUpdate(&ctx.WebhookRequestContext, nil, vm, oldVM)
		}

		expectMutationSuccess := func() {
			wasMutated, err := callMutatorOnUpdate()
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeTrue())
		}

		expectNoMutation := func() {
			wasMutated, err := callMutatorOnUpdate()
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeFalse())
		}

		// expectUnitNumbers asserts each interface's unit number by position.
		// A nil entry means the interface's unit number must be unset.
		expectUnitNumbers := func(expected ...*int32) {
			Expect(vm.Spec.Network.Interfaces).To(HaveLen(len(expected)))
			for i, want := range expected {
				iface := vm.Spec.Network.Interfaces[i]
				if want == nil {
					Expect(iface.UnitNumber).To(BeNil(), "interface %d", i)
				} else {
					Expect(iface.UnitNumber).To(Equal(want), "interface %d", i)
				}
			}
		}

		setInterfaces := func(unitNumbers ...*int32) {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
			for i, unit := range unitNumbers {
				vm.Spec.Network.Interfaces = append(
					vm.Spec.Network.Interfaces,
					vmopv1.VirtualMachineNetworkInterfaceSpec{
						Name:       "eth" + strconv.Itoa(i),
						UnitNumber: unit,
					})
			}
		}

		BeforeEach(func() {
			vm = builder.DummyVirtualMachine()
			obj, err := builder.ToUnstructured(vm)
			Expect(err).ToNot(HaveOccurred())
			ctx = suite.NewUnitTestContextForMutatingWebhook(obj)
		})

		Context("When VMNetworkUnitNumbers feature is disabled", func() {
			// The suite's base config leaves the feature disabled.

			It("should not mutate", func() {
				setInterfaces(nil, nil)
				expectNoMutation()
				expectUnitNumbers(nil, nil)
			})
		})

		Context("When VM is being created", func() {
			// A VM being created never has the upgrade annotations, and the
			// mutation is an update-path mutation: no unit numbers are
			// assigned at create time. This is the create-time behavior users
			// actually see: values exactly as submitted.

			It("should not mutate when oldVM is nil", func() {
				enableFeature()
				setInterfaces(nil, ptr.To(int32(9)))

				// Invoke the mutator directly with a nil oldVM to simulate
				// the create path.
				wasMutated, err := mutation.MutateNICUnitNumbersOnUpdate(
					&ctx.WebhookRequestContext,
					nil,
					vm,
					nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeFalse())
				expectUnitNumbers(nil, ptr.To(int32(9)))
			})
		})

		Context("When VM has no interfaces", func() {
			It("should not mutate", func() {
				enableFeature()
				setInterfaces()
				expectNoMutation()
			})

			It("should not mutate when spec.network is nil", func() {
				enableFeature()
				vm.Spec.Network = nil
				expectNoMutation()
			})
		})

		Context("When VM has not been schema upgraded", func() {
			// Regression guard for the brownfield race: the mutator must
			// assign nothing until the schema upgrade has recorded observed
			// unit numbers into the spec.

			It("should not assign any unit numbers", func() {
				enableFeature()
				setInterfaces(nil, nil)

				// vm does not have upgrade annotations - this is what
				// prevents mutation.
				vm.Annotations = nil

				oldVM := vm.DeepCopy()
				wasMutated, err := mutation.MutateNICUnitNumbersOnUpdate(
					&ctx.WebhookRequestContext,
					nil,
					vm,
					oldVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeFalse())
				expectUnitNumbers(nil, nil)
			})
		})

		Context("When VM is schema upgraded", func() {
			BeforeEach(func() {
				enableFeature()
			})

			It("should assign sequential unit numbers from 7 in list order", func() {
				setInterfaces(nil, nil, nil)
				expectMutationSuccess()
				expectUnitNumbers(
					ptr.To(int32(7)),
					ptr.To(int32(8)),
					ptr.To(int32(9)))
			})

			It("should preserve an explicit unit number and not reuse it", func() {
				setInterfaces(ptr.To(int32(16)), nil)
				expectMutationSuccess()
				expectUnitNumbers(
					ptr.To(int32(16)),
					ptr.To(int32(7)))
			})

			It("should assign the remainder around an explicit value in the middle of the list", func() {
				setInterfaces(nil, ptr.To(int32(8)), nil)
				expectMutationSuccess()
				expectUnitNumbers(
					ptr.To(int32(7)),
					ptr.To(int32(8)),
					ptr.To(int32(9)))
			})

			It("should assign when the annotations arrive in the same request", func() {
				// The schema-upgrade patch carries the annotation stamp along
				// with the backfilled fields: the mutator gates on the NEW
				// object, so assignment begins on that very request even
				// though oldVM predates the upgrade.
				setInterfaces(nil, nil)
				stampUpgradeAnnotations()
				oldVM := builder.DummyVirtualMachine()
				oldVM.Annotations = nil

				wasMutated, err := mutation.MutateNICUnitNumbersOnUpdate(
					&ctx.WebhookRequestContext,
					nil,
					vm,
					oldVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				expectUnitNumbers(ptr.To(int32(7)), ptr.To(int32(8)))
			})

			// Defensive-only branch (I22): an object with more than ten
			// interfaces cannot be submitted through the API
			// (MaxItems=10, ten valid slots, webhook-enforced uniqueness
			// imply a free slot exists for any admissible interface), so
			// this case is constructed by invoking the mutator directly.
			It("should leave an interface unassigned when no slot is available", func() {
				setInterfaces(
					ptr.To(int32(7)),
					ptr.To(int32(8)),
					ptr.To(int32(9)),
					ptr.To(int32(10)),
					ptr.To(int32(11)),
					ptr.To(int32(12)),
					ptr.To(int32(13)),
					ptr.To(int32(14)),
					ptr.To(int32(15)),
					ptr.To(int32(16)),
					nil, // Cannot be admitted through the API: 11 interfaces.
				)

				wasMutated, err := callMutatorOnUpdate()
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeFalse())
				expectUnitNumbers(
					ptr.To(int32(7)),
					ptr.To(int32(8)),
					ptr.To(int32(9)),
					ptr.To(int32(10)),
					ptr.To(int32(11)),
					ptr.To(int32(12)),
					ptr.To(int32(13)),
					ptr.To(int32(14)),
					ptr.To(int32(15)),
					ptr.To(int32(16)),
					nil,
				)
			})

			// Pins the intentional absence of a privileged-account bypass
			// (I18): the mutator also runs on VM Operator's own patches,
			// including the schema-upgrade backfill's, so an update made by
			// the VM Operator service account still assigns unit numbers.
			It("should mutate on an update made by the VM Operator service account", func() {
				setInterfaces(nil)
				ctx.WebhookRequestContext.IsVMOperatorAccount = true

				expectMutationSuccess()
				expectUnitNumbers(ptr.To(int32(7)))
			})
		})
	})
