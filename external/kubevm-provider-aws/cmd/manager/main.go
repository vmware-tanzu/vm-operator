// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Command manager runs the AWS provider for KubeVM.
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

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	awsv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/controllers"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/awsclient"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/options"
)

// scheme holds every type this manager reads or writes.
var scheme = runtime.NewScheme()

// init registers the API groups.
//
// Both groups: this provider's own, and the core's, because the controller
// reads the portable VirtualMachine every reconcile. A manager whose scheme
// lacks the core's types starts cleanly and then fails on the first List, at
// runtime, with an error that reads like a permissions problem.
func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(awsv1a1.AddToScheme(scheme))
	utilruntime.Must(kubevmv1a1.AddToScheme(scheme))
}

// main starts the manager.
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// run wires and starts the manager, returning rather than exiting so the
// failure path is testable.
func run() error {
	cfg := options.Bind(flag.CommandLine)

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Set the logger BEFORE anything asks for one. The reverse order --
	// calling ctrl.Log.WithName first and setting the logger from it -- makes
	// the logger delegate to itself, and the manager then dies with no output
	// at all. That cost a long time to diagnose once, because a process that
	// logs nothing looks like a process that never started.
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")

	if err := cfg.Validate(); err != nil {
		return err
	}

	ctx := ctrl.SetupSignalHandler()

	// Credentials are never a flag and never an API field. The SDK's own
	// chain finds them -- a Secret projected as environment variables here,
	// an IRSA token on EKS -- and this code cannot tell which, which is what
	// lets one Deployment manifest serve both.
	ec2Client, err := awsclient.New(ctx, cfg.Region)
	if err != nil {
		return fmt.Errorf("building an EC2 client: %w", err)
	}
	setupLog.Info("built an EC2 client", "region", cfg.Region,
		"syncPeriod", cfg.SyncPeriod)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), cfg.Manager(scheme))
	if err != nil {
		return fmt.Errorf("building the manager: %w", err)
	}

	if err := controllers.AddToManager(mgr, ec2Client); err != nil {
		return err
	}

	if err := addChecks(mgr); err != nil {
		return err
	}

	setupLog.Info("starting")
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("running the manager: %w", err)
	}
	return nil
}

// addChecks wires the health and readiness probes.
func addChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("adding the health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("adding the readiness check: %w", err)
	}
	return nil
}
