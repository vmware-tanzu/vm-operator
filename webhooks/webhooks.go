// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package webhooks

import (
	"fmt"
	"os"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/conversion"
	"github.com/vmware-tanzu/vm-operator/webhooks/persistentvolumeclaim"
	"github.com/vmware-tanzu/vm-operator/webhooks/unifiedstoragequota"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineclass"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinepublishrequest"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinereplicaset"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineservice"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinesetresourcepolicy"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinewebconsolerequest"
)

// AddToManager adds all webhooks and a certificate manager to the provided controller manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	containerType := os.Getenv("VMOP_CONTAINER_TYPE")
	switch containerType {
	case "admission-webhook":
		if err := persistentvolumeclaim.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize PersistentVolumeClaim webhook: %w", err)
		}
		if err := virtualmachine.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize VirtualMachine webhooks: %w", err)
		}
		if err := virtualmachineclass.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize VirtualMachineClass webhooks: %w", err)
		}
		if err := virtualmachinepublishrequest.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize VirtualMachinePublishRequest webhooks: %w", err)
		}
		if err := virtualmachineservice.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize VirtualMachineService webhooks: %w", err)
		}
		if err := virtualmachinesetresourcepolicy.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize VirtualMachineSetResourcePolicy webhooks: %w", err)
		}
		if err := virtualmachinewebconsolerequest.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize VirtualMachineWebConsoleRequest webhooks: %w", err)
		}

		if pkgcfg.FromContext(ctx).Features.K8sWorkloadMgmtAPI {
			if err := virtualmachinereplicaset.AddToManager(ctx, mgr); err != nil {
				return fmt.Errorf("failed to initialize VirtualMachineReplicaSet webhooks: %w", err)
			}
		}

		if pkgcfg.FromContext(ctx).Features.UnifiedStorageQuota {
			if err := unifiedstoragequota.AddToManager(ctx, mgr); err != nil {
				return fmt.Errorf("failed to initialize UnifiedStorageQuota webhooks: %w", err)
			}
		}
	case "conversion-webhook":
		if err := conversion.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize conversion webhooks: %w", err)
		}
	default:
		return fmt.Errorf("invalid container type: %s", containerType)
	}

	return nil
}
