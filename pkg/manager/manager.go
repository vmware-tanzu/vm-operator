// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	goctx "context"
	"fmt"

	"github.com/pkg/errors"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	// Load the GCP authentication plug-in.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// Manager is a VM Operator controller manager.
type Manager interface {
	ctrlmgr.Manager

	// GetContext returns the controller manager's context.
	GetContext() *context.ControllerManagerContext
}

// New returns a new VM Operator controller manager.
func New(opts Options) (Manager, error) {

	// Ensure the default options are set.
	opts.defaults()

	_ = clientgoscheme.AddToScheme(opts.Scheme)
	_ = vmopv1.AddToScheme(opts.Scheme)
	_ = cnsv1alpha1.AddToScheme(opts.Scheme)
	// +kubebuilder:scaffold:scheme

	// Build the controller manager.
	mgr, err := ctrlmgr.New(opts.KubeConfig, ctrlmgr.Options{
		Scheme:                  opts.Scheme,
		MetricsBindAddress:      opts.MetricsAddr,
		LeaderElection:          opts.LeaderElectionEnabled,
		LeaderElectionID:        opts.LeaderElectionID,
		LeaderElectionNamespace: opts.PodNamespace,
		SyncPeriod:              &opts.SyncPeriod,
		Namespace:               opts.WatchNamespace,
		NewCache:                opts.NewCache,
		CertDir:                 opts.WebhookSecretVolumeMountPath,
		Port:                    opts.WebhookServiceContainerPort,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create manager")
	}

	// Build the controller manager context.
	controllerManagerContext := &context.ControllerManagerContext{
		Context:                 goctx.Background(),
		Namespace:               opts.PodNamespace,
		Name:                    opts.PodName,
		LeaderElectionID:        opts.LeaderElectionID,
		LeaderElectionNamespace: opts.PodNamespace,
		MaxConcurrentReconciles: opts.MaxConcurrentReconciles,
		Client:                  mgr.GetClient(),
		Logger:                  opts.Logger.WithName(opts.PodName),
		Recorder:                record.New(mgr.GetEventRecorderFor(fmt.Sprintf("%s/%s", opts.PodNamespace, opts.PodName))),
		Scheme:                  opts.Scheme,
		ContainerNode:           opts.ContainerNode,
	}

	// Initialize the VMProvider early.
	if err := setupVmProvider(controllerManagerContext, opts.KubeConfig); err != nil {
		return nil, errors.Wrap(err, "failed to setup VM provider")
	}

	// Add the requested items to the manager.
	if err := opts.AddToManager(controllerManagerContext, mgr); err != nil {
		return nil, errors.Wrap(err, "failed to add resources to the manager")
	}

	// +kubebuilder:scaffold:builder

	return &manager{
		Manager: mgr,
		ctx:     controllerManagerContext,
	}, nil
}

type manager struct {
	ctrlmgr.Manager
	ctx *context.ControllerManagerContext
}

func (m *manager) GetContext() *context.ControllerManagerContext {
	return m.ctx
}
