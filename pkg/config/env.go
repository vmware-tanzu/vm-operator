// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"strconv"
	"time"

	"github.com/vmware-tanzu/vm-operator/pkg/config/env"
)

// FromEnv returns a new Config that has been initialized from environment
// variables.
func FromEnv() Config {
	config := Default()

	setString(env.JSONExtraConfig, &config.JSONExtraConfig)
	setDuration(env.ContentAPIWaitDuration, &config.ContentAPIWait)
	setInt(env.MaxCreateVMsOnProvider, &config.MaxCreateVMsOnProvider)
	setDuration(env.CreateVMRequeueDelay, &config.CreateVMRequeueDelay)
	setDuration(env.PoweredOnVMHasIPRequeueDelay, &config.PoweredOnVMHasIPRequeueDelay)
	setDuration(env.SyncImageRequeueDelay, &config.SyncImageRequeueDelay)
	setNetworkProviderType(env.NetworkProviderType, &config.NetworkProviderType)
	setString(env.LoadBalancerProvider, &config.LoadBalancerProvider)
	setBool(env.VSphereNetworking, &config.VSphereNetworking)
	setStringSlice(env.PrivilegedUsers, &config.PrivilegedUsers)
	setBool(env.LogSensitiveData, &config.LogSensitiveData)
	setBool(env.AsyncSignalEnabled, &config.AsyncSignalEnabled)
	setBool(env.AsyncCreateEnabled, &config.AsyncCreateEnabled)
	setDuration(env.MemStatsPeriod, &config.MemStatsPeriod)
	setString(env.FastDeployMode, &config.FastDeployMode)
	setString(env.PromoteDisksMode, &config.PromoteDisksMode)
	setString(env.VCCredsSecretName, &config.VCCredsSecretName)

	setDuration(env.InstanceStoragePVPlacementFailedTTL, &config.InstanceStorage.PVPlacementFailedTTL)
	setFloat64(env.InstanceStorageJitterMaxFactor, &config.InstanceStorage.JitterMaxFactor)
	setDuration(env.InstanceStorageSeedRequeueDuration, &config.InstanceStorage.SeedRequeueDuration)

	setBool(env.ContainerNode, &config.ContainerNode)
	setString(env.WatchNamespace, &config.WatchNamespace)
	setString(env.ProfilerAddr, &config.ProfilerAddr)
	setInt(env.RateLimitBurst, &config.RateLimitBurst)
	setInt(env.RateLimitQPS, &config.RateLimitQPS)
	setDuration(env.SyncPeriod, &config.SyncPeriod)
	setInt(env.MaxConcurrentReconciles, &config.MaxConcurrentReconciles)
	setString(env.LeaderElectionID, &config.LeaderElectionID)
	setBool(env.SIGUSR2RestartEnabled, &config.SIGUSR2RestartEnabled)
	setString(env.DeploymentName, &config.DeploymentName)
	setString(env.PodName, &config.PodName)
	setString(env.PodNamespace, &config.PodNamespace)
	setString(env.PodServiceAccountName, &config.PodServiceAccountName)

	setInt(env.WebhookServiceContainerPort, &config.WebhookServiceContainerPort)
	setString(env.WebhookServiceName, &config.WebhookServiceName)
	setString(env.WebhookServiceNamespace, &config.WebhookServiceNamespace)
	setString(env.WebhookSecretName, &config.WebhookSecretName)
	setString(env.WebhookSecretNamespace, &config.WebhookSecretNamespace)

	setBool(env.FSSInstanceStorage, &config.Features.InstanceStorage)
	setBool(env.FSSK8sWorkloadMgmtAPI, &config.Features.K8sWorkloadMgmtAPI)
	setBool(env.FSSPodVMOnStretchedSupervisor, &config.Features.PodVMOnStretchedSupervisor)
	setBool(env.FSSVMResize, &config.Features.VMResize)
	setBool(env.FSSVMResizeCPUMemory, &config.Features.VMResizeCPUMemory)
	setBool(env.FSSVMImportNewNet, &config.Features.VMImportNewNet)
	setBool(env.FSSVMIncrementalRestore, &config.Features.VMIncrementalRestore)
	setBool(env.FSSBringYourOwnEncryptionKey, &config.Features.BringYourOwnEncryptionKey)
	setBool(env.FSSFastDeploy, &config.Features.FastDeploy)
	setBool(env.FSSSVAsyncUpgrade, &config.Features.SVAsyncUpgrade)
	if !config.Features.SVAsyncUpgrade {
		// When SVAsyncUpgrade is enabled, we'll later use the capability CM to determine if
		// FSS's with a capability are enabled. TKGMultipleCL is special in that in predated
		// the SVAsyncUpgrade work.
		setBool(env.FSSWorkloadDomainIsolation, &config.Features.WorkloadDomainIsolation)
	}

	return config
}

func setBool(n env.VarName, p *bool) {
	if v := os.Getenv(n.String()); v != "" {
		if v, err := strconv.ParseBool(v); err == nil {
			*p = v
		}
	}
}

func setDuration(n env.VarName, p *time.Duration) {
	if v := os.Getenv(n.String()); v != "" {
		if v, err := time.ParseDuration(v); err == nil {
			*p = v
		}
	}
}

func setFloat64(n env.VarName, p *float64) {
	if v := os.Getenv(n.String()); v != "" {
		if v, err := strconv.ParseFloat(v, 64); err == nil {
			*p = v
		}
	}
}

func setInt(n env.VarName, p *int) {
	if v := os.Getenv(n.String()); v != "" {
		if v, err := strconv.Atoi(v); err == nil {
			*p = v
		}
	}
}

func setNetworkProviderType(n env.VarName, p *NetworkProviderType) {
	if v := os.Getenv(n.String()); v != "" {
		*p = NetworkProviderType(v)
	}
}

func setString(n env.VarName, p *string) {
	if v := os.Getenv(n.String()); v != "" {
		*p = v
	}
}

func setStringSlice(n env.VarName, p *string) {
	if v := os.Getenv(n.String()); v != "" {
		if v := StringToSlice(v); len(v) > 0 {
			*p = SliceToString(v)
		}
	}
}
