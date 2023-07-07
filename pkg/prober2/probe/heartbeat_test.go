// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	goctx "context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
)

type fakeVMProviderProber struct {
	status vmopv1.GuestHeartbeatStatus
	err    error
}

func (tp fakeVMProviderProber) GetVirtualMachineGuestHeartbeat(_ goctx.Context, _ *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error) {
	return tp.status, tp.err
}

var _ = Describe("Guest heartbeat probe", func() {
	var (
		vm                   *vmopv1.VirtualMachine
		fakeProvider         fakeVMProviderProber
		testVMwareToolsProbe Probe

		err error
		res Result
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ClassName:      "dummy-vmclass",
				ReadinessProbe: getVirtualMachineReadinessHeartbeatProbe(),
			},
		}

		fakeProvider = fakeVMProviderProber{}
		testVMwareToolsProbe = NewGuestHeartbeatProber(&fakeProvider)
	})

	JustBeforeEach(func() {
		probeCtx := &context.ProbeContext{
			Logger: ctrl.Log.WithName("Probe").WithValues("name", vm.NamespacedName()),
			VM:     vm,
		}

		res, err = testVMwareToolsProbe.Probe(probeCtx)
	})

	Context("Provider returns an error", func() {
		BeforeEach(func() { fakeProvider.err = fmt.Errorf("fake error") })

		It("returns error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Errorf("fake error")))
			Expect(res).To(Equal(Unknown))
		})
	})

	Context("Provider does not return an error", func() {

		Context("Provider returns empty status", func() {
			BeforeEach(func() { fakeProvider.status = "" })

			It("returns unknown", func() {
				Expect(res).To(Equal(Unknown))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Errorf("no heartbeat value")))
			})
		})

		Context("Provider returns gray status", func() {
			BeforeEach(func() { fakeProvider.status = "gray" })

			It("returns failure", func() {
				Expect(res).To(Equal(Failure))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Errorf(`heartbeat status "gray" is below threshold`)))
			})
		})

		Context("Provider returns red status", func() {
			BeforeEach(func() { fakeProvider.status = "red" })

			It("returns failure", func() {
				Expect(res).To(Equal(Failure))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Errorf(`heartbeat status "red" is below threshold`)))
			})
		})

		Context("Provider returns yellow status", func() {
			BeforeEach(func() { fakeProvider.status = vmopv1.YellowHeartbeatStatus })

			Context("Threshold status is yellow", func() {
				BeforeEach(func() { vm.Spec.ReadinessProbe.GuestHeartbeat.ThresholdStatus = vmopv1.YellowHeartbeatStatus })

				It("returns success", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(res).To(Equal(Success))
				})
			})

			Context("Threshold status is green", func() {
				BeforeEach(func() { vm.Spec.ReadinessProbe.GuestHeartbeat.ThresholdStatus = vmopv1.GreenHeartbeatStatus })

				It("returns failure", func() {
					Expect(res).To(Equal(Failure))
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf(`heartbeat status "yellow" is below threshold`)))
				})
			})
		})

		Context("Provider returns green status", func() {
			BeforeEach(func() {
				fakeProvider.status = vmopv1.GreenHeartbeatStatus
				vm.Spec.ReadinessProbe.GuestHeartbeat.ThresholdStatus = vmopv1.GreenHeartbeatStatus
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(Success))
			})
		})
	})
})

func getVirtualMachineReadinessHeartbeatProbe() vmopv1.VirtualMachineReadinessProbeSpec {
	return vmopv1.VirtualMachineReadinessProbeSpec{
		GuestHeartbeat: &vmopv1.GuestHeartbeatAction{
			ThresholdStatus: vmopv1.GreenHeartbeatStatus, // Default.
		},
		PeriodSeconds: 1,
	}
}
