// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	goctx "context"
	"sync"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	vcconfig "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
)

type Manager struct {
	sync.Mutex

	client    *vcclient.Client
	k8sClient ctrlruntime.Client
	sessions  map[string]*Session
}

func NewManager(k8sClient ctrlruntime.Client) Manager {
	return Manager{
		k8sClient: k8sClient,
		sessions:  map[string]*Session{},
	}
}

func (sm *Manager) KubeClient() ctrlruntime.Client {
	sm.Lock()
	defer sm.Unlock()
	return sm.k8sClient
}

func (sm *Manager) ClearSessionsAndClient(ctx goctx.Context) {
	sm.Lock()
	defer sm.Unlock()
	sm.clearSessionsAndClient(ctx)
}

func (sm *Manager) DeleteSession(
	ctx goctx.Context,
	namespace string) error {

	// Get all of the availability zones in order to delete the cached session
	// for each zone.
	availabilityZones, err := topology.GetAvailabilityZones(ctx, sm.k8sClient)
	if err != nil {
		return err
	}

	sm.Lock()
	defer sm.Unlock()

	for _, az := range availabilityZones {
		delete(sm.sessions, getSessionKey(az.Name, namespace))
	}

	return nil
}

func (sm *Manager) GetClient(ctx goctx.Context) (*vcclient.Client, error) {
	config, err := vcconfig.GetProviderConfig(ctx, sm.k8sClient)
	if err != nil {
		return nil, err
	}

	sm.Lock()
	defer sm.Unlock()

	return sm.getClient(ctx, config)
}

func (sm *Manager) WithClient(
	ctx goctx.Context,
	fn func(goctx.Context, *vcclient.Client) error) error {

	client, err := sm.GetClient(ctx)
	if err != nil {
		return err
	}
	return fn(ctx, client)
}

func (sm *Manager) GetSession(
	ctx goctx.Context,
	zone, namespace string) (*Session, error) {

	if !lib.IsWcpFaultDomainsFSSEnabled() {
		if zone == "" {
			zone = topology.DefaultAvailabilityZoneName
		}
	}

	sm.Lock()
	defer sm.Unlock()

	sessionKey := getSessionKey(zone, namespace)
	if session, ok := sm.sessions[sessionKey]; ok {
		return session, nil
	}

	newSession, err := sm.createSession(ctx, zone, namespace)
	if err != nil {
		return nil, err
	}
	sm.sessions[sessionKey] = newSession

	return newSession, nil
}

func (sm *Manager) GetSessionForVM(vmCtx context.VirtualMachineContext) (*Session, error) {
	return sm.GetSession(
		vmCtx,
		vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey],
		vmCtx.VM.Namespace)
}

func (sm *Manager) ComputeClusterCPUMinFrequency(ctx goctx.Context) error {
	// Get all the availability zones in order to calculate the minimum
	// CPU frequencies for each of the zones' vSphere clusters.
	availabilityZones, err := topology.GetAvailabilityZones(ctx, sm.k8sClient)
	if err != nil {
		return err
	}

	if !lib.IsWcpFaultDomainsFSSEnabled() {
		// Hack to fix up the default AZ to add the cluster MoID. Since in this setup,
		// all sessions share a single cluster, we can use any session.
		var clusterMoID string
		sm.Lock()
		for _, session := range sm.sessions {
			clusterMoID = session.Cluster().Reference().Value
			break
		}
		sm.Unlock()

		if clusterMoID == "" {
			return nil
		}

		// Only expect 1 AZ in this case.
		for i := range availabilityZones {
			availabilityZones[i].Spec.ClusterComputeResourceMoId = clusterMoID
		}
	}

	client, err := sm.GetClient(ctx)
	if err != nil {
		return err
	}

	var minFreq uint64
	for _, az := range availabilityZones {
		// Get the minimum frequency for this cluster.
		ccr := object.NewClusterComputeResource(
			client.VimClient(),
			types.ManagedObjectReference{
				Type:  "ClusterComputeResource",
				Value: az.Spec.ClusterComputeResourceMoId,
			},
		)
		freq, err := ComputeCPUInfo(ctx, ccr)
		if err != nil {
			return err
		}
		if minFreq == 0 || freq < minFreq {
			minFreq = freq
		}
	}

	if minFreq != 0 {
		// Iterate over each session, setting its minimum frequency to the global minimum.
		sm.Lock()
		for _, session := range sm.sessions {
			session.SetCPUMinMHzInCluster(minFreq)
		}
		sm.Unlock()
	}

	return nil
}

func (sm *Manager) UpdateVcPNID(ctx goctx.Context, vcPNID, vcPort string) error {
	updated, err := vcconfig.UpdateVcInConfigMap(ctx, sm.k8sClient, vcPNID, vcPort)
	if err != nil || !updated {
		return err
	}

	sm.Lock()
	defer sm.Unlock()

	sm.clearSessionsAndClient(ctx)

	return nil
}

func (sm *Manager) getClient(
	ctx goctx.Context,
	config *vcconfig.VSphereVMProviderConfig) (*vcclient.Client, error) {

	if sm.client != nil {
		return sm.client, nil
	}

	client, err := vcclient.NewClient(ctx, config)
	if err != nil {
		return nil, err
	}

	sm.client = client
	return sm.client, nil
}

func (sm *Manager) createSession(
	ctx goctx.Context,
	zone, namespace string) (*Session, error) {

	config, err := vcconfig.GetProviderConfigForNamespace(ctx, sm.k8sClient, zone, namespace)
	if err != nil {
		return nil, err
	}

	log.V(4).Info("Create session",
		"zone", zone,
		"namespace", namespace,
		"config", config)

	client, err := sm.getClient(ctx, config)
	if err != nil {
		return nil, err
	}

	ses, err := NewSessionAndConfigure(ctx, client, config, sm.k8sClient)
	if err != nil {
		return nil, err
	}

	return ses, nil
}

func (sm *Manager) clearSessionsAndClient(ctx goctx.Context) {
	for k := range sm.sessions {
		delete(sm.sessions, k)
	}

	if sm.client != nil {
		sm.client.Logout(ctx)
		sm.client = nil
	}
}

// Gets the session key for a given zone and namespace.
func getSessionKey(zone, namespace string) string {
	return zone + "/" + namespace
}
