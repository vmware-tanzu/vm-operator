// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"
)

// Config represents the internal configuration of VM Operator. It should only
// be read/written via the context functions.
//
// Please note that all fields in this type MUST be types that are copied by
// value, not reference. That means no string slices, maps, etc. The reason is
// to prevent the possibility of race conditions when reading/writing data to
// a Config instance stored in a context.
type Config struct {
	BuildCommit  string
	BuildNumber  string
	BuildVersion string
	BuildType    string

	ContainerNode bool

	ContentAPIWait  time.Duration
	JSONExtraConfig string

	// DefaultVMClassControllerName is the default value for the
	// VirtualMachineClass field spec.controllerName.
	//
	// Defaults to vmoperator.vmware.com/vsphere.
	DefaultVMClassControllerName string

	// Features reflects the feature states.
	Features FeatureStates

	// InstanceStorage contains configuration details related to the instance
	// storage feature.
	InstanceStorage InstanceStorage

	LeaderElectionID        string
	MaxConcurrentReconciles int

	// MaxCreateVMsOnProvider is the percentage of reconciler threads that can
	// be used to create VMs on the provider concurrently.
	//
	// Defaults to 80.
	MaxCreateVMsOnProvider int

	// MaxDeployThreadsOnProvider is the number of threads used to deploy VMs
	// concurrently.
	//
	// Callers should not read this value directly, instead callers should use
	// the GetMaxDeployThreadsOnProvider function.
	MaxDeployThreadsOnProvider int

	// CreateVMRequeueDelay is the requeue delay that is used to retry a VM
	// create that was unable to handled because MaxDeployThreadsOnProvider
	// creates where already in progress.
	// Defaults to 10 seconds.
	CreateVMRequeueDelay time.Duration

	// PoweredOnVMHasIPRequeueDelay is the requeue delay that is when a VM
	// is powered on but does not yet have an IP assigned. This is used so
	// the VM Status is updated sooner than waiting for the SyncPeriod to
	// occur.
	// Defaults to 10 seconds.
	PoweredOnVMHasIPRequeueDelay time.Duration

	NetworkProviderType  NetworkProviderType
	VSphereNetworking    bool
	LoadBalancerProvider string

	PodName               string
	PodNamespace          string
	PodServiceAccountName string

	// PrivilegedUsers is a comma-delimited a list of users that are, in
	// addition to the kube-admin and system users, treated as privileged by
	// VM Operator, ex. allowed to make changes to certain
	// annotations/labels/properties on resources.
	//
	// For information as to why this field is not a []string, please see the
	// GoDocs for the Config type.
	PrivilegedUsers string

	ProfilerAddr                 string
	RateLimitBurst               int
	RateLimitQPS                 int
	SyncPeriod                   time.Duration
	WatchNamespace               string
	WebhookServiceContainerPort  int
	WebhookServiceName           string
	WebhookServiceNamespace      string
	WebhookSecretName            string
	WebhookSecretNamespace       string
	WebhookSecretVolumeMountPath string

	// LogSensitiveData means that logs will potentially contain sensitive data
	// such as passwords. Defaults to false.
	LogSensitiveData bool

	// AsyncSignalDisabled may be set to false to disable the vm-watcher service
	// used to reconcile VirtualMachine objects if their backend state has
	// changed.
	AsyncSignalDisabled bool
}

// GetMaxDeployThreadsOnProvider returns MaxDeployThreadsOnProvider if it is >0
// or returns a percentage of MaxConcurrentReconciles based on
// MaxCreateVMsOnProvider.
func (c Config) GetMaxDeployThreadsOnProvider() int {
	if c.MaxDeployThreadsOnProvider > 0 {
		return c.MaxDeployThreadsOnProvider
	}
	return int(
		float64(c.MaxConcurrentReconciles) /
			(float64(100) / float64(c.MaxCreateVMsOnProvider)))
}

type FeatureStates struct {
	IsoSupport                 bool // FSS_WCP_VMSERVICE_ISO_SUPPORT
	InstanceStorage            bool // FSS_WCP_INSTANCE_STORAGE
	K8sWorkloadMgmtAPI         bool // FSS_WCP_VMSERVICE_K8S_WORKLOAD_MGMT_API
	PodVMOnStretchedSupervisor bool // FSS_PODVMONSTRETCHEDSUPERVISOR
	TKGMultipleCL              bool // FSS_WCP_TKG_Multiple_CL
	// TODO(akutz) This FSS is a placeholder until leadership can figure out the
	//             plan for FSSs going forward.
	UnifiedStorageQuota       bool // FSS_PLACEHOLDER_WCP_UNIFIED_STORAGE_QUOTA
	VMResize                  bool // FSS_WCP_VMSERVICE_RESIZE
	VMResizeCPUMemory         bool // FSS_WCP_VMSERVICE_RESIZE_CPU_MEMORY
	VMImportNewNet            bool // FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET
	WorkloadDomainIsolation   bool // FSS_WCP_WORKLOAD_DOMAIN_ISOLATION
	VMIncrementalRestore      bool // FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE
	BringYourOwnEncryptionKey bool // FSS_WCP_VMSERVICE_BYOK
	SVAsyncUpgrade            bool // FSS_WCP_SUPERVISOR_ASYNC_UPGRADE
	SimplifiedEnablement      bool // FSS_WCP_SIMPLIFIED_ENABLEMENT
}

type InstanceStorage struct {
	// PVPlacementFailedTTL is the wait time before declaring PV placement
	// failed after the error annotation is set on a PVC.
	//
	// Defaults to 5m.
	PVPlacementFailedTTL time.Duration

	// JitterMaxFactor is used to configure the exponential backoff jitter for
	// instance storage.
	//
	// Please note that wait.Jitter sets the maxFactor to 1.0 if the input
	// maxFactor is <= 0.0. For example, with a max factor of 1.0 and seed
	// duration of 10s, wait.Jitter returns a requeue delay between 11 and 19.
	// These numbers ensures multiple reconcile threads are not requeuing at the
	// same intervals.
	//
	// Defaults to 1.0.
	JitterMaxFactor float64

	// SeedRequeueDuration is the seed value for requeuing reconcile attempts
	// related to instance storage resources.
	//
	// Defaults to 10s.
	SeedRequeueDuration time.Duration
}

type NetworkProviderType string

const (
	NetworkProviderTypeNamed NetworkProviderType = "NAMED"
	NetworkProviderTypeNSXT  NetworkProviderType = "NSXT"
	NetworkProviderTypeVDS   NetworkProviderType = "VSPHERE_NETWORK"
	NetworkProviderTypeVPC   NetworkProviderType = "NSXT_VPC"
)
