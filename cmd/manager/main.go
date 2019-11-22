// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"k8s.io/klog"
	"k8s.io/klog/klogr"

	govmomidebug "github.com/vmware/govmomi/vim25/debug"
	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/controller"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	cnsv1alpha1 "gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
)

var (
	defaultProfilerAddr     = ":8073"
	defaultMetricsAddr      = ":8083"
	defaultSyncPeriod       = time.Minute * 10
	defaultRateLimiterQPS   = 500
	defaultRateLimiterBurst = 1000
)

// Serve REST endpoints for extracting the pprof info from this controller manager.
func runProfiler(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	_ = http.ListenAndServe(addr, mux)
}

func createHealthHTTPServer(listenAddress string) (*http.Server, error) {
	m := http.NewServeMux()
	m.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	s := &http.Server{
		Addr:    listenAddress,
		Handler: m,
	}

	return s, nil
}

func waitForVmOperatorGroupVersion(restConfig *rest.Config) error {
	// TODO: Add tests for waitForVmOperatorGroupVersion

	const gv = "vmoperator.vmware.com/v1alpha1"
	clientSet := kubernetes.NewForConfigOrDie(restConfig)

	// If the aggregated API server is not available in time, the controller will exit early
	// because the VM Operator resources are not registered. Poll here to try to avoid going
	// into a CrashLoopBackOff loop.
	err := wait.PollImmediate(100*time.Millisecond, 15*time.Second, func() (done bool, err error) {
		resources, err := clientSet.DiscoveryClient.ServerResourcesForGroupVersion(gv)
		if err != nil {
			if errors.IsServiceUnavailable(err) || meta.IsNoMatchError(err) {
				return false, nil
			}
			return false, err
		}

		return len(resources.APIResources) > 0, nil
	})

	return err
}

// Startup configuration for this controller-manager
type ControllerConfig struct {
	healthAddr           string
	profilerAddress      string
	metricsBindAddress   string
	enableGovmomiTracing bool
	syncPeriod           time.Duration
	rateLimiterQPS       int
	rateLimiterBurst     int
}

func parseConfig() (*ControllerConfig, error) {
	// glog is a dependency that is already defining a flag also defined by klog.InitFlags.  To avoid a redundant flag
	// registration issue, use a creative solution provided by BV to initialize the klog flag set.  Basically define a
	// custom flag set and then parse using the CLI args.
	flagSet := flag.NewFlagSet("klog-flags", flag.ExitOnError)
	ctrlConfig := ControllerConfig{}

	addStringFlag := func(value *string, flagName, defaultValue, usage string) {
		flag.StringVar(value, flagName, defaultValue, usage)
		flagSet.StringVar(value, flagName, defaultValue, usage)
	}

	// Configure various network endpoints that this container exposes various services on.
	addStringFlag(&ctrlConfig.healthAddr, "health-addr", ":49201",
		"The address on which an http server will listen on for readiness, liveness, etc health checks")

	addStringFlag(&ctrlConfig.profilerAddress, "profiler-address", defaultProfilerAddr, "Bind address to expose the pprof profiler")

	addStringFlag(&ctrlConfig.metricsBindAddress, "metrics-addr", defaultMetricsAddr, "The address the metric endpoint binds to.")

	addBoolFlag := func(value *bool, flagName string, defaultValue bool, usage string) {
		flag.BoolVar(value, flagName, defaultValue, usage)
		flagSet.BoolVar(value, flagName, defaultValue, usage)
	}

	// Configure gomvmomi tracing.
	addBoolFlag(&ctrlConfig.enableGovmomiTracing, "enable-govmomi-tracing", false,
		"Whether to enable govmomi debug tracing.")

	addDurationFlag := func(value *time.Duration, flagName string, defaultValue time.Duration, usage string) {
		flag.DurationVar(value, flagName, defaultValue, usage)
		flagSet.DurationVar(value, flagName, defaultValue, usage)
	}

	// Configure controller runtime sync period for the manager.
	addDurationFlag(&ctrlConfig.syncPeriod, "sync-period", defaultSyncPeriod, "The interval at which cluster-api objects are synchronized")

	addIntFlag := func(value *int, flagName string, defaultValue int, usage string) {
		flag.IntVar(value, flagName, defaultValue, usage)
		flagSet.IntVar(value, flagName, defaultValue, usage)
	}

	// Configure client-go rate limiter settings.
	addIntFlag(&ctrlConfig.rateLimiterQPS, "rate-limit-requests-per-second", defaultRateLimiterQPS,
		"The default number of requests per second to configure the k8s client rate limiter to allow.")
	addIntFlag(&ctrlConfig.rateLimiterBurst, "rate-limit-max-requests", defaultRateLimiterBurst,
		"The default number of maxium burst requests per second to configure the k8s client rate limiter to allow.")

	// Parse with the global flagset and the custom flagset so that all flags are consumed by all packages.
	flag.Parse()

	klog.InitFlags(flagSet)
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		return nil, err
	}

	return &ctrlConfig, nil
}

func main() {
	var (
		controllerName      = "vmoperator-controller-manager"
		controllerNamespace = os.Getenv("POD_NAMESPACE")
	)

	ctrlConfig, err := parseConfig()
	if err != nil {
		os.Exit(11)
	}

	logf.SetLogger(klogr.New())
	log := logf.Log.WithName("controller-entrypoint")

	verboseFlag := flag.Lookup("v")
	if verboseFlag != nil {
		log.Info("Logging level set to", "level", verboseFlag.Value)
	}

	log.Info("Starting vm-operator controller manager", "version", pkg.BuildVersion,
		"buildnumber", pkg.BuildNumber, "buildtype", pkg.BuildType)

	log.Info("Setting up health HTTP server", "address", ctrlConfig.healthAddr)
	srv, err := createHealthHTTPServer(ctrlConfig.healthAddr)
	if err != nil {
		log.Error(err, "Unable to create the health HTTP server")
		os.Exit(1)
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error(err, "Health HTTP server error")
		}
	}()

	if ctrlConfig.enableGovmomiTracing {
		log.Info("Enabling govmomi debug tracing")
		// Enable govmomi debug tracing
		govmomidebug.SetProvider(&govmomidebug.FileProvider{Path: "."})
	}

	// Get a config to talk to the apiserver
	log.Info("Setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "Unable to set up client config")
		os.Exit(1)
	}

	// Set up a custom ratelimiter in our config.  The default ratelimiter is set to a rate of 5 requests per second.
	// However, we are using this config in our various clients that are shared across all sessions.  Our new defaults
	// were chosen from empricial testing in the System Test environment.  These default settings enabled VM operator
	// to support ~100 Guest Clusters and 200+ VMs with 2-5 seconds of "noop" latency.
	log.Info("Configuring rate limiter", "QPS", ctrlConfig.rateLimiterQPS, "Burst", ctrlConfig.rateLimiterBurst)
	cfg.QPS = float32(ctrlConfig.rateLimiterQPS)
	cfg.Burst = ctrlConfig.rateLimiterBurst
	cfg.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(cfg.QPS, cfg.Burst)

	vmProvider := vsphere.NewVsphereMachineProviderFromRestConfig(cfg)

	// Register the vSphere provider
	log.Info("Setting up vSphere Provider")
	providerService := vmprovider.GetService()
	providerService.RegisterVmProvider(vmProvider)

	// Wait a bit for the aggregated apiserver to become available
	if err := waitForVmOperatorGroupVersion(cfg); err != nil {
		log.Error(err, "Timedout waiting for VM Operator Group/Version resources")
		// Keep going and let it fail if it is going to fail
	}

	// setting namespace when manager is not run in cluster (for testing)
	if controllerNamespace == "" {
		controllerNamespace = "default"
		log.Info("ControllerNamespace defaulted to ", controllerNamespace, ". controllerNamespace should be defaulted only in testing. Manager may function incorrectly in production with it defaulted.")
	}

	if ctrlConfig.profilerAddress != "" {
		log.Info("Profiler listening for requests", "profiler-address", ctrlConfig.profilerAddress)
		go runProfiler(ctrlConfig.profilerAddress)
	}

	leaderElectionId := controllerName + "-runtime"
	log.Info("Setting up manager", "Sync Period", ctrlConfig.syncPeriod,
		"Metrics Bind Address", ctrlConfig.metricsBindAddress,
		"LeaderElectionID", leaderElectionId, "LeaderElectionNamespace", controllerNamespace)

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		LeaderElection:          true,
		LeaderElectionID:        leaderElectionId,
		LeaderElectionNamespace: controllerNamespace,
		SyncPeriod:              &ctrlConfig.syncPeriod,
		MetricsBindAddress:      ctrlConfig.metricsBindAddress,
	})
	if err != nil {
		log.Error(err, "Unable to set up overall controller manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	log.Info("Setting up scheme")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "Unable add APIs to scheme")
		os.Exit(1)
	}

	if err := cnsv1alpha1.SchemeBuilder.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "Unable add APIs to scheme")
		os.Exit(1)
	}

	// Setup all Controllers
	log.Info("Setting up controller")
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "Unable to register controllers to the manager")
		os.Exit(1)
	}

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Unable to run the manager")
		os.Exit(1)
	}
}
