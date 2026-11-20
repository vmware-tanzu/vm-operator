// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	lifecyclev1 "github.com/vmware-tanzu/vm-operator/external/lifecycle/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/lifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// hookStageHelper reads and drives the LifecycleState of a VM for one stage,
// standing in for the Lifecycle Operator. The context and VM are fetched
// through getters because the specs assign them after the helper is built.
type hookStageHelper struct {
	getCtx func() *builder.TestContextForVCSim
	getVM  func() *vmopv1.VirtualMachine
	stage  string
}

// subscribe lists the stage in the namespace's LifecycleSubscribedStages.
func (h hookStageHelper) subscribe() {
	ctx, vm := h.getCtx(), h.getVM()
	ExpectWithOffset(1, ctx.Client.Create(ctx, &lifecyclev1.LifecycleSubscribedStages{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vm.Namespace,
			Name: lifecyclev1.SubscribedStagesName(
				lifecycle.TargetGroup, lifecycle.TargetKind),
		},
		Spec: lifecyclev1.LifecycleSubscribedStagesSpec{
			Stages: []string{h.stage},
		},
	})).To(Succeed())
}

func (h hookStageHelper) state() *lifecyclev1.LifecycleState {
	ctx, vm := h.getCtx(), h.getVM()
	state := &lifecyclev1.LifecycleState{}
	ExpectWithOffset(1, ctx.Client.Get(ctx, client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, vm.UID),
	}, state)).To(Succeed())
	return state
}

func (h hookStageHelper) stageSpec() lifecyclev1.StageSpec {
	for _, s := range h.state().Spec.Stages {
		if s.Name == h.stage {
			return s
		}
	}
	Fail("stage " + h.stage + " not found")
	return lifecyclev1.StageSpec{}
}

func (h hookStageHelper) paused() bool  { return h.stageSpec().WorkflowPaused }
func (h hookStageHelper) resumed() bool { return h.stageSpec().WorkflowResumed }

// setHooksReady simulates the Lifecycle Operator reporting the hooks'
// outcome. A nil status clears the stage's entry, as the Lifecycle Operator
// does once a stage has been resumed.
func (h hookStageHelper) setHooksReady(status *metav1.ConditionStatus) {
	ctx := h.getCtx()
	state := h.state()
	state.Status.Stages = nil
	if status != nil {
		state.Status.Stages = []lifecyclev1.StageStatus{{
			Name: h.stage,
			Conditions: []metav1.Condition{{
				Type:               lifecycle.HooksReadyConditionType,
				Status:             *status,
				Reason:             "AllHooksSucceeded",
				LastTransitionTime: metav1.Now(),
			}},
		}}
	}
	ExpectWithOffset(1, ctx.Client.Status().Update(ctx, state)).To(Succeed())
}

func (h hookStageHelper) hookReadyCondition() *metav1.Condition {
	return conditions.Get(
		h.getVM(), vmopv1.VirtualMachineConditionLifecycleHookReady)
}

func vmPowerStateChangeHookTests() {
	const stage = lifecycle.StagePowerStateChange

	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
		vcVM    *object.VirtualMachine
		moVM    mo.VirtualMachine
	)

	h := hookStageHelper{
		getCtx: func() *builder.TestContextForVCSim { return ctx },
		getVM:  func() *vmopv1.VirtualMachine { return vm },
		stage:  stage,
	}

	BeforeEach(func() {
		parentCtx = newVMTestParentContext()
		testConfig = newVMTestConfig()
		testConfig.WithVMSnapshots = true

		vmClass, vm = newVMTestObjects("test-vm")
		vm.UID = types.UID("power-state-hook-uid")
		vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeDisabled
	})

	JustBeforeEach(func() {
		ctx, vmProvider, nsInfo = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		var err error
		vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		// Power the VM on before the hook is enabled.
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
		Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

		h.subscribe()
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.LifecycleHooks = true
		})
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil
		vcVM = nil

		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	stagePaused := h.paused
	stageResumed := h.resumed
	setHooksReady := h.setHooksReady
	hookReadyCondition := h.hookReadyCondition

	expectVCPowerState := func(expected vimtypes.VirtualMachinePowerState) {
		ExpectWithOffset(1, vcVM.Properties(
			ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
		ExpectWithOffset(1, moVM.Summary.Runtime.PowerState).To(Equal(expected))
	}

	lastRestartTime := func() string {
		ExpectWithOffset(1, vcVM.Properties(
			ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
		for i := range moVM.Config.ExtraConfig {
			ov := moVM.Config.ExtraConfig[i].GetOptionValue()
			if ov.Key == "vmservice.lastRestartTime" {
				return ov.Value.(string)
			}
		}
		return ""
	}

	isSetPowerState := func(err error) bool {
		return err == nil || errors.Is(err, vsphere.ErrSetPowerState)
	}

	ready := metav1.ConditionTrue

	It("does not create a LifecycleState when no power change is wanted", func() {
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

		err := ctx.Client.Get(ctx, client.ObjectKey{
			Namespace: vm.Namespace,
			Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, vm.UID),
		}, &lifecyclev1.LifecycleState{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	When("powering off the VM", func() {
		JustBeforeEach(func() {
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
		})

		It("holds the power-off until the hooks resolve", func() {
			By("pausing the stage without powering off or failing")
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOn)
			Expect(stagePaused()).To(BeTrue())
			c := hookReadyCondition()
			Expect(c).ToNot(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(vmopv1.VirtualMachineLifecycleHookReadyPowerStateChangeStageReason))

			By("staying held while the hooks are not ready")
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOn)

			By("powering off once the hooks are ready")
			setHooksReady(&ready)
			Expect(isSetPowerState(createOrUpdateVM(ctx, vmProvider, vm))).To(BeTrue())
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOff)
			Expect(stagePaused()).To(BeFalse())
			Expect(stageResumed()).To(BeTrue())
			Expect(hookReadyCondition().Status).To(Equal(metav1.ConditionTrue))
		})

		It("still runs the rest of the update, including snapshot create", func() {
			vmSnapshot := builder.DummyVirtualMachineSnapshot(
				nsInfo.Namespace, "power-hook-snap", vm.Name)
			Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

			// The snapshot must be owned by the VM resource.
			o := vmopv1.VirtualMachine{}
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
			Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
			Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

			By("holding the power-off")
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOn)
			Expect(stagePaused()).To(BeTrue())

			By("creating the snapshot")
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.RootSnapshotList).ToNot(BeEmpty())
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal(vmSnapshot.Name))
		})

		It("abandons the hold when the power-off is reverted", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(stagePaused()).To(BeTrue())

			By("reverting spec.powerState")
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(stagePaused()).To(BeFalse())
			Expect(stageResumed()).To(BeTrue())
			Expect(hookReadyCondition().Status).To(Equal(metav1.ConditionTrue))
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOn)

			By("pausing afresh on the next request")
			setHooksReady(nil)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(stagePaused()).To(BeTrue())
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOn)
		})

		It("keeps the hold when the VM returns early", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(stagePaused()).To(BeTrue())

			vm.Annotations = map[string]string{vmopv1.PauseAnnotation: "true"}
			Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrIsPaused)).To(BeTrue())
			Expect(stagePaused()).To(BeTrue())
		})

		It("keeps the hold when the reconcile returns an error", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(stagePaused()).To(BeTrue())

			By("returning a parse error before the power change is decided")
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Spec.NextRestartTime = "not-a-timestamp"
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(HaveOccurred())
			Expect(stagePaused()).To(BeTrue())
		})

		It("pauses power-on independently of power-off", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			setHooksReady(&ready)
			Expect(isSetPowerState(createOrUpdateVM(ctx, vmProvider, vm))).To(BeTrue())
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOff)

			By("the Lifecycle Operator clearing the resumed stage")
			setHooksReady(nil)

			By("holding the power-on as a new pass")
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(stagePaused()).To(BeTrue())
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOff)

			By("powering on once the hooks are ready")
			setHooksReady(&ready)
			Expect(isSetPowerState(createOrUpdateVM(ctx, vmProvider, vm))).To(BeTrue())
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOn)
		})

		When("the capability is off", func() {
			JustBeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.LifecycleHooks = false
				})
			})

			It("powers off without pausing", func() {
				Expect(isSetPowerState(createOrUpdateVM(ctx, vmProvider, vm))).To(BeTrue())
				expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOff)
				Expect(hookReadyCondition()).To(BeNil())
			})
		})
	})

	When("suspending the VM", func() {
		JustBeforeEach(func() {
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
		})

		It("holds the suspend until the hooks resolve", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			expectVCPowerState(vimtypes.VirtualMachinePowerStatePoweredOn)
			Expect(stagePaused()).To(BeTrue())

			setHooksReady(&ready)
			Expect(isSetPowerState(createOrUpdateVM(ctx, vmProvider, vm))).To(BeTrue())
			expectVCPowerState(vimtypes.VirtualMachinePowerStateSuspended)
			Expect(stagePaused()).To(BeFalse())
		})
	})

	When("restarting the VM", func() {
		var oldLastRestartTime string

		JustBeforeEach(func() {
			vm.Spec.RestartMode = vmopv1.VirtualMachinePowerOpModeHard
			oldLastRestartTime = lastRestartTime()
		})

		When("the restart is due", func() {
			JustBeforeEach(func() {
				vm.Spec.NextRestartTime = time.Now().UTC().Format(time.RFC3339Nano)
			})

			It("holds the restart until the hooks resolve", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(stagePaused()).To(BeTrue())
				Expect(lastRestartTime()).To(Equal(oldLastRestartTime))

				setHooksReady(&ready)
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(lastRestartTime()).ToNot(Equal(oldLastRestartTime))
				Expect(stagePaused()).To(BeFalse())
			})
		})

		When("the restart is not due", func() {
			JustBeforeEach(func() {
				vm.Spec.NextRestartTime = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
			})

			It("does not pause", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(hookReadyCondition()).To(BeNil())
				Expect(lastRestartTime()).To(Equal(oldLastRestartTime))

				state := &lifecyclev1.LifecycleState{}
				err := ctx.Client.Get(ctx, client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, vm.UID),
				}, state)
				if err == nil {
					Expect(stagePaused()).To(BeFalse())
				}
			})
		})
	})
}

func vmCreateHookTests() {
	const stage = lifecycle.StageCreate

	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
	)

	h := hookStageHelper{
		getCtx: func() *builder.TestContextForVCSim { return ctx },
		getVM:  func() *vmopv1.VirtualMachine { return vm },
		stage:  stage,
	}

	BeforeEach(func() {
		parentCtx = newVMTestParentContext()
		testConfig = newVMTestConfig()

		vmClass, vm = newVMTestObjects("test-vm")
		vm.UID = types.UID("create-hook-uid")
	})

	JustBeforeEach(func() {
		ctx, vmProvider, _ = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		h.subscribe()
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.LifecycleHooks = true
		})
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil
		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	stagePaused := h.paused
	hookReadyCondition := h.hookReadyCondition
	setHooksReady := func(status metav1.ConditionStatus) { h.setHooksReady(&status) }

	It("holds the create until the hooks resolve", func() {
		By("pausing the stage without creating the VM")
		err := createOrUpdateVM(ctx, vmProvider, vm)
		Expect(errors.Is(err, lifecycle.ErrHooksBlocked)).To(BeTrue())
		Expect(vm.Status.UniqueID).To(BeEmpty())
		Expect(stagePaused()).To(BeTrue())
		c := hookReadyCondition()
		Expect(c).ToNot(BeNil())
		Expect(c.Status).To(Equal(metav1.ConditionFalse))
		Expect(c.Reason).To(Equal(vmopv1.VirtualMachineLifecycleHookReadyCreateStageReason))

		By("staying held while the hooks are not ready")
		err = createOrUpdateVM(ctx, vmProvider, vm)
		Expect(errors.Is(err, lifecycle.ErrHooksBlocked)).To(BeTrue())
		Expect(vm.Status.UniqueID).To(BeEmpty())

		By("creating the VM once the hooks are ready")
		setHooksReady(metav1.ConditionTrue)
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
		Expect(vm.Status.UniqueID).ToNot(BeEmpty())
		Expect(stagePaused()).To(BeFalse())
		Expect(hookReadyCondition().Status).To(Equal(metav1.ConditionTrue))
	})

	When("a create prerequisite fails", func() {
		JustBeforeEach(func() {
			Expect(ctx.Client.Delete(ctx, vmClass)).To(Succeed())
		})

		It("does not pause the stage", func() {
			err := createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, lifecycle.ErrHooksBlocked)).To(BeFalse())
			Expect(vm.Status.UniqueID).To(BeEmpty())
			Expect(hookReadyCondition()).To(BeNil())

			state := &lifecyclev1.LifecycleState{}
			err = ctx.Client.Get(ctx, client.ObjectKey{
				Namespace: vm.Namespace,
				Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, vm.UID),
			}, state)
			Expect(err).To(HaveOccurred())
		})
	})

	When("the VM already exists with its status wiped", func() {
		It("does not reach the gate", func() {
			By("creating the VM once the hooks are ready")
			Expect(errors.Is(
				createOrUpdateVM(ctx, vmProvider, vm),
				lifecycle.ErrHooksBlocked)).To(BeTrue())
			setHooksReady(metav1.ConditionTrue)
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

			By("updating after the Lifecycle Operator clears the stage")
			state := h.state()
			state.Status.Stages = nil
			Expect(ctx.Client.Status().Update(ctx, state)).To(Succeed())
			vm.Status = vmopv1.VirtualMachineStatus{}
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(stagePaused()).To(BeFalse())
		})
	})

	When("the capability is off", func() {
		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.LifecycleHooks = false
			})
		})

		It("creates without pausing", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(vm.Status.UniqueID).ToNot(BeEmpty())
			Expect(hookReadyCondition()).To(BeNil())
		})
	})

	When("no hook is registered for the stage", func() {
		JustBeforeEach(func() {
			Expect(ctx.Client.DeleteAllOf(
				ctx,
				&lifecyclev1.LifecycleSubscribedStages{},
				client.InNamespace(vm.Namespace))).To(Succeed())
		})

		It("creates without pausing", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(vm.Status.UniqueID).ToNot(BeEmpty())
		})
	})
}

func vmDeleteHookTests() {
	const stage = lifecycle.StageDelete

	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface

		vm       *vmopv1.VirtualMachine
		vmClass  *vmopv1.VirtualMachineClass
		uniqueID string
	)

	h := hookStageHelper{
		getCtx: func() *builder.TestContextForVCSim { return ctx },
		getVM:  func() *vmopv1.VirtualMachine { return vm },
		stage:  stage,
	}

	BeforeEach(func() {
		parentCtx = newVMTestParentContext()
		testConfig = newVMTestConfig()

		vmClass, vm = newVMTestObjects("test-vm")
		vm.UID = types.UID("delete-hook-uid")
	})

	JustBeforeEach(func() {
		ctx, vmProvider, _ = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		// Create the VM before the hook is enabled.
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
		uniqueID = vm.Status.UniqueID
		Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

		h.subscribe()
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.LifecycleHooks = true
		})
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil
		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	stagePaused := h.paused
	hookReadyCondition := h.hookReadyCondition

	It("holds the delete until the hooks resolve", func() {
		By("pausing the stage without deleting the vSphere VM")
		err := vmProvider.DeleteVirtualMachine(ctx, vm)
		Expect(errors.Is(err, lifecycle.ErrHooksBlocked)).To(BeTrue())
		Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())
		Expect(stagePaused()).To(BeTrue())
		c := hookReadyCondition()
		Expect(c).ToNot(BeNil())
		Expect(c.Status).To(Equal(metav1.ConditionFalse))
		Expect(c.Reason).To(Equal(vmopv1.VirtualMachineLifecycleHookReadyDeleteStageReason))

		By("staying held while the hooks are not ready")
		err = vmProvider.DeleteVirtualMachine(ctx, vm)
		Expect(errors.Is(err, lifecycle.ErrHooksBlocked)).To(BeTrue())
		Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

		By("deleting the vSphere VM once the hooks are ready")
		ready := metav1.ConditionTrue
		h.setHooksReady(&ready)
		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
		Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
		Expect(stagePaused()).To(BeFalse())
	})

	When("the vSphere VM does not exist", func() {
		JustBeforeEach(func() {
			// Remove the vSphere VM out of band.
			vcVM := ctx.GetVMFromMoID(uniqueID)
			Expect(vcVM).ToNot(BeNil())
			powerOff, err := vcVM.PowerOff(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(powerOff.Wait(ctx)).To(Succeed())
			destroy, err := vcVM.Destroy(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(destroy.Wait(ctx)).To(Succeed())
		})

		It("returns nil before the gate", func() {
			Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
			Expect(hookReadyCondition()).To(BeNil())

			err := ctx.Client.Get(ctx, client.ObjectKey{
				Namespace: vm.Namespace,
				Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, vm.UID),
			}, &lifecyclev1.LifecycleState{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	When("the capability is off", func() {
		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.LifecycleHooks = false
			})
		})

		It("deletes without pausing", func() {
			Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
			Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
			Expect(hookReadyCondition()).To(BeNil())
		})
	})

	When("no hook is registered for the stage", func() {
		JustBeforeEach(func() {
			Expect(ctx.Client.DeleteAllOf(
				ctx,
				&lifecyclev1.LifecycleSubscribedStages{},
				client.InNamespace(vm.Namespace))).To(Succeed())
		})

		It("deletes without pausing", func() {
			Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
			Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
		})
	})
}
