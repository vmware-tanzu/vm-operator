// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"net/http"

	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

const (
	SourceVirtualMachineType = "VirtualMachine"

	// vAPICtxActIDHttpHeader represents the http header in vAPI to pass down the activation ID.
	vAPICtxActIDHttpHeader = "vapi-ctx-actid"
)

func CreateOVF(vmCtx context.VirtualMachineContext, client *rest.Client,
	vmPubReq *vmopv1alpha1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error) {
	createSpec := vcenter.CreateSpec{
		Name:        vmPubReq.Spec.Target.Item.Name,
		Description: vmPubReq.Spec.Target.Item.Description,
	}

	vm := vmCtx.VM
	source := vcenter.ResourceID{
		Type:  SourceVirtualMachineType,
		Value: vm.Status.UniqueID,
	}

	target := vcenter.LibraryTarget{
		LibraryID: cl.Spec.UUID,
	}

	ovf := vcenter.OVF{
		Spec:   createSpec,
		Source: source,
		Target: target,
	}

	vmCtx.Logger.Info("creating OVF from VM", "spec", ovf, "actid", actID)

	// Use vmpublish uid as the act id passed down to the content library service, so that we can track
	// the task status by the act id.
	ctxHeader := client.WithHeader(vmCtx, http.Header{vAPICtxActIDHttpHeader: []string{actID}})
	return vcenter.NewManager(client).CreateOVF(ctxHeader, ovf)
}
