/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"flag"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	govmomidebug "github.com/vmware/govmomi/vim25/debug"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/controller"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	cnsv1alpha1 "gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
)

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

var (
	defaultProfilerAddr = ":8073"
	defaultMetricsAddr  = ":8083"
	defaultSyncPeriod   = time.Second * 10
)

func main() {
	//klog.InitFlags(nil) Usually needed but already called via an init() somewhere
	var (
		controllerName      = "vmoperator-controller-manager"
		controllerNamespace = os.Getenv("POD_NAMESPACE")
	)

	var healthAddr string
	flag.StringVar(&healthAddr, "health-addr", ":49201",
		"The address on which an http server will listen on for readiness, liveness, etc health checks")
	if err := flag.Set("v", "2"); err != nil {
		klog.Fatalf("klog level flag has changed from -v: %v", err)
	}

	var profilerAddress, metricsBindAddress string
	var enableGovmomiTracing bool
	var syncPeriod time.Duration
	flag.StringVar(&profilerAddress, "profiler-address", defaultProfilerAddr, "Bind address to expose the pprof profiler")
	flag.StringVar(&metricsBindAddress, "metrics-addr", defaultMetricsAddr, "The address the metric endpoint binds to.")
	flag.BoolVar(&enableGovmomiTracing, "enable-govmomi-tracing", false, "Whether to enable govmomi debug tracing.")
	flag.DurationVar(&syncPeriod, "sync-period", defaultSyncPeriod, "The interval at which cluster-api objects are synchronized")

	flag.Parse()

	logf.SetLogger(klogr.New())
	log := logf.Log.WithName("entrypoint")

	log.Info("Starting vm-operator controller manager", "version", pkg.BuildVersion,
		"buildnumber", pkg.BuildNumber, "buildtype", pkg.BuildType)

	log.Info("Setting up health HTTP server")
	srv, err := createHealthHTTPServer(healthAddr)
	if err != nil {
		log.Error(err, "unable to create the health HTTP server")
		os.Exit(1)
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error(err, "health HTTP server error")
		}
	}()
	if enableGovmomiTracing {
		log.Info("Enabling govmomi debug tracing")
		// Enable govmomi debug tracing
		govmomidebug.SetProvider(&govmomidebug.FileProvider{Path: "."})
	}

	// Get a config to talk to the apiserver
	log.Info("setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Register the vSphere provider
	log.Info("setting up vSphere Provider")
	if _, err := vsphere.RegisterVsphereVmProvider(cfg); err != nil {
		log.Error(err, "unable to register vSphere VM provider")
		os.Exit(1)
	}

	// Wait a bit for the aggregated apiserver to become available
	if err := waitForVmOperatorGroupVersion(cfg); err != nil {
		log.Error(err, "timedout waiting for VM Operator Group/Version resources")
		// Keep going and let it fail if it is going to fail
	}

	// setting namespace when manager is not run in cluster (for testing)
	if controllerNamespace == "" {
		controllerNamespace = "default"
		log.Info("ControllerNamespace defaulted to ", controllerNamespace, ". controllerNamespace should be defaulted only in testing. Manager may function incorrectly in production with it defaulted.")
	}

	if profilerAddress != "" {
		log.Info("Profiler listening for requests", "profiler-address", profilerAddress)
		go runProfiler(profilerAddress)
	}

	leaderElectionId := controllerName + "-runtime"
	log.Info("setting up manager",
		"LeaderElectionID", leaderElectionId,
		"LeaderElectionNamespace", controllerNamespace,
		"SyncPeriod", syncPeriod,
		"MetricsBindAddres", metricsBindAddress)

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		LeaderElection:          true,
		LeaderElectionID:        leaderElectionId,
		LeaderElectionNamespace: controllerNamespace,
		SyncPeriod:              &syncPeriod,
		MetricsBindAddress:      metricsBindAddress,
	})
	if err != nil {
		log.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	log.Info("setting up scheme")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable add APIs to scheme")
		os.Exit(1)
	}

	if err := cnsv1alpha1.SchemeBuilder.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable add APIs to scheme")
		os.Exit(1)
	}

	// Setup all Controllers
	log.Info("Setting up controller")
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "unable to register controllers to the manager")
		os.Exit(1)
	}

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}

func runProfiler(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	_ = http.ListenAndServe(addr, mux)
}
