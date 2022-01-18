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
	vmNamespaceLabel     = "namespace"
	conditionTypeLabel   = "condition_type"
	conditionReasonLabel = "condition_reason"
	vmStatusLabel        = "vm_status"
)

type VMMetrics struct {
	statusConditionStatus             *prometheus.GaugeVec
	statusConditionLastTransitionTime *prometheus.HistogramVec
	statusPhase                       *prometheus.GaugeVec
}

func NewVMMetrics() *VMMetrics {
	vmm := &VMMetrics{
		statusConditionStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "vm_status_condition_status",
				Help:      "True/False/Unknown status of a specific condition on a VM resource"},
			[]string{vmNameLabel, vmNamespaceLabel, conditionTypeLabel, conditionReasonLabel},
		),
		statusConditionLastTransitionTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Name:      "vm_status_condition_last_transition_time",
				Help:      "Unix timestamp of last transition of a specific condition on a VM resource"},
			[]string{vmNameLabel, vmNamespaceLabel, conditionTypeLabel, conditionReasonLabel},
		),
		statusPhase: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "vm_status_phase",
				Help:      "True/False/Unknown status of a creating/created/unknown on a VM resource"},
			[]string{vmNameLabel, vmNamespaceLabel, vmStatusLabel},
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
		)
	})
	return vmm
}

func (vmm *VMMetrics) RegisterVMCreateOrUpdateMetrics(vmCtx *context.VirtualMachineContext) {
	vmm.registerVMStatusConditions(vmCtx)
	vmm.registerVMStatusCreationPhase(vmCtx)
}

func (vmm *VMMetrics) RegisterVMDeletionMetrics(vmCtx *context.VirtualMachineContext) {
	vmm.registerVMStatusPhaseDeletion(vmCtx)
}

func (vmm *VMMetrics) registerVMStatusCreationPhase(vmCtx *context.VirtualMachineContext) {
	vmCtx.Logger.V(5).Info("Adding metrics for VM status creation phase")
	vm := vmCtx.VM
	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		vmStatusLabel:    string(vm.Status.Phase),
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

// DeleteMetrics deletes metrics for a specific VM post deletion as without deletion. It is critical to delete the
// metrics to stop reporting metrics for a deleted resource.
func (vmm *VMMetrics) DeleteMetrics(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Deleting metrics for VM")
	for _, condition := range vm.Status.Conditions {
		labels := prometheus.Labels{
			vmNameLabel:          vm.Name,
			vmNamespaceLabel:     vm.Namespace,
			conditionTypeLabel:   string(condition.Type),
			conditionReasonLabel: condition.Reason,
		}
		vmm.statusConditionStatus.Delete(labels)
		vmm.statusConditionLastTransitionTime.Delete(labels)

	}
	// Delete the metrics for VM current phase
	statusLabels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		vmStatusLabel:    string(vm.Status.Phase),
	}
	vmm.statusPhase.Delete(statusLabels)
	// Delete the metrics for VM's previously recorded phases and also delete metrics for statusPhase set to ""
	phases := []vmopv1alpha1.VMStatusPhase{vmopv1alpha1.Creating, vmopv1alpha1.Created, vmopv1alpha1.Deleting,
		vmopv1alpha1.Deleted, vmopv1alpha1.Unknown, vmopv1alpha1.VMStatusPhase("")}

	for _, phase := range phases {
		statusLabels := prometheus.Labels{
			vmNameLabel:      vm.Name,
			vmNamespaceLabel: vm.Namespace,
			vmStatusLabel:    string(phase),
		}
		vmm.statusPhase.Delete(statusLabels)
	}
}

func (vmm *VMMetrics) registerVMStatusPhaseDeletion(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM status deletion phase")
	labels := prometheus.Labels{
		vmNameLabel:      vm.Name,
		vmNamespaceLabel: vm.Namespace,
		vmStatusLabel:    string(vm.Status.Phase),
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

func (vmm *VMMetrics) registerVMStatusConditions(vmCtx *context.VirtualMachineContext) {
	vm := vmCtx.VM
	vmCtx.Logger.V(5).Info("Adding metrics for VM condition")
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
		vmm.statusConditionLastTransitionTime.With(labels).Observe(float64(condition.LastTransitionTime.Unix()))
	}
}
