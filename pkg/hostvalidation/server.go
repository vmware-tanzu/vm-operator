// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package hostvalidation

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	vcconfig "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
)

// K8sClient is used to create the Vim client.
// Making it public to allow mocking in test.
var K8sClient ctrlruntime.Client

// RunServer runs the host validation server at the given addr and path.
func RunServer(addr, path string) error {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	ctrlruntimeClient, err := ctrlruntime.New(restConfig, ctrlruntime.Options{})
	if err != nil {
		return err
	}

	K8sClient = ctrlruntimeClient

	mux := http.NewServeMux()
	mux.HandleFunc(path, HandleHostValidation)

	return http.ListenAndServe(addr, mux)
}

// HandleHostValidation handles the host validation requests.
func HandleHostValidation(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		http.Error(w, "'host' param is empty", http.StatusBadRequest)
		return
	}

	vmMoID := r.URL.Query().Get("vm_moid")
	if vmMoID == "" {
		http.Error(w, "'vm_moid' param is empty", http.StatusBadRequest)
		return
	}

	// Set up a new logger with the received params as the context.
	logger := ctrllog.Log.WithName("hostvalidation").WithValues("host", host, "vm_moid", vmMoID)

	valid, err := isValid(r.Context(), host, vmMoID, logger)
	if err != nil {
		logger.Error(err, "Error occurred in validating the host and vm.")
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if valid {
		logger.Info("The requested host is a valid ESXi/VC host for its vm.")
		w.WriteHeader(http.StatusOK)
	} else {
		logger.Info("The requested host is NOT a valid ESXi/VC host for its vm.")
		w.WriteHeader(http.StatusForbidden)
	}
}

// isValid returns true if the given host is a valid ESXi/VC host of the given VM.
func isValid(ctx context.Context, host, vmMoID string, logger logr.Logger) (bool, error) {
	// Get a new Vim client to retrieve the VM and its host info.
	vimClient, err := getNewVimClient(ctx)
	if err != nil {
		logger.Error(err, "Error occurred in getting a new vSphere Vim client.")
		return false, err
	}

	// Using context.Background() incase the original context is canceled.
	defer logoutVimClient(context.Background(), vimClient, logger)

	// Get the VirtualMachine object from the given MoID.
	objVM := object.NewVirtualMachine(vimClient, types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: vmMoID,
	})

	// Get the HostSystem object from the above VM for verifying against its ESXi host.
	// Return early if it matches the requested host.
	objHostSystem, err := objVM.HostSystem(ctx)
	if err != nil {
		return false, err
	}

	esxiHostname, err := objHostSystem.ObjectName(ctx)
	if err != nil {
		logger.Error(err, "Failed to get the VM's ESXi hostname.")
	} else if esxiHostname == host {
		return true, nil
	}

	logger.Info("The VM's ESXi hostname does not match the requested host. Continuing to check its IPs.")

	esxiHostIPs, err := objHostSystem.ManagementIPs(ctx)
	if err != nil {
		logger.Error(err, "Failed to get the VM's ESXi IPs. Continuing to check against the VC host.")
	} else {
		for _, ip := range esxiHostIPs {
			if ip.String() == host {
				// The VM's ESXi IP matches the requested host.
				return true, nil
			}
		}

		logger.Info("None of the VM's ESXi IPs matches the requested host. Continuing to check against the VC host.")
	}

	// Verify against the VC IP.
	var moHost mo.HostSystem
	vcIPKey := []string{"summary.managementServerIp"}
	if err := objHostSystem.Properties(ctx, objHostSystem.Reference(), vcIPKey, &moHost); err != nil {
		return false, err
	}

	return host == moHost.Summary.ManagementServerIp, nil
}

// getNewVimClient returns a new Vim client.
func getNewVimClient(ctx context.Context) (*vim25.Client, error) {
	if K8sClient == nil {
		return nil, errors.New("k8s controller-runtime client is not set")
	}

	config, err := vcconfig.GetProviderConfig(ctx, K8sClient)
	if err != nil {
		return nil, err
	}

	vimClient, _, err := vcclient.NewVimClient(ctx, config)

	return vimClient, err
}

// logoutVimClient logs out the given Vim client.
func logoutVimClient(ctx context.Context, client *vim25.Client, logger logr.Logger) {
	if client == nil || client.RoundTripper == nil {
		return
	}

	logoutRequest := types.Logout{
		This: *client.ServiceContent.SessionManager,
	}
	if _, err := methods.Logout(ctx, client.RoundTripper, &logoutRequest); err != nil {
		logger.Error(err, "Error occurred in logging out the vSphere Vim client.")
	}
}
