// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configpolicy_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/util/configpolicy"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("MatchesExtraConfigKey", func() {

	key := func(
		t vimv1.MatchType, k string) vimv1.VirtualMachineConfigPolicyExtraConfigKey {
		return vimv1.VirtualMachineConfigPolicyExtraConfigKey{Type: t, Key: k}
	}

	DescribeTable("matching",
		func(
			k vimv1.VirtualMachineConfigPolicyExtraConfigKey,
			testKey string, expected bool) {
			Ω(configpolicy.MatchesExtraConfigKey(k, testKey)).Should(Equal(expected))
		},
		Entry("Fixed matches an exact key",
			key(vimv1.MatchTypeFixed, "foo.bar"), "foo.bar", true),
		Entry("Fixed does not match a different key",
			key(vimv1.MatchTypeFixed, "foo.bar"), "foo.baz", false),

		Entry("Glob matches via filepath.Match semantics",
			key(vimv1.MatchTypeGlob, "guestinfo.*"), "guestinfo.foo", true),
		Entry("Glob does not match outside the pattern",
			key(vimv1.MatchTypeGlob, "guestinfo.*"), "otherinfo.foo", false),
		Entry("Glob does not match across path separators",
			key(vimv1.MatchTypeGlob, "guestinfo.*"), "guestinfo.foo/bar", false),
		Entry("a malformed Glob pattern is treated as a non-match",
			key(vimv1.MatchTypeGlob, "[invalid"), "anything", false),

		Entry("Regex matches via regexp.MatchString semantics",
			key(vimv1.MatchTypeRegex, `^guestinfo\..+$`), "guestinfo.foo", true),
		Entry("Regex does not match outside the pattern",
			key(vimv1.MatchTypeRegex, `^guestinfo\..+$`), "otherinfo.foo", false),
		Entry("a malformed Regex pattern is treated as a non-match",
			key(vimv1.MatchTypeRegex, `(unterminated`), "anything", false),

		Entry("an empty/unrecognized Type falls back to an exact (Fixed-like) match",
			vimv1.VirtualMachineConfigPolicyExtraConfigKey{Key: "foo.bar"},
			"foo.bar", true),
	)
})

var _ = Describe("AppliesToVM", func() {

	asPolicy := vimv1.VirtualMachineConfigPolicyVMClassModeAsPolicy
	asConfig := vimv1.VirtualMachineConfigPolicyVMClassModeAsConfig

	When("className is empty", func() {
		It("returns true regardless of VMClassMode", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{VMClassMode: asPolicy}
			Ω(configpolicy.AppliesToVM(spec, "")).Should(BeTrue())
		})
	})

	When("className is non-empty and VMClassMode is AsConfig", func() {
		It("returns true", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{VMClassMode: asConfig}
			Ω(configpolicy.AppliesToVM(spec, "small")).Should(BeTrue())
		})
	})

	When("className is non-empty and VMClassMode is AsPolicy", func() {
		It("returns false", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{VMClassMode: asPolicy}
			Ω(configpolicy.AppliesToVM(spec, "small")).Should(BeFalse())
		})
	})

	When("className is non-empty and VMClassMode is unset", func() {
		It("returns false, since the zero value is AsPolicy", func() {
			emptySpec := vimv1.VirtualMachineConfigPolicySpec{}
			Ω(configpolicy.AppliesToVM(emptySpec, "small")).Should(BeFalse())
		})
	})
})

var _ = Describe("Violations", func() {

	When("err is nil", func() {
		It("returns nil", func() {
			Ω(configpolicy.Violations(nil)).Should(BeNil())
		})
	})

	When("err is a single, non-joined error", func() {
		It("returns a single-element slice containing err", func() {
			err := &configpolicy.ErrIOMMUViolation{}
			Ω(configpolicy.Violations(err)).Should(ConsistOf(err))
		})
	})

	When("err is the result of errors.Join", func() {
		It("returns every joined error", func() {
			a := &configpolicy.ErrIOMMUViolation{}
			b := &configpolicy.ErrHugePagesViolation{}
			Ω(configpolicy.Violations(errors.Join(a, b))).Should(ConsistOf(a, b))
		})
	})

	When("err is a nested join", func() {
		It("flattens every leaf error", func() {
			a := &configpolicy.ErrIOMMUViolation{}
			b := &configpolicy.ErrHugePagesViolation{}
			c := &configpolicy.ErrMemoryLockedToMaxViolation{}
			nested := errors.Join(errors.Join(a, b), c)
			Ω(configpolicy.Violations(nested)).Should(ConsistOf(a, b, c))
		})
	})
})

var _ = Describe("InputFromVM", func() {

	newVM := func() *vmopv1.VirtualMachine {
		return &vmopv1.VirtualMachine{}
	}

	When("the VM's spec has none of the relevant fields set", func() {
		It("returns a zero-value Input", func() {
			Ω(configpolicy.InputFromVM(newVM())).Should(Equal(configpolicy.Input{}))
		})
	})

	When("spec.advanced.extraConfig is set", func() {
		It("collects every key, in order", func() {
			vm := newVM()
			vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ExtraConfig: []vmopv1common.KeyValuePair{
					{Key: "guestinfo.foo", Value: "1"},
					{Key: "guestinfo.bar", Value: "2"},
				},
			}
			want := []string{"guestinfo.foo", "guestinfo.bar"}
			Ω(configpolicy.InputFromVM(vm).ExtraConfigKeys).Should(Equal(want))
		})
	})

	When("spec.minHardwareVersion is set", func() {
		It("sets HardwareVersion", func() {
			vm := newVM()
			vm.Spec.MinHardwareVersion = int32(vimv1.VMX19)
			Ω(configpolicy.InputFromVM(vm).HardwareVersion).Should(Equal(vimv1.VMX19))
		})
	})

	When("spec.minHardwareVersion is unset", func() {
		It("leaves HardwareVersion as the zero value", func() {
			Ω(configpolicy.InputFromVM(newVM()).HardwareVersion).Should(BeZero())
		})
	})

	When("spec.resources.size is set", func() {
		It("sets NumCPUCores and Memory", func() {
			cpu := resource.MustParse("4")
			mem := resource.MustParse("8Gi")
			vm := newVM()
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &cpu, Memory: &mem},
			}

			in := configpolicy.InputFromVM(vm)
			Ω(in.NumCPUCores).ShouldNot(BeNil())
			Ω(*in.NumCPUCores).Should(Equal(int32(4)))
			Ω(in.Memory).Should(Equal(&mem))
		})
	})

	When("spec.resources is unset", func() {
		It("leaves NumCPUCores and Memory nil", func() {
			in := configpolicy.InputFromVM(newVM())
			Ω(in.NumCPUCores).Should(BeNil())
			Ω(in.Memory).Should(BeNil())
		})
	})

	When("spec.cpuAdvanced.topology/iommuEnabled are set", func() {
		It("sets NumNUMANodes and IOMMUEnabled", func() {
			vm := newVM()
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: ptr.To(int32(2)),
				},
				IOMMUEnabled: ptr.To(true),
			}

			in := configpolicy.InputFromVM(vm)
			Ω(in.NumNUMANodes).ShouldNot(BeNil())
			Ω(*in.NumNUMANodes).Should(Equal(int32(2)))
			Ω(in.IOMMUEnabled).ShouldNot(BeNil())
			Ω(*in.IOMMUEnabled).Should(BeTrue())
		})
	})

	When("spec.memoryAdvanced.reservationLockedToMax is set", func() {
		It("sets MemoryLockedToMax", func() {
			vm := newVM()
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			Ω(*configpolicy.InputFromVM(vm).MemoryLockedToMax).Should(BeTrue())
		})
	})

	When("spec.advanced.hugePages1GEnabled is set", func() {
		It("sets HugePagesEnabled", func() {
			vm := newVM()
			vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				HugePages1GEnabled: ptr.To(true),
			}
			Ω(*configpolicy.InputFromVM(vm).HugePagesEnabled).Should(BeTrue())
		})
	})
})

var _ = Describe("InputFromConfigInfo", func() {

	newCfg := func() vimtypes.VirtualMachineConfigInfo {
		return vimtypes.VirtualMachineConfigInfo{
			Hardware: vimtypes.VirtualHardware{NumCPU: 4, MemoryMB: 8192},
		}
	}

	It("always sets NumCPUCores and Memory from Hardware", func() {
		in := configpolicy.InputFromConfigInfo(newCfg())
		Ω(in.NumCPUCores).ShouldNot(BeNil())
		Ω(*in.NumCPUCores).Should(Equal(int32(4)))
		Ω(in.Memory).ShouldNot(BeNil())
		Ω(in.Memory.Value()).Should(Equal(int64(8192) * 1024 * 1024))
	})

	It("always leaves NumNUMANodes and HugePagesEnabled unset", func() {
		in := configpolicy.InputFromConfigInfo(newCfg())
		Ω(in.NumNUMANodes).Should(BeNil())
		Ω(in.HugePagesEnabled).Should(BeNil())
	})

	When("Version is a valid hardware version string", func() {
		It("sets HardwareVersion", func() {
			cfg := newCfg()
			cfg.Version = "vmx-19"
			hv := configpolicy.InputFromConfigInfo(cfg).HardwareVersion
			Ω(hv).Should(Equal(vimv1.VMX19))
		})
	})

	When("Version is empty or unparsable", func() {
		It("leaves HardwareVersion as the zero value", func() {
			cfg := newCfg()
			cfg.Version = "not-a-version"
			Ω(configpolicy.InputFromConfigInfo(cfg).HardwareVersion).Should(BeZero())
		})
	})

	When("ExtraConfig is set", func() {
		It("collects every key, in order", func() {
			cfg := newCfg()
			cfg.ExtraConfig = []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{Key: "guestinfo.foo", Value: "1"},
				&vimtypes.OptionValue{Key: "guestinfo.bar", Value: "2"},
			}
			want := []string{"guestinfo.foo", "guestinfo.bar"}
			Ω(configpolicy.InputFromConfigInfo(cfg).ExtraConfigKeys).Should(Equal(want))
		})
	})

	When("Flags.VvtdEnabled and MemoryReservationLockedToMax are set", func() {
		It("sets IOMMUEnabled and MemoryLockedToMax", func() {
			cfg := newCfg()
			cfg.Flags.VvtdEnabled = ptr.To(true)
			cfg.MemoryReservationLockedToMax = ptr.To(true)

			in := configpolicy.InputFromConfigInfo(cfg)
			Ω(in.IOMMUEnabled).ShouldNot(BeNil())
			Ω(*in.IOMMUEnabled).Should(BeTrue())
			Ω(in.MemoryLockedToMax).ShouldNot(BeNil())
			Ω(*in.MemoryLockedToMax).Should(BeTrue())
		})
	})
})

var _ = Describe("Validate", func() {

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	When("in is fully compliant with an empty spec", func() {
		It("returns nil", func() {
			emptySpec := vimv1.VirtualMachineConfigPolicySpec{}
			Ω(configpolicy.Validate(
				ctx, emptySpec, configpolicy.Input{})).Should(Succeed())
		})
	})

	Describe("extraConfig", func() {
		fixed := func(key string) vimv1.VirtualMachineConfigPolicyExtraConfigKey {
			return vimv1.VirtualMachineConfigPolicyExtraConfigKey{
				Type: vimv1.MatchTypeFixed, Key: key,
			}
		}

		When("a key matches a denied entry", func() {
			It("returns an ErrExtraConfigViolation with Denied=true", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{
					ExtraConfig: &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							fixed("guestinfo.foo"),
						},
					},
				}
				in := configpolicy.Input{ExtraConfigKeys: []string{"guestinfo.foo"}}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrExtraConfigViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Key).Should(Equal("guestinfo.foo"))
				Ω(violation.Denied).Should(BeTrue())
				Ω(violation.Error()).Should(
					ContainSubstring("denied by the namespace's VirtualMachineConfigPolicy"))
			})
		})

		When("allowed is non-empty and a key matches no allowed entry", func() {
			It("returns an ErrExtraConfigViolation with Denied=false", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{
					ExtraConfig: &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Allowed: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							fixed("guestinfo.bar"),
						},
					},
				}
				in := configpolicy.Input{ExtraConfigKeys: []string{"guestinfo.foo"}}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrExtraConfigViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Denied).Should(BeFalse())
				Ω(violation.Error()).Should(ContainSubstring("allowed list"))
			})
		})

		When("multiple keys each violate the policy", func() {
			It("returns one violation per key", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{
					ExtraConfig: &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							fixed("guestinfo.foo"), fixed("guestinfo.bar"),
						},
					},
				}
				in := configpolicy.Input{
					ExtraConfigKeys: []string{
						"guestinfo.foo", "guestinfo.bar", "guestinfo.baz",
					},
				}

				err := configpolicy.Validate(ctx, spec, in)
				Ω(configpolicy.Violations(err)).Should(HaveLen(2))
			})
		})

		When("no key violates the policy", func() {
			It("returns nil", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{
					ExtraConfig: &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
						Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
							fixed("guestinfo.foo"),
						},
					},
				}
				in := configpolicy.Input{ExtraConfigKeys: []string{"guestinfo.bar"}}
				Ω(configpolicy.Validate(ctx, spec, in)).Should(Succeed())
			})
		})
	})

	Describe("hardwareVersion", func() {
		fullRange := &vimv1.HardwareVersionRange{Min: vimv1.VMX13, Max: vimv1.VMX19}

		When("the VM's hardware version is below the policy's minimum", func() {
			It("returns an ErrHardwareVersionViolation with Above=false", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{HardwareVersions: fullRange}
				in := configpolicy.Input{HardwareVersion: vimv1.VMX11}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrHardwareVersionViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Above).Should(BeFalse())
				Ω(violation.Error()).Should(
					ContainSubstring("below the minimum hardware version"))
			})
		})

		When("the VM's hardware version exceeds the policy's maximum", func() {
			It("returns an ErrHardwareVersionViolation with Above=true", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{HardwareVersions: fullRange}
				in := configpolicy.Input{HardwareVersion: vimv1.VMX21}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrHardwareVersionViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Above).Should(BeTrue())
				Ω(violation.Error()).Should(
					ContainSubstring("exceeds the maximum hardware version"))
			})
		})

		When("the VM's hardware version is the zero value", func() {
			It("returns nil, even with a range set", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{HardwareVersions: fullRange}
				Ω(configpolicy.Validate(
					ctx, spec, configpolicy.Input{})).Should(Succeed())
			})
		})

		When("Min is zero", func() {
			It("treats the range as having no minimum", func() {
				maxOnly := &vimv1.HardwareVersionRange{Max: vimv1.VMX19}
				spec := vimv1.VirtualMachineConfigPolicySpec{HardwareVersions: maxOnly}
				in := configpolicy.Input{HardwareVersion: vimv1.VMX3}
				Ω(configpolicy.Validate(ctx, spec, in)).Should(Succeed())
			})
		})

		When("Max is zero", func() {
			It("treats the range as having no maximum", func() {
				minOnly := &vimv1.HardwareVersionRange{Min: vimv1.VMX13}
				spec := vimv1.VirtualMachineConfigPolicySpec{HardwareVersions: minOnly}
				in := configpolicy.Input{HardwareVersion: vimv1.MaxValidHardwareVersion}
				Ω(configpolicy.Validate(ctx, spec, in)).Should(Succeed())
			})
		})
	})

	Describe("numCPUCores", func() {
		cpuRange := &vimv1.IntRange{Min: 2, Max: 8}

		When("Input.NumCPUCores is nil", func() {
			It("skips the check, returning nil", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{NumCPUCores: cpuRange}
				Ω(configpolicy.Validate(
					ctx, spec, configpolicy.Input{})).Should(Succeed())
			})
		})

		When("the VM's CPU cores are below the policy's minimum", func() {
			It("returns an ErrCPUCoresViolation with Above=false", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{NumCPUCores: cpuRange}
				in := configpolicy.Input{NumCPUCores: ptr.To(int32(1))}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrCPUCoresViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Above).Should(BeFalse())
				Ω(violation.Got).Should(Equal(int32(1)))
			})
		})

		When("the VM's CPU cores exceed the policy's maximum", func() {
			It("returns an ErrCPUCoresViolation with Above=true", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{NumCPUCores: cpuRange}
				in := configpolicy.Input{NumCPUCores: ptr.To(int32(16))}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrCPUCoresViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Above).Should(BeTrue())
			})
		})

		When("the VM's CPU cores are within range", func() {
			It("returns nil", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{NumCPUCores: cpuRange}
				in := configpolicy.Input{NumCPUCores: ptr.To(int32(4))}
				Ω(configpolicy.Validate(ctx, spec, in)).Should(Succeed())
			})
		})
	})

	Describe("memory", func() {
		gi := func(n int64) resource.Quantity {
			return *resource.NewQuantity(n*1024*1024*1024, resource.BinarySI)
		}
		memRange := &vimv1.ResourceQuantityRange{Min: gi(2), Max: gi(64)}

		When("Input.Memory is nil", func() {
			It("skips the check, returning nil", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{Memory: memRange}
				Ω(configpolicy.Validate(
					ctx, spec, configpolicy.Input{})).Should(Succeed())
			})
		})

		When("the VM's memory is below the policy's minimum", func() {
			It("returns an ErrMemoryViolation with Above=false", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{Memory: memRange}
				mem := gi(1)
				in := configpolicy.Input{Memory: &mem}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrMemoryViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Above).Should(BeFalse())
			})
		})

		When("the VM's memory exceeds the policy's maximum", func() {
			It("returns an ErrMemoryViolation with Above=true", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{Memory: memRange}
				mem := gi(128)
				in := configpolicy.Input{Memory: &mem}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrMemoryViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Above).Should(BeTrue())
			})
		})

		When("the VM's memory is within range", func() {
			It("returns nil", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{Memory: memRange}
				mem := gi(8)
				in := configpolicy.Input{Memory: &mem}
				Ω(configpolicy.Validate(ctx, spec, in)).Should(Succeed())
			})
		})
	})

	Describe("numNUMANodes", func() {
		When("Input.NumNUMANodes is nil", func() {
			It("skips the check, returning nil", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{
					NumNUMANodes: &vimv1.IntRange{Min: 1, Max: 2},
				}
				Ω(configpolicy.Validate(
					ctx, spec, configpolicy.Input{})).Should(Succeed())
			})
		})

		When("the VM's vNUMA node count exceeds the policy's maximum", func() {
			It("returns an ErrNUMANodesViolation with Above=true", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{
					NumNUMANodes: &vimv1.IntRange{Max: 2},
				}
				in := configpolicy.Input{NumNUMANodes: ptr.To(int32(4))}

				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrNUMANodesViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
				Ω(violation.Above).Should(BeTrue())
			})
		})
	})

	Describe("capability checks (iommu, memoryLockedToMax, hugePages)", func() {
		When("Input's field is nil", func() {
			It("skips every check, returning nil", func() {
				spec := vimv1.VirtualMachineConfigPolicySpec{}
				Ω(configpolicy.Validate(
					ctx, spec, configpolicy.Input{})).Should(Succeed())
			})
		})

		When("the VM wants IOMMU but the policy does not support it", func() {
			It("returns an ErrIOMMUViolation", func() {
				in := configpolicy.Input{IOMMUEnabled: ptr.To(true)}
				spec := vimv1.VirtualMachineConfigPolicySpec{IOMMUSupported: false}
				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrIOMMUViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
			})
		})

		When("the VM wants IOMMU and the policy supports it", func() {
			It("returns nil", func() {
				in := configpolicy.Input{IOMMUEnabled: ptr.To(true)}
				spec := vimv1.VirtualMachineConfigPolicySpec{IOMMUSupported: true}
				Ω(configpolicy.Validate(ctx, spec, in)).Should(Succeed())
			})
		})

		When("the VM does not want IOMMU", func() {
			It("returns nil regardless of policy support", func() {
				in := configpolicy.Input{IOMMUEnabled: ptr.To(false)}
				spec := vimv1.VirtualMachineConfigPolicySpec{IOMMUSupported: false}
				Ω(configpolicy.Validate(ctx, spec, in)).Should(Succeed())
			})
		})

		When("the VM wants memory locked to max, unsupported by the policy", func() {
			It("returns an ErrMemoryLockedToMaxViolation", func() {
				in := configpolicy.Input{MemoryLockedToMax: ptr.To(true)}
				spec := vimv1.VirtualMachineConfigPolicySpec{
					MemoryLockedToMaxSupported: false,
				}
				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrMemoryLockedToMaxViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
			})
		})

		When("the VM wants huge pages but the policy does not support them", func() {
			It("returns an ErrHugePagesViolation", func() {
				in := configpolicy.Input{HugePagesEnabled: ptr.To(true)}
				spec := vimv1.VirtualMachineConfigPolicySpec{HugePagesSupported: false}
				err := configpolicy.Validate(ctx, spec, in)
				var violation *configpolicy.ErrHugePagesViolation
				Ω(errors.As(err, &violation)).Should(BeTrue())
			})
		})
	})

	When("in violates multiple dimensions at once", func() {
		It("joins every violation", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{
				NumCPUCores:    &vimv1.IntRange{Max: 4},
				IOMMUSupported: false,
			}
			in := configpolicy.Input{
				NumCPUCores:  ptr.To(int32(8)),
				IOMMUEnabled: ptr.To(true),
			}

			err := configpolicy.Validate(ctx, spec, in)
			violations := configpolicy.Violations(err)
			Ω(violations).Should(HaveLen(2))

			var cpuViolation *configpolicy.ErrCPUCoresViolation
			var iommuViolation *configpolicy.ErrIOMMUViolation
			Ω(errors.As(err, &cpuViolation)).Should(BeTrue())
			Ω(errors.As(err, &iommuViolation)).Should(BeTrue())
		})
	})

	When("in violates every possible dimension at once", func() {
		It("joins one violation per dimension", func() {
			spec := vimv1.VirtualMachineConfigPolicySpec{
				ExtraConfig: &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
					Denied: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
						{Type: vimv1.MatchTypeFixed, Key: "guestinfo.foo"},
					},
				},
				HardwareVersions: &vimv1.HardwareVersionRange{Max: vimv1.VMX19},
				NumCPUCores:      &vimv1.IntRange{Max: 4},
				Memory: &vimv1.ResourceQuantityRange{
					Max: resource.MustParse("64Gi"),
				},
				NumNUMANodes:               &vimv1.IntRange{Max: 2},
				IOMMUSupported:             false,
				MemoryLockedToMaxSupported: false,
				HugePagesSupported:         false,
			}

			mem := resource.MustParse("128Gi")
			in := configpolicy.Input{
				ExtraConfigKeys:   []string{"guestinfo.foo"},
				HardwareVersion:   vimv1.VMX21,
				NumCPUCores:       ptr.To(int32(8)),
				Memory:            &mem,
				NumNUMANodes:      ptr.To(int32(4)),
				IOMMUEnabled:      ptr.To(true),
				MemoryLockedToMax: ptr.To(true),
				HugePagesEnabled:  ptr.To(true),
			}

			err := configpolicy.Validate(ctx, spec, in)
			Ω(configpolicy.Violations(err)).Should(HaveLen(8))

			var (
				extraConfigViolation *configpolicy.ErrExtraConfigViolation
				hardwareVersionErr   *configpolicy.ErrHardwareVersionViolation
				cpuCoresViolation    *configpolicy.ErrCPUCoresViolation
				memoryViolation      *configpolicy.ErrMemoryViolation
				numaNodesViolation   *configpolicy.ErrNUMANodesViolation
				iommuViolation       *configpolicy.ErrIOMMUViolation
				lockedToMaxViolation *configpolicy.ErrMemoryLockedToMaxViolation
				hugePagesViolation   *configpolicy.ErrHugePagesViolation
			)
			Ω(errors.As(err, &extraConfigViolation)).Should(BeTrue())
			Ω(errors.As(err, &hardwareVersionErr)).Should(BeTrue())
			Ω(errors.As(err, &cpuCoresViolation)).Should(BeTrue())
			Ω(errors.As(err, &memoryViolation)).Should(BeTrue())
			Ω(errors.As(err, &numaNodesViolation)).Should(BeTrue())
			Ω(errors.As(err, &iommuViolation)).Should(BeTrue())
			Ω(errors.As(err, &lockedToMaxViolation)).Should(BeTrue())
			Ω(errors.As(err, &hugePagesViolation)).Should(BeTrue())
		})
	})
})

var _ = Describe("error formatting", func() {

	It("ErrExtraConfigViolation includes the wrapped Err via %w", func() {
		cause := errors.New("boom")
		violation := &configpolicy.ErrExtraConfigViolation{
			Key: "guestinfo.foo", Denied: true, Err: cause,
		}
		Ω(violation.Error()).Should(ContainSubstring("boom"))
		Ω(errors.Is(violation, cause)).Should(BeTrue())
	})

	It("ErrIOMMUViolation formats without a wrapped Err", func() {
		Ω((&configpolicy.ErrIOMMUViolation{}).Error()).Should(Equal(
			"IOMMU is not supported by the namespace's VirtualMachineConfigPolicy"))
	})
})
