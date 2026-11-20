// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe(
	"Flex VM fields webhook validation",
	Label(
		testlabels.Create,
		testlabels.Update,
		testlabels.API,
		testlabels.Validation,
		testlabels.Webhook,
	),
	func() {
		const (
			notSupported = "not supported yet"
			needsCap     = "requires the VMServiceClasslessVM capability"
		)

		type flexField struct {
			path string
			// set sets a non-empty value; v (0 or 1) picks one of two values.
			set func(vm *vmopv1.VirtualMachine, v int)
			// unset restores the field to its empty value.
			unset func(vm *vmopv1.VirtualMachine)
		}

		fields := map[string]flexField{
			"spec.hardware.vGPUs": {
				path: "spec.hardware.vGPUs",
				set: func(vm *vmopv1.VirtualMachine, v int) {
					if vm.Spec.Hardware == nil {
						vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
					}
					vm.Spec.Hardware.VGPUs = []vmopv1.VirtualMachineVGPUDevice{
						{Name: "gpu-0", ProfileName: []string{"a", "b"}[v]},
					}
				},
				unset: func(vm *vmopv1.VirtualMachine) {
					if vm.Spec.Hardware != nil {
						vm.Spec.Hardware.VGPUs = nil
					}
				},
			},
			"spec.hardware.dynamicDirectPathIODevices": {
				path: "spec.hardware.dynamicDirectPathIODevices",
				set: func(vm *vmopv1.VirtualMachine, v int) {
					if vm.Spec.Hardware == nil {
						vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
					}
					vm.Spec.Hardware.DynamicDirectPathIODevices = []vmopv1.VirtualMachineDynamicDirectPathIODevice{
						{Name: "pci-0", CustomLabel: []string{"a", "b"}[v]},
					}
				},
				unset: func(vm *vmopv1.VirtualMachine) {
					if vm.Spec.Hardware != nil {
						vm.Spec.Hardware.DynamicDirectPathIODevices = nil
					}
				},
			},
			"spec.reservedProfileName": {
				path: "spec.reservedProfileName",
				set: func(vm *vmopv1.VirtualMachine, v int) {
					vm.Spec.ReservedProfileName = ptr.To([]string{"a", "b"}[v])
				},
				unset: func(vm *vmopv1.VirtualMachine) { vm.Spec.ReservedProfileName = nil },
			},
		}

		for name, f := range fields {
			Context(name, func() {
				setCap := func(ctx *unitValidatingWebhookContext, enabled bool) {
					pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
						c.Features.VMServiceClasslessVM = enabled
					})
				}

				expectErrs := func(wantNotSupported, wantCap bool) func(admission.Response) {
					return func(r admission.Response) {
						reasons := string(r.Result.Reason)
						if wantNotSupported {
							Expect(reasons).To(ContainSubstring(f.path + ": Forbidden: " + notSupported))
						} else {
							Expect(reasons).ToNot(ContainSubstring(notSupported))
						}
						if wantCap {
							Expect(reasons).To(ContainSubstring(f.path + ": Forbidden: " + needsCap))
						} else {
							Expect(reasons).ToNot(ContainSubstring(needsCap))
						}
					}
				}

				Context("create", func() {
					var ctx *unitValidatingWebhookContext

					BeforeEach(func() {
						ctx = newUnitTestContextForValidatingWebhook(false)
					})

					DescribeTable("rejects a set value",
						func(capEnabled bool) {
							doTestWithContext(ctx, testParams{
								setup: func(ctx *unitValidatingWebhookContext) {
									setCap(ctx, capEnabled)
									f.set(ctx.vm, 0)
								},
								expectAllowed: false,
								validate:      expectErrs(true, !capEnabled),
							})
						},
						Entry("capability enabled: not supported yet only", true),
						Entry("capability disabled: not supported yet and capability", false),
					)

					It("allows an empty value", func() {
						doTestWithContext(ctx, testParams{
							setup: func(ctx *unitValidatingWebhookContext) {
								setCap(ctx, false)
								f.unset(ctx.vm)
							},
							expectAllowed: true,
						})
					})
				})

				Context("update", func() {
					var ctx *unitValidatingWebhookContext

					BeforeEach(func() {
						ctx = newUnitTestContextForValidatingWebhook(true)
					})

					DescribeTable("rejects a changed value",
						func(capEnabled, fromSet bool) {
							doTestWithContext(ctx, testParams{
								setup: func(ctx *unitValidatingWebhookContext) {
									setCap(ctx, capEnabled)
									if fromSet {
										f.set(ctx.oldVM, 0)
									}
									f.set(ctx.vm, 1)
								},
								expectAllowed: false,
								validate:      expectErrs(true, !capEnabled),
							})
						},
						Entry("unset to set, capability enabled", true, false),
						Entry("unset to set, capability disabled", false, false),
						Entry("set to another value, capability enabled", true, true),
						Entry("set to another value, capability disabled", false, true),
					)

					DescribeTable("allows an unchanged value",
						func(capEnabled bool) {
							doTestWithContext(ctx, testParams{
								setup: func(ctx *unitValidatingWebhookContext) {
									setCap(ctx, capEnabled)
									f.set(ctx.oldVM, 0)
									f.set(ctx.vm, 0)
								},
								expectAllowed: true,
							})
						},
						Entry("capability enabled", true),
						Entry("capability disabled", false),
					)

					It("allows clearing the value", func() {
						doTestWithContext(ctx, testParams{
							setup: func(ctx *unitValidatingWebhookContext) {
								setCap(ctx, false)
								f.set(ctx.oldVM, 0)
								f.unset(ctx.vm)
							},
							expectAllowed: true,
						})
					})
				})
			})
		}

		Context("spec.reservedProfileName empty string", func() {
			It("is rejected on create because a non-nil pointer is set", func() {
				ctx := newUnitTestContextForValidatingWebhook(false)
				doTestWithContext(ctx, testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.ReservedProfileName = ptr.To("")
					},
					expectAllowed: false,
					validate:      doValidateWithMsg("spec.reservedProfileName: Forbidden: " + notSupported),
				})
			})
		})
	},
)
