/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"sync"

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
