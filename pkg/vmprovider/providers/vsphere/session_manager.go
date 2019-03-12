/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"sync"
)

// Until we have support for per-namespace sessions.
const globalSessionKey = "***"

type SessionManager struct {
	mutex sync.Mutex
	// sessions contains the map of sessions for each namespace.
	sessions map[string]*Session
}

func NewSessionManager() SessionManager {
	return SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (sm *SessionManager) NewSession(config *VSphereVmProviderConfig) (*Session, error) {
	ses, err := NewSession(context.TODO(), config)
	if err != nil {
		return nil, err
	}

	sm.mutex.Lock()
	sm.sessions[globalSessionKey] = ses
	sm.mutex.Unlock()

	return ses, nil
}

func (sm *SessionManager) GetSession() (*Session, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	return sm.sessions[globalSessionKey], nil
}
