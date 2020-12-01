package worker

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
)

// Worker represents a prober worker interface.
type Worker interface {
	GetQueue() workqueue.DelayingInterface
	AddAfterToQueue(item client.ObjectKey, periodSeconds int32)
	DoProbe(vm *vmopv1alpha1.VirtualMachine) error
	ProcessProbeResult(vm *vmopv1alpha1.VirtualMachine, res probe.Result, err error) error
}

// SetVirtualMachineCondition sets the given condition status, reason and message in VirtualMachineCondition.
func SetVirtualMachineCondition(vm *vmopv1alpha1.VirtualMachine,
	conditionType vmopv1alpha1.VirtualMachineConditionType,
	status metav1.ConditionStatus,
	reason, message string) (statusChanged bool) {

	newCondition := vmopv1alpha1.VirtualMachineCondition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	for idx, c := range vm.Status.Conditions {
		if c.Type != newCondition.Type {
			continue
		}

		if c.Status == newCondition.Status {
			return false
		}

		// Replace old condition if the new condition is a transition.
		vm.Status.Conditions[idx] = newCondition
		return true
	}

	vm.Status.Conditions = append(vm.Status.Conditions, newCondition)
	return true
}
