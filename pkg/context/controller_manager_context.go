// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

// ControllerManagerContext is the context of the controller that owns the
// controllers.
type ControllerManagerContext struct {
	context.Context

	// Namespace is the namespace in which the resource is located responsible
	// for running the controller manager.
	Namespace string

	// Name is the name of the controller manager.
	Name string

	// ServiceAccountName is the name of the pod's service account.
	ServiceAccountName string

	// LeaderElectionID is the information used to identify the object
	// responsible for synchronizing leader election.
	LeaderElectionID string

	// LeaderElectionNamespace is the namespace in which the LeaderElection
	// object is located.
	LeaderElectionNamespace string

	// WatchNamespace is the namespace the controllers watch for changes. If
	// no value is specified then all namespaces are watched.
	WatchNamespace string

	// Logger is the controller manager's logger.
	Logger logr.Logger

	// Recorder is used to record events.
	Recorder record.Recorder

	// MaxConcurrentReconciles is the maximum number of reconcile requests this
	// controller will receive concurrently.
	MaxConcurrentReconciles int

	// WebhookServiceNamespace is the namespace in which the webhook service
	// is located.
	WebhookServiceNamespace string

	// WebhookServiceName is the name of the webhook service.
	WebhookServiceName string

	// WebhookSecretNamespace is the namespace in which the webhook secret
	// is located.
	WebhookSecretNamespace string

	// WebhookSecretName is the name of the webhook secret.
	WebhookSecretName string

	// ContainerNode should be true if we're running guest cluster nodes in containers.
	ContainerNode bool

	// SyncPeriod determines the minimum frequency at which watched resources are
	// reconciled. A lower period will correct entropy more quickly, but reduce
	// responsiveness to change if there are many watched resources.
	SyncPeriod time.Duration

	// VMProvider is the controller manager's VM Provider
	VMProvider vmprovider.VirtualMachineProviderInterface

	// VMProviderA2 is the controller manager's VM Provider for v1alpha2
	VMProviderA2 vmprovider.VirtualMachineProviderInterfaceA2
}

// String returns ControllerManagerName.
func (c *ControllerManagerContext) String() string {
	return c.Name
}
