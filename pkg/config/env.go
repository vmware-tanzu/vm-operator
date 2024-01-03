// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
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
	setString(env.DefaultVMClassControllerName, &config.DefaultVMClassControllerName)
	setInt(env.MaxCreateVMsOnProvider, &config.MaxCreateVMsOnProvider)
	setNetworkProviderType(env.NetworkProviderType, &config.NetworkProviderType)
	setString(env.LoadBalancerProvider, &config.LoadBalancerProvider)
	setBool(env.VSphereNetworking, &config.VSphereNetworking)
	setStringSlice(env.PrivilegedUsers, &config.PrivilegedUsers)

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
	setString(env.PodName, &config.PodName)
	setString(env.PodNamespace, &config.PodNamespace)
	setString(env.PodServiceAccountName, &config.PodServiceAccountName)

	setInt(env.WebhookServiceContainerPort, &config.WebhookServiceContainerPort)
	setString(env.WebhookServiceName, &config.WebhookServiceName)
	setString(env.WebhookServiceNamespace, &config.WebhookServiceNamespace)
	setString(env.WebhookSecretName, &config.WebhookSecretName)
	setString(env.WebhookSecretNamespace, &config.WebhookSecretNamespace)

	setBool(env.FSSFaultDomains, &config.Features.FaultDomains)
	setBool(env.FSSVMOpV1Alpha2, &config.Features.VMOpV1Alpha2)
	setBool(env.FSSInstanceStorage, &config.Features.InstanceStorage)
	setBool(env.FSSUnifiedTKG, &config.Features.UnifiedTKG)
	setBool(env.FSSVMClassAsConfig, &config.Features.VMClassAsConfig)
	setBool(env.FSSVMClassAsConfigDaynDate, &config.Features.VMClassAsConfigDayNDate)
	setBool(env.FSSImageRegistry, &config.Features.ImageRegistry)
	setBool(env.FSSNamespacedVMClass, &config.Features.NamespacedVMClass)
	setBool(env.FSSWindowsSysprep, &config.Features.WindowsSysprep)
	setBool(env.FSSVMServiceBackupRestore, &config.Features.AutoVADPBackupRestore)

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
