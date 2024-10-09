// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"

	"github.com/vmware-tanzu/vm-operator/pkg"
)

const defaultPrefix = "vmoperator-"

// Default returns a Config object with default values.
func Default() Config {
	return Config{
		BuildCommit:  pkg.BuildCommit,
		BuildNumber:  pkg.BuildNumber,
		BuildVersion: pkg.BuildVersion,
		BuildType:    pkg.BuildType,

		ContainerNode:                false,
		ContentAPIWait:               1 * time.Second,
		DefaultVMClassControllerName: "vmoperator.vmware.com/vsphere",
		Features: FeatureStates{
			InstanceStorage:            true,
			PodVMOnStretchedSupervisor: false,
			TKGMultipleCL:              false,
			UnifiedStorageQuota:        false,
			WorkloadDomainIsolation:    false,
		},
		InstanceStorage: InstanceStorage{
			JitterMaxFactor:      1.0,
			PVPlacementFailedTTL: 5 * time.Minute,
			SeedRequeueDuration:  10 * time.Second,
		},
		LeaderElectionID:             defaultPrefix + "controller-manager-runtime",
		MaxCreateVMsOnProvider:       80,
		MaxConcurrentReconciles:      1,
		AsyncSignalDisabled:          false,
		CreateVMRequeueDelay:         10 * time.Second,
		PoweredOnVMHasIPRequeueDelay: 10 * time.Second,
		NetworkProviderType:          NetworkProviderTypeNamed,
		PodName:                      defaultPrefix + "controller-manager",
		PodNamespace:                 defaultPrefix + "system",
		PodServiceAccountName:        "default",
		ProfilerAddr:                 ":8073",
		RateLimitBurst:               1000,
		RateLimitQPS:                 500,
		SyncPeriod:                   10 * time.Minute,
		WatchNamespace:               "",
		WebhookServiceContainerPort:  9878,
		WebhookServiceName:           defaultPrefix + "webhook-service",
		WebhookServiceNamespace:      defaultPrefix + "system",
		WebhookSecretName:            defaultPrefix + "webhook-server-cert",
		WebhookSecretNamespace:       defaultPrefix + "system",
		WebhookSecretVolumeMountPath: "/tmp/k8s-webhook-server/serving-certs",
	}
}
