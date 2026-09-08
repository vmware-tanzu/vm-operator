// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Command controller runs the generic kube-vm.io core reconciler. It is
// intended to run out-of-cluster against a Supervisor namespace for the PoC
// demo; see ../../../hack/demo/kubevm/README.md.
package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlsig "sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm/controller/controllers/virtualmachine"
)

func main() {
	var metricsAddr string
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. \"0\" disables it.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.Parse()

	log := ctrl.Log.WithName("kubevm-controller")
	ctrl.SetLogger(log)

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		log.Error(err, "unable to add client-go scheme")
		os.Exit(1)
	}
	if err := kubevmv1a1.AddToScheme(scheme); err != nil {
		log.Error(err, "unable to add kube-vm.io scheme")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
	})
	if err != nil {
		log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := virtualmachine.AddToManager(mgr); err != nil {
		log.Error(err, "unable to create controller", "controller", "VirtualMachine")
		os.Exit(1)
	}

	log.Info("starting manager")
	if err := mgr.Start(ctrlsig.SetupSignalHandler()); err != nil {
		log.Error(err, "problem running manager")
		os.Exit(1)
	}
}
