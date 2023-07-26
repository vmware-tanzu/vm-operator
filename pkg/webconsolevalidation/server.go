// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package webconsolevalidation

import (
	"context"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
)

// K8sClient is used to get the webconsolerequest resource from UUID and namespace.
var K8sClient ctrlruntime.Client

// InitServer initializes a K8sClient used by the web-console validation server.
func InitServer() error {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	scheme := runtime.NewScheme()
	if err = vmopv1.AddToScheme(scheme); err != nil {
		return err
	}

	ctrlruntimeClient, err := ctrlruntime.New(restConfig, ctrlruntime.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	K8sClient = ctrlruntimeClient
	return nil
}

// RunServer runs the web-console validation server at the given addr and path.
func RunServer(addr, path string) error {
	mux := http.NewServeMux()
	mux.HandleFunc(path, HandleWebConsoleValidation)

	return http.ListenAndServe(addr, mux)
}

// HandleWebConsoleValidation handles the web-console validation server requests.
func HandleWebConsoleValidation(w http.ResponseWriter, r *http.Request) {
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

	found, err := isResourceFound(r.Context(), uuid, namespace)
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

func isResourceFound(goCtx context.Context, uuid, namespace string) (bool, error) {
	labelSelector := ctrlruntime.MatchingLabels{
		v1alpha1.UUIDLabelKey: uuid,
	}

	// TODO: Use an Informer to avoid hitting the API server for every request.
	wcrObjectList := &vmopv1.WebConsoleRequestList{}
	if err := K8sClient.List(goCtx, wcrObjectList, ctrlruntime.InNamespace(namespace), labelSelector); err != nil {
		return false, err
	}

	return len(wcrObjectList.Items) > 0, nil
}
