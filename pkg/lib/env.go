// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package lib

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	trueString                    = "true"
	TrueString                    = "true"
	VmopNamespaceEnv              = "POD_NAMESPACE"
	WcpFaultDomainsFSS            = "FSS_WCP_FAULTDOMAINS"
	VMServiceFSS                  = "FSS_WCP_VMSERVICE"
	VMServiceV1Alpha2FSS          = "FSS_WCP_VMSERVICE_V1ALPHA2"
	InstanceStorageFSS            = "FSS_WCP_INSTANCE_STORAGE"
	UnifiedTKGBYOIFSS             = "FSS_WCP_VMSERVICE_UNIFIEDTKG_BYOI"
	VMServiceBackupRestoreFSS     = "FSS_WCP_VMSERVICE_BACKUPRESTORE"
	VMServicePublicCloudBYOIFSS   = "FSS_WCP_VMSERVICE_PUBLIC_CLOUD_BYOI"
	UnifiedTKGFSS                 = "FSS_WCP_Unified_TKG"
	MaxCreateVMsOnProviderEnv     = "MAX_CREATE_VMS_ON_PROVIDER"
	DefaultMaxCreateVMsOnProvider = 80

	InstanceStoragePVPlacementFailedTTLEnv = "INSTANCE_STORAGE_PV_PLACEMENT_FAILED_TTL"
	// DefaultInstanceStoragePVPlacementFailedTTL is the default wait time before declaring PV placement failed
	// after error annotation is set on PVC.
	DefaultInstanceStoragePVPlacementFailedTTL = 5 * time.Minute
	// InstanceStorageJitterMaxFactorEnv is env variable for setting max factor to be used in wait.Jitter
	// for instance storage.
	InstanceStorageJitterMaxFactorEnv = "INSTANCE_STORAGE_JITTER_MAX_FACTOR"
	// DefaultInstanceStorageJitterMaxFactor is the default max factor to compute jitter requeue duration
	// for instance storage.
	// Note that wait.Jitter sets the maxFactor to 1.0 if the input maxFactor is <= 0.0. With this default
	// max factor and 10s default seed duration, wait.Jitter returns requeue delay between 11 and 19.
	// These numbers ensures multiple reconcile threads aren't re queuing VMs at the exact interval. We want
	// to have a little entropy around the requeue time.
	DefaultInstanceStorageJitterMaxFactor = 1.0
	// InstanceStorageSeedRequeueDurationEnv is environment variable for setting seed requeue
	// duration for instance storage.
	InstanceStorageSeedRequeueDurationEnv = "INSTANCE_STORAGE_SEED_REQUEUE_DURATION"
	// DefaultInstanceStorageSeedRequeueDuration is the default seed requeue duration for instance storage.
	DefaultInstanceStorageSeedRequeueDuration = 10 * time.Second
)

// SetVMOpNamespaceEnv sets the VM Operator pod's namespace in the environment.
func SetVMOpNamespaceEnv(namespace string) error {
	err := os.Setenv(VmopNamespaceEnv, namespace)
	if err != nil {
		return fmt.Errorf("failed to set env var: %v", err)
	}
	return nil
}

// GetVMOpNamespaceFromEnv resolves the VM Operator pod's namespace from the environment.
func GetVMOpNamespaceFromEnv() (string, error) {
	vmopNamespace, vmopNamespaceExists := os.LookupEnv(VmopNamespaceEnv)
	if !vmopNamespaceExists {
		return "", fmt.Errorf("VM Operator namespace envvar %s is not set", VmopNamespaceEnv)
	}
	return vmopNamespace, nil
}

var IsWcpFaultDomainsFSSEnabled = func() bool {
	return os.Getenv(WcpFaultDomainsFSS) == trueString
}

var IsVMServiceFSSEnabled = func() bool {
	return os.Getenv(VMServiceFSS) == trueString
}

var IsVMServiceV1Alpha2FSSEnabled = func() bool {
	return os.Getenv(VMServiceV1Alpha2FSS) == trueString
}

var IsInstanceStorageFSSEnabled = func() bool {
	return os.Getenv(InstanceStorageFSS) == trueString
}

var IsUnifiedTKGBYOIFSSEnabled = func() bool {
	return os.Getenv(UnifiedTKGBYOIFSS) == trueString
}

var IsUnifiedTKGFSSEnabled = func() bool {
	return os.Getenv(UnifiedTKGFSS) == trueString
}

var IsVMServiceBackupRestoreFSSEnabled = func() bool {
	return os.Getenv(VMServiceBackupRestoreFSS) == trueString
}

var IsVMServicePublicCloudBYOIFSSEnabled = func() bool {
	return os.Getenv(VMServicePublicCloudBYOIFSS) == trueString
}

// MaxConcurrentCreateVMsOnProvider returns the percentage of reconciler threads that can be used to create VMs on the provider
// concurrently. The default is 80.
// TODO: Remove the env lookup once we have tuned this value from system tests.
var MaxConcurrentCreateVMsOnProvider = func() int {
	v := os.Getenv(MaxCreateVMsOnProviderEnv)
	if v == "" {
		return DefaultMaxCreateVMsOnProvider
	}

	// Return default in case of an invalid value.
	val, err := strconv.Atoi(v)
	if err != nil {
		return DefaultMaxCreateVMsOnProvider
	}

	return val
}

// GetInstanceStoragePVPlacementFailedTTL returns the configured wait time before declaring PV placement
// failed after error annotation is set on PVC.
func GetInstanceStoragePVPlacementFailedTTL() time.Duration {
	if delay := os.Getenv(InstanceStoragePVPlacementFailedTTLEnv); len(delay) > 0 {
		if duration, err := time.ParseDuration(delay); err == nil {
			return duration
		}
	}
	return DefaultInstanceStoragePVPlacementFailedTTL
}

// GetInstanceStorageRequeueDelay returns requeue delay for instance storage.
func GetInstanceStorageRequeueDelay() time.Duration {
	maxFactor := DefaultInstanceStorageJitterMaxFactor
	seedDuration := DefaultInstanceStorageSeedRequeueDuration

	if s := os.Getenv(InstanceStorageJitterMaxFactorEnv); len(s) > 0 {
		if factor, err := strconv.ParseFloat(s, 64); err == nil {
			maxFactor = factor
		}
	}
	if s := os.Getenv(InstanceStorageSeedRequeueDurationEnv); len(s) > 0 {
		if duration, err := time.ParseDuration(s); err == nil {
			seedDuration = duration
		}
	}

	return wait.Jitter(seedDuration, maxFactor)
}
