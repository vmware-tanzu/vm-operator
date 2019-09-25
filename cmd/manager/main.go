/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"flag"
	"io"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"k8s.io/klog/klogr"

	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/controller"
	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func registerVsphereVmProvider(restConfig *rest.Config) error {
	clientSet := kubernetes.NewForConfigOrDie(restConfig)
	ncpclient := ncpclientset.NewForConfigOrDie(restConfig)

	provider, err := vsphere.NewVSphereVmProvider(clientSet, ncpclient)
	if err != nil {
		return err
	}

	vmprovider.RegisterVmProvider(provider)
	return nil
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
	const gv = "vmoperator.vmware.com/v1alpha1"
	clientSet := kubernetes.NewForConfigOrDie(restConfig)

	/*
	 * If the aggregated API server is not available in time, the controller will exit early
	 * because the VM Operator resources are not registered. Poll here to try to avoid going
	 * into a CrashLoopBackOff loop.
	 */
	err := wait.PollImmediate(100*time.Millisecond, 15*time.Second, func() (done bool, err error) {
		resources, err := clientSet.DiscoveryClient.ServerResourcesForGroupVersion(gv)
		if err != nil {
			if errors.IsServiceUnavailable(err) {
				return false, nil
			}
			return false, err
		}

		return len(resources.APIResources) > 0, nil
	})

	return err
}

func main() {
	//klog.InitFlags(nil) Usually needed but already called via an init() somewhere

	var healthAddr string
	flag.StringVar(&healthAddr, "health-addr", ":49201",
		"The address on which an http server will listen on for readiness, liveness, etc health checks")
	if err := flag.Set("v", "2"); err != nil {
		klog.Fatalf("klog level flag has changed from -v: %v", err)
	}
	flag.Parse()

	logf.SetLogger(klogr.New())
	log := logf.Log.WithName("entrypoint")

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

	// Get a config to talk to the apiserver
	log.Info("setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Register the vSphere provider
	log.Info("setting up vSphere Provider")
	if err := registerVsphereVmProvider(cfg); err != nil {
		log.Error(err, "unable to register vSphere VM provider")
		os.Exit(1)
	}

	// Wait a bit for the aggregated apiserver to become available
	if err := waitForVmOperatorGroupVersion(cfg); err != nil {
		log.Error(err, "timedout waiting for VM Operator Group/Version resources")
		// Keep going and let it fail if it is going to fail
	}

	// Until we get VC events
	syncPeriod := 10 * time.Second

	// Create a new Cmd to provide shared dependencies and start components
	log.Info("setting up manager")
	mgr, err := manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod})
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

	// Setup all Controllers
	log.Info("Setting up controller")
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "unable to register controllers to the manager")
		os.Exit(1)
	}

	/*
		log.Info("setting up webhooks")
		if err := webhook.AddToManager(mgr); err != nil {
			log.Error(err, "unable to register webhooks to the manager")
			os.Exit(1)
		}
	*/

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
