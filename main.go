// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlsig "sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/vmware-tanzu/vm-operator/controllers"
	"github.com/vmware-tanzu/vm-operator/controllers/infra/capability"
	"github.com/vmware-tanzu/vm-operator/pkg"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	pkgmgrinit "github.com/vmware-tanzu/vm-operator/pkg/manager/init"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
	"github.com/vmware-tanzu/vm-operator/services"
	"github.com/vmware-tanzu/vm-operator/webhooks"
	// +kubebuilder:scaffold:imports
)

const (
	// serverKeyName is the name of the server private key.
	serverKeyName = "tls.key"
	// serverCertName is the name of the serving certificate.
	serverCertName = "tls.crt"
)

var defaultConfig = pkgcfg.FromEnv()

func main() {
	klog.InitFlags(nil)
	ctrllog.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))
	setupLog := ctrllog.Log.WithName("entrypoint")

	setupLog.Info("Starting VM Operator controller", "version", pkg.BuildVersion,
		"buildnumber", pkg.BuildNumber, "buildtype", pkg.BuildType, "commit", pkg.BuildCommit)

	rateLimiterQPS := flag.Int(
		"rate-limit-requests-per-second",
		defaultConfig.RateLimitQPS,
		"The default number of requests per second to configure the k8s client rate limiter to allow.",
	)
	rateLimiterBurst := flag.Int(
		"rate-limit-max-requests",
		defaultConfig.RateLimitBurst,
		"The default number of maximum burst requests per second to configure the k8s client rate limiter to allow.",
	)

	var managerOpts pkgmgr.Options

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

	flag.Parse()

	if managerOpts.WatchNamespace != "" {
		setupLog.Info(
			"Watching objects only in namespace for reconciliation",
			"namespace", managerOpts.WatchNamespace)
	}

	if *rateLimiterQPS != 0 || *rateLimiterBurst != 0 {
		cfg := ctrl.GetConfigOrDie()

		qps, burst := *rateLimiterQPS, *rateLimiterBurst
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

	setupLog.Info("wait for webhook certificates")
	waitForWebhookCertificates(setupLog, managerOpts)

	// Create a function that adds all of the controllers, services, and
	// webhooks to the manager.
	addToManager := func(
		ctx *pkgctx.ControllerManagerContext,
		mgr ctrlmgr.Manager) error {

		// Always load the capability controller.
		if err := capability.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize infra capability controller: %w", err)
		}

		containerType := os.Getenv("VMOP_CONTAINER_TYPE")
		switch containerType {
		case "controller-manager":
			if err := controllers.AddToManager(ctx, mgr); err != nil {
				return err
			}
			if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
				if err := services.AddToManager(ctx, mgr); err != nil {
					return err
				}
			}
		case "admission-webhook", "conversion-webhook":
			return webhooks.AddToManager(ctx, mgr)
		default:
			return fmt.Errorf("invalid container type: %s", containerType)
		}

		return nil
	}

	setupLog.Info("creating controller manager")
	managerOpts.InitializeProviders = pkgmgrinit.InitializeProviders
	managerOpts.AddToManager = addToManager

	ctx := pkgcfg.WithConfig(defaultConfig)
	ctx = cource.WithContext(ctx)
	ctx = watcher.WithContext(ctx)

	initFeaturesFromCapabilities(ctx, setupLog)
	setupLog.Info("Initial features", "features", pkgcfg.FromContext(ctx).Features)

	mgr, err := pkgmgr.New(ctx, managerOpts)
	if err != nil {
		setupLog.Error(err, "problem creating controller manager")
		os.Exit(1)
	}

	setupLog.Info("setting up webhook server TLS config")
	webhookServer := mgr.GetWebhookServer()
	srv := webhookServer.(*webhook.DefaultServer)
	configureWebhookTLS(&srv.Options)

	setupLog.Info("adding readiness check to controller manager")
	if err := mgr.AddReadyzCheck("webhook", webhookServer.StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create readiness check")
		os.Exit(1)
	}

	setupLog.Info("starting controller manager")
	sigHandler := ctrlsig.SetupSignalHandler()
	if err := mgr.Start(sigHandler); err != nil {
		setupLog.Error(err, "problem running controller manager")
		os.Exit(1)
	}
}

func configureWebhookTLS(opts *webhook.Options) {
	tlsCfgFunc := func(cfg *tls.Config) {
		cfg.MinVersion = tls.VersionTLS12
		cfg.CipherSuites = []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		}
	}
	opts.TLSOpts = []func(*tls.Config){
		tlsCfgFunc,
	}
}

func waitForWebhookCertificates(setupLog logr.Logger, managerOpts pkgmgr.Options) {
	waitOnCertsStartTime := time.Now()
	for {
		select {
		case <-certDirReady(managerOpts.WebhookSecretVolumeMountPath):
			return
		case <-time.After(time.Second * 5):
			setupLog.Info("waiting on certificates", "elapsed", time.Since(waitOnCertsStartTime).String())
		}
	}
}

// initFeaturesFromCapabilities updates our enabled/disabled features based on
// the capabilities. The inability to get the capabilities should not prevent
// the container from starting as the features will be processed later by the
// capabilities controller.
func initFeaturesFromCapabilities(ctx context.Context, logger logr.Logger) {
	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{})
	if err != nil {
		logger.Error(err, "failed to create client for updating capabilities")
	} else if _, err := capabilities.UpdateCapabilities(ctx, c); err != nil {
		logger.Error(err, "failed to update capabilities")
	}
}

// CertDirReady returns a channel that is closed when there are certificates
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
