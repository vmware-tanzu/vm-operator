// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"
	"net"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
)

// tcpProber implements the Probe interface.
type tcpProber struct{}

// NewTCPProber creates a new tcp prober which implements the Probe interface to execute tcp probes.
func NewTCPProber() Probe {
	return &tcpProber{}
}

func (pr tcpProber) Probe(ctx *context.ProbeContext) (Result, error) {
	vm := ctx.VM
	p := ctx.ProbeSpec

	portProto := corev1.ProtocolTCP
	portNum, err := findPort(vm, p.TCPSocket.Port, portProto)
	if err != nil {
		return Failure, err
	}

	var host string
	if p.TCPSocket.Host != "" {
		host = p.TCPSocket.Host
	} else {
		ctx.Logger.V(4).Info("TCPSocket Host not specified, using VM IP", "probe", ctx.String())
		if host = vm.Status.VmIp; host == "" {
			return Failure, fmt.Errorf("VM %s doesn't have an IP assigned", vm.NamespacedName())
		}
	}

	var timeout time.Duration
	if p.TimeoutSeconds <= 0 {
		timeout = defaultConnectTimeout
	} else {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	}

	if err := checkConnection("tcp", host, strconv.Itoa(portNum), timeout); err != nil {
		return Failure, err
	}

	return Success, nil
}

func findPort(vm *vmoperatorv1alpha1.VirtualMachine, portName intstr.IntOrString, portProto corev1.Protocol) (int, error) {
	switch portName.Type {
	case intstr.String:
		name := portName.StrVal
		for _, port := range vm.Spec.Ports {
			if port.Name == name && port.Protocol == portProto {
				return port.Port, nil
			}
		}
	case intstr.Int:
		return portName.IntValue(), nil
	}

	return 0, fmt.Errorf("no suitable port for manifest: %s", vm.UID)
}

func checkConnection(proto, host, port string, timeout time.Duration) error {
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout(proto, address, timeout)
	if err != nil {
		return err
	}

	return conn.Close()
}
