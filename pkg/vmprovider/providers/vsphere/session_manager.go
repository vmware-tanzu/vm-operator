/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"sync"

	"github.com/vmware/govmomi/vapi/library"

	"github.com/pkg/errors"

	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
)

type SessionManager struct {
	clientset *kubernetes.Clientset
	ncpclient ncpclientset.Interface

	mutex sync.Mutex
	// sessions contains the map of sessions for each namespace.
	sessions map[string]*Session
}

func NewSessionManager(clientset *kubernetes.Clientset, ncpclient ncpclientset.Interface) SessionManager {
	return SessionManager{
		clientset: clientset,
		ncpclient: ncpclient,
		sessions:  make(map[string]*Session),
	}
}

func (sm *SessionManager) NewSession(namespace string, config *VSphereVmProviderConfig) (*Session, error) {
	ses, err := NewSessionAndConfigure(context.TODO(), config, sm.clientset, sm.ncpclient)
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

	ses, err := NewSessionAndConfigure(ctx, config, sm.clientset, sm.ncpclient)
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

		config, err := GetProviderConfigFromConfigMap(sm.clientset, namespace)
		if err != nil {
			return nil, err
		}

		if sm.isContentSourceUnchanged(ses.contentlib, config.ContentSource) {
			return ses, nil
		}
		//If Content Source has changed, logout and empty existing session.
		ses.Logout(ctx)

		sm.mutex.Lock()
		delete(sm.sessions, namespace)
		sm.mutex.Unlock()
	}

	ses, err := sm.createSession(ctx, namespace)
	if err != nil {
		return nil, err
	}

	sm.mutex.Lock()
	ses2, ok := sm.sessions[namespace]

	if ok {
		//Install new session and cleanup the racing session (ses2)
		sm.sessions[namespace] = ses
		sm.mutex.Unlock()

		ses2.Logout(ctx)
		return ses, nil
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

func (sm *SessionManager) isContentSourceUnchanged(cl *library.Library, contentSource string) bool {

	if (cl != nil) && (cl.ID == contentSource) {
		return true
	}

	if (cl == nil) && (contentSource == "") {
		return true
	}

	return false
}
