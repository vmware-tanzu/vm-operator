// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	// +kubebuilder:scaffold:imports

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManagerFunc is a function that can be optionally specified with
// the manager's Options in order to explicitly decide what controllers and
// webhooks to add to the manager.
type AddToManagerFunc func(*pkgctx.ControllerManagerContext, ctrlmgr.Manager) error

var AddToManagerNoopFn AddToManagerFunc = func(_ *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
	return nil
}

// InitializeProvidersFunc is a function that can be optionally specified with
// the manager's Options in order to explicitly decide what providers in the
// context are initialized.
type InitializeProvidersFunc func(*pkgctx.ControllerManagerContext, ctrlmgr.Manager) error

var InitializeProvidersNoopFn InitializeProvidersFunc = func(_ *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
	return nil
}

// Options describes the options used to create a new GCM manager.
type Options struct {
	// LeaderElectionEnabled is a flag that enables leader election.
	LeaderElectionEnabled bool

	// LeaderElectionID is the name of the config map to use as the
	// locking resource when configuring leader election.
	LeaderElectionID string

	// HealthProbeBindAddress is the TCP address that the controller should bind to
	// for serving health probes
	HealthProbeBindAddress string

	// PprofBindAddress is the address that the controller should bind to for
	// serving pprof endpoints.
	PprofBindAddress string

	// SyncPeriod is the amount of time to wait between syncing the local
	// object cache with the API server.
	SyncPeriod time.Duration

	// MaxConcurrentReconciles the maximum number of allowed, concurrent
	// reconciles.
	//
	// Defaults to the eponymous constant in this package.
	MaxConcurrentReconciles int

	// MetricsAddr is the net.Addr string for the metrics server.
	MetricsAddr string

	// PodNamespace is the namespace in which the pod running the controller
	// manager is located.
	//
	// Defaults to the eponymous constant in this package.
	PodNamespace string

	// PodName is the name of the pod running the controller manager.
	//
	// Defaults to the eponymous constant in this package.
	PodName string

	// PodServiceAccountName is the name of the pod's service account.
	//
	// Defaults to the eponymous constant in this package.
	PodServiceAccountName string

	// WatchNamespace is the namespace the controllers watch for changes. If
	// no value is specified then all namespaces are watched.
	//
	// Defaults to the eponymous constant in this package.
	WatchNamespace string

	// WebhookServiceContainerPort is the port on which the webhook service
	// expects the webhook server to listen for incoming requests.
	//
	// Defaults to the eponymous constant in this package.
	WebhookServiceContainerPort int

	// WebhookServiceNamespace is the namespace in which the webhook service
	// is located.
	//
	// Defaults to the eponymous constant in this package.
	WebhookServiceNamespace string

	// WebhookServiceName is the name of the webhook service.
	//
	// Defaults to the eponymous constant in this package.
	WebhookServiceName string

	// WebhookSecretNamespace is the namespace in which the webhook secret
	// is located.
	//
	// Defaults to the eponymous constant in this package.
	WebhookSecretNamespace string

	// WebhookSecretName is the name of the webhook secret.
	//
	// Defaults to the eponymous constant in this package.
	WebhookSecretName string

	// WebhookSecretVolumeMountPath is the filesystem path to which the webhook
	// secret is mounted.
	//
	// Defaults to the eponymous constant in this package.
	WebhookSecretVolumeMountPath string

	// EnableWebhookClientVerification determines whether to use client certificate
	// verification for authentication to webhook requests.
	//
	// Defaults to false.
	EnableWebhookClientVerification bool

	// ContainerNode flags whether guest cluster nodes are run in containers (with vcsim).
	//
	// Defaults to the eponymous constant in this package.
	ContainerNode bool

	Logger     *logr.Logger
	KubeConfig *rest.Config
	Scheme     *runtime.Scheme
	NewCache   cache.NewCacheFunc

	// InitializeProviders is a function that can be optionally specified with the manager's Options in order
	// to explicitly initialize the providers.
	InitializeProviders InitializeProvidersFunc

	// AddToManager is a function that can be optionally specified with the manager's Options in order
	// to explicitly decide what controllers and webhooks to add to the manager.
	AddToManager AddToManagerFunc
}

func (o *Options) defaults() {
	if o.Logger == nil {
		o.Logger = &ctrllog.Log
	}

	if o.PodNamespace == "" {
		o.PodNamespace = DefaultPodNamespace
	}

	if o.PodName == "" {
		o.PodName = DefaultPodName
	}

	if o.PodServiceAccountName == "" {
		o.PodServiceAccountName = DefaultPodServiceAccountName
	}

	if o.SyncPeriod == 0 {
		o.SyncPeriod = DefaultSyncPeriod
	}

	if o.KubeConfig == nil {
		o.KubeConfig = config.GetConfigOrDie()
	}

	if o.Scheme == nil {
		o.Scheme = runtime.NewScheme()
	}

	if o.WatchNamespace == "" {
		o.WatchNamespace = DefaultWatchNamespace
	}

	if o.MaxConcurrentReconciles == 0 {
		o.MaxConcurrentReconciles = DefaultMaxConcurrentReconciles
	}

	if o.WebhookServiceContainerPort == 0 {
		o.WebhookServiceContainerPort = DefaultWebhookServiceContainerPort
	}

	if o.WebhookServiceNamespace == "" {
		o.WebhookServiceNamespace = DefaultWebhookServiceNamespace
	}

	if o.WebhookServiceName == "" {
		o.WebhookServiceName = DefaultWebhookServiceName
	}

	if o.WebhookSecretNamespace == "" {
		o.WebhookSecretNamespace = DefaultWebhookSecretNamespace
	}

	if o.WebhookSecretName == "" {
		o.WebhookSecretName = DefaultWebhookSecretName
	}

	if o.WebhookSecretVolumeMountPath == "" {
		o.WebhookSecretVolumeMountPath = DefaultWebhookSecretVolumeMountPath
	}

	if o.InitializeProviders == nil {
		o.InitializeProviders = InitializeProvidersNoopFn
	}

	if o.AddToManager == nil {
		o.AddToManager = AddToManagerNoopFn
	}
}
