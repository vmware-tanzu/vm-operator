// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package sequence

import (
	"context"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

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
		log.Error(err, "Failed to delete VM")
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
		log.Info("Executing step", "step", step.Name())
		if err != nil {
			log.Error(err, "Step failed", "step", step.Name())
			return err
		}
		log.Info("Step succeeded", "name", step.Name())
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
