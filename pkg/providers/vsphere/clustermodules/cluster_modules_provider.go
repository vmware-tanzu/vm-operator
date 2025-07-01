// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package clustermodules

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/vapi/cluster"
	"github.com/vmware/govmomi/vapi/rest"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

type Provider interface {
	CreateModule(ctx context.Context, clusterRef vimtypes.ManagedObjectReference) (string, error)
	DeleteModule(ctx context.Context, moduleID string) error
	DoesModuleExist(ctx context.Context, moduleID string, cluster vimtypes.ManagedObjectReference) (bool, error)

	IsMoRefModuleMember(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) (bool, error)
	AddMoRefToModule(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) error
	RemoveMoRefFromModule(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) error
}

type provider struct {
	manager *cluster.Manager
}

func NewProvider(restClient *rest.Client) Provider {
	return &provider{
		manager: cluster.NewManager(restClient),
	}
}

func (cm *provider) CreateModule(ctx context.Context, clusterRef vimtypes.ManagedObjectReference) (string, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Creating cluster module", "cluster", clusterRef)

	moduleID, err := cm.manager.CreateModule(ctx, clusterRef)
	if err != nil {
		return "", err
	}

	logger.Info("Created cluster module", "moduleID", moduleID)
	return moduleID, nil
}

func (cm *provider) DeleteModule(ctx context.Context, moduleID string) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Deleting cluster module", "moduleID", moduleID)

	err := cm.manager.DeleteModule(ctx, moduleID)
	if err != nil && !util.IsNotFoundError(err) {
		return err
	}

	logger.Info("Deleted cluster module", "moduleID", moduleID)
	return nil
}

func (cm *provider) DoesModuleExist(ctx context.Context, moduleID string, clusterRef vimtypes.ManagedObjectReference) (bool, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("Checking if cluster module exists", "moduleID", moduleID, "clusterRef", clusterRef)

	if moduleID == "" {
		return false, nil
	}

	// This is not efficient for as we use DoesModuleExist().
	modules, err := cm.manager.ListModules(ctx)
	if err != nil {
		return false, err
	}

	for _, mod := range modules {
		if mod.Cluster == clusterRef.Value && mod.Module == moduleID {
			return true, nil
		}
	}

	logger.V(4).Info("Cluster module doesn't exist", "moduleID", moduleID, "clusterRef", clusterRef)
	return false, nil
}

func (cm *provider) IsMoRefModuleMember(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) (bool, error) {
	moduleMembers, err := cm.manager.ListModuleMembers(ctx, moduleID)
	if err != nil {
		return false, err
	}

	for _, member := range moduleMembers {
		if member.Reference() == moRef.Reference() {
			return true, nil
		}
	}

	return false, nil
}

func (cm *provider) AddMoRefToModule(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) error {
	isMember, err := cm.IsMoRefModuleMember(ctx, moduleID, moRef)
	if err != nil {
		return err
	}

	if !isMember {
		logr.FromContextOrDiscard(ctx).Info("Adding moRef to cluster module", "moduleID", moduleID, "moRef", moRef)
		// TODO: Should we just skip the IsMoRefModuleMember() and always call this since we're already
		// ignoring the first return value?
		_, err := cm.manager.AddModuleMembers(ctx, moduleID, moRef.Reference())
		if err != nil {
			return err
		}
	}

	return nil
}

func (cm *provider) RemoveMoRefFromModule(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Removing moRef from cluster module", "moduleID", moduleID, "moRef", moRef)

	_, err := cm.manager.RemoveModuleMembers(ctx, moduleID, moRef)
	if err != nil {
		return err
	}

	logger.Info("Removed moRef from cluster module", "moduleID", moduleID, "moRef", moRef)
	return nil
}
