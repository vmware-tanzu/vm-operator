// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manager

import "time"

const (
	defaultPrefix = "vmoperator-"

	// DefaultWebhookServiceContainerPort is the default value for the eponymous
	// manager option.
	DefaultWebhookServiceContainerPort = 9878

	// DefaultSyncPeriod is the default value for the eponymous
	// manager option.
	DefaultSyncPeriod = time.Minute * 10

	// DefaultMaxConcurrentReconciles is the default value for the eponymous
	// manager option.
	DefaultMaxConcurrentReconciles = 1

	// DefaultPodNamespace is the default value for the eponymous manager
	// option.
	DefaultPodNamespace = defaultPrefix + "system"

	// DefaultPodName is the default value for the eponymous manager option.
	DefaultPodName = defaultPrefix + "controller-manager"

	// DefaultPodServiceAccountName is the default name of the pod service account.
	DefaultPodServiceAccountName = "default"

	// DefaultLeaderElectionID is the default value for the eponymous manager option.
	DefaultLeaderElectionID = DefaultPodName + "-runtime"

	// DefaultWatchNamespace is the default value for the eponymous manager
	// option.
	DefaultWatchNamespace = ""

	// DefaultWebhookServiceNamespace is the default value for the eponymous
	// manager option.
	DefaultWebhookServiceNamespace = defaultPrefix + "system"

	// DefaultWebhookServiceName is the default value for the eponymous manager
	// option.
	DefaultWebhookServiceName = defaultPrefix + "webhook-service"

	// DefaultWebhookSecretNamespace is the default value for the eponymous
	// manager option.
	DefaultWebhookSecretNamespace = defaultPrefix + "system"

	// DefaultWebhookSecretName is the default value for the eponymous manager
	// option.
	DefaultWebhookSecretName = defaultPrefix + "webhook-server-cert"

	// DefaultWebhookSecretVolumeMountPath is the default value for the
	// eponymous manager option.
	//nolint:gosec
	DefaultWebhookSecretVolumeMountPath = "/tmp/k8s-webhook-server/serving-certs"

	// DefaultContainerNode is the default value for the eponymous manager option.
	DefaultContainerNode = false

	// DefaultInstanceStoragePVPlacementFailedTTL is the default wait time before declaring PV placement failed
	// after error annotation is set on PVC.
	DefaultInstanceStoragePVPlacementFailedTTL = 5 * time.Minute
)
