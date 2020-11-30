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

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
)

var _ = Describe("TCP probe", func() {
	var (
		vm           *vmopv1alpha1.VirtualMachine
		testTcpProbe Probe

		testServer *httptest.Server
		testHost   string
		testPort   int
	)

	BeforeEach(func() {
		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ClassName: "dummy-vmclass",
			},
		}

		testServer, testHost, testPort = setupTestServer()
		testTcpProbe = NewTcpProber()
	})

	AfterEach(func() {
		testServer.Close()
	})

	It("TCP probe succeeds, with TCP host set in VM spec ", func() {
		vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe(testHost, testPort)
		probeCtx := &context.ProbeContext{
			VM:        vm,
			ProbeSpec: vm.Spec.ReadinessProbe,
			Logger:    ctrl.Log.WithName("Probe").WithValues("name", vm.NamespacedName()),
		}

		res, err := testTcpProbe.Probe(probeCtx)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).To(Equal(Success))
	})

	It("TCP probe succeeds, with empty TCP host", func() {
		vm.Status.VmIp = testHost
		vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe("", testPort)
		probeCtx := &context.ProbeContext{
			VM:        vm,
			ProbeSpec: vm.Spec.ReadinessProbe,
			Logger:    ctrl.Log.WithName("Probe").WithValues("name", vm.NamespacedName()),
		}

		res, err := testTcpProbe.Probe(probeCtx)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).To(Equal(Success))
	})

	It("TCP probe fails", func() {
		vm.Spec.ReadinessProbe = getVirtualMachineReadinessTCPProbe(testHost, 10001)
		probeCtx := &context.ProbeContext{
			VM:        vm,
			ProbeSpec: vm.Spec.ReadinessProbe,
		}

		res, err := testTcpProbe.Probe(probeCtx)
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

func getVirtualMachineReadinessTCPProbe(host string, port int) *vmopv1alpha1.Probe {
	return &vmopv1alpha1.Probe{
		TCPSocket: &vmopv1alpha1.TCPSocketAction{
			Host: host,
			Port: intstr.FromInt(port),
		},
		PeriodSeconds: 1,
	}
}
