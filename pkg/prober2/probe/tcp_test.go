// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
)

var _ = Describe("TCP probe", func() {
	var (
		vm           *vmopv1.VirtualMachine
		testTCPProbe Probe

		testServer *httptest.Server
		testHost   string
		testPort   int
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ClassName: "dummy-vmclass",
			},
			Status: vmopv1.VirtualMachineStatus{
				Network: &vmopv1.VirtualMachineNetworkStatus{},
			},
		}

		testServer, testHost, testPort = setupTestServer()
		testTCPProbe = NewTCPProber()
	})

	AfterEach(func() {
		testServer.Close()
	})

	It("TCP probe succeeds, with TCP host set in VM spec ", func() {
		vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe(testHost, testPort)
		probeCtx := &context.ProbeContext{
			VM:     vm,
			Logger: ctrl.Log.WithName("Probe").WithValues("name", vm.NamespacedName()),
		}

		res, err := testTCPProbe.Probe(probeCtx)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).To(Equal(Success))
	})

	It("TCP probe succeeds, with empty TCP host", func() {
		vm.Status.Network.PrimaryIP4 = testHost
		vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe("", testPort)
		probeCtx := &context.ProbeContext{
			VM:     vm,
			Logger: ctrl.Log.WithName("Probe").WithValues("name", vm.NamespacedName()),
		}

		res, err := testTCPProbe.Probe(probeCtx)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).To(Equal(Success))
	})

	It("TCP probe fails", func() {
		vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe(testHost, 10001)
		probeCtx := &context.ProbeContext{
			VM: vm,
		}

		res, err := testTCPProbe.Probe(probeCtx)
		Expect(err).Should(HaveOccurred())
		Expect(res).To(Equal(Failure))
	})
})

func TestTCPProbe(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TCP probe")
}

func setupTestServer() (*httptest.Server, string, int) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	host, port, err := net.SplitHostPort(s.Listener.Addr().String())
	Expect(err).NotTo(HaveOccurred())
	portInt, err := strconv.Atoi(port)
	Expect(err).NotTo(HaveOccurred())
	return s, host, portInt
}

func getVirtualMachineReadinessTCPProbe(host string, port int) vmopv1.VirtualMachineReadinessProbeSpec {
	return vmopv1.VirtualMachineReadinessProbeSpec{
		TCPSocket: &vmopv1.TCPSocketAction{
			Host: host,
			Port: intstr.FromInt(port),
		},
		PeriodSeconds: 1,
	}
}
