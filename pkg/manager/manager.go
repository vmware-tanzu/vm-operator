// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	// Load the GCP authentication plug-in.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	appv1a1 "github.com/vmware-tanzu/vm-operator/external/appplatform/api/v1alpha1"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// Manager is a VM Operator controller manager.
type Manager interface {
	ctrlmgr.Manager

	// GetContext returns the controller manager's pkgctx.
	GetContext() *pkgctx.ControllerManagerContext
}

// New returns a new VM Operator controller manager.
func New(ctx context.Context, opts Options) (Manager, error) {
	// Ensure the default options are set.
	opts.defaults()

	_ = appsv1.AddToScheme(opts.Scheme)
	_ = clientgoscheme.AddToScheme(opts.Scheme)
	_ = ncpv1alpha1.AddToScheme(opts.Scheme)
	_ = cnsv1alpha1.AddToScheme(opts.Scheme)
	_ = netopv1alpha1.AddToScheme(opts.Scheme)
	_ = topologyv1.AddToScheme(opts.Scheme)
	_ = imgregv1a1.AddToScheme(opts.Scheme)
	_ = spqv1.AddToScheme(opts.Scheme)
	_ = byokv1.AddToScheme(opts.Scheme)
	_ = capv1.AddToScheme(opts.Scheme)
	_ = appv1a1.AddToScheme(opts.Scheme)

	_ = vmopv1a1.AddToScheme(opts.Scheme)
	_ = vmopv1a2.AddToScheme(opts.Scheme)
	_ = vmopv1.AddToScheme(opts.Scheme)

	if pkgcfg.FromContext(ctx).NetworkProviderType == pkgcfg.NetworkProviderTypeVPC {
		_ = vpcv1alpha1.AddToScheme(opts.Scheme)
	}

	// Build the controller manager.
	mgr, err := ctrlmgr.New(opts.KubeConfig, ctrlmgr.Options{
		Scheme: opts.Scheme,
		Cache: cache.Options{
			DefaultNamespaces: GetNamespaceCacheConfigs(opts.WatchNamespace),
			DefaultTransform:  cache.TransformStripManagedFields(),
			SyncPeriod:        &opts.SyncPeriod,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					// An informer is created for each watched resource. Due to the
					// number of ConfigMap and Secret resources that may exist,
					// watching each one can result in VM Operator being terminated
					// due to an out-of-memory error, i.e. OOMKill. To avoid this
					// outcome, ConfigMap and Secret resources are not cached.
					&corev1.ConfigMap{},
					&corev1.Secret{},

					// The pkg/exit.Restart function gets a Deployment resource
					// in order to patch it to restart the pods in the
					// deployment when capabilities have changed.
					// Capabilities do not change often enough to warrant
					// caching the Deployment resource, and thus there is no
					// reason to cache Deployment resources as nothing else in
					// VM Operator gets them.
					&appsv1.Deployment{},
				},
			},
		},
		Metrics: metricsserver.Options{
			BindAddress: opts.MetricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			CertDir: opts.WebhookSecretVolumeMountPath,
			Port:    opts.WebhookServiceContainerPort,
		}),
		HealthProbeBindAddress:  opts.HealthProbeBindAddress,
		PprofBindAddress:        opts.PprofBindAddress,
		LeaderElection:          opts.LeaderElectionEnabled,
		LeaderElectionID:        opts.LeaderElectionID,
		LeaderElectionNamespace: opts.PodNamespace,
		NewCache:                opts.NewCache,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create manager: %w", err)
	}

	// Prefix the logger with the pod name.
	logger := opts.Logger.WithName(opts.PodName)

	// Build the controller manager pkgctx.
	controllerManagerContext := &pkgctx.ControllerManagerContext{
		Context:                 logr.NewContext(ctx, logger),
		Namespace:               opts.PodNamespace,
		Name:                    opts.PodName,
		ServiceAccountName:      opts.PodServiceAccountName,
		LeaderElectionID:        opts.LeaderElectionID,
		LeaderElectionNamespace: opts.PodNamespace,
		MaxConcurrentReconciles: opts.MaxConcurrentReconciles,
		Logger:                  logger,
		Recorder:                record.New(mgr.GetEventRecorderFor(fmt.Sprintf("%s/%s", opts.PodNamespace, opts.PodName))),
		ContainerNode:           opts.ContainerNode,
		SyncPeriod:              opts.SyncPeriod,
	}

	if err := opts.InitializeProviders(controllerManagerContext, mgr); err != nil {
		return nil, err
	}

	// Add the requested items to the manager.
	if err := opts.AddToManager(controllerManagerContext, mgr); err != nil {
		return nil, fmt.Errorf("failed to add resources to the manager: %w", err)
	}

	return &manager{
		Manager: mgr,
		ctx:     controllerManagerContext,
	}, nil
}

type manager struct {
	ctrlmgr.Manager
	ctx *pkgctx.ControllerManagerContext
}

func (m *manager) GetContext() *pkgctx.ControllerManagerContext {
	return m.ctx
}
