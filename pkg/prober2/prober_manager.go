// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package prober2

import (
	goctx "context"
	"fmt"
	"reflect"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/worker"
	vmoprecord "github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	proberManagerName       = "virtualmachine-prober-manager"
	readinessProbeQueueName = "readinessProbeQueue"

	// defaultPeriodSeconds represents the default value for the frequency (in seconds) to perform the probe.
	// We use the same default value as the kubernetes container probe.
	defaultPeriodSeconds = 10

	// the number of readiness workers.
	// TODO: find a way to calibrate it.
	numberOfReadinessWorkers = 5
)

// Manager represents a prober manager interface.
type Manager interface {
	ctrlmgr.Runnable
	AddToProberManager(vm *vmopv1.VirtualMachine)
	RemoveFromProberManager(vm *vmopv1.VirtualMachine)
}

// manager represents the probe manager, which implements the ManagerInterface.
type manager struct {
	client         client.Client
	readinessQueue workqueue.DelayingInterface
	prober         *probe.Prober
	log            logr.Logger
	recorder       vmoprecord.Recorder

	workersWG sync.WaitGroup

	// We will use AddAfter to add an item to the queue, which will insert the item to a heap first
	// if the time duration set in the AddAfter is not zero. vmReadinessProbeList can be used to avoid
	// adding VMs to the readiness queue when this VM is already in the heap but not in the queue.
	readinessMutex       sync.Mutex
	vmReadinessProbeList map[string]vmopv1.VirtualMachineReadinessProbeSpec
}

// NewManger initializes a prober manager.
func NewManger(client client.Client, record vmoprecord.Recorder, vmProvider vmprovider.VirtualMachineProviderInterfaceA2) Manager {
	probeManager := &manager{
		client:               client,
		readinessQueue:       workqueue.NewNamedDelayingQueue(readinessProbeQueueName),
		prober:               probe.NewProber(vmProvider),
		log:                  ctrl.Log.WithName(proberManagerName),
		recorder:             record,
		vmReadinessProbeList: make(map[string]vmopv1.VirtualMachineReadinessProbeSpec),
	}
	return probeManager
}

// AddToManager adds the probe manager controller manager.
func AddToManager(mgr ctrlmgr.Manager, vmProvider vmprovider.VirtualMachineProviderInterfaceA2) (Manager, error) {
	probeRecorder := vmoprecord.New(mgr.GetEventRecorderFor(proberManagerName))

	// Add the probe manager explicitly as runnable in order to receive a Start() event.
	m := NewManger(mgr.GetClient(), probeRecorder, vmProvider)
	if err := mgr.Add(m); err != nil {
		return nil, err
	}

	return m, nil
}

// AddToProberManager adds a VM to the prober manager.
func (m *manager) AddToProberManager(vm *vmopv1.VirtualMachine) {
	vmName := vm.NamespacedName()
	m.log.V(4).Info("Add to prober manager", "vm", vmName)

	m.readinessMutex.Lock()
	defer m.readinessMutex.Unlock()

	if vm.Spec.ReadinessProbe.TCPSocket != nil || vm.Spec.ReadinessProbe.GuestHeartbeat != nil {
		// if the VM is not in the list, or its readiness probe spec has been updated, immediately add it to the queue
		// otherwise, ignore it.
		if oldProbe, ok := m.vmReadinessProbeList[vmName]; ok && reflect.DeepEqual(oldProbe, vm.Spec.ReadinessProbe) {
			m.log.V(4).Info("VM is already in the readiness probe list and its probe spec is not updated, skip it", "vm", vmName)
			return
		}

		m.readinessQueue.Add(client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace})
		m.vmReadinessProbeList[vmName] = vm.Spec.ReadinessProbe
	} else {
		delete(m.vmReadinessProbeList, vmName)
	}
}

// RemoveFromProberManager removes a VM from the prober manager.
func (m *manager) RemoveFromProberManager(vm *vmopv1.VirtualMachine) {
	vmName := vm.NamespacedName()
	m.log.V(4).Info("Remove from prober manager", "vm", vmName)

	m.readinessMutex.Lock()
	defer m.readinessMutex.Unlock()
	delete(m.vmReadinessProbeList, vmName)
}

// Start starts the probe manager.
func (m *manager) Start(ctx goctx.Context) error {
	m.log.Info("Start VirtualMachine Probe Manager")
	defer m.log.Info("Stop VirtualMachine Probe Manager")

	m.log.Info("Starting readiness workers", "count", numberOfReadinessWorkers)
	m.workersWG.Add(numberOfReadinessWorkers)
	for i := 0; i < numberOfReadinessWorkers; i++ {
		readinessWorker := worker.NewReadinessWorker(m.readinessQueue, m.prober, m.client, m.recorder)
		m.worker(readinessWorker)
	}

	<-ctx.Done()

	m.readinessQueue.ShutDown()
	m.workersWG.Wait()
	return nil
}

// worker starts the probe manager workers, which are used to process items from queues.
func (m *manager) worker(in worker.Worker) {
	go func() {
		defer m.workersWG.Done()

		for {
			if quit := m.processItemFromQueue(in); quit {
				return
			}
		}
	}()
}

// processItemFromQueue gets a VM from a probe queue and processes the VM.
func (m *manager) processItemFromQueue(w worker.Worker) bool {
	queue := w.GetQueue()
	itemIf, quit := queue.Get()
	if quit {
		// stop the probe manager
		return true
	}
	defer queue.Done(itemIf)

	item := itemIf.(client.ObjectKey)

	vm := &vmopv1.VirtualMachine{}
	if err := m.client.Get(goctx.Background(), item, vm); err != nil {
		if !apierrors.IsNotFound(err) {
			// Get VM error, immediately re-queue the VM.
			queue.Add(item)
		}
		return false
	}

	ctx, err := w.CreateProbeContext(vm)
	if err != nil || ctx == nil {
		return false
	}

	if !vm.ObjectMeta.DeletionTimestamp.IsZero() {
		ctx.Logger.V(4).Info("the VirtualMachine is marked for deletion, skip running the probe")
		return false
	}

	err = m.processVMProbe(w, ctx)
	// Immediately re-queue the request if error occurs.
	m.addItemToQueue(queue, ctx, item, err != nil)

	return false
}

// processVMProbe processes the Probe specified in VM spec.
func (m *manager) processVMProbe(w worker.Worker, ctx *context.ProbeContext) error {
	if ctx.VM.Status.PowerState != vmopv1.VirtualMachinePowerStateOn {
		// If a vm is not powered on, we don't run probes against it and translate probe result to failure.
		// Populate the Condition and update the VM status.
		ctx.Logger.V(4).Info("the VirtualMachine is not powered on")
		return w.ProcessProbeResult(ctx, probe.Failure, fmt.Errorf("virtual machine is not powered on"))
	}
	return w.DoProbe(ctx)
}

// addItemToQueue adds the vm to the queue. If immediate is true, immediately add the item.
// Otherwise, add to queue after a time period.
func (m *manager) addItemToQueue(queue workqueue.DelayingInterface, ctx *context.ProbeContext, item client.ObjectKey, immediate bool) {
	if immediate {
		queue.Add(item)
	} else {
		periodSeconds := ctx.PeriodSeconds
		if periodSeconds <= 0 {
			periodSeconds = defaultPeriodSeconds
		}
		queue.AddAfter(item, time.Duration(periodSeconds)*time.Second)
	}
}
