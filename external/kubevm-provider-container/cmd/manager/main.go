// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Command manager runs the container provider for KubeVM.
package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	containerv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/controllers"
)

// scheme holds every type this manager reads or writes.
var scheme = runtime.NewScheme()

// init registers the API groups.
//
// Both groups: this provider's own, and the core's, because the controller
// reads the portable VirtualMachine every reconcile. A manager whose scheme
// lacks the core's types starts cleanly and then fails on the first Get, at
// runtime, with an error that reads like a permissions problem — see
// implementing-a-provider.md, "Register both schemes", for the fuller
// explanation.
func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(containerv1a1.AddToScheme(scheme))
	utilruntime.Must(kubevmv1a1.AddToScheme(scheme))
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var metricsAddr string
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0",
		`The address the metrics endpoint binds to. "0" disables it.`)
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	flag.Parse()

	// ctrl.Log has no sink of its own — it only forwards to whatever
	// SetLogger installs. Naming it before installing a real
	// implementation (zap here) silently discards every log line.
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	log := ctrl.Log.WithName("kubevm-provider-container")

	ctx := ctrl.SetupSignalHandler()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
	})
	if err != nil {
		return fmt.Errorf("building the manager: %w", err)
	}

	if err := controllers.AddToManager(mgr); err != nil {
		return err
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("adding the health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("adding the readiness check: %w", err)
	}

	log.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("running the manager: %w", err)
	}
	return nil
}
