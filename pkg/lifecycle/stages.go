// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	lifecyclev1 "github.com/vmware-tanzu/vm-operator/external/lifecycle/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

const (
	// StagesName is the name of the cluster-scoped LifecycleStages instance
	// VM Operator publishes for VirtualMachine.
	StagesName = "vmoperator-stages"

	stageTypeReentrant = "Reentrant"
)

// publishBackoff bounds the retries of PublishStages so that a failing API
// server never holds up startup for long.
var publishBackoff = wait.Backoff{
	Duration: 100 * time.Millisecond,
	Factor:   2,
	Steps:    4,
}

// desiredStages returns the spec VM Operator publishes. VM Operator owns the
// whole instance, so this is the only definition of it.
func desiredStages() lifecyclev1.LifecycleStagesSpec {
	return lifecyclev1.LifecycleStagesSpec{
		Target: lifecyclev1.LifecycleStagesTarget{
			Group: TargetGroup,
			Kind:  TargetKind,
		},
		Stages: []lifecyclev1.LifecycleStageDefinition{
			{
				Name:          StageCreate,
				Description:   "Before the VM is created in vSphere.",
				Type:          stageTypeReentrant,
				Blocking:      true,
				EventName:     "CreateSuccess",
				ConditionName: vmopv1.VirtualMachineConditionLifecycleHookReady,
			},
			{
				Name:          StagePowerStateChange,
				Description:   "Before a power operation (on, off, suspend, restart) is applied.",
				Type:          stageTypeReentrant,
				Blocking:      true,
				EventName:     "UpdateSuccess",
				ConditionName: vmopv1.VirtualMachineConditionLifecycleHookReady,
			},
			{
				Name:          StageDelete,
				Description:   "Before the VM is deleted from vSphere.",
				Type:          stageTypeReentrant,
				Blocking:      true,
				EventName:     "DeleteSuccess",
				ConditionName: vmopv1.VirtualMachineConditionLifecycleHookReady,
			},
			{
				Name:          StageResourceDelete,
				Description:   "Before the VM's Kubernetes finalizer is removed.",
				Type:          stageTypeReentrant,
				Blocking:      true,
				ConditionName: vmopv1.VirtualMachineConditionLifecycleHookReady,
			},
		},
	}
}

// +kubebuilder:rbac:groups=lifecycle.vcfa.vmware.com,resources=lifecyclestages,verbs=get;create;patch

// PublishStages applies the vmoperator-stages LifecycleStages instance so the
// Lifecycle Operator can discover VM Operator's stages. It creates the object
// if absent and otherwise overwrites its spec, and does not write an unchanged
// object. It does nothing when the capability is off, and never deletes.
//
// Failures are retried with a short bounded backoff. The caller logs the
// returned error and continues: a failed publish must never block startup.
func PublishStages(
	ctx context.Context,
	k8sClient ctrlclient.Client) error {

	if !pkgcfg.FromContext(ctx).Features.LifecycleHooks {
		return nil
	}

	var lastErr error
	err := wait.ExponentialBackoffWithContext(
		ctx,
		publishBackoff,
		func(ctx context.Context) (bool, error) {
			stages := &lifecyclev1.LifecycleStages{
				ObjectMeta: metav1.ObjectMeta{Name: StagesName},
			}
			res, err := controllerutil.CreateOrPatch(
				ctx, k8sClient, stages, func() error {
					stages.Spec = desiredStages()
					return nil
				})
			if err != nil {
				lastErr = err
				pkglog.FromContextOrDefault(ctx).Error(
					err, "Failed to publish LifecycleStages, will retry",
					"name", StagesName)
				return false, nil
			}
			pkglog.FromContextOrDefault(ctx).V(4).Info(
				"Published LifecycleStages", "name", StagesName, "result", res)
			return true, nil
		})
	if err != nil {
		if lastErr != nil {
			err = lastErr
		}
		return fmt.Errorf("failed to publish LifecycleStages %s: %w", StagesName, err)
	}
	return nil
}
