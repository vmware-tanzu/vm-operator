// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
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

func (vmm *VMMetrics) RegisterVMCreateOrUpdateMetrics(ctx context.Context, vm *vmopv1.VirtualMachine) {
	vmm.registerVMStatusConditions(ctx, vm)
	vmm.registerVMStatusCreationPhase(ctx, vm)
	vmm.registerVMPowerState(ctx, vm)
	vmm.registerVMStatusIP(ctx, vm)
}

// DeleteMetrics deletes metrics for a specific VM post deletion reconcile.
// It is critical to stop reporting metrics for a deleted VM resource.
func (vmm *VMMetrics) DeleteMetrics(ctx context.Context, name, namespace string) {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(5).Info("Deleting metrics for VM")

	labels := prometheus.Labels{
		vmNameLabel:      name,
		vmNamespaceLabel: namespace,
	}

	vmm.statusConditionStatus.DeletePartialMatch(labels)
	vmm.statusPhase.DeletePartialMatch(labels)
	vmm.powerState.DeletePartialMatch(labels)
	vmm.statusIP.DeletePartialMatch(labels)
}

func (vmm *VMMetrics) registerVMStatusConditions(ctx context.Context, vm *vmopv1.VirtualMachine) {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(5).Info("Adding metrics for VM condition")

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
		vmm.statusConditionStatus.With(labels).Set(conditionStatusToFloat(condition.Status))
	}
}

func (vmm *VMMetrics) registerVMStatusCreationPhase(ctx context.Context, vm *vmopv1.VirtualMachine) {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(5).Info("Adding metrics for VM status creation phase")

	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
	}
	vmm.statusPhase.DeletePartialMatch(labels)

	var phase string
	// v1a2 dropped the Phase field. In practice, the only Phase we'd typically see is
	// Created & Creating so use the created condition to determine that.
	if c := conditions.Get(vm, vmopv1.VirtualMachineConditionCreated); c != nil {
		switch c.Status {
		case metav1.ConditionTrue:
			phase = "Created"
		case metav1.ConditionFalse:
			phase = "Creating"
		default:
			phase = "Unknown"
		}
	}

	newLabels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		phaseLabel:       phase,
	}
	vmm.statusPhase.With(newLabels).Set(1)
}

func (vmm *VMMetrics) registerVMPowerState(ctx context.Context, vm *vmopv1.VirtualMachine) {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(5).Info("Adding metrics for VM power state")

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

func (vmm *VMMetrics) registerVMStatusIP(ctx context.Context, vm *vmopv1.VirtualMachine) {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(5).Info("Adding metrics for VM IP address assignment status")

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

func conditionStatusToFloat(status metav1.ConditionStatus) float64 {
	switch status {
	case metav1.ConditionTrue:
		return 1
	case metav1.ConditionFalse:
		return 0
	default:
		return -1
	}
}
