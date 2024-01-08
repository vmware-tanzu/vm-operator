// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustermodules

import (
	"context"
	"net/http"

	"github.com/vmware/govmomi/vapi/cluster"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

type Provider interface {
	CreateModule(ctx context.Context, clusterRef types.ManagedObjectReference) (string, error)
	DeleteModule(ctx context.Context, moduleID string) error
	DoesModuleExist(ctx context.Context, moduleID string) (bool, error)

	IsMoRefModuleMember(ctx context.Context, moduleID string, moRef types.ManagedObjectReference) (bool, error)
	AddMoRefToModule(ctx context.Context, moduleID string, moRef types.ManagedObjectReference) error
	RemoveMoRefFromModule(ctx context.Context, moduleID string, moRef types.ManagedObjectReference) error
}

type provider struct {
	manager *cluster.Manager
}

func NewProvider(restClient *rest.Client) Provider {
	return &provider{
		manager: cluster.NewManager(restClient),
	}
}

func (cm *provider) CreateModule(ctx context.Context, clusterRef types.ManagedObjectReference) (string, error) {
	log.Info("Creating cluster module", "cluster", clusterRef)

	moduleID, err := cm.manager.CreateModule(ctx, clusterRef)
	if err != nil {
		return "", err
	}

	log.Info("Created cluster module", "moduleID", moduleID)
	return moduleID, nil
}

func (cm *provider) DeleteModule(ctx context.Context, moduleID string) error {
	log.Info("Deleting cluster module", "moduleID", moduleID)

	err := cm.manager.DeleteModule(ctx, moduleID)
	if err != nil && !util.IsNotFoundError(err) {
		return err
	}

	log.Info("Deleted cluster module", "moduleID", moduleID)
	return nil
}

func (cm *provider) DoesModuleExist(ctx context.Context, moduleID string) (bool, error) {
	log.V(4).Info("Checking if cluster module exists", "moduleID", moduleID)

	if moduleID == "" {
		return false, nil
	}

	_, err := cm.manager.ListModuleMembers(ctx, moduleID)
	if err == nil {
		log.V(4).Info("Cluster module exists")
		return true, nil
	}

	if rest.IsStatusError(err, http.StatusNotFound) {
		log.V(4).Info("Cluster module doesn't exist")
		return false, nil
	}
	return false, err
}

func (cm *provider) IsMoRefModuleMember(ctx context.Context, moduleID string, moRef types.ManagedObjectReference) (bool, error) {
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

func (cm *provider) AddMoRefToModule(ctx context.Context, moduleID string, moRef types.ManagedObjectReference) error {
	isMember, err := cm.IsMoRefModuleMember(ctx, moduleID, moRef)
	if err != nil {
		return err
	}

	if !isMember {
		log.Info("Adding moRef to cluster module", "moduleID", moduleID, "moRef", moRef)
		// TODO: Should we just skip the IsMoRefModuleMember() and always call this since we're already
		// ignoring the first return value?
		_, err := cm.manager.AddModuleMembers(ctx, moduleID, moRef.Reference())
		if err != nil {
			return err
		}
	}

	return nil
}

func (cm *provider) RemoveMoRefFromModule(ctx context.Context, moduleID string, moRef types.ManagedObjectReference) error {
	log.Info("Removing moRef from cluster module", "moduleID", moduleID, "moRef", moRef)

	_, err := cm.manager.RemoveModuleMembers(ctx, moduleID, moRef)
	if err != nil {
		return err
	}

	log.Info("Removed moRef from cluster module", "moduleID", moduleID, "moRef", moRef)
	return nil
}
