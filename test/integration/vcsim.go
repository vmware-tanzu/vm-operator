/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package integration

import (
	"context"
	"crypto/tls"
	"os"
	"strconv"

	"k8s.io/klog/klogr"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/simulator/vpx"
	vapi "github.com/vmware/govmomi/vapi/simulator"
)

type VcSimInstance struct {
	vcsim  *simulator.Model
	server *simulator.Server
	IP     string
	Port   int
}

var log = klogr.New()

func NewVcSimInstance() *VcSimInstance {
	vpx := simulator.VPX()
	err := vpx.Create()
	if err != nil {
		vpx.Remove()
		log.Error(err, "Fail to create vc simulator")
		os.Exit(255)
	}
	return &VcSimInstance{vcsim: vpx}
}

func (v *VcSimInstance) Start() (vcAddress string, vcPort int) {
	var err error
	v.vcsim.Service.TLS = new(tls.Config)
	v.server = v.vcsim.Service.NewServer()
	v.IP = v.server.URL.Hostname()
	v.Port, err = strconv.Atoi(v.server.URL.Port())
	if err != nil {
		v.server.Close()
		log.Error(err, "Fail to find vc simulator port")
		os.Exit(255)
	}
	//register for vapi/rest calls
	path, handler := vapi.New(v.server.URL, vpx.Setting)
	v.vcsim.Service.Handle(path, handler)
	return v.IP, v.Port
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
