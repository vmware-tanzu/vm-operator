// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	vmopclientset "github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset"
	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
)

type SessionManager struct {
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

func (sm *SessionManager) NewSession(namespace string, config *VSphereVmProviderConfig) (*Session, error) {
	log.V(4).Info("New session", "namespace", namespace, "config", config)
	ses, err := NewSessionAndConfigure(context.TODO(), config, sm.clientset, sm.ncpclient, sm.vmopclient)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create session for namespace %s", namespace)
	}

	sm.mutex.Lock()
	sm.sessions[namespace] = ses
	sm.mutex.Unlock()

	return ses, nil
}

func (sm *SessionManager) createSession(ctx context.Context, namespace string) (*Session, error) {
	config, err := GetProviderConfigFromConfigMap(sm.clientset, namespace)
	if err != nil {
		return nil, err
	}

	log.V(4).Info("Create session", "namespace", namespace, "config", config)

	ses, err := NewSessionAndConfigure(ctx, config, sm.clientset, sm.ncpclient, sm.vmopclient)
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
	ses2, ok := sm.sessions[namespace]
	if ok {
		sm.mutex.Unlock()
		ses.Logout(ctx)
		return ses2, nil
	}

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

	if sm.isPnidUnchanged(config.VcPNID, clusterCfg.VCHost) {
		return nil
	}

	if err = PatchPnidInConfigMap(sm.clientset, clusterCfg.VCHost); err != nil {
		return err
	}

	sm.clearClientAndSessions(ctx)

	return nil
}

func (sm *SessionManager) clearClientAndSessions(ctx context.Context) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for ns, s := range sm.sessions {
		s.Logout(ctx)
		delete(sm.sessions, ns)
	}
}

func (sm *SessionManager) isPnidUnchanged(oldPnid, newPnid string) bool {
	return oldPnid == newPnid
}
