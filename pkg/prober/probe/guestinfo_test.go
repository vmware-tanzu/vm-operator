// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	proberctx "github.com/vmware-tanzu/vm-operator/pkg/prober/context"
)

type fakeGuestInfoProvider struct {
	guestInfo map[string]any
	err       error
}

func (f fakeGuestInfoProvider) GetVirtualMachineProperties(ctx context.Context, vm *vmopv1.VirtualMachine, propertyPaths []string) (map[string]any, error) {
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

		probeCtx := &proberctx.ProbeContext{
			Logger: ctrl.Log.WithName("Probe").WithValues("name", vm.NamespacedName()),
			VM:     vm,
		}

		res, err = prober.Probe(probeCtx)
	})

	Context("Provider returns an error", func() {
		BeforeEach(func() {
			fakeProvider.err = fmt.Errorf("fake error")
		})

		When("there are guestinfo actions", func() {
			BeforeEach(func() {
				guestInfoAction = []vmopv1.GuestInfoAction{
					{
						Key: "key1",
					},
				}
			})
			It("returns err + Unknown", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Errorf("fake error")))
				Expect(res).To(Equal(Unknown))
			})
		})

		When("there are no guestinfo actions", func() {
			BeforeEach(func() {
				guestInfoAction = nil
			})
			It("returns Unknown", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(Unknown))
			})
		})
	})

	Context("Provider does not return an error", func() {
		toKey := func(s string) string {
			return fmt.Sprintf(`config.extraConfig["guestinfo.%s"]`, s)
		}
		toVal := func(k, v string) vimtypes.OptionValue {
			return vimtypes.OptionValue{
				Key:   toKey(k),
				Value: v,
			}
		}

		Context("Key does not exist in GuestInfo", func() {
			BeforeEach(func() {
				fakeProvider.guestInfo = map[string]any{
					toKey("key1"): toVal("key1", "ignored-because-no-value-in-action"),
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
				fakeProvider.guestInfo = map[string]any{
					toKey("key1"): toVal("key1", "ignored-because-no-value-in-action"),
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
				fakeProvider.guestInfo = map[string]any{
					toKey("key2"): toVal("key2", "vmware"),
				}
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
				fakeProvider.guestInfo = map[string]any{
					toKey("key3"): toVal("key3", "aaaaa"),
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
