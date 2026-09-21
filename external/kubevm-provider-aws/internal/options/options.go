// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package options holds the manager's command-line configuration.
//
// Separate from cmd/manager so it can be tested. The manager imports the AWS
// client, and a test binary that imports it would fail this repository's own
// guard that no test can reach AWS credentials; this package imports nothing
// from AWS.
package options

import (
	"errors"
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// DefaultSyncPeriod is how often every AWSMachine is reconciled even when
// nothing in the cluster has changed.
//
// EC2 sends no events, so a change made outside Kubernetes -- an instance
// stopped from the console, retired by AWS, or given a new address -- reaches
// the AWSMachine only on its next reconcile. For a settled machine that is the
// next cache resync, and controller-runtime's default is ten hours. Cluster
// API's AWS provider defaults its --sync-period to ten minutes; this matches
// it. The cost is one DescribeInstances per settled machine per period.
const DefaultSyncPeriod = 10 * time.Minute

// Options is the manager's configuration.
type Options struct {
	// MetricsAddr is where to serve metrics, or "0" to serve none. They are
	// served unauthenticated, so the shipped Deployment turns them off.
	MetricsAddr string

	// ProbeAddr is where to serve the liveness and readiness probes.
	ProbeAddr string

	// LeaderElection makes one replica act while the others stand by.
	LeaderElection bool

	// Region is the AWS region this manager serves. One deployment, one
	// account, one region: the EC2 client is built from it once at startup.
	Region string

	// SyncPeriod is how often every object is reconciled even when nothing
	// has changed. It is what bounds staleness here, because EC2 announces
	// nothing and the KubeVM core does not watch this provider's objects.
	SyncPeriod time.Duration
}

// Bind registers the manager's flags on fs and returns the Options they fill.
func Bind(fs *flag.FlagSet) *Options {
	o := &Options{}
	fs.StringVar(&o.MetricsAddr, "metrics-bind-address", ":8080",
		"Where to serve metrics.")
	fs.StringVar(&o.ProbeAddr, "health-probe-bind-address", ":8081",
		"Where to serve health probes.")
	fs.BoolVar(&o.LeaderElection, "leader-elect", false,
		"Elect a leader before acting, so only one replica reconciles.")
	fs.StringVar(&o.Region, "region", os.Getenv("AWS_REGION"),
		"The AWS region. One deployment serves one account and one region.")
	fs.DurationVar(&o.SyncPeriod, "sync-period", DefaultSyncPeriod,
		"How often every AWSMachine is reconciled even when nothing changed, "+
			"which bounds how long a change made outside Kubernetes goes unseen.")
	return o
}

// Validate reports the first thing wrong with the configuration.
func (o *Options) Validate() error {
	if o.Region == "" {
		return errors.New("no AWS region: set AWS_REGION or pass --region")
	}
	if o.SyncPeriod <= 0 {
		return errors.New("--sync-period must be positive")
	}
	return nil
}

// Manager returns the controller-runtime manager options these describe.
func (o *Options) Manager(scheme *runtime.Scheme) ctrl.Options {
	sync := o.SyncPeriod
	return ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: o.MetricsAddr},
		HealthProbeBindAddress: o.ProbeAddr,
		LeaderElection:         o.LeaderElection,
		LeaderElectionID:       "awsmachine.infrastructure.kube-vm.io",
		Cache:                  cache.Options{SyncPeriod: &sync},
	}
}
