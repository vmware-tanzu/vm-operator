// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
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
	It("returns a non-nil Reconciler", func() {
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
		r          vmconfig.Reconciler
	)

	BeforeEach(func() {
		r = vmconfextraconfig.New()
		ctx = telcoCtx()
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

	It("does NOT inject vmx.reboot.powerCycle for bag-key-only changes", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{{Key: "telco.feature", Value: "enabled"}},
		}
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "telco.feature")).To(Equal("enabled"))
		Expect(getECVal(configSpec.ExtraConfig, vsphereconst.ExtraConfigManagedKeysKey)).To(Equal("telco.feature"))
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

	It("emits new bag key and writes managed-keys tracking entry", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{{Key: "foo", Value: "bar"}},
		}
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "foo")).To(Equal("bar"))
		Expect(getECVal(configSpec.ExtraConfig, vsphereconst.ExtraConfigManagedKeysKey)).To(Equal("foo"))
	})

	It("emits update when bag key value changes", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{{Key: "foo", Value: "new"}},
		}
		moVM = moVMWithEC("foo", "old", vsphereconst.ExtraConfigManagedKeysKey, "foo")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "foo")).To(Equal("new"))
		// Managed-keys sentinel is not re-emitted because the key set did not change.
		Expect(getECVal(configSpec.ExtraConfig, vsphereconst.ExtraConfigManagedKeysKey)).To(BeEmpty())
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

	It("injects vmx.reboot.powerCycle when PNUMANodeAffinity changes on powered-on VM", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{PNUMANodeAffinity: []int32{0}}
		moVM = moVMWithEC("numa.nodeAffinity", "1,2")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		Expect(getECVal(configSpec.ExtraConfig, "numa.nodeAffinity")).To(Equal("0"))
		Expect(getECVal(configSpec.ExtraConfig,
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle)).To(Equal("TRUE"))
	})

	It("defers clearing a PowerOff-mode key when VM is powered on", func() {
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		// Spec sets HugePages to nil (clear intent), observed still has the key.
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{HugePages1GEnabled: nil}
		moVM = moVMWithEC("sched.mem.lpage.enable1GPage", "TRUE")
		Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
		// The clear is deferred — no entries emitted.
		Expect(configSpec.ExtraConfig).To(BeEmpty())
	})
})

// OnResult now only ever marks ExtraConfigSynced=False, and only for a real
// Reconfigure task failure. Everything else — True, PowerOffRequired,
// PowerCyclePending — is decided by reconcileStatusExtraConfig
// (pkg/providers/vsphere/vmlifecycle), computed fresh from moVM and
// spec.advanced every reconcile. See that package's "ExtraConfig status"
// tests for those cases.
var _ = Describe("OnResult", func() {

	var (
		ctx  context.Context
		vm   *vmopv1.VirtualMachine
		moVM mo.VirtualMachine
		r    vmconfig.Reconciler
	)

	BeforeEach(func() {
		r = vmconfextraconfig.New()
		ctx = telcoCtx()
		vm = makeVM()
		moVM = moVMWithEC()
	})

	It("panics when ctx is nil", func() {
		Expect(func() { _ = vmconfextraconfig.New().OnResult(nil, vm, moVM, nil) }).To(Panic()) //nolint:staticcheck
	})
	It("panics when vm is nil", func() {
		Expect(func() { _ = r.OnResult(ctx, nil, moVM, nil) }).To(Panic())
	})

	It("no-ops when TelcoVMServiceAPI is off", func() {
		offCtx := vmconfig.WithContext(pkgcfg.NewContextWithDefaultConfig())
		Expect(vmconfextraconfig.New().OnResult(offCtx, vm, moVM, nil)).To(Succeed())
		Expect(findCond(vm)).To(BeNil())
	})

	It("sets Error condition on resultErr", func() {
		Expect(r.OnResult(ctx, vm, moVM, errors.New("boom"))).To(Succeed())
		cond := findCond(vm)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmopv1.VirtualMachineExtraConfigErrorReason))
	})

	It("does not set Error condition on NoRequeueNoError", func() {
		// NoRequeueNoError is a sentinel used for non-error stop conditions
		// (e.g. snapshot revert, VM just created); it must not be treated as
		// a Reconfigure failure.
		Expect(r.OnResult(ctx, vm, moVM, pkgerr.NoRequeueNoErr("updated vm"))).To(Succeed())
		Expect(findCond(vm)).To(BeNil())
	})

	It("leaves the condition untouched on success", func() {
		Expect(r.OnResult(ctx, vm, moVM, nil)).To(Succeed())
		Expect(findCond(vm)).To(BeNil())
	})
})

// --- helpers ---

func findCond(vm *vmopv1.VirtualMachine) *metav1.Condition {
	for i := range vm.Status.Conditions {
		if vm.Status.Conditions[i].Type == vmopv1.VirtualMachineExtraConfigSynced {
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
