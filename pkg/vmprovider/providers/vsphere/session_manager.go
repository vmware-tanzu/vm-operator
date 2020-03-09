// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"

	vmopclientset "github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset"
)

type SessionManager struct {
	client     *Client
	clientset  *kubernetes.Clientset
	ncpclient  ncpclientset.Interface
	vmopclient vmopclientset.Interface

	mutex sync.Mutex
	// sessions contains the map of sessions for each namespace.
	sessions map[string]*Session
}

func NewSessionManager(clientset *kubernetes.Clientset, ncpclient ncpclientset.Interface, vmopclient vmopclientset.Interface) SessionManager {
	return SessionManager{
		clientset:  clientset,
		ncpclient:  ncpclient,
		vmopclient: vmopclient,
		sessions:   make(map[string]*Session),
	}
}

func (sm *SessionManager) getClient(context context.Context, config *VSphereVmProviderConfig) (*Client, error) {
	if sm.client != nil {
		return sm.client, nil
	}

	client, err := NewClient(context, config)
	if err != nil {
		return nil, err
	}

	sm.client = client

	return sm.client, nil
}

// NewSession is only used in testing
func (sm *SessionManager) NewSession(namespace string, config *VSphereVmProviderConfig) (*Session, error) {
	log.V(4).Info("New session", "namespace", namespace, "config", config)
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	client, err := sm.getClient(context.TODO(), config)
	if err != nil {
		return nil, err
	}

	ses, err := NewSessionAndConfigure(context.TODO(), client, config, sm.clientset, sm.ncpclient, sm.vmopclient)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create session for namespace %s", namespace)
	}

	sm.sessions[namespace] = ses
	return ses, nil

}

func (sm *SessionManager) createSession(ctx context.Context, namespace string) (*Session, error) {
	config, err := GetProviderConfigFromConfigMap(sm.clientset, namespace)
	if err != nil {
		return nil, err
	}

	log.V(4).Info("Create session", "namespace", namespace, "config", config)

	client, err := sm.getClient(ctx, config)
	if err != nil {
		return nil, err
	}

	ses, err := NewSessionAndConfigure(ctx, client, config, sm.clientset, sm.ncpclient, sm.vmopclient)
	if err != nil {
		return nil, err
	}

	return ses, nil
}

func (sm *SessionManager) GetSession(ctx context.Context, namespace string) (*Session, error) {
	sm.mutex.Lock()
	ses, ok := sm.sessions[namespace]
	sm.mutex.Unlock()

	if ok {
		return ses, nil
	}

	ses, err := sm.createSession(ctx, namespace)
	if err != nil {
		return nil, err
	}

	sm.mutex.Lock()
	sm.sessions[namespace] = ses
	sm.mutex.Unlock()

	return ses, nil
}

func (sm *SessionManager) ComputeClusterCpuMinFrequency(ctx context.Context) (err error) {
	minFreq := uint64(0)

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// All sessions connect to the same infra
	// Get a session, compute, and set freq in all the sessions
	for _, s := range sm.sessions {
		minFreq, err = s.computeCPUInfo(ctx)
		break
	}
	if err != nil {
		return err
	}

	for _, s := range sm.sessions {
		s.SetCpuMinMHzInCluster(minFreq)
	}

	return nil
}

func (sm *SessionManager) UpdateVcPNID(ctx context.Context, clusterConfigMap *corev1.ConfigMap) error {
	clusterCfg, err := BuildNewWcpClusterConfig(clusterConfigMap.Data)
	if err != nil {
		return err
	}

	config, err := GetProviderConfigFromConfigMap(sm.clientset, "")
	if err != nil {
		return err
	}

	if sm.isVcURLUnchanged(config, clusterCfg) {
		return nil
	}

	if err = PatchVcURLInConfigMap(sm.clientset, clusterCfg); err != nil {
		return err
	}

	sm.clearSessionsAndClient(ctx)

	return err
}

func (sm *SessionManager) clearSessionsAndClient(ctx context.Context) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for ns := range sm.sessions {
		delete(sm.sessions, ns)
	}

	if sm.client != nil {
		sm.client.Logout(ctx)
		sm.client = nil
	}

}

func (sm *SessionManager) isVcURLUnchanged(oldConfig *VSphereVmProviderConfig, newConfig *WcpClusterConfig) bool {
	return oldConfig.VcPNID == newConfig.VcPNID && oldConfig.VcPort == newConfig.VcPort
}
