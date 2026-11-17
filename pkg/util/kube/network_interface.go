// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

const (
	// VMExternalNetworkInterfacesIndexKey identifies the VM external interface index.
	VMExternalNetworkInterfacesIndexKey = "spec.network.interfaces.externalRef"
)

// NetworkInterfaceReferenceKey identifies a namespaced interface independently of API version.
func NetworkInterfaceReferenceKey(apiVersion, kind, name string) string {
	if kind == "" || name == "" {
		return ""
	}

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil || gv.Group == "" {
		return ""
	}

	return gv.Group + "/" + kind + "/" + name
}

// VMExternalNetworkInterfacesIndexerFunc indexes external interface selections.
func VMExternalNetworkInterfacesIndexerFunc(obj ctrlclient.Object) []string {
	vm, ok := obj.(*vmopv1.VirtualMachine)
	if !ok || vm.Spec.Network == nil {
		return nil
	}

	var keys []string

	for _, iface := range vm.Spec.Network.Interfaces {
		if ref := iface.ExternalRef; ref != nil {
			key := NetworkInterfaceReferenceKey(ref.APIVersion, ref.Kind, ref.Name)
			if key != "" {
				keys = append(keys, key)
			}
		}
	}

	return keys
}

// IndexVMExternalNetworkInterfaces registers the external interface reference index.
func IndexVMExternalNetworkInterfaces(ctx context.Context, mgr manager.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&vmopv1.VirtualMachine{},
		VMExternalNetworkInterfacesIndexKey,
		VMExternalNetworkInterfacesIndexerFunc); err != nil {

		return fmt.Errorf("index VM external network interfaces: %w", err)
	}

	return nil
}

// NetworkInterfaceToVMMapper maps a network interface CR to the VMs that own
// it, reference it with an ExternalRef, or are labeled as associated with it.
func NetworkInterfaceToVMMapper(c ctrlclient.Client) func(context.Context, ctrlclient.Object) []reconcile.Request {
	return func(ctx context.Context, obj ctrlclient.Object) []reconcile.Request {
		names := sets.New[string]()

		for _, owner := range obj.GetOwnerReferences() {
			if owner.Kind == "VirtualMachine" && strings.HasPrefix(owner.APIVersion, vmopv1.GroupName+"/") {
				names.Insert(owner.Name)
			}
		}

		// External SubnetPorts are labeled with the associated VM so the VM
		// can release the association after the reference is removed.
		if name := obj.GetLabels()[pkgconst.NetworkInterfaceVMNameLabel]; name != "" {
			names.Insert(name)
		}

		var apiVersion, kind string
		switch obj.(type) {
		case *netopv1alpha1.NetworkInterface:
			apiVersion, kind = netopv1alpha1.SchemeGroupVersion.String(), "NetworkInterface"
		case *ncpv1alpha1.VirtualNetworkInterface:
			apiVersion, kind = ncpv1alpha1.SchemeGroupVersion.String(), "VirtualNetworkInterface"
		case *vpcv1alpha1.SubnetPort:
			apiVersion, kind = vpcv1alpha1.SchemeGroupVersion.String(), "SubnetPort"
		}

		ns := obj.GetNamespace()

		if kind != "" {
			var list vmopv1.VirtualMachineList
			if err := c.List(
				ctx,
				&list,
				ctrlclient.InNamespace(ns),
				ctrlclient.MatchingFields{
					VMExternalNetworkInterfacesIndexKey: NetworkInterfaceReferenceKey(apiVersion, kind, obj.GetName()),
				},
				ctrlclient.UnsafeDisableDeepCopy); err != nil {

				pkglog.FromContextOrDefault(ctx).Error(err,
					"Failed to list VMs referencing network interface",
					"interface", ctrlclient.ObjectKeyFromObject(obj))
			}
			for i := range list.Items {
				names.Insert(list.Items[i].Name)
			}
		}

		requests := make([]reconcile.Request, 0, names.Len())
		for _, name := range sets.List(names) {
			requests = append(requests, reconcile.Request{
				NamespacedName: ctrlclient.ObjectKey{Namespace: ns, Name: name},
			})
		}
		return requests
	}
}
