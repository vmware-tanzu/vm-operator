/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package sequence

import (
	"context"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/vmprovider/providers/vsphere/resources"

	"github.com/golang/glog"
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
		glog.Errorf("Failed to delete Vm: %s", err)
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
		glog.Infof("Executing step %s", step.Name())
		if err != nil {
			glog.Infof("Step %s failed %s", step.Name(), err)
			return err
		}
		glog.Infof("Step %s succeeded", step.Name())
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
