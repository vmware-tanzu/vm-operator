// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"
	"path"
	"time"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlsig "sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers"
	"github.com/vmware-tanzu/vm-operator/pkg"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgcrd "github.com/vmware-tanzu/vm-operator/pkg/crd"
	pkgexit "github.com/vmware-tanzu/vm-operator/pkg/exit"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	pkgmgrinit "github.com/vmware-tanzu/vm-operator/pkg/manager/init"
	"github.com/vmware-tanzu/vm-operator/pkg/mem"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
	"github.com/vmware-tanzu/vm-operator/services"
	"github.com/vmware-tanzu/vm-operator/webhooks"
)

const (
	// serverKeyName is the name of the server private key.
	serverKeyName = "tls.key"
	// serverCertName is the name of the serving certificate.
	serverCertName = "tls.crt"
)

var (
	ctx              context.Context
	mgr              pkgmgr.Manager
	managerOpts      pkgmgr.Options
	rateLimiterQPS   int
	rateLimiterBurst int
	defaultConfig    = pkgcfg.FromEnv()
	logOptions       = logs.NewOptions()
	setupLog         = klog.Background().WithName("setup")
)

// main is the entrypoint for the application. Please note, unless otherwise
// stated, the order of the functions in main is by-design, and the functions
// should only be re-ordered with care.
func main() {
	setupLog.Info("Starting VM Operator controller",
		"version", pkg.BuildVersion,
		"buildnumber", pkg.BuildNumber,
		"buildtype", pkg.BuildType,
		"commit", pkg.BuildCommit)

	initContext()

	initFlags()

	initLogging()

	initMemStats()

	initFeatures()

	initCRDs()

	initRateLimiting()

	waitForWebhookCertificates()

	initManager()

	initWebhookServer(managerOpts.EnableWebhookClientVerification)

	initSIGUSR2RestartHandler()

	setupLog.Info("Starting controller manager")
	sigHandler := ctrlsig.SetupSignalHandler()
	if err := mgr.Start(sigHandler); err != nil {
		setupLog.Error(err, "Problem running controller manager")
		os.Exit(1)
	}
}

// initFeatures updates our enabled/disabled features based on the capabilities.
// The inability to get the capabilities should not prevent the container from
// starting as the features will be processed later by the capabilities
// controller.
func initFeatures() {
	setupLog.Info("Initial features from environment",
		"features", pkgcfg.FromContext(ctx).Features)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = capv1.AddToScheme(scheme)

	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "Failed to create client for updating capabilities")
	} else if _, err := capabilities.UpdateCapabilities(logr.NewContext(ctx, setupLog), c); err != nil {
		setupLog.Error(err, "Failed to update capabilities")
	}

	setupLog.Info("Initial features from capabilities",
		"features", pkgcfg.FromContext(ctx).Features)
}

func initCRDs() {
	setupLog.Info("Installing/updating CRDs",
		"features", pkgcfg.FromContext(ctx).Features)

	scheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)

	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "Failed to create client for installing/updating CRDs")
	}

	if err := pkgcrd.Install(ctx, c, nil); err != nil {
		setupLog.Error(err, "Failed to install/update CRDs")
	}
}

func initMemStats() {
	mem.Start(
		ctrl.Log,
		defaultConfig.MemStatsPeriod,
		metrics.Registry.MustRegister)
}

func initContext() {
	ctx = pkgcfg.WithConfig(defaultConfig)
	ctx = cource.WithContext(ctx)
	ctx = watcher.WithContext(ctx)
	ctx = ovfcache.WithContext(ctx)
}

func initRateLimiting() {
	if rateLimiterQPS == 0 && rateLimiterBurst == 0 {
		return
	}
	cfg := ctrl.GetConfigOrDie()

	qps, burst := rateLimiterQPS, rateLimiterBurst
	if qps != 0 {
		cfg.QPS = float32(qps)
	}
	if burst != 0 {
		cfg.Burst = burst
	}
	if burst != 0 && qps != 0 {
		setupLog.Info("Configuring rate limiter", "QPS", qps, "burst", burst)
		cfg.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(cfg.QPS, cfg.Burst)
	}

	managerOpts.KubeConfig = cfg
}

func initFlags() {
	flag.IntVar(
		&rateLimiterQPS,
		"rate-limit-requests-per-second",
		defaultConfig.RateLimitQPS,
		"The default number of requests per second to configure the k8s client rate limiter to allow.",
	)
	flag.IntVar(
		&rateLimiterBurst,
		"rate-limit-max-requests",
		defaultConfig.RateLimitBurst,
		"The default number of maximum burst requests per second to configure the k8s client rate limiter to allow.",
	)
	flag.StringVar(
		&managerOpts.MetricsAddr,
		"metrics-addr",
		":8083",
		"The address the metric endpoint binds to.")
	flag.StringVar(
		&managerOpts.HealthProbeBindAddress,
		"health-addr",
		":9445",
		"The address the health probe endpoint binds to.")
	flag.StringVar(
		&managerOpts.PprofBindAddress,
		"profiler-address",
		defaultConfig.ProfilerAddr,
		"Bind address to expose the pprof profiler.")
	flag.BoolVar(
		&managerOpts.LeaderElectionEnabled,
		"enable-leader-election",
		true,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(
		&managerOpts.LeaderElectionID,
		"leader-election-id",
		defaultConfig.LeaderElectionID,
		"Name of the config map to use as the locking resource when configuring leader election.")
	flag.StringVar(
		&managerOpts.WatchNamespace,
		"watch-namespace",
		defaultConfig.WatchNamespace,
		"Namespace that the controller watches to reconcile vm operator objects. If unspecified, the controller watches for vm operator objects across all namespaces.")
	flag.DurationVar(
		&managerOpts.SyncPeriod,
		"sync-period",
		defaultConfig.SyncPeriod,
		"The interval at which objects are synchronized.")
	flag.IntVar(
		&managerOpts.MaxConcurrentReconciles,
		"max-concurrent-reconciles",
		defaultConfig.MaxConcurrentReconciles,
		"The maximum number of allowed, concurrent reconciles.")
	flag.StringVar(
		&managerOpts.PodNamespace,
		"pod-namespace",
		defaultConfig.PodNamespace,
		"The namespace in which the pod running the controller manager is located.")
	flag.StringVar(
		&managerOpts.PodName,
		"pod-name",
		defaultConfig.PodName,
		"The name of the pod running the controller manager.")
	flag.StringVar(
		&managerOpts.PodServiceAccountName,
		"pod-service-account-name",
		defaultConfig.PodServiceAccountName,
		"The service account name of the pod running the controller manager.")
	flag.IntVar(
		&managerOpts.WebhookServiceContainerPort,
		"webhook-service-container-port",
		defaultConfig.WebhookServiceContainerPort,
		"The port on which the webhook service expects the webhook server to listen for incoming requests.")
	flag.StringVar(
		&managerOpts.WebhookServiceNamespace,
		"webhook-service-namespace",
		defaultConfig.WebhookServiceNamespace,
		"The namespace in which the webhook service is located.")
	flag.StringVar(
		&managerOpts.WebhookServiceName,
		"webhook-service-name",
		defaultConfig.WebhookServiceName,
		"The name of the webhook service.")
	flag.StringVar(
		&managerOpts.WebhookSecretNamespace,
		"webhook-secret-namespace",
		defaultConfig.WebhookSecretNamespace,
		"The namespace in which the webhook secret is located.")
	flag.StringVar(
		&managerOpts.WebhookSecretName,
		"webhook-secret-name",
		defaultConfig.WebhookSecretName,
		"The name of the webhook secret.")
	flag.StringVar(
		&managerOpts.WebhookSecretVolumeMountPath,
		"webhook-secret-volume-mount-path",
		defaultConfig.WebhookSecretVolumeMountPath,
		"The filesystem path to which the webhook secret is mounted.")
	flag.BoolVar(
		&managerOpts.ContainerNode,
		"container-node",
		defaultConfig.ContainerNode,
		"Should be true if we're running nodes in containers (with vcsim).",
	)
	flag.BoolVar(
		&managerOpts.EnableWebhookClientVerification,
		"enable-webhook-client-verification",
		false,
		"Enable webhook client verification on the webhook server.",
	)
	flag.BoolVar(
		managerOpts.UsePriorityQueue,
		"use-priority-queue",
		true,
		"Enable the priority queue feature.",
	)

	logsv1.AddGoFlags(logOptions, flag.CommandLine)

	// Set log level 2 as default.
	if err := flag.Set("v", "2"); err != nil {
		setupLog.Error(err, "Failed to set default log level")
		os.Exit(1)
	}

	flag.Parse()
}

func initLogging() {
	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "Failed to validate logging configuration")
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())
}

func waitForWebhookCertificates() {
	setupLog.Info("Waiting for webhook certificates")
	waitOnCertsStartTime := time.Now()
	for {
		select {
		case <-certDirReady(managerOpts.WebhookSecretVolumeMountPath):
			return
		case <-time.After(time.Second * 5):
			setupLog.Info(
				"Waiting on certificates",
				"elapsed", time.Since(waitOnCertsStartTime).String())
		}
	}
}

// certDirReady returns a channel that is closed when there are certificates
// available in the configured certificate directory. If CertDir is
// empty or the specified directory does not exist, then the returned channel
// is never closed.
func certDirReady(certDir string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		crtPath := path.Join(certDir, serverCertName)
		keyPath := path.Join(certDir, serverKeyName)
		for {
			if file, err := os.Stat(crtPath); err == nil {
				if file.Size() > 0 {
					if file, err := os.Stat(keyPath); err == nil {
						if file.Size() > 0 {
							close(done)
							return
						}
					}
				}
			}
			time.Sleep(time.Second * 1)
		}
	}()
	return done
}

func initManager() {
	// Create a function that adds all of the controllers, services, and
	// webhooks to the manager.
	addToManager := func(
		ctx *pkgctx.ControllerManagerContext,
		mgr ctrlmgr.Manager) error {

		if err := controllers.AddToManager(ctx, mgr); err != nil {
			return err
		}
		if err := services.AddToManager(ctx, mgr); err != nil {
			return err
		}
		return webhooks.AddToManager(ctx, mgr)
	}

	setupLog.Info("Creating controller manager")
	managerOpts.InitializeProviders = pkgmgrinit.InitializeProviders
	managerOpts.AddToManager = addToManager
	if managerOpts.WatchNamespace != "" {
		setupLog.Info(
			"Watching objects only in namespace for reconciliation",
			"namespace", managerOpts.WatchNamespace)
	}

	var err error
	mgr, err = pkgmgr.New(ctx, managerOpts)
	if err != nil {
		setupLog.Error(err, "Problem creating controller manager")
		os.Exit(1)
	}
}

func initWebhookServer(enableWebhookClientVerification bool) {
	setupLog.Info("Setting up webhook server TLS config")
	webhookServer := mgr.GetWebhookServer()
	srv := webhookServer.(*webhook.DefaultServer)

	tlsCfgFunc := func(cfg *tls.Config) {
		cfg.MinVersion = tls.VersionTLS12
		cfg.CipherSuites = []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		}
	}
	srv.Options.TLSOpts = []func(*tls.Config){
		tlsCfgFunc,
	}

	clientCfgFunc := func(cfg *tls.Config) {
		cfg.ClientAuth = tls.RequestClientCert
	}
	if enableWebhookClientVerification {
		srv.Options.TLSOpts = append(srv.Options.TLSOpts, clientCfgFunc)
	}

	setupLog.Info("Adding readiness check to controller manager")
	if err := mgr.AddReadyzCheck("webhook", webhookServer.StartedChecker()); err != nil {
		setupLog.Error(err, "Unable to create readiness check")
		os.Exit(1)
	}
}

func initSIGUSR2RestartHandler() {
	setupLog.Info("SIGUSR2 restart handler",
		"enabled", defaultConfig.SIGUSR2RestartEnabled)

	if !defaultConfig.SIGUSR2RestartEnabled {
		return
	}

	// Allow the pod to restart via pkg/exit.Restart when SIGUSR2 is received.
	// This simulates the behavior when capabilities are changed and the pod
	// is the leader.
	_ = pkgexit.NewRestartSignalHandler(
		logr.NewContext(ctx, setupLog),
		mgr.GetClient(),
		mgr.Elected())
}
