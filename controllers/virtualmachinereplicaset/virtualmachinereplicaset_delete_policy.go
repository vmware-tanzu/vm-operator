/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinereplicaset

import (
	"sort"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

type (
	deletePriority     float64
	deletePriorityFunc func(machine *vmopv1.VirtualMachine) deletePriority
)

const (
	mustDelete    deletePriority = 100.0
	couldDelete   deletePriority = 50.0
	mustNotDelete deletePriority = 0.0
)

func randomDeletePolicy(vm *vmopv1.VirtualMachine) deletePriority {
	if !vm.DeletionTimestamp.IsZero() {
		return mustDelete
	}

	// TODO: when we support a Ready/healthy condition, that should have a
	// higher priority for deletion.  For now, all VMs that are not marked
	// for deletion get the same priority.
	return couldDelete
}

type sortableMachines struct {
	machines []*vmopv1.VirtualMachine
	priority deletePriorityFunc
}

func (m sortableMachines) Len() int      { return len(m.machines) }
func (m sortableMachines) Swap(i, j int) { m.machines[i], m.machines[j] = m.machines[j], m.machines[i] }
func (m sortableMachines) Less(i, j int) bool {
	priorityI, priorityJ := m.priority(m.machines[i]), m.priority(m.machines[j])
	if priorityI == priorityJ {
		// In cases where the priority is identical, it should be ensured that
		// the same machine order is returned each time.
		// Ordering by name is a simple way to do this.
		return m.machines[i].Name < m.machines[j].Name
	}
	return priorityJ < priorityI // high to low
}

func getMachinesToDeletePrioritized(filteredMachines []*vmopv1.VirtualMachine, diff int, fun deletePriorityFunc) []*vmopv1.VirtualMachine {
	if diff >= len(filteredMachines) {
		return filteredMachines
	} else if diff <= 0 {
		return []*vmopv1.VirtualMachine{}
	}

	sortable := sortableMachines{
		machines: filteredMachines,
		priority: fun,
	}
	sort.Sort(sortable)

	return sortable.machines[:diff]
}

// TODO(muchhals): Needed when we add delete priority field.
func getDeletePriorityFunc(_ *vmopv1.VirtualMachineReplicaSet) (deletePriorityFunc, error) {
	// TODO: Expand when we start supporting other delete policies.
	return randomDeletePolicy, nil
}
