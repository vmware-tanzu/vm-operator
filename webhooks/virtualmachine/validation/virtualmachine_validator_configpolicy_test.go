// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"VirtualMachineConfigPolicy webhook enforcement",
	Label(
		testlabels.Update,
		testlabels.API,
		testlabels.Validation,
		testlabels.Webhook,
	),
	func() {
		var (
			ctx    *unitValidatingWebhookContext
			policy *vimv1.VirtualMachineConfigPolicy
		)

		// denyPolicy returns a VirtualMachineConfigPolicy for the dummy zone
		// with every mode set to Deny (so tests only have to opt individual
		// modes back to Allow) and VMClassMode set to AsConfig (so the
		// default dummy VM's spec.className does not bypass the policy).
		denyPolicy := func() *vimv1.VirtualMachineConfigPolicy {
			return &vimv1.VirtualMachineConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      builder.DummyZoneName,
					Namespace: dummyNamespaceName,
				},
				Spec: vimv1.VirtualMachineConfigPolicySpec{
					Zone:        builder.DummyZoneName,
					CreateMode:  vimv1.VirtualMachineConfigPolicyModeDeny,
					UpdateMode:  vimv1.VirtualMachineConfigPolicyModeDeny,
					PowerOnMode: vimv1.VirtualMachineConfigPolicyModeDeny,
					VMClassMode: vimv1.VirtualMachineConfigPolicyVMClassModeAsConfig,
				},
			}
		}

		BeforeEach(func() {
			ctx = newUnitTestContextForValidatingWebhook(true)

			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VirtualMachineConfigPolicy = true
			})

			ctx.vm.Labels[corev1.LabelTopologyZone] = builder.DummyZoneName
			ctx.oldVM.Labels[corev1.LabelTopologyZone] = builder.DummyZoneName

			// Direct config, not VM Class-derived, so the default
			// vmClassMode=AsPolicy does not bypass the policy underneath us.
			ctx.vm.Spec.ClassName = ""
			ctx.oldVM.Spec.ClassName = ""

			policy = denyPolicy()
			Expect(ctx.Client.Create(ctx, policy)).To(Succeed())
		})

		AfterEach(func() {
			ctx = nil
			policy = nil
		})

		updatePolicy := func(
			mutate func(spec *vimv1.VirtualMachineConfigPolicySpec)) {
			mutate(&policy.Spec)
			ExpectWithOffset(1, ctx.Client.Update(ctx, policy)).To(Succeed())
		}

		setExtraConfig := func(vm *vmopv1.VirtualMachine, key string) {
			vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ExtraConfig: []vmopv1common.KeyValuePair{{Key: key, Value: "dummy-value"}},
			}
		}

		doTest := func(args testParams) {
			doTestWithContext(ctx, args)
		}

		noopSetup := func(*unitValidatingWebhookContext) {}

		Context("VM not yet placed in a zone", func() {
			It("allows a request that would otherwise be denied", func() {
				ctx.vm.Labels[corev1.LabelTopologyZone] = ""
				ctx.oldVM.Labels[corev1.LabelTopologyZone] = ""

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: true,
				})
			})
		})

		Context("no policy exists for the VM's zone", func() {
			It("allows a request that would otherwise be denied", func() {
				Expect(ctx.Client.Delete(ctx, policy)).To(Succeed())

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: true,
				})
			})
		})

		Context("vmClassMode", func() {
			BeforeEach(func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
					}
				})
			})

			It("AsPolicy (default) bypasses the policy for VM Class config", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.VMClassMode = vimv1.VirtualMachineConfigPolicyVMClassModeAsPolicy
				})
				ctx.vm.Spec.ClassName = builder.DummyClassName
				ctx.oldVM.Spec.ClassName = builder.DummyClassName

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: true,
				})
			})

			It("AsConfig applies the policy to VM Class-derived config", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.VMClassMode = vimv1.VirtualMachineConfigPolicyVMClassModeAsConfig
				})
				ctx.vm.Spec.ClassName = builder.DummyClassName
				ctx.oldVM.Spec.ClassName = builder.DummyClassName

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: false,
				})
			})
		})

		Context("mode enforcement", func() {
			It("updateMode=Allow permits an otherwise-denied update", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.UpdateMode = vimv1.VirtualMachineConfigPolicyModeAllow
					spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
					}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: true,
				})
			})

			It("updateMode=Deny rejects a denied update", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
					}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: false,
				})
			})

			It("createMode governs create requests", func() {
				createCtx := newUnitTestContextForValidatingWebhook(false)
				pkgcfg.SetContext(createCtx, func(config *pkgcfg.Config) {
					config.Features.VirtualMachineConfigPolicy = true
				})
				createCtx.vm.Labels[corev1.LabelTopologyZone] = builder.DummyZoneName
				setExtraConfig(createCtx.vm, "guestinfo.foo")

				createPolicy := denyPolicy()
				createPolicy.Spec.ExtraConfig =
					&vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
					}
				Expect(createCtx.Client.Create(createCtx, createPolicy)).To(Succeed())

				doTestWithContext(createCtx, testParams{
					setup:         noopSetup,
					expectAllowed: false,
				})

				createPolicyKey := client.ObjectKeyFromObject(createPolicy)
				err := createCtx.Client.Get(createCtx, createPolicyKey, createPolicy)
				Expect(err).To(Succeed())
				createPolicy.Spec.CreateMode = vimv1.VirtualMachineConfigPolicyModeAllow
				// The dummy VM is created already powered on, so also lift
				// PowerOnMode to isolate this assertion to createMode alone.
				createPolicy.Spec.PowerOnMode = vimv1.VirtualMachineConfigPolicyModeAllow
				Expect(createCtx.Client.Update(createCtx, createPolicy)).To(Succeed())

				doTestWithContext(createCtx, testParams{
					setup:         noopSetup,
					expectAllowed: true,
				})
			})

			It("powerOnMode governs power-on even when updateMode=Allow", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.UpdateMode = vimv1.VirtualMachineConfigPolicyModeAllow
					spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
					}
				})
				ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: false,
				})
			})
		})

		Context("extraConfig enforcement", func() {
			DescribeTable("denied entries",
				func(matchType vimv1.MatchType, patternKey, vmKey string) {
					updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
						spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
							Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
								{Type: matchType, Key: patternKey},
							},
						}
					})

					doTest(testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							setExtraConfig(ctx.vm, vmKey)
						},
						expectAllowed: false,
					})
				},
				Entry("Fixed match",
					vimv1.MatchTypeFixed, "guestinfo.foo", "guestinfo.foo"),
				Entry("Glob match",
					vimv1.MatchTypeGlob, "guestinfo.*", "guestinfo.foo"),
				Entry("Regex match",
					vimv1.MatchTypeRegex, "^guestinfo\\..+$", "guestinfo.foo"),
			)

			It("allows a key that matches none of the denied entries", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
					}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.bar")
					},
					expectAllowed: true,
				})
			})

			It("rejects a key absent from a non-empty allowed list", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Allowed: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.bar"},
						},
					}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: false,
				})
			})

			It("allows a key present in a non-empty allowed list", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Allowed: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
					}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: true,
				})
			})

			It("denied takes precedence over allowed", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Allowed: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
						},
					}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						setExtraConfig(ctx.vm, "guestinfo.foo")
					},
					expectAllowed: false,
				})
			})
		})

		Context("hardware version enforcement", func() {
			It("rejects a minHardwareVersion exceeding hardwareVersions.max", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.HardwareVersions = &vimv1.HardwareVersionRange{
						Max: vimv1.MustParseHardwareVersion("vmx-19"),
					}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Powering off alongside the upgrade avoids tripping
						// the pre-existing "cannot upgrade hardware version
						// unless powered off" guard, isolating this case to
						// the policy-driven check under test.
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.MinHardwareVersion = 21
					},
					expectAllowed: false,
				})
			})

			It("allows a spec.minHardwareVersion within hardwareVersions.max", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.HardwareVersions = &vimv1.HardwareVersionRange{
						Max: vimv1.MustParseHardwareVersion("vmx-21"),
					}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.MinHardwareVersion = 21
					},
					expectAllowed: true,
				})
			})

			It("allows the request when hardwareVersions is unset", func() {
				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.MinHardwareVersion = 21
					},
					expectAllowed: true,
				})
			})
		})

		Context("capability checks", func() {
			// qp parses a resource.Quantity string and returns a pointer to
			// the result, for spec.resources.size.{cpu,memory}.
			qp := func(s string) *resource.Quantity {
				q := resource.MustParse(s)
				return &q
			}

			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.TelcoVMServiceAPI = true
				})
				bypassUpgradeCheck(&ctx.Context, ctx.vm, ctx.oldVM)
			})

			It("rejects a spec.resources.size.cpu exceeding numCPUCores.max", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.NumCPUCores = &vimv1.IntRange{Max: 4}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{CPU: qp("8")},
						}
					},
					expectAllowed: false,
				})
			})

			It("allows a spec.resources.size.cpu within numCPUCores", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.NumCPUCores = &vimv1.IntRange{Max: 8}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{CPU: qp("8")},
						}
					},
					expectAllowed: true,
				})
			})

			It("rejects a spec.resources.size.memory exceeding memory.max", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.Memory = &vimv1.ResourceQuantityRange{Max: resource.MustParse("8Gi")}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{Memory: qp("16Gi")},
						}
					},
					expectAllowed: false,
				})
			})

			It("allows a spec.resources.size.memory within policy.spec.memory", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.Memory = &vimv1.ResourceQuantityRange{Max: resource.MustParse("16Gi")}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{Memory: qp("16Gi")},
						}
					},
					expectAllowed: true,
				})
			})

			It("rejects a vnumaNodeCount exceeding numNUMANodes.max", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.NumNUMANodes = &vimv1.IntRange{Max: 2}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{CPU: qp("8")},
						}
						ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
							Topology: &vmopv1.VirtualMachineCPUTopologySpec{
								CoresPerSocket: ptr.To(int32(1)),
								VNUMANodeCount: ptr.To(int32(4)),
							},
						}
					},
					expectAllowed: false,
				})
			})

			It("allows a vnumaNodeCount within policy.spec.numNUMANodes", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.NumNUMANodes = &vimv1.IntRange{Max: 4}
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{CPU: qp("8")},
						}
						ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
							Topology: &vmopv1.VirtualMachineCPUTopologySpec{
								CoresPerSocket: ptr.To(int32(1)),
								VNUMANodeCount: ptr.To(int32(4)),
							},
						}
					},
					expectAllowed: true,
				})
			})

			It("rejects iommuEnabled when policy.spec.iommuSupported is false", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.IOMMUSupported = false
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
							IOMMUEnabled: ptr.To(true),
						}
					},
					expectAllowed: false,
				})
			})

			It("allows iommuEnabled when policy.spec.iommuSupported is true", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.IOMMUSupported = true
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
							IOMMUEnabled: ptr.To(true),
						}
					},
					expectAllowed: true,
				})
			})

			It("rejects reservationLockedToMax when unsupported by the policy", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.MemoryLockedToMaxSupported = false
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
							ReservationLockedToMax: ptr.To(true),
						}
					},
					expectAllowed: false,
				})
			})

			It("allows reservationLockedToMax when supported by the policy", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.MemoryLockedToMaxSupported = true
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
							ReservationLockedToMax: ptr.To(true),
						}
					},
					expectAllowed: true,
				})
			})

			It("rejects hugePages1GEnabled when unsupported by the policy", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.HugePagesSupported = false
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
							HugePages1GEnabled: ptr.To(true),
						}
					},
					expectAllowed: false,
				})
			})

			It("allows hugePages1GEnabled when supported by the policy", func() {
				updatePolicy(func(spec *vimv1.VirtualMachineConfigPolicySpec) {
					spec.HugePagesSupported = true
				})

				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
							HugePages1GEnabled: ptr.To(true),
						}
					},
					expectAllowed: true,
				})
			})
		})
	},
)
