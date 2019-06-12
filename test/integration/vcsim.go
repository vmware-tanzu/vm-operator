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
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"k8s.io/klog"

	vmoperator "github.com/vmware-tanzu/vm-operator"
)

const (
	vcSimIp = "127.0.0.1"
)

type VcSimInstance struct {
	cmd *exec.Cmd
}

func NewVcSimInstance() *VcSimInstance {
	return &VcSimInstance{}
}

func getPort() int {
	// nolint:gosec
	// Ignore G102: Binds to all network interfaces (gosec)
	l, _ := net.Listen("tcp", ":0")
	defer func() { _ = l.Close() }()
	pieces := strings.Split(l.Addr().String(), ":")
	i, err := strconv.Atoi(pieces[len(pieces)-1])
	if err != nil {
		panic(err)
	}
	return i
}

// Wait for a vcSim instance to start by polling its "about" REST endpoint until
// we see a successful reply.  Sleep for a second between attempts
func waitForStart(hostPort string) error {
	addr := fmt.Sprintf("https://%s/about", hostPort)
	client := &http.Client{
		Transport: &http.Transport{
			// nolint:gosec
			// Ignore the SSL check
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	for i := 0; i < 20; i++ {
		if _, err := client.Get(addr); err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timedout waiting for vcSim to start")
}

func (v *VcSimInstance) Start() (vcAddress string, vcPort int) {
	host, port := vcSimIp, getPort()
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	vcSimPath := path.Join(vmoperator.Rootpath, "hack/run-vcsim.sh")
	v.cmd = exec.Command(vcSimPath, address)
	v.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := v.cmd.Start(); err != nil {
		klog.Fatalf("Failed to start vcSim: %v", err)
	}

	if err := waitForStart(address); err != nil {
		klog.Fatalf("Failed to wait until vcSim was ready")
	}

	klog.Infof("vcSim running as pid %d on %s", v.cmd.Process.Pid, address)
	return vcSimIp, port
}

func (v *VcSimInstance) Stop() {
	if v.cmd == nil {
		return
	}

	klog.Infof("Send SIGKILL to vcSim running as process group for pid %d", v.cmd.Process.Pid)
	if err := syscall.Kill(-v.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		klog.Fatalf("Failed to kill %d: %v", v.cmd.Process.Pid, err)
	}

	state, err := v.cmd.Process.Wait()
	if err != nil {
		klog.Fatalf("Failed to wait for process state %s: %v", state, err)
	}

	// TODO(bryanv) Needed?
	time.Sleep(5 * time.Second)
}
