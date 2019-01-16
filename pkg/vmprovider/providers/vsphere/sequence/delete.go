/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package sequence

import (
	"context"
	"github.com/golang/glog"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
)

type VirtualMachinePowerOffStep struct {
	desiredVm *v1beta1.VirtualMachine
	actualVm *resources.VM
}

func (step VirtualMachinePowerOffStep) Name() string {return "PowerOff"}

func (step VirtualMachinePowerOffStep) Execute(ctx context.Context) error {
	ps, err := step.actualVm.VirtualMachine.PowerState(ctx)
	if err != nil {
		glog.Errorf("Failed to acquire power state: %s", err.Error())
		return err
	}

	glog.Infof("Current power state: %s, desired power state: %s", ps, step.desiredVm.Spec.PowerState)

	if string(ps) == string(v1beta1.VirtualMachinePoweredOff) {
		glog.Info("Vm is already powered off")
		return nil
	}

	// Bring PowerState into conformance
	task, err := step.actualVm.VirtualMachine.PowerOff(ctx)
	if err != nil {
		glog.Errorf("Failed to change power state to %s", v1beta1.VirtualMachinePoweredOff)
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VM Power State change task failed %s", err.Error())
		return err
	}

	return nil
}

type VirtualMachineDeleteStep struct {
	desiredVm *v1beta1.VirtualMachine
	actualVm *resources.VM
}

func (step VirtualMachineDeleteStep) Name() string {return "Delete Vm"}

func (step VirtualMachineDeleteStep) Execute(ctx context.Context) error {
	task, err := step.actualVm.Delete(ctx)
	if err != nil {
		glog.Errorf("Failed to delete Vm: %s", err)
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("Failed to delete Vm: %s", err)
		return err
	}

	return nil
}

type VirtualMachineDeleteSequence struct {
	desiredVm *v1beta1.VirtualMachine
	actualVm *resources.VM
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

func NewVirtualMachineDeleteSequence(desiredVm *v1beta1.VirtualMachine, actualVm *resources.VM) VirtualMachineDeleteSequence {
	m := []SequenceStep{
		VirtualMachinePowerOffStep{desiredVm, actualVm},
		VirtualMachineDeleteStep{desiredVm, actualVm},
	}

	return VirtualMachineDeleteSequence{desiredVm, actualVm, m}
}
