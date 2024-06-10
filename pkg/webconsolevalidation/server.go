// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package webconsolevalidation

import (
	"context"
	"errors"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
)

// Server represents a web console validation server.
type Server struct {
	Addr, Path string
	KubeClient ctrlclient.Client
}

// NewServer creates a new web console validation server.
func NewServer(
	addr, path string,
	inClusterConfigFunc func() (*rest.Config, error),
	addToSchemeFunc func(*runtime.Scheme) error,
	newClientFunc func(*rest.Config, ctrlclient.Options) (ctrlclient.Client, error)) (*Server, error) {
	if addr == "" || path == "" {
		return nil, errors.New("server addr and path cannot be empty")
	}

	// Init Kubernetes client.
	restConfig, err := inClusterConfigFunc()
	if err != nil {
		return nil, err
	}
	scheme := runtime.NewScheme()
	if err := addToSchemeFunc(scheme); err != nil {
		return nil, err
	}
	client, err := newClientFunc(restConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return &Server{
		Addr:       addr,
		Path:       path,
		KubeClient: client,
	}, nil
}

// Run starts the web console validation server.
func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.Path, s.HandleWebConsoleValidation)

	server := &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return server.ListenAndServe()
}

// HandleWebConsoleValidation verifies a web console validation request by
// checking if a WebConsoleRequest resource exists with the given UUID in query.
func (s *Server) HandleWebConsoleValidation(w http.ResponseWriter, r *http.Request) {
	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		http.Error(w, "'uuid' param is empty", http.StatusBadRequest)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		http.Error(w, "'namespace' param is empty", http.StatusBadRequest)
		return
	}

	logger := ctrllog.Log.WithName(r.URL.Path).WithValues("uuid", uuid).WithValues("namespace", namespace)

	found, err := isResourceFound(r.Context(), uuid, namespace, s.KubeClient)
	if err != nil {
		logger.Error(err, "Error occurred in finding a webconsolerequest resource with the given params.")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if found {
		logger.Info("Found a webconsolerequest resource with the given params. Returning 200.")
		w.WriteHeader(http.StatusOK)
	} else {
		logger.Info("Didn't find a webconsolerequest resource with the given params. Returning 403.")
		w.WriteHeader(http.StatusForbidden)
	}
}

func isResourceFound(
	ctx context.Context,
	uuid, namespace string,
	kubeClient ctrlclient.Client) (bool, error) {
	labelSelector := ctrlclient.MatchingLabels{
		v1alpha1.UUIDLabelKey: uuid,
	}

	// TODO: Use an Informer to avoid hitting the API server for every request.
	wcrObjectList := &vmopv1a1.WebConsoleRequestList{}
	if err := kubeClient.List(
		ctx,
		wcrObjectList,
		ctrlclient.InNamespace(namespace),
		labelSelector,
	); err != nil {
		return false, err
	}

	return len(wcrObjectList.Items) > 0, nil
}
