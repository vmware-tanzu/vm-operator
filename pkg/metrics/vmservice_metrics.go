// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

var (
	vmMetricsOnce sync.Once
	vmMetrics     *VMMetrics
)

type VMMetrics struct {
	statusConditionStatus *prometheus.GaugeVec
	statusPhase           *prometheus.GaugeVec
	powerState            *prometheus.GaugeVec
	statusIP              *prometheus.GaugeVec
}

func NewVMMetrics() *VMMetrics {
	vmMetricsOnce.Do(func() {
		vmMetrics = &VMMetrics{
			statusConditionStatus: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: metricsNamespace,
					Name:      "vm_status_condition_status",
					Help:      "True/False/Unknown status of a specific condition on a VM resource"},
				[]string{vmNameLabel, vmNamespaceLabel, conditionTypeLabel, conditionReasonLabel},
			),
			statusPhase: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: metricsNamespace,
					Name:      "vm_status_phase",
					Help:      "True/False/Unknown status of a creating/created/unknown on a VM resource"},
				[]string{vmNameLabel, vmNamespaceLabel, phaseLabel},
			),
			powerState: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: metricsNamespace,
					Name:      "vm_powerstate",
					Help:      "Desired and current power state on a VM resource"},
				[]string{vmNameLabel, vmNamespaceLabel, specLabel, statusLabel},
			),

			statusIP: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: metricsNamespace,
					Name:      "vm_status_ip",
					Help:      "IP address assignment status of a VM resource"},
				[]string{vmNameLabel, vmNamespaceLabel},
			),
		}

		metrics.Registry.MustRegister(
			vmMetrics.statusConditionStatus,
			vmMetrics.statusPhase,
			vmMetrics.powerState,
			vmMetrics.statusIP,
		)
	})

	return vmMetrics
}

func (vmm *VMMetrics) RegisterVMCreateOrUpdateMetrics(vmCtx *context.VirtualMachineContext) {
	vmm.registerVMStatusConditions(vmCtx)
	vmm.registerVMStatusCreationPhase(vmCtx)
	vmm.registerVMPowerState(vmCtx)
	vmm.registerVMStatusIP(vmCtx)
}

// DeleteMetrics deletes metrics for a specific VM post deletion reconcile.
// It is critical to stop reporting metrics for a deleted VM resource.
func (vmm *VMMetrics) DeleteMetrics(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Deleting metrics for VM")

	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
	}

	// Delete the 'vm.status.condition' metrics.
	vmm.statusConditionStatus.DeletePartialMatch(labels)

	// Delete the 'vm.status.phase' metrics.
	vmm.statusPhase.DeletePartialMatch(labels)

	// Delete the 'vm.spec.powerState' metrics.
	vmm.powerState.DeletePartialMatch(labels)

	// Delete the 'vm.status.ip' metrics.
	vmm.statusIP.DeletePartialMatch(labels)
}

func (vmm *VMMetrics) registerVMStatusConditions(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM condition")

	// Delete the previous metrics to address any VM condition reason update.
	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
	}
	vmm.statusConditionStatus.DeletePartialMatch(labels)

	for _, condition := range vm.Status.Conditions {
		labels := prometheus.Labels{
			vmNameLabel:          vm.Name,
			vmNamespaceLabel:     vm.Namespace,
			conditionTypeLabel:   string(condition.Type),
			conditionReasonLabel: condition.Reason,
		}
		vmm.statusConditionStatus.With(labels).Set(func() float64 {
			switch condition.Status {
			case corev1.ConditionTrue:
				return 1
			case corev1.ConditionFalse:
				return 0
			case corev1.ConditionUnknown:
				return -1
			}
			return -1
		}())
	}
}

func (vmm *VMMetrics) registerVMStatusCreationPhase(vmCtx *context.VirtualMachineContext) {
	vmCtx.Logger.V(5).Info("Adding metrics for VM status creation phase")
	vm := vmCtx.VM

	// Delete the previous metrics to address any VM status phase update.
	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
	}
	vmm.statusPhase.DeletePartialMatch(labels)

	newLabels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		phaseLabel:       string(vm.Status.Phase),
	}
	vmm.statusPhase.With(newLabels).Set(1)
}

func (vmm *VMMetrics) registerVMPowerState(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM power state")

	// Delete the existing power state metrics to address any VM's power state change.
	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
	}
	vmm.powerState.DeletePartialMatch(labels)

	newLabels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		specLabel:        string(vm.Spec.PowerState),
		statusLabel:      string(vm.Status.PowerState),
	}
	vmm.powerState.With(newLabels).Set(1)
}

func (vmm *VMMetrics) registerVMStatusIP(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM IP address assignment status")

	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
	}
	vmm.statusIP.With(labels).Set(func() float64 {
		if vm.Status.VmIp == "" {
			return 0
		}
		return 1
	}())
}
