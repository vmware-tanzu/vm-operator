// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

//nolint:revive,stylecheck
package internal

import (
	"context"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

type InvokeFSR_TaskRequest struct {
	This vimtypes.ManagedObjectReference `xml:"_this"`
}

type InvokeFSR_TaskResponse struct {
	Returnval vimtypes.ManagedObjectReference `xml:"returnval"`
}

type InvokeFSR_TaskBody struct {
	Req    *InvokeFSR_TaskRequest  `xml:"urn:vim25 InvokeFSR_Task,omitempty"`
	Res    *InvokeFSR_TaskResponse `xml:"InvokeFSR_TaskResponse,omitempty"`
	Fault_ *soap.Fault             `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *InvokeFSR_TaskBody) Fault() *soap.Fault {
	return b.Fault_
}

func InvokeFSR_Task(ctx context.Context, r soap.RoundTripper, req *InvokeFSR_TaskRequest) (*InvokeFSR_TaskResponse, error) {
	var reqBody, resBody InvokeFSR_TaskBody
	reqBody.Req = req
	if err := r.RoundTrip(ctx, &reqBody, &resBody); err != nil {
		return nil, err
	}
	return resBody.Res, nil
}

func VirtualMachineFSR(ctx context.Context, vm vimtypes.ManagedObjectReference, client *vim25.Client) (*object.Task, error) {
	req := InvokeFSR_TaskRequest{
		This: vm,
	}
	res, err := InvokeFSR_Task(ctx, client, &req)
	if err != nil {
		return nil, err
	}
	return object.NewTask(client, res.Returnval), nil
}

type CustomizationCloudinitPrep struct {
	vimtypes.CustomizationIdentitySettings

	Metadata string `xml:"metadata"`
	Userdata string `xml:"userdata,omitempty"`
}
