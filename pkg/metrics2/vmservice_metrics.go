// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package metrics2

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
			vmMetrics.powerState,
			vmMetrics.statusIP,
		)
	})

	return vmMetrics
}

func (vmm *VMMetrics) RegisterVMCreateOrUpdateMetrics(vmCtx *context.VirtualMachineContextA2) {
	vmm.registerVMStatusConditions(vmCtx)
	vmm.registerVMPowerState(vmCtx)
	vmm.registerVMStatusIP(vmCtx)
}

// DeleteMetrics deletes metrics for a specific VM post deletion reconcile.
// It is critical to stop reporting metrics for a deleted VM resource.
func (vmm *VMMetrics) DeleteMetrics(vmCtx *context.VirtualMachineContextA2) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Deleting metrics for VM")

	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
	}

	// Delete the 'vm.status.condition' metrics.
	vmm.statusConditionStatus.DeletePartialMatch(labels)

	// Delete the 'vm.spec.powerState' metrics.
	vmm.powerState.DeletePartialMatch(labels)

	// Delete the 'vm.status.ip' metrics.
	vmm.statusIP.DeletePartialMatch(labels)
}

func (vmm *VMMetrics) registerVMStatusConditions(vmCtx *context.VirtualMachineContextA2) {
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
			conditionTypeLabel:   condition.Type,
			conditionReasonLabel: condition.Reason,
		}
		vmm.statusConditionStatus.With(labels).Set(func() float64 {
			switch condition.Status {
			case metav1.ConditionTrue:
				return 1
			case metav1.ConditionFalse:
				return 0
			case metav1.ConditionUnknown:
				return -1
			}
			return -1
		}())
	}
}

func (vmm *VMMetrics) registerVMPowerState(vmCtx *context.VirtualMachineContextA2) {
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

func (vmm *VMMetrics) registerVMStatusIP(vmCtx *context.VirtualMachineContextA2) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM IP address assignment status")

	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
	}
	vmm.statusIP.With(labels).Set(func() float64 {
		var ip string
		if vm.Status.Network != nil {
			if ip = vm.Status.Network.PrimaryIP4; ip == "" {
				ip = vm.Status.Network.PrimaryIP6
			}
		}
		if ip == "" {
			return 0
		}
		return 1
	}())
}
