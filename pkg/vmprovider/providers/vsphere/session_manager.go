/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"sync"

	"k8s.io/client-go/kubernetes"
)

type SessionManager struct {
	clientset *kubernetes.Clientset

	mutex sync.Mutex
	// sessions contains the map of sessions for each namespace.
	sessions map[string]*Session
}

func NewSessionManager(clientset *kubernetes.Clientset) SessionManager {
	return SessionManager{
		clientset: clientset,
		sessions:  make(map[string]*Session),
	}
}

func (sm *SessionManager) NewSession(namespace string, config *VSphereVmProviderConfig) (*Session, error) {
	ses, err := NewSession(context.TODO(), config)
	if err != nil {
		return nil, err
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

	ses, err := NewSession(ctx, config)
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
