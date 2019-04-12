/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package integration

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	vmoperator "github.com/vmware-tanzu/vm-operator"
)

var (
	vcsimIp = "127.0.0.1"
)

func makeAddress(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

type VcSimInstance struct {
	cmd *exec.Cmd
}

func NewVcSimInstance() *VcSimInstance {
	return &VcSimInstance{}
}

func (v *VcSimInstance) getPort() int {
	l, _ := net.Listen("tcp", ":0")
	defer l.Close()
	pieces := strings.Split(l.Addr().String(), ":")
	i, err := strconv.Atoi(pieces[len(pieces)-1])
	if err != nil {
		panic(err)
	}
	return i
}

// Wait for a vcsim instance to start by polling its "about" REST endpoint until
// we see a successful reply.  Sleep for a second between attempts
func (v *VcSimInstance) waitForStart(address string) error {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	var err error
	for i := 0; i < 20; i++ {
		_, err = client.Get(fmt.Sprintf("https://%s/about", address))
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return err
}

func (v *VcSimInstance) Start() (vcAddress string, vcPort int) {
	glog.V(4).Infof("Basepath is %s", vmoperator.Rootpath)
	vcsimPort := v.getPort()

	var address = makeAddress(vcsimPort)
	glog.Infof("Starting vcsim on address %s", address)

	path := fmt.Sprintf("%s/hack/run-vcsim.sh", vmoperator.Rootpath)
	v.cmd = exec.Command(path, address)
	v.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := v.cmd.Start(); err != nil {
		glog.Fatalf("Failed to start vcsim: %s", err)
	}

	glog.Infof("vcsim started as pid %d", v.cmd.Process.Pid)

	err := v.waitForStart(address)
	if err != nil {
		glog.Fatalf("Failed to wait until vcsim was ready")
	}

	glog.Infof("vcsim running as pid %d", v.cmd.Process.Pid)
	return vcsimIp, vcsimPort
}

func (v *VcSimInstance) Stop() {
	if v.cmd == nil {
		return
	}

	glog.Infof("Send SIGKILL to vcsim running as process group for pid %d", v.cmd.Process.Pid)
	if err := syscall.Kill(-v.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		glog.Fatalf("Failed to kill %d: %s", v.cmd.Process.Pid, err)
	}

	state, err := v.cmd.Process.Wait()
	if err != nil {
		glog.Fatalf("Failed to wait for process: state %s err %s", state, err)
	} else {
		glog.Infof("Waited for terminating process: %s", state)
	}
	time.Sleep(5 * time.Second)

}
