// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func nicUnitNumberTests() {
	Describe(
		"NIC Unit Number Validation",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Validation,
			testlabels.Webhook,
		),
		nicUnitNumberValidationTests,
	)
}

func nicUnitNumberValidationTests() {
	var (
		ctx *unitValidatingWebhookContext

		unitNumberPath = func(i int) *field.Path {
			return field.NewPath("spec", "network", "interfaces").Index(i).Child("unitNumber")
		}
		featureNotEnabledErr = field.Forbidden(
			field.NewPath("spec", "network", "interfaces").Index(0).Child("unitNumber"),
			"the VM Network Unit Numbers feature is not enabled",
		)
		powerOnErrFor = func(i int) string {
			return field.Forbidden(
				unitNumberPath(i),
				"updates to this field are not allowed when VM power is on",
			).Error()
		}
		rangeErrFor = func(i int, unit int32) string {
			return field.Invalid(
				unitNumberPath(i),
				unit,
				"unit number must be between 7 and 16",
			).Error()
		}
		inUseErrFor = func(i int, unit int32) string {
			return field.Invalid(
				unitNumberPath(i),
				unit,
				"unit number is already used by another network interface on this VM",
			).Error()
		}
	)

	// setFeature turns the VMNetworkUnitNumbers capability on or off in the
	// request context, and sets BuildVersion to match the upgrade
	// annotations stamped by setUpgradedAnnotations. The request context's
	// config starts from defaults (not the suite's), so sibling features
	// relied on by the validator are set here too, like the controller
	// tests do.
	setFeature := func(enabled bool) {
		pkgcfg.SetContext(&ctx.WebhookRequestContext, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
			config.Features.VMNetworkUnitNumbers = enabled
			config.BuildVersion = testBuildVersion
		})
	}

	// setUpgradedAnnotations stamps the schema-upgrade annotations on v,
	// exactly as ReconcileSchemaUpgrade does. The feature version reflects
	// the feature flags currently set in the request context.
	setUpgradedAnnotations := func(v *vmopv1.VirtualMachine) {
		if v.Annotations == nil {
			v.Annotations = make(map[string]string)
		}
		v.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey] = testBuildVersion
		v.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey] = vmopv1.GroupVersion.Version
		v.Annotations[pkgconst.UpgradedToFeatureVersionAnnotationKey] = vmopv1util.ActivatedFeatureVersion(&ctx.WebhookRequestContext).String()
	}

	// stripAnnotations removes all annotations, used to simulate a VM that
	// has not yet been schema-upgraded.
	stripAnnotations := func(v *vmopv1.VirtualMachine) {
		v.Annotations = nil
	}

	// syncContext syncs ctx.Obj and ctx.OldObj from ctx.vm and ctx.oldVM.
	syncContext := func() {
		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())
		if ctx.oldVM != nil {
			ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVM)
			Expect(err).ToNot(HaveOccurred())
		} else {
			ctx.WebhookRequestContext.OldObj = nil
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)

		ctx.vm.Status.UniqueID = "vm-123"

		// Powered off by default so the powered-on rules do not interfere;
		// the powered-on tests set both VMs to On explicitly.
		ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
		ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff

		setFeature(true)
		setUpgradedAnnotations(ctx.vm)
		setUpgradedAnnotations(ctx.oldVM)
		syncContext()
	})

	AfterEach(func() {
		ctx = nil
	})

	Context("Create", func() {
		BeforeEach(func() {
			ctx = newUnitTestContextForValidatingWebhook(false)
			ctx.vm.Status.UniqueID = "vm-123"
			setFeature(true)
			setUpgradedAnnotations(ctx.vm)
			syncContext()
		})

		It("should allow a single interface with a valid unit number", func() {
			ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(7))
			syncContext()

			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})

		It("should allow multiple interfaces with distinct, valid unit numbers", func() {
			ctx.vm.Spec.Network.Interfaces = append(ctx.vm.Spec.Network.Interfaces,
				vmopv1.VirtualMachineNetworkInterfaceSpec{Name: "eth1"})
			ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(7))
			ctx.vm.Spec.Network.Interfaces[1].UnitNumber = ptr.To(int32(16))
			syncContext()

			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})

		It("should deny duplicate unit numbers", func() {
			ctx.vm.Spec.Network.Interfaces = append(ctx.vm.Spec.Network.Interfaces,
				vmopv1.VirtualMachineNetworkInterfaceSpec{Name: "eth1"})
			ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
			ctx.vm.Spec.Network.Interfaces[1].UnitNumber = ptr.To(int32(9))
			syncContext()

			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring(inUseErrFor(1, 9)))
		})

		It("should deny a unit number below the valid range", func() {
			ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(3))
			syncContext()

			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring(rangeErrFor(0, 3)))
		})

		It("should deny a unit number above the valid range", func() {
			ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(17))
			syncContext()

			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring(rangeErrFor(0, 17)))
		})

		Context("feature disabled", func() {
			BeforeEach(func() {
				setFeature(false)
				setUpgradedAnnotations(ctx.vm)
			})

			It("should deny any interface with a unit number", func() {
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				syncContext()

				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring(featureNotEnabledErr.Error()))
			})

			It("should allow interfaces without unit numbers", func() {
				syncContext()

				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	Context("Update", func() {
		Context("powered off", func() {
			It("should allow changing a unit number on a powered-off VM", func() {
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(10))
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		Context("powered on, schema upgraded", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			})

			It("should deny changing an already-set unit number", func() {
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(10))
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring(powerOnErrFor(0)))
			})

			It("should deny clearing an already-set unit number for a user", func() {
				ctx.IsVMOperatorAccount = false
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = nil
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring(powerOnErrFor(0)))
			})

			It("should deny clearing an already-set unit number even for the VM Operator account", func() {
				ctx.IsVMOperatorAccount = true
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = nil
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring(powerOnErrFor(0)))
			})

			It("should allow setting a previously-unset unit number for the VM Operator account", func() {
				// The schema-upgrade backfill records observed values into a
				// powered-on VM's spec through the VM Operator account.
				ctx.IsVMOperatorAccount = true
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = nil
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})

			It("should deny setting a previously-unset unit number for a user", func() {
				ctx.IsVMOperatorAccount = false
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = nil
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring(powerOnErrFor(0)))
			})

			It("should allow a newly-added interface with an assigned unit number", func() {
				// A new interface's unit number comes from the mutation
				// webhook; only a unit number change on a pre-existing
				// interface is a change. MutableNetworks is enabled because
				// that is what gates interface add/remove.
				pkgcfg.SetContext(&ctx.WebhookRequestContext, func(config *pkgcfg.Config) {
					config.Features.MutableNetworks = true
				})
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = nil
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = nil
				ctx.vm.Spec.Network.Interfaces = append(ctx.vm.Spec.Network.Interfaces,
					vmopv1.VirtualMachineNetworkInterfaceSpec{
						Name:       "eth1",
						UnitNumber: ptr.To(int32(10)),
					})
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})

			It("should allow an unchanged unit number", func() {
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		Context("powered on, not schema-upgraded", func() {
			It("should skip the powered-on checks entirely", func() {
				// The schema-upgrade patch carries the upgrade annotations
				// together with any backfilled values; the old VM is not yet
				// upgraded, so the powered-on rules must not judge it. The
				// request comes from the VM Operator service account, as the
				// backfill write does — represented here by the
				// system:masters group that validateSchemaUpgrade bypasses
				// on, since the fake webhook context has no service account
				// namespace to match IsVMOperatorServiceAccount against.
				ctx.IsVMOperatorAccount = true
				ctx.UserInfo = authenticationv1.UserInfo{
					Groups: []string{"system:masters"},
				}
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(10))
				stripAnnotations(ctx.oldVM)
				setUpgradedAnnotations(ctx.vm)
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		Context("feature disabled", func() {
			BeforeEach(func() {
				setFeature(false)
				setUpgradedAnnotations(ctx.vm)
				setUpgradedAnnotations(ctx.oldVM)
			})

			It("should deny a new unit number on an interface that had none", func() {
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = nil
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring(featureNotEnabledErr.Error()))
			})

			It("should deny a changed unit number", func() {
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(10))
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring(featureNotEnabledErr.Error()))
			})

			It("should deny clearing a set unit number", func() {
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = nil
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring(featureNotEnabledErr.Error()))
			})

			It("should allow an unchanged pre-existing unit number", func() {
				// Regression guard (I14): every UPDATE is validated as a
				// whole object with no early-out, so rejecting mere presence
				// would make an already-backfilled VM undeletable while the
				// feature is off.
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})

			It("should allow an update that only adds a label and finalizer when a unit number is present", func() {
				ctx.oldVM.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Spec.Network.Interfaces[0].UnitNumber = ptr.To(int32(9))
				ctx.vm.Labels["app"] = "test"
				ctx.vm.Finalizers = append(ctx.vm.Finalizers, "test.vmoperator.vmware.com/finalizer")
				// builder.ToUnstructured carries labels and finalizers through,
				// so re-syncing the context objects is sufficient.
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})

			It("should allow an update when no interface has a unit number", func() {
				syncContext()

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})
}
