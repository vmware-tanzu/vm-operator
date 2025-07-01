// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
)

var ErrUpgradeSchema = pkgerr.NoRequeueNoErr("upgraded vm schema")

// ReconcileSchemaUpgrade ensures the VM's spec is upgraded to match the current
// expectations for the data that should be present on a VirtualMachine object.
// This may include back-filling data from the underlying vSphere VM into the
// API object's spec.
//
// Please note, each time VM Operator is upgraded, patch/update operations
// against VirtualMachine objects by unprivileged users will be denied until
// ReconcileSchemaUpgrade is executed. This ensures the objects are not changing
// while the object is being back-filled.
func ReconcileSchemaUpgrade(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) error {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if moVM.Config == nil {
		panic("moVM.config is nil")
	}

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM")

	var (
		curBuildVersion  = pkgcfg.FromContext(ctx).BuildVersion
		curSchemaVersion = vmopv1.GroupVersion.Version

		vmBuildVersion  = vm.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey]
		vmSchemaVersion = vm.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey]
	)

	if vmBuildVersion == curBuildVersion &&
		vmSchemaVersion == curSchemaVersion {

		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM" +
			" that is already upgraded")
		return nil
	}

	reconcileBIOSUUID(ctx, vm, moVM)
	reconcileInstanceUUID(ctx, vm, moVM)
	reconcileCloudInitInstanceUUID(ctx, vm, moVM)

	// Indicate the VM has been upgraded.
	if vm.Annotations == nil {
		vm.Annotations = map[string]string{}
	}
	vm.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey] = curBuildVersion
	vm.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey] = curSchemaVersion

	logger.V(4).Info("Upgraded VM schema version",
		"buildVersion", curBuildVersion,
		"schemaVersion", curSchemaVersion)

	return ErrUpgradeSchema
}

func reconcileBIOSUUID(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) {

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM BIOS UUID")

	if vm.Spec.BiosUUID != "" {
		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM " +
			"instance BIOS that is already upgraded")
		return
	}

	if moVM.Config.Uuid == "" {
		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM " +
			"BIOS UUID with empty config.uuid")
		return
	}

	vm.Spec.BiosUUID = moVM.Config.Uuid
	logger.V(4).Info("Reconciled schema upgrade for VM BIOS UUID")
}

func reconcileInstanceUUID(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) {

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM instance UUID")

	if vm.Spec.InstanceUUID != "" {
		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM " +
			"instance UUID that is already upgraded")
		return
	}

	if moVM.Config.InstanceUuid == "" {
		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM " +
			"instance UUID with empty config.instanceUuid")
		return
	}

	vm.Spec.InstanceUUID = moVM.Config.InstanceUuid
	logger.V(4).Info("Reconciled schema upgrade for VM instance UUID")
}

func reconcileCloudInitInstanceUUID(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	_ mo.VirtualMachine) {

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info(
		"Reconciling schema upgrade for VM cloud-init instance UUID")

	if vm.Spec.Bootstrap == nil {
		logger.V(4).Info(
			"Skipping reconciliation of schema upgrade for VM cloud-init " +
				"instance UUID with nil spec.bootstrap")
		return
	}

	if vm.Spec.Bootstrap.CloudInit == nil {
		logger.V(4).Info(
			"Skipping reconciliation of schema upgrade for VM cloud-init " +
				"instance UUID with nil spec.bootstrap.cloudInit")
		return
	}

	if vm.Spec.Bootstrap.CloudInit.InstanceID != "" {
		logger.V(4).Info(
			"Skipping reconciliation of schema upgrade for VM cloud-init" +
				"instance UUID that is already upgraded")
		return
	}

	_ = vmlifecycle.BootStrapCloudInitInstanceID(
		vm,
		vm.Spec.Bootstrap.CloudInit)

	logger.V(4).Info(
		"Reconciled schema upgrade for VM cloud-init instance UUID")
}
