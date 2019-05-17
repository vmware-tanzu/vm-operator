/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package sequence

import (
	"context"

	"k8s.io/klog"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

type VirtualMachinePowerOffStep struct {
	vm    *v1alpha1.VirtualMachine
	resVm *resources.VirtualMachine
}

func (step VirtualMachinePowerOffStep) Name() string { return "PowerOff" }

func (step VirtualMachinePowerOffStep) Execute(ctx context.Context) error {

	err := step.resVm.SetPowerState(ctx, v1alpha1.VirtualMachinePoweredOff)
	if err != nil {
		return err
	}

	return nil
}

type VirtualMachineDeleteStep struct {
	vm    *v1alpha1.VirtualMachine
	resVm *resources.VirtualMachine
}

func (step VirtualMachineDeleteStep) Name() string { return "Delete Vm" }

func (step VirtualMachineDeleteStep) Execute(ctx context.Context) error {
	err := step.resVm.Delete(ctx)
	if err != nil {
		klog.Errorf("Failed to delete Vm: %s", err)
		return err
	}

	return nil
}

type VirtualMachineDeleteSequence struct {
	steps []SequenceStep
}

func (seq VirtualMachineDeleteSequence) GetName() string {
	return "VirtualMachineDeleteSequence"
}

func (seq VirtualMachineDeleteSequence) Execute(ctx context.Context) error {
	for _, step := range seq.steps {
		err := step.Execute(ctx)
		klog.Infof("Executing step %s", step.Name())
		if err != nil {
			klog.Infof("Step %s failed %s", step.Name(), err)
			return err
		}
		klog.Infof("Step %s succeeded", step.Name())
	}
	return nil
}

func NewVirtualMachineDeleteSequence(vm *v1alpha1.VirtualMachine, resVm *resources.VirtualMachine) VirtualMachineDeleteSequence {
	m := []SequenceStep{
		VirtualMachinePowerOffStep{vm, resVm},
		VirtualMachineDeleteStep{vm, resVm},
	}

	return VirtualMachineDeleteSequence{m}
}
