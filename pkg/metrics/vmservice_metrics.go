package metrics

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

var (
	once sync.Once
)

const (
	metricsNamespace     = "vmservice"
	vmNameLabel          = "vm_name"
	vmNamespaceLabel     = "vm_namespace"
	vmUniqueIDLabel      = "vm_uid"
	conditionTypeLabel   = "condition_type"
	conditionReasonLabel = "condition_reason"
	phaseLabel           = "phase"
	specLabel            = "spec"
	statusLabel          = "status"
)

type VMMetrics struct {
	statusConditionStatus             *prometheus.GaugeVec
	statusConditionLastTransitionTime *prometheus.HistogramVec
	statusPhase                       *prometheus.GaugeVec
	powerState                        *prometheus.GaugeVec
	statusIP                          *prometheus.GaugeVec
}

func NewVMMetrics() *VMMetrics {
	vmm := &VMMetrics{
		statusConditionStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "vm_status_condition_status",
				Help:      "True/False/Unknown status of a specific condition on a VM resource"},
			[]string{vmNameLabel, vmNamespaceLabel, conditionTypeLabel, conditionReasonLabel, vmUniqueIDLabel},
		),
		statusConditionLastTransitionTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Name:      "vm_status_condition_last_transition_time",
				Help:      "Unix timestamp of last transition of a specific condition on a VM resource"},
			[]string{vmNameLabel, vmNamespaceLabel, conditionTypeLabel, conditionReasonLabel, vmUniqueIDLabel},
		),
		statusPhase: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "vm_status_phase",
				Help:      "True/False/Unknown status of a creating/created/unknown on a VM resource"},
			[]string{vmNameLabel, vmNamespaceLabel, phaseLabel, vmUniqueIDLabel},
		),
		powerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "vm_powerstate",
				Help:      "Desired and current power state on a VM resource"},
			[]string{vmNameLabel, vmNamespaceLabel, specLabel, statusLabel, vmUniqueIDLabel},
		),

		statusIP: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "vm_status_ip",
				Help:      "IP address assignment status of a VM resource"},
			[]string{vmNameLabel, vmNamespaceLabel, vmUniqueIDLabel},
		),
	}
	return vmm.registerMetrics()
}

func (vmm *VMMetrics) registerMetrics() *VMMetrics {
	once.Do(func() {
		metrics.Registry.MustRegister(
			vmm.statusConditionStatus,
			vmm.statusConditionLastTransitionTime,
			vmm.statusPhase,
			vmm.powerState,
			vmm.statusIP,
		)
	})
	return vmm
}

func (vmm *VMMetrics) RegisterVMCreateOrUpdateMetrics(vmCtx *context.VirtualMachineContext) {
	vmm.registerVMStatusConditions(vmCtx)
	vmm.registerVMStatusCreationPhase(vmCtx)
	vmm.registerVMPowerState(vmCtx)
	vmm.registerVMStatusIP(vmCtx)
}

func (vmm *VMMetrics) RegisterVMDeletionMetrics(vmCtx *context.VirtualMachineContext) {
	vmm.registerVMStatusPhaseDeletion(vmCtx)
}

// DeleteMetrics deletes metrics for a specific VM post deletion reconcile.
// It is critical to stop reporting metrics for a deleted VM resource.
func (vmm *VMMetrics) DeleteMetrics(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Deleting metrics for VM")

	// Delete the 'vm.status.condition' metrics with the current condition types and reasons.
	// Note: We actually can't delete all condition metrics here as we don't have the exact previous reasons stored.
	// TODO: Apply the DeletePartialMatch() function to delete all condition metrics once the following PR is released.
	// https://github.com/prometheus/client_golang/pull/1013
	for _, condition := range vm.Status.Conditions {
		conditionLabels := prometheus.Labels{
			vmNameLabel:          vm.Name,
			vmNamespaceLabel:     vm.Namespace,
			vmUniqueIDLabel:      vm.Status.UniqueID,
			conditionTypeLabel:   string(condition.Type),
			conditionReasonLabel: condition.Reason,
		}
		vmm.statusConditionStatus.Delete(conditionLabels)
		vmm.statusConditionLastTransitionTime.Delete(conditionLabels)
	}

	// Delete the 'vm.status.phase' metrics with all possible status phases.
	phases := []vmopv1alpha1.VMStatusPhase{
		vmopv1alpha1.Creating,
		vmopv1alpha1.Created,
		vmopv1alpha1.Deleting,
		vmopv1alpha1.Deleted,
		vmopv1alpha1.Unknown,
		vmopv1alpha1.VMStatusPhase(""),
	}
	for _, phase := range phases {
		phaseLabels := prometheus.Labels{
			vmNameLabel:      vm.Name,
			vmNamespaceLabel: vm.Namespace,
			vmUniqueIDLabel:  vm.Status.UniqueID,
			phaseLabel:       string(phase),
		}
		vmm.statusPhase.Delete(phaseLabels)
	}

	// Delete the 'vm.spec.powerState' metrics.
	vmm.deleteAllPowerStateMetrics(vmCtx)

	// Delete the 'vm.status.ip' metrics.
	statusIPLabels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		vmUniqueIDLabel:  vm.Status.UniqueID,
	}
	vmm.statusIP.Delete(statusIPLabels)
}

func (vmm *VMMetrics) registerVMStatusConditions(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM condition")
	for _, condition := range vm.Status.Conditions {
		labels := prometheus.Labels{
			vmNameLabel:          vm.Name,
			vmNamespaceLabel:     vm.Namespace,
			vmUniqueIDLabel:      vm.Status.UniqueID,
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
		vmm.statusConditionLastTransitionTime.With(labels).Observe(float64(condition.LastTransitionTime.Unix()))
	}
}

func (vmm *VMMetrics) registerVMStatusCreationPhase(vmCtx *context.VirtualMachineContext) {
	vmCtx.Logger.V(5).Info("Adding metrics for VM status creation phase")
	vm := vmCtx.VM
	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		vmUniqueIDLabel:  vm.Status.UniqueID,
		phaseLabel:       string(vm.Status.Phase),
	}
	vmm.statusPhase.With(labels).Set(func() float64 {
		switch vm.Status.Phase {
		case vmopv1alpha1.Created:
			return 1
		case vmopv1alpha1.Creating:
			return 0
		case vmopv1alpha1.Unknown:
			return -1
		}
		return -1
	}())
}

func (vmm *VMMetrics) registerVMPowerState(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM power state")

	// Delete the existing power state metrics to address any VM's power state change.
	vmm.deleteAllPowerStateMetrics(vmCtx)

	newLabels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		vmUniqueIDLabel:  vm.Status.UniqueID,
		specLabel:        string(vm.Spec.PowerState),
		statusLabel:      string(vm.Status.PowerState),
	}
	// Set the value to 1 to indicate the VM's current spec and status power state.
	vmm.powerState.With(newLabels).Set(1)
}

func (vmm *VMMetrics) registerVMStatusIP(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM IP address assignment status")

	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		vmUniqueIDLabel:  vm.Status.UniqueID,
	}
	vmm.statusIP.With(labels).Set(func() float64 {
		if vm.Status.VmIp == "" {
			return 0
		}
		return 1
	}())
}

func (vmm *VMMetrics) registerVMStatusPhaseDeletion(vmCtx *context.VirtualMachineContext) {
	// TODO: This is not working since we will delete all status phase metrics in the DeleteMetrics() function.
	// We should probably use a new metrics here to store the deleteion related data (e.g. deletion state and time, etc.).
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM status deletion phase")
	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		vmUniqueIDLabel:  vm.Status.UniqueID,
		phaseLabel:       string(vm.Status.Phase),
	}
	vmm.statusPhase.With(labels).Set(func() float64 {
		switch vm.Status.Phase {
		case vmopv1alpha1.Deleted:
			return 1
		case vmopv1alpha1.Deleting:
			return 0
		case vmopv1alpha1.Unknown:
			return -1
		}
		return -1
	}())
}

func (vmm *VMMetrics) deleteAllPowerStateMetrics(vmCtx *context.VirtualMachineContext) {
	// TODO: Apply the DeletePartialMatch() function to simply the code below
	// once https://github.com/prometheus/client_golang/pull/1013 is released.
	vm := vmCtx.VM
	powerStates := []vmopv1alpha1.VirtualMachinePowerState{
		vmopv1alpha1.VirtualMachinePoweredOff,
		vmopv1alpha1.VirtualMachinePoweredOn,
		vmopv1alpha1.VirtualMachinePowerState(""),
	}
	for _, spec := range powerStates {
		for _, status := range powerStates {
			labels := prometheus.Labels{
				vmNameLabel:      vm.Name,
				vmNamespaceLabel: vm.Namespace,
				vmUniqueIDLabel:  vm.Status.UniqueID,
				specLabel:        string(spec),
				statusLabel:      string(status),
			}
			vmm.powerState.Delete(labels)
		}
	}
}
