// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	goctx "context"
	"sync"

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
	sm.Lock()
	defer sm.Unlock()

	if sm.client != nil {
		return sm.client, nil
	}

	config, err := vcconfig.GetProviderConfig(ctx, sm.k8sClient)
	if err != nil {
		return nil, err
	}

	return sm.getClient(ctx, config)
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
		// NOTE: We're assuming here that the config is the same.
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
