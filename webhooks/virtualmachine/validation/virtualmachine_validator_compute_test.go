// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"

	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe(
	"ComputeConfig webhook validation",
	Label(
		testlabels.Update,
		testlabels.API,
		testlabels.Validation,
		testlabels.Webhook,
	),
	func() {
		var ctx *unitValidatingWebhookContext

		// q parses a resource.Quantity string and returns a pointer to the result.
		q := func(s string) *resource.Quantity {
			p := resource.MustParse(s)
			return &p
		}

		BeforeEach(func() {
			ctx = newUnitTestContextForValidatingWebhook(true)
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.TelcoVMServiceAPI = true
			})
			bypassUpgradeCheck(&ctx.Context, ctx.vm, ctx.oldVM)
		})

		doTest := func(args testParams) {
			doTestWithContext(ctx, args)
		}

		// ------------------------------------------------------------------ //
		// Capability gate
		// ------------------------------------------------------------------ //

		Context("capability gate — TelcoVMServiceAPI required", func() {
			DescribeTable("fields require the TelcoVMServiceAPI supervisor capability",
				doTest,
				Entry("spec.resources non-nil without capability → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
								c.Features.TelcoVMServiceAPI = false
							})
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources: Forbidden: the VM Compute Config (via TelcoVMServiceAPI) feature is not enabled"),
					},
				),
				Entry("spec.cpuAdvanced non-nil without capability → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
								c.Features.TelcoVMServiceAPI = false
							})
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced: Forbidden: the VM Compute Config (via TelcoVMServiceAPI) feature is not enabled"),
					},
				),
				Entry("spec.memoryAdvanced non-nil without capability → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
								c.Features.TelcoVMServiceAPI = false
							})
							ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.memoryAdvanced: Forbidden: the VM Compute Config (via TelcoVMServiceAPI) feature is not enabled"),
					},
				),
			)
		})

		// ------------------------------------------------------------------ //
		// Size field validations
		// ------------------------------------------------------------------ //

		Context("size field validations", func() {
			DescribeTable("size.{cpu,memory} must be > 0 when set",
				doTest,
				Entry("size.cpu = 0 → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{CPU: q("0")},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources.size.cpu: Invalid value", "must be greater than 0 when set"),
					},
				),
				Entry("size.memory = 0 → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{Memory: q("0")},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources.size.memory: Invalid value", "must be greater than 0 when set"),
					},
				),
			)
		})

		// ------------------------------------------------------------------ //
		// Limits field validations
		// ------------------------------------------------------------------ //

		Context("limits field validations", func() {
			DescribeTable("limits.{cpu,memory} must be > 0 when set (nil = unlimited)",
				doTest,
				Entry("limits.cpu = 0 → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Limits: &vmopv1.VirtualMachineResourceQuantity{CPU: q("0")},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources.limits.cpu: Invalid value", "must be greater than 0 or -1 (unlimited) when set"),
					},
				),
				Entry("limits.memory = 0 → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Limits: &vmopv1.VirtualMachineResourceQuantity{Memory: q("0")},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources.limits.memory: Invalid value", "must be greater than 0 or -1 (unlimited) when set"),
					},
				),
				Entry("limits.cpu = -1 (unlimited sentinel) → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Limits: &vmopv1.VirtualMachineResourceQuantity{CPU: q("-1")},
							}
						},
						expectAllowed: true,
					},
				),
			)
		})

		// ------------------------------------------------------------------ //
		// Ordering validations
		// ------------------------------------------------------------------ //

		Context("ordering validations", func() {
			DescribeTable("requests/size/limits ordering constraints",
				doTest,
				Entry("requests.cpu > limits.cpu → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Requests: &vmopv1.VirtualMachineResourceQuantity{CPU: q("3000")},
								Limits:   &vmopv1.VirtualMachineResourceQuantity{CPU: q("2000")},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources.requests.cpu: Invalid value", "must be less than or equal to limits.cpu"),
					},
				),
				Entry("requests.memory > size.memory → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size:     &vmopv1.VirtualMachineResourceQuantity{Memory: q("4Gi")},
								Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources.requests.memory: Invalid value", "must be less than or equal to size.memory"),
					},
				),
				Entry("requests.memory > limits.memory → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
								Limits:   &vmopv1.VirtualMachineResourceQuantity{Memory: q("4Gi")},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources.requests.memory: Invalid value", "must be less than or equal to limits.memory"),
					},
				),
				Entry("size.memory > limits.memory → accepted (size and limits are independent)",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size:   &vmopv1.VirtualMachineResourceQuantity{Memory: q("16Gi")},
								Limits: &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
							}
						},
						expectAllowed: true,
					},
				),
			)
		})

		// ------------------------------------------------------------------ //
		// LatencySensitivity full reservation (CPU + memory)
		// ------------------------------------------------------------------ //

		Context("LatencySensitivity full reservation", func() {
			DescribeTable("High/HighWithHyperthreading requires full CPU and memory reservation",
				doTest,
				Entry("High + requests.memory != size.memory + no reservationLockedToMax → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size:     &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
								Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: q("4Gi")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh),
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced.latencySensitivity: Invalid value", "requires full memory reservation"),
					},
				),
				Entry("High + requests.memory == size.memory + requests.cpu > 0 → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
								Requests: &vmopv1.VirtualMachineResourceQuantity{
									Memory: q("8Gi"),
									CPU:    q("2000"),
								},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh),
							}
						},
						expectAllowed: true,
					},
				),
				Entry("High + memoryAdvanced.reservationLockedToMax=true + requests.cpu > 0 → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Requests: &vmopv1.VirtualMachineResourceQuantity{CPU: q("2000")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh),
							}
							ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
								ReservationLockedToMax: ptr.To(true),
							}
						},
						expectAllowed: true,
					},
				),
				Entry("High + requests.memory nil + size.memory set + no reservationLockedToMax → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh),
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced.latencySensitivity: Invalid value", "requires full memory reservation"),
					},
				),
				Entry("HighWithHyperthreading + requests.memory != size.memory → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size:     &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
								Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: q("4Gi")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading),
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced.latencySensitivity: Invalid value", "requires full memory reservation"),
					},
				),
				Entry("High + memory satisfied + no CPU reservation → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size:     &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
								Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh),
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced.latencySensitivity: Invalid value", "High Latency Sensitivity requires you to set 100% CPU reservation for this VM"),
					},
				),
			)
		})

		// ------------------------------------------------------------------ //
		// reservationLockedToMax mutual exclusion
		// ------------------------------------------------------------------ //

		Context("reservationLockedToMax mutual exclusion with requests", func() {
			DescribeTable("mutual exclusion constraints",
				doTest,
				Entry("memoryAdvanced.reservationLockedToMax=true + requests.memory != size.memory → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size:     &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
								Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: q("4Gi")},
							}
							ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
								ReservationLockedToMax: ptr.To(true),
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.memoryAdvanced.reservationLockedToMax: Invalid value: true: mutually exclusive with spec.resources.requests.memory unless requests.memory equals size.memory"),
					},
				),
				Entry("memoryAdvanced.reservationLockedToMax=true + requests.memory == size.memory → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size:     &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
								Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: q("8Gi")},
							}
							ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
								ReservationLockedToMax: ptr.To(true),
							}
						},
						expectAllowed: true,
					},
				),
				Entry("memoryAdvanced.reservationLockedToMax=true + requests.memory nil → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
								ReservationLockedToMax: ptr.To(true),
							}
						},
						expectAllowed: true,
					},
				),
			)
		})

		// ------------------------------------------------------------------ //
		// vNUMA topology validation
		// ------------------------------------------------------------------ //

		Context("vNUMA topology validation", func() {
			DescribeTable("vnumaNodeCount constraints",
				doTest,
				Entry("vnumaNodeCount set without coresPerSocket → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								Topology: &vmopv1.VirtualMachineCPUTopologySpec{
									VNUMANodeCount: ptr.To(int32(4)),
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced.topology.vnumaNodeCount: Invalid value", "requires coresPerSocket to be set to an explicit (non-zero) value"),
					},
				),
				Entry("vnumaNodeCount set with coresPerSocket=0 (auto) → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								Topology: &vmopv1.VirtualMachineCPUTopologySpec{
									CoresPerSocket: ptr.To(int32(0)),
									VNUMANodeCount: ptr.To(int32(4)),
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced.topology.vnumaNodeCount: Invalid value", "requires coresPerSocket to be set to an explicit (non-zero) value"),
					},
				),
				Entry("vnumaNodeCount=0 (auto sentinel) with coresPerSocket=0 (auto) → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								Topology: &vmopv1.VirtualMachineCPUTopologySpec{
									CoresPerSocket: ptr.To(int32(0)),
									VNUMANodeCount: ptr.To(int32(0)),
								},
							}
						},
						expectAllowed: true,
					},
				),
				Entry("size.cpu % vnumaNodeCount != 0 → rejected (uneven division)",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{CPU: q("7")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								Topology: &vmopv1.VirtualMachineCPUTopologySpec{
									CoresPerSocket: ptr.To(int32(1)),
									VNUMANodeCount: ptr.To(int32(2)),
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced.topology.vnumaNodeCount: Invalid value", "size.cpu must be evenly divisible by vnumaNodeCount"),
					},
				),
				Entry("coresPerNumaNode neither multiple nor divisor of coresPerSocket → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							// size.cpu=8, vnumaNodeCount=4 → coresPerNode=2; coresPerSocket=3: 2%3!=0 and 3%2!=0
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{CPU: q("8")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								Topology: &vmopv1.VirtualMachineCPUTopologySpec{
									CoresPerSocket: ptr.To(int32(3)),
									VNUMANodeCount: ptr.To(int32(4)),
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced.topology.vnumaNodeCount: Invalid value", "derived coresPerNumaNode must be a multiple or divisor of coresPerSocket"),
					},
				),
				Entry("coresPerNumaNode is a divisor of coresPerSocket → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							// size.cpu=8, vnumaNodeCount=4 → coresPerNode=2; coresPerSocket=4: 4%2==0
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{CPU: q("8")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								Topology: &vmopv1.VirtualMachineCPUTopologySpec{
									CoresPerSocket: ptr.To(int32(4)),
									VNUMANodeCount: ptr.To(int32(4)),
								},
							}
						},
						expectAllowed: true,
					},
				),
				Entry("coresPerNumaNode is a multiple of coresPerSocket → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							// size.cpu=8, vnumaNodeCount=2 → coresPerNode=4; coresPerSocket=2: 4%2==0
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{CPU: q("8")},
							}
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								Topology: &vmopv1.VirtualMachineCPUTopologySpec{
									CoresPerSocket: ptr.To(int32(2)),
									VNUMANodeCount: ptr.To(int32(2)),
								},
							}
						},
						expectAllowed: true,
					},
				),
			)

		})

		// ------------------------------------------------------------------ //
		// Classless VM
		// ------------------------------------------------------------------ //

		Context("classless VM", func() {
			It("spec.resources = nil → passes without size check", func() {
				doTest(testParams{
					setup:         func(ctx *unitValidatingWebhookContext) { ctx.vm.Spec.Resources = nil },
					expectAllowed: true,
				})
			})
		})

		// ------------------------------------------------------------------ //
		// Freeze guard
		// ------------------------------------------------------------------ //

		Context("freeze guard — compute fields immutable until schema upgrade completes", func() {
			// markNotUpgraded simulates a VM that has not yet been schema-upgraded
			// by clearing the upgrade annotations that bypassUpgradeCheck set.
			markNotUpgraded := func(ctx *unitValidatingWebhookContext) {
				ctx.IsPrivilegedAccount = false
				ctx.vm.Annotations = map[string]string{}
				ctx.oldVM.Annotations = map[string]string{}
			}

			DescribeTable("changes to compute fields are rejected during upgrade window",
				doTest,
				Entry("spec.resources changed → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							markNotUpgraded(ctx)
							ctx.oldVM.Spec.Resources = nil
							ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
								Size: &vmopv1.VirtualMachineResourceQuantity{CPU: q("4")},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.resources: Forbidden: modifying this VM is not allowed until it is upgraded"),
					},
				),
				Entry("spec.cpuAdvanced changed → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							markNotUpgraded(ctx)
							ctx.oldVM.Spec.CPUAdvanced = nil
							ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
								HotAddEnabled: ptr.To(true),
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.cpuAdvanced: Forbidden: modifying this VM is not allowed until it is upgraded"),
					},
				),
				Entry("spec.memoryAdvanced changed → rejected",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							markNotUpgraded(ctx)
							ctx.oldVM.Spec.MemoryAdvanced = nil
							ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
								HotAddEnabled: ptr.To(true),
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.memoryAdvanced: Forbidden: modifying this VM is not allowed until it is upgraded"),
					},
				),
				Entry("compute fields unchanged during upgrade window → accepted",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							markNotUpgraded(ctx)
							ctx.vm.Spec.Resources = nil
							ctx.oldVM.Spec.Resources = nil
							ctx.vm.Spec.CPUAdvanced = nil
							ctx.oldVM.Spec.CPUAdvanced = nil
							ctx.vm.Spec.MemoryAdvanced = nil
							ctx.oldVM.Spec.MemoryAdvanced = nil
						},
						expectAllowed: true,
					},
				),
			)
		})

		// ------------------------------------------------------------------ //
		// Comprehensive acceptance
		// ------------------------------------------------------------------ //

		Context("comprehensive acceptance — full valid compute spec", func() {
			It("accepts all compute fields set to valid values", func() {
				doTest(testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// size.cpu=16 / size.memory=32Gi
						// requests < size, requests < limits, limits > requests
						// size.memory (32Gi) > limits.memory (16Gi) — independent fields
						ctx.vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    q("16"),
								Memory: q("32Gi"),
							},
							Requests: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    q("4000m"),
								Memory: q("8Gi"),
							},
							Limits: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    q("8"),
								Memory: q("16Gi"),
							},
						}
						// coresPerSocket=4, vnumaNodeCount=4: coresPerNumaNode = 16/4 = 4, 4%4==0 → valid
						ctx.vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
							LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityNormal),
							Topology: &vmopv1.VirtualMachineCPUTopologySpec{
								CoresPerSocket: ptr.To(int32(4)),
								VNUMANodeCount: ptr.To(int32(4)),
							},
							HotAddEnabled:                       ptr.To(true),
							IOMMUEnabled:                        ptr.To(true),
							NestedHardwareVirtualizationEnabled: ptr.To(true),
							PerformanceCountersEnabled:          ptr.To(true),
						}
						ctx.vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
							HotAddEnabled: ptr.To(true),
						}
					},
					expectAllowed: true,
				})
			})
		})
	},
)
