// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	goctx "context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
)

type fakeGuestInfoProvider struct {
	guestInfo map[string]string
	err       error
}

func (f fakeGuestInfoProvider) GetVirtualMachineGuestInfo(ctx goctx.Context, vm *vmopv1.VirtualMachine) (map[string]string, error) {
	return f.guestInfo, f.err
}

var _ = Describe("Guest info probe", func() {
	var (
		fakeProvider    fakeGuestInfoProvider
		prober          Probe
		guestInfoAction []vmopv1.GuestInfoAction

		err error
		res Result
	)

	BeforeEach(func() {
		fakeProvider = fakeGuestInfoProvider{}
		prober = NewGuestInfoProber(&fakeProvider)
		guestInfoAction = nil
	})

	JustBeforeEach(func() {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ReadinessProbe: getVirtualMachineReadinessGuestInfoProbe(guestInfoAction),
			},
		}

		probeCtx := &context.ProbeContext{
			Logger: ctrl.Log.WithName("Probe").WithValues("name", vm.NamespacedName()),
			VM:     vm,
		}

		res, err = prober.Probe(probeCtx)
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
		const prefix = "guestinfo."

		Context("Key does not exist in GuestInfo", func() {
			BeforeEach(func() {
				fakeProvider.guestInfo = map[string]string{
					prefix + "key1": "ignored-because-no-value-in-action",
				}

				guestInfoAction = []vmopv1.GuestInfoAction{
					{
						Key: "missing-key",
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(Failure))
			})
		})

		Context("Matches key when value is not set", func() {
			BeforeEach(func() {
				fakeProvider.guestInfo = map[string]string{
					prefix + "key1": "ignored-because-no-value-in-action",
				}

				guestInfoAction = []vmopv1.GuestInfoAction{
					{
						Key: "key1",
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(Success))
			})
		})

		Context("Matches key and has plain string value", func() {
			BeforeEach(func() {
				fakeProvider.guestInfo = map[string]string{prefix + "key2": "vmware"}
			})

			Context("Values match", func() {
				BeforeEach(func() {
					guestInfoAction = []vmopv1.GuestInfoAction{
						{
							Key:   "key2",
							Value: "vmware",
						},
					}
				})

				It("returns success", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(res).To(Equal(Success))
				})
			})

			Context("Values don't match", func() {
				BeforeEach(func() {
					guestInfoAction = []vmopv1.GuestInfoAction{
						{
							Key:   "key2",
							Value: "broadcom",
						},
					}
				})

				It("returns failure", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(res).To(Equal(Failure))
				})
			})
		})

		Context("Matches key and has regex value", func() {
			BeforeEach(func() {
				fakeProvider.guestInfo = map[string]string{
					prefix + "key3": "aaaaa",
				}

			})

			Context("regex matches", func() {
				BeforeEach(func() {
					guestInfoAction = []vmopv1.GuestInfoAction{
						{
							Key:   "key3",
							Value: "^a{5}$",
						},
					}
				})

				It("returns success", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(res).To(Equal(Success))
				})
			})

			Context("regex does not match", func() {
				BeforeEach(func() {
					guestInfoAction = []vmopv1.GuestInfoAction{
						{
							Key:   "key3",
							Value: "^a{42}$",
						},
					}
				})

				It("returns failure", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(res).To(Equal(Failure))
				})
			})

			Context("Invalid regex", func() {
				BeforeEach(func() {
					guestInfoAction = []vmopv1.GuestInfoAction{
						{
							Key:   "key3",
							Value: "a(",
						},
					}
				})

				It("returns success by treating it as a wildcard", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(res).To(Equal(Success))
				})
			})
		})
	})
})

func getVirtualMachineReadinessGuestInfoProbe(actions []vmopv1.GuestInfoAction) *vmopv1.VirtualMachineReadinessProbeSpec {
	return &vmopv1.VirtualMachineReadinessProbeSpec{
		GuestInfo:     actions,
		PeriodSeconds: 1,
	}
}
