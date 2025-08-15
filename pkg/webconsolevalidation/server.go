// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package webconsolevalidation

import (
	"context"
	"errors"
	"net/http"
	"time"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

const UUIDLabelKey = "vmoperator.vmware.com/webconsolerequest-uuid"

// Server represents a web console validation server.
type Server struct {
	Addr, Path string
	KubeClient ctrlclient.Client
}

// NewServer creates a new web console validation server.
func NewServer(addr, path string, client ctrlclient.Client) (*Server, error) {
	if addr == "" || path == "" {
		return nil, errors.New("server addr and path cannot be empty")
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
		UUIDLabelKey: uuid,
	}

	// TODO: Use an Informer to avoid hitting the API server for every request.
	vmwcrObjectList := &vmopv1.VirtualMachineWebConsoleRequestList{}
	if err := kubeClient.List(
		ctx,
		vmwcrObjectList,
		ctrlclient.InNamespace(namespace),
		labelSelector,
	); err != nil {
		return false, err
	}

	if len(vmwcrObjectList.Items) > 0 {
		return true, nil
	}

	// NOTE: In v1a1 this CRD has a different name - WebConsoleRequest - so this
	// is still required until we stop supporting v1a1.
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
