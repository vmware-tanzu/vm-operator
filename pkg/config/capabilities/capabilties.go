// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package capabilities

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

const (
	// ConfigMapName is the name of the capabilities ConfigMap.
	ConfigMapName = "wcp-cluster-capabilities"

	// ConfigMapNamespace is the namespace of the capabilities ConfigMap.
	ConfigMapNamespace = "kube-system"

	// CapabilitiesName is the name of the cluster-scoped Capabilities resource
	// used to discover Supervisor capabilities.
	CapabilitiesName = "supervisor-capabilities"

	// CapabilityKeyBringYourOwnKeyProvider is the name of capability key
	// defined in the capabilities ConfigMap and/or CRD.
	CapabilityKeyBringYourOwnKeyProvider = "Bring_Your_Own_Key_Provider_Supported"

	// CapabilityKeyTKGMultipleContentLibraries is the name of capability key
	// defined in the capabilities ConfigMap and/or CRD.
	CapabilityKeyTKGMultipleContentLibraries = "MultipleCL_For_TKG_Supported"

	// CapabilityKeyWorkloadIsolation is the name of capability key
	// defined in the capabilities ConfigMap and/or CRD.
	CapabilityKeyWorkloadIsolation = "Workload_Domain_Isolation_Supported"

	// CapabilityKeyMutableNetworks is the name of capability key
	// defined in the capabilities ConfigMap and/or CRD.
	CapabilityKeyMutableNetworks = "supports_VM_service_mutable_networks"

	// CapabilityKeyVMGroups is the name of capability key defined in the
	// Supervisor capabilities CRD.
	CapabilityKeyVMGroups = "supports_VM_service_VM_groups"

	// CapabilityKeyImmutableClasses is the name of capability key defined in the
	// Supervisor capabilities CRD.
	CapabilityKeyImmutableClasses = "supports_VM_service_immutable_VM_classes"

	// CapabilityKeyVMSnapshots is the name of the capability key
	// defined in the Supervisor capabilities CRD for the VM snapshots
	// capability.
	CapabilityKeyVMSnapshots = "supports_VM_service_VM_snapshots"
)

var (
	// CapabilitiesKey is the ObjectKey for the cluster-scoped Capabilities
	// resource.
	CapabilitiesKey = ctrlclient.ObjectKey{Name: CapabilitiesName}

	// ConfigMapKey is the ObjectKey for the capabilities ConfigMap.
	ConfigMapKey = ctrlclient.ObjectKey{
		Name:      ConfigMapName,
		Namespace: ConfigMapNamespace,
	}
)

// UpdateCapabilities updates the capabilities in the context from either the
// capabilities ConfigMap or cluster-scoped CRD resource.
func UpdateCapabilities(
	ctx context.Context,
	k8sClient ctrlclient.Client) (bool, error) {

	if pkgcfg.FromContext(ctx).Features.SVAsyncUpgrade {
		var obj capv1.Capabilities
		if err := k8sClient.Get(ctx, CapabilitiesKey, &obj); err != nil {
			return false, err
		}
		_, ok := UpdateCapabilitiesFeatures(ctx, obj)
		return ok, nil
	}

	var obj corev1.ConfigMap
	if err := k8sClient.Get(ctx, ConfigMapKey, &obj); err != nil {
		return false, err
	}
	_, ok := UpdateCapabilitiesFeatures(ctx, obj)
	return ok, nil
}

type updateCapTypes interface {
	map[string]string | corev1.ConfigMap | capv1.Capabilities
}

// UpdateCapabilitiesFeatures updates the features in the context config.
//
// Please note, this call impacts the configuration available to contexts
// throughout the process, not just *this* context or its children.
// Please refer to the pkg/config package for more information on SetContext
// and its behavior.
//
// The return value indicates if any of the features changed.
func UpdateCapabilitiesFeatures[T updateCapTypes](
	ctx context.Context,
	obj T) (string, bool) {

	return updateCapabilitiesFeatures(ctx, obj, false)
}

// WouldUpdateCapabilitiesFeatures is like UpdateCapabilitiesFeatures but does
// not actually cause a change to the capabilities.
func WouldUpdateCapabilitiesFeatures[T updateCapTypes](
	ctx context.Context,
	obj T) (string, bool) {

	return updateCapabilitiesFeatures(ctx, obj, true)
}

func updateCapabilitiesFeatures[T updateCapTypes](
	ctx context.Context,
	obj T,
	dryRun bool) (string, bool) {

	var (
		newFeat pkgcfg.FeatureStates
		logger  = logr.FromContextOrDiscard(ctx).WithValues("dryRun", dryRun)
		oldFeat = pkgcfg.FromContext(ctx).Features
	)

	logger.Info(
		"Checking if capabilities would update features",
		"oldFeat", oldFeat)

	switch tObj := (any)(obj).(type) {
	case map[string]string:
		newFeat = updateCapabilitiesFeaturesFromMap(tObj, oldFeat)
	case corev1.ConfigMap:
		newFeat = updateCapabilitiesFeaturesFromMap(tObj.Data, oldFeat)
	case capv1.Capabilities:
		newFeat = updateCapabilitiesFeaturesFromCRD(tObj, oldFeat)
	}

	if oldFeat != newFeat {
		if !dryRun {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features = newFeat
			})
		}

		// Use a custom diff reporter to get a sanitized version of the
		// differences between the old and new features.
		r := &diffReporter{}
		_ = cmp.Equal(oldFeat, newFeat, cmp.Reporter(r))
		diff := r.String()

		logger.Info("Updated features from capabilities", "diff", diff)
		return diff, true
	}

	logger.Info("Features not updated from capabilities")
	return "", false
}

func updateCapabilitiesFeaturesFromMap(
	data map[string]string,
	fs pkgcfg.FeatureStates) pkgcfg.FeatureStates {

	// TKGMultipleCL is unique in that it is a capability but it predates
	// SVAsyncUpgrade.
	fs.TKGMultipleCL = isEnabled(data[CapabilityKeyTKGMultipleContentLibraries])

	if fs.SVAsyncUpgrade {
		// All other capabilities are gated by SVAsyncUpgrade.
		fs.WorkloadDomainIsolation = isEnabled(data[CapabilityKeyWorkloadIsolation])
	}

	return fs
}

func updateCapabilitiesFeaturesFromCRD(
	obj capv1.Capabilities,
	fs pkgcfg.FeatureStates) pkgcfg.FeatureStates {

	for capName, capStatus := range obj.Status.Supervisor {
		switch capName {
		case CapabilityKeyBringYourOwnKeyProvider:
			fs.BringYourOwnEncryptionKey = capStatus.Activated
		case CapabilityKeyTKGMultipleContentLibraries:
			fs.TKGMultipleCL = capStatus.Activated
		case CapabilityKeyWorkloadIsolation:
			fs.WorkloadDomainIsolation = capStatus.Activated
		case CapabilityKeyMutableNetworks:
			fs.MutableNetworks = capStatus.Activated
		case CapabilityKeyVMGroups:
			fs.VMGroups = capStatus.Activated
		case CapabilityKeyImmutableClasses:
			fs.ImmutableClasses = capStatus.Activated
		case CapabilityKeyVMSnapshots:
			fs.VMSnapshots = capStatus.Activated
		}

	}
	return fs
}

func isEnabled(v string) bool {
	ok, _ := strconv.ParseBool(v)
	return ok
}

// diffReporter is a custom reporter for comparing config.Features structs
// that lists only the features that have changed using a single line of
// comma-separated values.
type diffReporter struct {
	path  cmp.Path
	diffs []string
}

func (r *diffReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *diffReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		capName := strings.TrimPrefix(r.path.Last().String(), ".")
		_, newVal := r.path.Last().Values()
		r.diffs = append(r.diffs, fmt.Sprintf("%s=%v", capName, newVal))
	}
}

func (r *diffReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *diffReporter) String() string {
	slices.Sort(r.diffs)
	return strings.Join(r.diffs, ",")
}
