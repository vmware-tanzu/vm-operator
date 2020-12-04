// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	goctx "context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/worker"
	vmoprecord "github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	proberManagerName       = "virtualmachine-prober-manager"
	readinessProbeQueueName = "readinessProbeQueue"

	// defaultPeriodSeconds represents the default value for the frequency (in seconds) to perform the probe.
	// We use the same default value as the kubernetes container probe.
	defaultPeriodSeconds = 10
)

// Manager represents a prober manager interface.
// TODO: add runnable interface and other functions
type Manager interface {
}

// manager represents the probe manager, which implements the ManagerInterface.
type manager struct {
	client         client.Client
	readinessQueue workqueue.DelayingInterface
	prober         *probe.Prober
	log            logr.Logger
	recorder       vmoprecord.Recorder
}

// NewManger initializes a prober manager.
func NewManger(client client.Client, record vmoprecord.Recorder) Manager {
	probeManager := &manager{
		client:         client,
		readinessQueue: workqueue.NewNamedDelayingQueue(readinessProbeQueueName),
		prober:         probe.NewProber(),
		log:            ctrl.Log.WithName(proberManagerName),
		recorder:       record,
	}
	return probeManager
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

	vm := &vmoperatorv1alpha1.VirtualMachine{}
	if err := m.client.Get(goctx.Background(), item, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		// Get VM error, immediately re-queue the VM.
		queue.Add(item)
		return false
	}

	ctx, err := w.CreateProbeContext(vm)
	if err != nil {
		return false
	}

	if ctx.ProbeSpec == nil {
		ctx.Logger.V(4).Info("probe is not specified")
		return false
	}

	if !vm.ObjectMeta.DeletionTimestamp.IsZero() {
		ctx.Logger.V(4).Info("the VirtualMachine is marked for deletion, skip running the probe")
		return false
	}

	err = m.processVMProbe(w, ctx)
	// Set addAfter as true if there is no error. Immediately re-queue the request if error occurs.
	addAfter := err == nil
	m.addItemToQueue(queue, ctx, addAfter)

	return false
}

// processVMProbe processes the Probe specified in VM spec.
func (m *manager) processVMProbe(w worker.Worker, ctx *context.ProbeContext) error {
	vm := ctx.VM
	if vm.Status.PowerState != vmoperatorv1alpha1.VirtualMachinePoweredOn {
		// If a vm is not powered on, we don't run probes against it and translate probe result to failure.
		// Populate the Condition and update the VM status.
		ctx.Logger.V(4).Info("the VirtualMachine is not powered on")
		return w.ProcessProbeResult(ctx, probe.Failure, fmt.Errorf("virtual machine is not powered on"))
	}
	return w.DoProbe(ctx)
}

// addItemToQueue adds the vm to the queue. If addAfter is false, immediately add the item.
// Otherwise, add to queue after a time period.
func (m *manager) addItemToQueue(queue workqueue.DelayingInterface, ctx *context.ProbeContext, addAfter bool) {
	vm := ctx.VM
	item := client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace}
	if !addAfter {
		// if fails to update conditions in VM status, immediately re-queue the request
		queue.Add(item)
	} else {
		periodSeconds := ctx.ProbeSpec.PeriodSeconds
		if periodSeconds <= 0 {
			periodSeconds = defaultPeriodSeconds
		}
		queue.AddAfter(item, time.Duration(periodSeconds)*time.Second)
	}
}
