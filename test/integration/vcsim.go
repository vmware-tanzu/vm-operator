/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package integration

import (
	"context"
	"crypto/tls"
	"strconv"

	"k8s.io/klog"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/simulator/vpx"
	vapi "github.com/vmware/govmomi/vapi/simulator"
)

type VcSimInstance struct {
	vcsim  *simulator.Model
	server *simulator.Server
}

func NewVcSimInstance() *VcSimInstance {
	vpx := simulator.VPX()
	err := vpx.Create()
	if err != nil {
		vpx.Remove()
		klog.Fatalf("Fail to create vc simulator: %#v", err)
	}
	return &VcSimInstance{vcsim: vpx}
}

func (v *VcSimInstance) Start() (vcAddress string, vcPort int) {
	v.vcsim.Service.TLS = new(tls.Config)
	v.server = v.vcsim.Service.NewServer()
	ip := v.server.URL.Hostname()
	port, err := strconv.Atoi(v.server.URL.Port())
	if err != nil {
		v.server.Close()
		klog.Fatalf("Fail to find vc simulator port: %#v", err)
	}
	//register for vapi/rest calls
	path, handler := vapi.New(v.server.URL, vpx.Setting)
	v.vcsim.Service.Handle(path, handler)

	return ip, port
}

func (v *VcSimInstance) Stop() {
	if v.server != nil {
		v.server.Close()
	}
	if v.vcsim != nil {
		v.vcsim.Remove()
	}
}

func (v *VcSimInstance) NewClient(ctx context.Context) (*govmomi.Client, error) {
	return govmomi.NewClient(ctx, v.server.URL, true)
}
