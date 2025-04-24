// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package env

import (
	"os"
)

// VarName is the name of an environment variable.
type VarName uint8

const (
	_varNameBegin VarName = iota

	DefaultVMClassControllerName
	MaxCreateVMsOnProvider
	CreateVMRequeueDelay
	PoweredOnVMHasIPRequeueDelay
	SyncImageRequeueDelay
	PrivilegedUsers
	NetworkProviderType
	LoadBalancerProvider
	VSphereNetworking
	ContentAPIWaitDuration
	JSONExtraConfig
	LogSensitiveData
	AsyncSignalEnabled
	AsyncCreateEnabled
	FastDeployMode
	VCCredsSecretName
	InstanceStoragePVPlacementFailedTTL
	InstanceStorageJitterMaxFactor
	InstanceStorageSeedRequeueDuration
	ContainerNode
	ProfilerAddr
	RateLimitQPS
	RateLimitBurst
	SyncPeriod
	MaxConcurrentReconciles
	MemStatsPeriod
	LeaderElectionID
	SIGUSR2RestartEnabled
	DeploymentName
	PodName
	PodNamespace
	PodServiceAccountName
	WatchNamespace
	WebhookServiceContainerPort
	WebhookServiceName
	WebhookServiceNamespace
	WebhookSecretName
	WebhookSecretNamespace
	FSSInstanceStorage
	FSSK8sWorkloadMgmtAPI
	FSSPodVMOnStretchedSupervisor
	FSSVMResize
	FSSVMResizeCPUMemory
	FSSVMImportNewNet
	FSSWorkloadDomainIsolation
	FSSVMIncrementalRestore
	FSSBringYourOwnEncryptionKey
	FSSSVAsyncUpgrade
	FSSFastDeploy
	_varNameEnd
)

// Unset unsets all environment variables related to VM Operator.
func Unset() {
	for _, n := range All() {
		// os.Unsetenv cannot return an error on Linux, where VM Operator runs
		_ = os.Unsetenv(n.String())
	}
}

// All returns all of the environment variable names.
func All() []VarName {
	all := make([]VarName, _varNameEnd-1)
	i := 0
	for n := _varNameBegin + 1; n < _varNameEnd; n++ {
		all[i] = n
		i++
	}
	return all
}

// String returns the stringified version of the environment variable name.
//
//nolint:gocyclo
func (n VarName) String() string {
	switch n {
	case DefaultVMClassControllerName:
		return "DEFAULT_VM_CLASS_CONTROLLER_NAME"
	case MaxCreateVMsOnProvider:
		return "MAX_CREATE_VMS_ON_PROVIDER"
	case CreateVMRequeueDelay:
		return "CREATE_VM_REQUEUE_DELAY"
	case PoweredOnVMHasIPRequeueDelay:
		return "POWERED_ON_VM_HAS_IP_REQUEUE_DELAY"
	case SyncImageRequeueDelay:
		return "SYNC_IMAGE_REQUEUE_DELAY"
	case PrivilegedUsers:
		return "PRIVILEGED_USERS"
	case NetworkProviderType:
		return "NETWORK_PROVIDER"
	case LoadBalancerProvider:
		return "LB_PROVIDER"
	case VSphereNetworking:
		return "VSPHERE_NETWORKING"
	case ContentAPIWaitDuration:
		return "CONTENT_API_WAIT_SECS"
	case JSONExtraConfig:
		return "JSON_EXTRA_CONFIG"
	case LogSensitiveData:
		return "LOG_SENSITIVE_DATA"
	case AsyncSignalEnabled:
		return "ASYNC_SIGNAL_ENABLED"
	case AsyncCreateEnabled:
		return "ASYNC_CREATE_ENABLED"
	case FastDeployMode:
		return "FAST_DEPLOY_MODE"
	case VCCredsSecretName:
		return "VC_CREDS_SECRET_NAME"
	case InstanceStoragePVPlacementFailedTTL:
		return "INSTANCE_STORAGE_PV_PLACEMENT_FAILED_TTL"
	case InstanceStorageJitterMaxFactor:
		return "INSTANCE_STORAGE_JITTER_MAX_FACTOR"
	case InstanceStorageSeedRequeueDuration:
		return "INSTANCE_STORAGE_SEED_REQUEUE_DURATION"
	case ContainerNode:
		return "CONTAINER_NODE"
	case ProfilerAddr:
		return "PROFILER_ADDR"
	case RateLimitQPS:
		return "RATE_LIMIT_QPS"
	case RateLimitBurst:
		return "RATE_LIMIT_BURST"
	case SyncPeriod:
		return "SYNC_PERIOD"
	case MaxConcurrentReconciles:
		return "MAX_CONCURRENT_RECONCILES"
	case MemStatsPeriod:
		return "MEM_STATS_PERIOD"
	case LeaderElectionID:
		return "LEADER_ELECTION_ID"
	case SIGUSR2RestartEnabled:
		return "SIGUSR2_RESTART_ENABLED"
	case DeploymentName:
		return "DEPLOYMENT_NAME"
	case PodName:
		return "POD_NAME"
	case PodNamespace:
		return "POD_NAMESPACE"
	case PodServiceAccountName:
		return "POD_SERVICE_ACCOUNT_NAME"
	case WatchNamespace:
		return "WATCH_NAMESPACE"
	case WebhookServiceContainerPort:
		return "WEBHOOK_SERVICE_CONTAINER_PORT"
	case WebhookServiceName:
		return "WEBHOOK_SERVICE_NAME"
	case WebhookServiceNamespace:
		return "WEBHOOK_SERVICE_NAMESPACE"
	case WebhookSecretName:
		return "WEBHOOK_SECRET_NAME"
	case WebhookSecretNamespace:
		return "WEBHOOK_SECRET_NAMESPACE"
	case FSSInstanceStorage:
		return "FSS_WCP_INSTANCE_STORAGE"
	case FSSK8sWorkloadMgmtAPI:
		return "FSS_WCP_VMSERVICE_K8S_WORKLOAD_MGMT_API"
	case FSSPodVMOnStretchedSupervisor:
		return "FSS_PODVMONSTRETCHEDSUPERVISOR"
	case FSSVMResize:
		return "FSS_WCP_VMSERVICE_RESIZE"
	case FSSVMResizeCPUMemory:
		return "FSS_WCP_VMSERVICE_RESIZE_CPU_MEMORY"
	case FSSVMImportNewNet:
		return "FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET"
	case FSSWorkloadDomainIsolation:
		return "FSS_WCP_WORKLOAD_DOMAIN_ISOLATION"
	case FSSVMIncrementalRestore:
		return "FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE"
	case FSSBringYourOwnEncryptionKey:
		return "FSS_WCP_VMSERVICE_BYOK"
	case FSSSVAsyncUpgrade:
		return "FSS_WCP_SUPERVISOR_ASYNC_UPGRADE"
	case FSSFastDeploy:
		return "FSS_WCP_VMSERVICE_FAST_DEPLOY"
	}
	panic("unknown environment variable")
}
