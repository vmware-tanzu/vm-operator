// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	goctx "context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
)

var log = logf.Log.WithName("vsphere").WithName("session")

var DefaultExtraConfig = map[string]string{
	constants.EnableDiskUUIDExtraConfigKey:       constants.ExtraConfigTrue,
	constants.GOSCIgnoreToolsCheckExtraConfigKey: constants.ExtraConfigTrue,
}

type Session struct {
	Client    *client.Client
	k8sClient ctrlruntime.Client

	Finder     *find.Finder
	datacenter *object.Datacenter
	cluster    *object.ClusterComputeResource
	datastore  *object.Datastore

	networkProvider network.Provider

	extraConfig           map[string]string
	storageClassRequired  bool
	useInventoryForImages bool

	mutex              sync.Mutex
	cpuMinMHzInCluster uint64 // CPU Min Frequency across all Hosts in the cluster
}

func NewSessionAndConfigure(
	ctx goctx.Context,
	client *client.Client,
	config *config.VSphereVMProviderConfig,
	k8sClient ctrlruntime.Client) (*Session, error) {

	if log.V(4).Enabled() {
		configCopy := *config
		configCopy.VcCreds = nil
		log.V(4).Info("Creating new Session", "config", &configCopy)
	}

	s := &Session{
		Client:                client,
		k8sClient:             k8sClient,
		storageClassRequired:  config.StorageClassRequired,
		useInventoryForImages: config.UseInventoryAsContentSource,
	}

	if err := s.initSession(ctx, config); err != nil {
		return nil, err
	}

	log.V(4).Info("New session created and configured", "session", s.String())
	return s, nil
}

func (s *Session) Cluster() *object.ClusterComputeResource {
	return s.cluster
}

func (s *Session) initSession(
	ctx goctx.Context,
	cfg *config.VSphereVMProviderConfig) error {

	s.Finder = find.NewFinder(s.Client.VimClient(), false)

	dc, err := s.Finder.ObjectReference(ctx, types.ManagedObjectReference{Type: "Datacenter", Value: cfg.Datacenter})
	if err != nil {
		return errors.Wrapf(err, "failed to init Datacenter %q", cfg.Datacenter)
	}
	s.datacenter = dc.(*object.Datacenter)
	s.Finder.SetDatacenter(s.datacenter)

	resourcePool, err := s.GetResourcePoolByMoID(ctx, cfg.ResourcePool)
	if err != nil {
		return errors.Wrapf(err, "failed to init Resource Pool %q", cfg.ResourcePool)
	}

	rpOwner, err := resourcePool.Owner(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get Resource Pool owner")
	}

	cluster, ok := rpOwner.(*object.ClusterComputeResource)
	if !ok {
		return fmt.Errorf("owner of the ResourcePool is not a cluster but %T", rpOwner)
	}
	s.cluster = cluster

	minFreq, err := vcenter.ClusterMinCPUFreq(ctx, s.cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to init minimum CPU frequency")
	}
	s.SetCPUMinMHzInCluster(minFreq)

	if s.storageClassRequired {
		if cfg.Datastore != "" {
			log.V(4).Info("Ignoring configured datastore since storage class is required")
		}
	} else {
		if cfg.Datastore != "" {
			s.datastore, err = s.Finder.Datastore(ctx, cfg.Datastore)
			if err != nil {
				return errors.Wrapf(err, "failed to init Datastore %q", cfg.Datastore)
			}
		}
	}

	// Apply default extra config values. Allow for the option to specify extraConfig to be applied to all VMs
	s.extraConfig = DefaultExtraConfig
	if jsonExtraConfig := os.Getenv("JSON_EXTRA_CONFIG"); jsonExtraConfig != "" {
		extraConfig := make(map[string]string)
		if err := json.Unmarshal([]byte(jsonExtraConfig), &extraConfig); err != nil {
			return errors.Wrap(err, "Unable to parse value of 'JSON_EXTRA_CONFIG' environment variable")
		}
		log.V(4).Info("Using Json extraConfig", "extraConfig", extraConfig)
		// Over-write the default extra config values
		for k, v := range extraConfig {
			s.extraConfig[k] = v
		}
	}

	s.networkProvider = network.NewProvider(s.k8sClient, s.Client.VimClient(), s.Finder, s.cluster)

	return nil
}

func (s *Session) lookupVMByName(ctx goctx.Context, name string) (*res.VirtualMachine, error) {
	vm, err := s.Finder.VirtualMachine(ctx, name)
	if err != nil {
		return nil, err
	}
	return res.NewVMFromObject(vm), nil
}

func (s *Session) invokeFsrVirtualMachine(vmCtx context.VirtualMachineContext, resVM *res.VirtualMachine) error {
	vmCtx.Logger.Info("Invoking FSR on VM")

	task, err := internal.VirtualMachineFSR(vmCtx, resVM.MoRef(), s.Client.VimClient())
	if err != nil {
		vmCtx.Logger.Error(err, "InvokeFSR call failed")
		return err
	}

	if err = task.Wait(vmCtx); err != nil {
		vmCtx.Logger.Error(err, "InvokeFSR task failed")
		return err
	}

	return nil
}

// GetResourcePoolByMoID returns resource pool for a given a moref.
func (s *Session) GetResourcePoolByMoID(ctx goctx.Context, moID string) (*object.ResourcePool, error) {
	return vcenter.GetResourcePoolByMoID(ctx, s.Finder, moID)
}

func (s *Session) GetCPUMinMHzInCluster() uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.cpuMinMHzInCluster
}

func (s *Session) SetCPUMinMHzInCluster(minFreq uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.cpuMinMHzInCluster != minFreq {
		prevFreq := s.cpuMinMHzInCluster
		s.cpuMinMHzInCluster = minFreq
		log.V(4).Info("Successfully set CPU min frequency", "prevFreq", prevFreq, "newFreq", minFreq)
	}
}

func (s *Session) String() string {
	var sb strings.Builder
	sb.WriteString("{")
	if s.datacenter != nil {
		sb.WriteString(fmt.Sprintf("datacenter: %s, ", s.datacenter.Reference().Value))
	}
	if s.cluster != nil {
		sb.WriteString(fmt.Sprintf("cluster: %s, ", s.cluster.Reference().Value))
	}
	if s.datastore != nil {
		sb.WriteString(fmt.Sprintf("datastore: %s, ", s.datastore.Reference().Value))
	}
	sb.WriteString(fmt.Sprintf("cpuMinMHzInCluster: %v, ", s.GetCPUMinMHzInCluster()))
	sb.WriteString("}")
	return sb.String()
}

// RenameSessionCluster renames the cluster corresponding to this session. Used only in integration tests for now.
func (s *Session) RenameSessionCluster(ctx goctx.Context, name string) error {
	task, err := s.cluster.Rename(ctx, name)
	if err != nil {
		log.Error(err, "Failed to invoke rename for cluster", "clusterMoID", s.cluster.Reference().Value)
		return err
	}

	if taskResult, err := task.WaitForResult(ctx, nil); err != nil {
		msg := ""
		if taskResult != nil && taskResult.Error != nil {
			msg = taskResult.Error.LocalizedMessage
		}
		log.Error(err, "Error in renaming cluster", "clusterMoID", s.cluster.Reference().Value, "msg", msg)
		return err
	}

	return nil
}
