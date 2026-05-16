// Copyright (c) 2026 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	vmconfextraconfig "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/extraconfig"
)

func makeVM() *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "vm"},
		Status:     vmopv1.VirtualMachineStatus{PowerState: vmopv1.VirtualMachinePowerStateOff},
	}
}

func moVMWithEC(pairs ...string) mo.VirtualMachine {
	if len(pairs)%2 != 0 {
		panic("moVMWithEC: odd argument count")
	}
	ec := make([]vimtypes.BaseOptionValue, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		ec[i/2] = &vimtypes.OptionValue{Key: pairs[i], Value: pairs[i+1]}
	}
	return mo.VirtualMachine{
		Config: &vimtypes.VirtualMachineConfigInfo{ExtraConfig: ec},
	}
}

func telcoCtx() context.Context {
	cfg := pkgcfg.Config{}
	cfg.Features.TelcoVMServiceAPI = true
	ctx := pkgcfg.WithConfig(cfg)
	return vmconfig.WithContext(ctx)
}

var _ = Describe("New", func() {
	It("returns a non-nil ReconcilerWithContext", func() {
		Expect(vmconfextraconfig.New()).ToNot(BeNil())
	})
	It("has name 'extraconfig'", func() {
		Expect(vmconfextraconfig.New().Name()).To(Equal("extraconfig"))
	})
})

var _ = Describe("Reconcile", func() {

	var (
		ctx        context.Context
		vm         *vmopv1.VirtualMachine
		moVM       mo.VirtualMachine
		configSpec *vimtypes.VirtualMachineConfigSpec
		r          vmconfig.ReconcilerWithContext
	)

	BeforeEach(func() {
		r = vmconfextraconfig.New()
		ctx = r.WithContext(telcoCtx())
		vm = makeVM()
		moVM = moVMWithEC()
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	It("panics when ctx is nil", func() {
		Expect(func() { _ = vmconfextraconfig.Reconcile(nil, nil, nil, vm, moVM, configSpec) }).To(Panic()) //nolint:staticcheck
	})
	It("panics when vm is nil", func() {
		Expect(func() { _ = vmconfextraconfig.Reconcile(ctx, nil, nil, nil, moVM, configSpec) }).To(Panic())
	})
	It("panics when configSpec is nil", func() {
		Expect(func() { _ = vmconfextraconfig.Reconcile(ctx, nil, nil, vm, moVM, nil) }).To(Panic())
	})

	It("no-ops when TelcoVMServiceAPI is off", func() {
		offCtx := vmconfig.WithContext(pkgcfg.NewContextWithDefaultConfig())
		offCtx = vmconfextraconfig.New().WithContext(offCtx)
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(true)}
		Expect(vmconfextraconfig.Reconcile(offCtx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(configSpec.ExtraConfig).To(BeEmpty())
	})

	It("no-ops when moVM.Config is nil", func() {
		moVM.Config = nil
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(true)}
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(configSpec.ExtraConfig).To(BeEmpty())
	})

	It("emits nothing when spec matches observed", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(true)}
		moVM = moVMWithEC("numa.vcpu.preferHT", "TRUE")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(configSpec.ExtraConfig).To(BeEmpty())
	})

	It("suppresses first-class key when semantically equal (ESXi lowercase)", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(true)}
		moVM = moVMWithEC("numa.vcpu.preferHT", "true") // lowercase from ESXi
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(configSpec.ExtraConfig).To(BeEmpty())
	})

	It("emits first-class update when spec differs from observed", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(false)}
		moVM = moVMWithEC("numa.vcpu.preferHT", "TRUE")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "numa.vcpu.preferHT")).To(Equal("FALSE"))
	})

	It("clears first-class key when spec is nil and observed has it", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: nil}
		moVM = moVMWithEC("numa.vcpu.preferHT", "TRUE")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "numa.vcpu.preferHT")).To(Equal(""))
	})

	It("injects vmx.reboot.powerCycle when powered-on VM has pending first-class update", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(false)}
		moVM = moVMWithEC("numa.vcpu.preferHT", "TRUE")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig,
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle)).To(Equal("TRUE"))
	})

	It("does NOT inject vmx.reboot.powerCycle for bag-key-only changes", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{{Key: "telco.feature", Value: "enabled"}},
		}
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "telco.feature")).To(Equal("enabled"))
		Expect(getECVal(configSpec.ExtraConfig,
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle)).To(BeEmpty())
	})

	It("defers HugePages1G when VM is powered on (poweroff mode)", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{HugePages1GEnabled: ptr.To(true)}
		moVM = moVMWithEC("sched.mem.lpage.enable1GPage", "FALSE")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "sched.mem.lpage.enable1GPage")).To(BeEmpty())
		Expect(getECVal(configSpec.ExtraConfig,
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle)).To(BeEmpty())
	})

	It("emits new bag key and writes managed-keys tracking entry", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{{Key: "foo", Value: "bar"}},
		}
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "foo")).To(Equal("bar"))
		Expect(getECVal(configSpec.ExtraConfig, vsphereconst.ExtraConfigManagedKeysKey)).To(Equal("foo"))
	})

	It("clears first-class (PowerCycle) key and injects vmx.reboot.powerCycle when VM is on", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		// Spec has nil PreferHTEnabled — clear intent
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: nil}
		moVM = moVMWithEC("numa.vcpu.preferHT", "TRUE")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		// Clear entry emitted
		Expect(getECVal(configSpec.ExtraConfig, "numa.vcpu.preferHT")).To(Equal(""))
		// Clearing a PowerCycle key also needs a power cycle
		Expect(getECVal(configSpec.ExtraConfig,
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle)).To(Equal("TRUE"))
	})

	It("applies PowerOff key when VM is powered off (no deferral)", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{HugePages1GEnabled: ptr.To(true)}
		moVM = moVMWithEC("sched.mem.lpage.enable1GPage", "FALSE")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "sched.mem.lpage.enable1GPage")).To(Equal("TRUE"))
		// No power-cycle injection (this is a poweroff-mode key, not powercycle-mode)
		Expect(getECVal(configSpec.ExtraConfig,
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle)).To(BeEmpty())
	})

	It("emits update when bag key value changes", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{{Key: "foo", Value: "new"}},
		}
		moVM = moVMWithEC("foo", "old", vsphereconst.ExtraConfigManagedKeysKey, "foo")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "foo")).To(Equal("new"))
	})

	It("emits nothing for unchanged bag key", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{{Key: "foo", Value: "bar"}},
		}
		moVM = moVMWithEC("foo", "bar", vsphereconst.ExtraConfigManagedKeysKey, "foo")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(configSpec.ExtraConfig).To(BeEmpty())
	})

	It("clears bag key removed from spec (managed cleanup)", func() {
		// Spec no longer has "foo", but observed still has it from a previous run.
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
		moVM = moVMWithEC("foo", "bar", vsphereconst.ExtraConfigManagedKeysKey, "foo")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "foo")).To(Equal(""))
		Expect(getECVal(configSpec.ExtraConfig, vsphereconst.ExtraConfigManagedKeysKey)).To(Equal(""))
	})

	It("filters system-reserved keys from spec.advanced.extraConfig", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{
				{Key: vsphereconst.EnableDiskUUIDExtraConfigKey, Value: "TRUE"},
				{Key: "safe.key", Value: "v"},
			},
		}
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, vsphereconst.EnableDiskUUIDExtraConfigKey)).To(BeEmpty())
		Expect(getECVal(configSpec.ExtraConfig, "safe.key")).To(Equal("v"))
	})

	It("first-class spec value overrides the same key from another reconciler in configSpec", func() {
		// Another reconciler (e.g. class) placed preferHT=FALSE in configSpec already.
		configSpec.ExtraConfig = []vimtypes.BaseOptionValue{
			&vimtypes.OptionValue{Key: "numa.vcpu.preferHT", Value: "FALSE"},
		}
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(true)}
		moVM = moVMWithEC()
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "numa.vcpu.preferHT")).To(Equal("TRUE"))
	})
})

var _ = Describe("OnResult", func() {

	var (
		ctx        context.Context
		vm         *vmopv1.VirtualMachine
		moVM       mo.VirtualMachine
		r          vmconfig.ReconcilerWithContext
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		r = vmconfextraconfig.New()
		ctx = r.WithContext(telcoCtx())
		vm = makeVM()
		moVM = moVMWithEC()
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	reconcileAndOnResult := func(resultErr error) {
		GinkgoHelper()
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(r.OnResult(ctx, vm, moVM, resultErr)).To(Succeed())
	}

	It("panics when ctx is nil", func() {
		Expect(func() { _ = vmconfextraconfig.New().OnResult(nil, vm, moVM, nil) }).To(Panic()) //nolint:staticcheck
	})
	It("panics when vm is nil", func() {
		Expect(func() { _ = r.OnResult(ctx, nil, moVM, nil) }).To(Panic())
	})

	It("no-ops when TelcoVMServiceAPI is off", func() {
		offCtx := vmconfig.WithContext(pkgcfg.NewContextWithDefaultConfig())
		offCtx = vmconfextraconfig.New().WithContext(offCtx)
		Expect(vmconfextraconfig.New().OnResult(offCtx, vm, moVM, nil)).To(Succeed())
		Expect(findCond(vm)).To(BeNil())
	})

	It("sets Error condition on resultErr", func() {
		Expect(r.OnResult(ctx, vm, moVM, assertError("boom"))).To(Succeed())
		cond := findCond(vm)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmconfextraconfig.ReasonError))
	})

	It("sets True condition when spec matches observed", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(true)}
		moVM = moVMWithEC("numa.vcpu.preferHT", "TRUE")
		reconcileAndOnResult(nil)
		cond := findCond(vm)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("sets True condition with no advanced spec", func() {
		reconcileAndOnResult(nil)
		cond := findCond(vm)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("sets PowerOffRequired when HugePages deferred on powered-on VM", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{HugePages1GEnabled: ptr.To(true)}
		moVM = moVMWithEC("sched.mem.lpage.enable1GPage", "FALSE")
		reconcileAndOnResult(nil)
		cond := findCond(vm)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmconfextraconfig.ReasonPowerOffRequired))
		Expect(cond.Message).To(ContainSubstring("sched.mem.lpage.enable1GPage"))
	})

	It("sets PowerCyclePending when first-class key injected this cycle", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(false)}
		moVM = moVMWithEC("numa.vcpu.preferHT", "TRUE")
		reconcileAndOnResult(nil)
		cond := findCond(vm)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Reason).To(Equal(vmconfextraconfig.ReasonPowerCyclePending))
	})

	It("persists PowerCyclePending when vmx.reboot.powerCycle already on VM", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(true)}
		moVM = moVMWithEC(
			"numa.vcpu.preferHT", "TRUE",
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle, "TRUE",
		)
		reconcileAndOnResult(nil)
		cond := findCond(vm)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Reason).To(Equal(vmconfextraconfig.ReasonPowerCyclePending))
	})

	It("status.ExtraConfig omits first-class keys not yet on the VM (observed-based)", func() {
		// Spec sets preferHT=true but the VM doesn't have it yet (e.g. first apply).
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PreferHTEnabled: ptr.To(true)}
		moVM = moVMWithEC() // preferHT not in observed yet
		reconcileAndOnResult(nil)
		for _, kv := range vm.Status.ExtraConfig {
			Expect(kv.Key).ToNot(Equal("numa.vcpu.preferHT"),
				"status should reflect observed VM, not desired")
		}
	})

	It("populates status.ExtraConfig with only user-controlled keys", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			PreferHTEnabled: ptr.To(true),
			ExtraConfig:     []vmopv1common.KeyValuePair{{Key: "foo", Value: "bar"}},
		}
		moVM = moVMWithEC(
			"numa.vcpu.preferHT", "TRUE",
			"tools.guest.desktop.autolock", "FALSE", // class-derived, not in spec
			"disk.enableUUID", "TRUE",               // internal
			"foo", "bar",
			vsphereconst.ExtraConfigManagedKeysKey, "foo",
		)
		reconcileAndOnResult(nil)
		statusKeys := make(map[string]string)
		for _, kv := range vm.Status.ExtraConfig {
			statusKeys[kv.Key] = kv.Value
		}
		Expect(statusKeys).To(HaveKey("numa.vcpu.preferHT"))
		Expect(statusKeys).To(HaveKey("foo"))
		Expect(statusKeys).ToNot(HaveKey("tools.guest.desktop.autolock"))
		Expect(statusKeys).ToNot(HaveKey("disk.enableUUID"))
	})
})

// --- helpers ---

func findCond(vm *vmopv1.VirtualMachine) *metav1.Condition {
	for i := range vm.Status.Conditions {
		if vm.Status.Conditions[i].Type == vmopv1.VirtualMachineConditionExtraConfigSynced {
			return &vm.Status.Conditions[i]
		}
	}
	return nil
}

func getECVal(ec []vimtypes.BaseOptionValue, key string) string {
	for _, bov := range ec {
		kv := bov.GetOptionValue()
		if kv != nil && kv.Key == key {
			v, _ := kv.Value.(string)
			return v
		}
	}
	return ""
}

type assertError string

func (e assertError) Error() string { return string(e) }
