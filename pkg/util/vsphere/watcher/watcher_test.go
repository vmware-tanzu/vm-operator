// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package watcher_test

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

const (
	fakeStr = "fake"
)

func idStr(i int) string {
	return "id-" + strconv.Itoa(i)
}

var _ = Describe("Start", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		model           *simulator.Model
		server          *simulator.Server
		client          *govmomi.Client
		w               *watcher.Watcher
		closeServerOnce sync.Once

		cluster1 *object.ClusterComputeResource
		cluster2 *object.ClusterComputeResource

		cluster1vm1 *object.VirtualMachine
		cluster1vm2 *object.VirtualMachine
		cluster2vm1 *object.VirtualMachine

		lookupFnVerified bool
		lookupFnDeleted  bool
	)

	addNamespaceName := func(
		vm *object.VirtualMachine,
		namespace, name string) {

		t, err := vm.Reconfigure(
			ctx,
			vimtypes.VirtualMachineConfigSpec{
				ExtraConfig: []vimtypes.BaseOptionValue{
					&vimtypes.OptionValue{
						Key:   "vmservice.namespacedName",
						Value: namespace + "/" + name,
					},
				},
			},
		)
		ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
		ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())
	}

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		ctx, cancel = context.WithCancel(ctx)
		ctx = logr.NewContext(ctx, testutil.GinkgoLogr(5))
		ctx = watcher.WithContext(ctx)

		closeServerOnce = sync.Once{}

		lookupFnVerified = false

		model = simulator.VPX()
		model.Datacenter = 1
		model.Cluster = 2
		model.ClusterHost = 3
		model.Host = 0
		model.Machine = 2
		model.Autostart = false

		if model.Service == nil {
			Expect(model.Create()).To(Succeed())
			model.Service.TLS = &tls.Config{}
		}

		model.Service.RegisterEndpoints = true
		server = model.Service.NewServer()

		var err error
		client, err = govmomi.NewClient(ctx, server.URL, true)
		Expect(err).ToNot(HaveOccurred())

		finder := find.NewFinder(client.Client)
		datacenter, err := finder.DatacenterOrDefault(ctx, "*")
		Expect(err).ToNot(HaveOccurred())
		Expect(datacenter).ToNot(BeNil())

		folder, err := finder.DefaultFolder(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(folder).ToNot(BeNil())

		clusters, err := finder.ClusterComputeResourceList(ctx, "*")
		Expect(err).ToNot(HaveOccurred())
		Expect(clusters).To(HaveLen(2))
		cluster1 = clusters[0]
		cluster2 = clusters[1]

		var (
			cluster1vms []*object.VirtualMachine
			cluster2vms []*object.VirtualMachine
		)
		vms, err := finder.VirtualMachineList(ctx, "*")
		Expect(err).ToNot(HaveOccurred())
		Expect(vms).To(HaveLen(4))
		for i := range vms {
			vm := vms[i]
			host, err := vm.HostSystem(ctx)
			Expect(err).ToNot(HaveOccurred())
			pool, err := host.ResourcePool(ctx)
			Expect(err).ToNot(HaveOccurred())
			owner, err := pool.Owner(ctx)
			Expect(err).ToNot(HaveOccurred())
			if owner.Reference() == cluster1.Reference() {
				cluster1vms = append(cluster1vms, vm)
			} else {
				cluster2vms = append(cluster2vms, vm)
			}
		}

		Expect(cluster1vms).To(HaveLen(2))
		Expect(cluster2vms).To(HaveLen(2))
		cluster1vm1 = cluster1vms[0]
		cluster1vm2 = cluster1vms[1]
		cluster2vm1 = cluster2vms[0]
	})

	JustBeforeEach(func() {
		var err error
		w, err = watcher.Start(
			ctx,
			client.Client,
			nil,
			[]string{
				"ignoredKey1",
				"ignoredKey1",
				"ignoredKey2",
			},
			func(
				_ context.Context,
				moRef vimtypes.ManagedObjectReference,
				namespace, name string) watcher.LookupNamespacedNameResult {

				if moRef == cluster2vm1.Reference() {
					return watcher.LookupNamespacedNameResult{
						Namespace: "my-namespace-4",
						Name:      "my-name-1",
						Verified:  lookupFnVerified,
						Deleted:   lookupFnDeleted,
					}
				}
				return watcher.LookupNamespacedNameResult{}
			},
			map[vimtypes.ManagedObjectReference][]string{
				cluster1.Reference(): {idStr(0)},
			},
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(w).ToNot(BeNil())
	})

	AfterEach(func() {
		cancel()
		Eventually(w.Done(), time.Second*5).Should(BeClosed())
		_ = client.Logout(ctx)
		closeServerOnce.Do(server.Close)
		model.Remove()
	})

	assertResult := func(
		vm *object.VirtualMachine,
		namespace, name string) {

		var result watcher.Result
		EventuallyWithOffset(1, w.Result(), time.Second*5).Should(
			Receive(&result, Equal(watcher.Result{
				Namespace: namespace,
				Name:      name,
				Ref:       vm.Reference(),
			})))
	}

	assertNoError := func() {
		ConsistentlyWithOffset(1, w.Err()).Should(BeNil())
	}

	assertNoResult := func() {
		ConsistentlyWithOffset(1, w.Result()).ShouldNot(Receive())
	}

	assertError := func() {
		EventuallyWithOffset(1, func() error { return w.Err() }).ShouldNot(BeNil())

		var urlErr *url.Error
		ExpectWithOffset(1, errors.As(w.Err(), &urlErr)).To(BeTrue())
		ExpectWithOffset(1, urlErr.Op).To(Equal("Post"))

		var tErr *net.OpError
		switch {
		case errors.As(urlErr.Err, &tErr):
			ExpectWithOffset(1, tErr.Op).To(Equal("dial"))
			ExpectWithOffset(1, tErr.Net).To(Equal("tcp"))
			ExpectWithOffset(1, tErr.Err).ToNot(BeNil())
			var sysErr *os.SyscallError
			ExpectWithOffset(1, errors.As(tErr.Err, &sysErr)).To(BeTrue())
			ExpectWithOffset(1, sysErr.Syscall).To(Equal("connect"))
		default:
			ExpectWithOffset(1, urlErr.Err).To(HaveOccurred())
		}
	}

	When("the context is cancelled", FlakeAttempts(5), func() {
		JustBeforeEach(func() {
			cancel()
		})
		When("no vms have namespace/name information", func() {
			Specify("no error or result should be returned", func() {
				assertNoError()
				assertNoResult()
			})
		})
		When("one vm has namespace/name information", func() {
			BeforeEach(func() {
				addNamespaceName(cluster1vm1, "my-namespace-1", "my-name-1")
			})
			Specify("no error or result is returned", func() {
				assertNoError()
				assertNoResult()
			})
		})
	})

	When("the connection to vSphere is interrupted", FlakeAttempts(5), func() {
		JustBeforeEach(func() {
			closeServerOnce.Do(server.Close)
		})

		When("no vms have namespace/name information", func() {
			Specify("an error should be returned", func() {
				assertError()
				assertNoResult()
			})
		})

		When("a vm has namespace/name information", func() {
			BeforeEach(func() {
				addNamespaceName(cluster1vm1, "my-namespace-1", "my-name-1")
			})
			Specify("an error should be returned", func() {
				assertError()
				assertNoResult()
			})
		})
	})

	When("no vms have namespace/name information", func() {
		Specify("no error or results", func() {
			assertNoError()
			assertNoResult()
		})
	})

	When("removal of container that does not exist", func() {
		Specify("no error or results", func() {
			moRef := cluster1vm1.Reference()
			moRef.Value = "bogus"
			Expect(watcher.Remove(ctx, moRef, idStr(0))).To(Succeed())
			assertNoError()
			assertNoResult()
		})
	})

	When("one vm has namespace/name information", func() {
		BeforeEach(func() {
			addNamespaceName(cluster1vm1, "my-namespace-1", "my-name-1")
		})
		Specify("the result channel should receive a result", func() {
			// Assert that a result is signaled due to the VM entering the
			// scope of the watcher.
			assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

			// Assert no more results are signaled.
			assertNoResult()

			// Assert no error either.
			assertNoError()
		})

		When("the vm is powered on", func() {
			Specify("the result channel should receive a result", func() {
				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no results are signaled until the VM's power state is
				// updated.
				assertNoResult()

				// Power on the VM.
				t, err := cluster1vm1.PowerOn(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(t.Wait(ctx)).To(Succeed())

				// Assert a result is signaled as a result of the PowerOn op.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Assert no error either.
				assertNoError()
			})
		})

		When("the only change is for ignored extraConfig keys", func() {
			Specify("no result should be received", func() {
				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Add both of the ignored extraConfig keys.
				t, err := cluster1vm1.Reconfigure(
					ctx,
					vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "ignoredKey1",
								Value: fakeStr,
							},
							&vimtypes.OptionValue{
								Key:   "ignoredKey2",
								Value: fakeStr,
							},
						},
					},
				)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())

				// Assert no more results are signaled.
				assertNoResult()

				// Assert no error either.
				assertNoError()
			})
		})

		When("there is a change to a non-ignored extraConfig key", func() {
			Specify("a result should be received", func() {
				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Add a mixture of ignored and non-ignored keys.
				t, err := cluster1vm1.Reconfigure(
					ctx,
					vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "ignoredKey1",
								Value: fakeStr,
							},
							&vimtypes.OptionValue{
								Key:   "guestinfo.fromTheGuest",
								Value: "1.2.3.4",
							},
							&vimtypes.OptionValue{
								Key:   "ignoredKey2",
								Value: fakeStr,
							},
						},
					},
				)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())

				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Assert no error either.
				assertNoError()
			})
		})

		When("there is a change to rootSnapshot, and currentSnapshot", func() {
			Specify("a result should be received", func() {
				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Invoke a snapshot operation that should update the
				// root snapshots, and current snapshot of the VM.
				t, err := cluster1vm1.CreateSnapshot(ctx, "snap-1", "description", false, false)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())

				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Assert no error either.
				assertNoError()

				// Take another snapshot to elongate the chain.
				t, err = cluster1vm1.CreateSnapshot(ctx, "snap-2", "description", false, false)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())

				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Assert no error either.
				assertNoError()

				// Revert the VM to the first snapshot so current
				// snapshot is updated.
				t, err = cluster1vm1.RevertToSnapshot(ctx, "snap-1", false)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())

				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Assert no error either.
				assertNoError()
			})
		})

		When("the container is removed from the watcher", func() {
			Specify("no result should be received", func() {
				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Remove the cluster from the scope of the watcher.
				Expect(watcher.Remove(ctx, cluster1.Reference(), idStr(0))).To(Succeed())

				// Add a non-ignored ExtraConfig key.
				t, err := cluster1vm1.Reconfigure(
					ctx,
					vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.fromTheGuest",
								Value: "1.2.3.4",
							},
							&vimtypes.OptionValue{
								Key:   "ignoredKey2",
								Value: fakeStr,
							},
						},
					},
				)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())

				// Assert no error either.
				assertNoError()

				// Assert no more results are signaled.
				assertNoResult()

				// Duplicate remove should noop.
				Expect(watcher.Remove(ctx, cluster1.Reference(), idStr(0))).To(Succeed())
			})
		})

		When("the container has multiple adds", func() {
			JustBeforeEach(func() {
				Expect(watcher.Add(ctx, cluster1.Reference(), idStr(1))).To(Succeed())
				Expect(watcher.Add(ctx, cluster1.Reference(), idStr(2))).To(Succeed())
			})
			Specify("it should need an equal number of removes before it stops being watched", func() {
				// Assert that a result is signaled due to the VM entering the
				// scope of the watcher.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				// Remove the cluster from the scope of the watcher.
				Expect(watcher.Remove(ctx, cluster1.Reference(), idStr(0))).To(Succeed())

				// Add a non-ignored ExtraConfig key.
				t, err := cluster1vm1.Reconfigure(
					ctx,
					vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.0",
								Value: "0",
							},
						},
					},
				)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())

				// Assert no error either.
				assertNoError()

				// Assert that a result is signaled due to there still being a
				// ref on the container and thus it is still being watched.
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

				// Assert no more results are signaled.
				assertNoResult()

				//
				// Do this again with next ID.
				//
				Expect(watcher.Remove(ctx, cluster1.Reference(), idStr(1))).To(Succeed())
				t, err = cluster1vm1.Reconfigure(
					ctx,
					vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.1",
								Value: "1",
							},
						},
					},
				)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())
				assertNoError()
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")
				assertNoResult()

				//
				// Do it again with the same ID. This should be a noop so we still expect events.
				//
				Expect(watcher.Remove(ctx, cluster1.Reference(), idStr(1))).To(Succeed())
				t, err = cluster1vm1.Reconfigure(
					ctx,
					vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.1",
								Value: "2",
							},
						},
					},
				)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())
				assertNoError()
				assertResult(cluster1vm1, "my-namespace-1", "my-name-1")
				assertNoResult()

				//
				// Remove the container with the final ID, and it should finally be
				// removed from the watch.
				//
				Expect(watcher.Remove(ctx, cluster1.Reference(), idStr(2))).To(Succeed())
				t, err = cluster1vm1.Reconfigure(
					ctx,
					vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.2",
								Value: "1",
							},
						},
					},
				)
				ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
				ExpectWithOffset(1, t.WaitEx(ctx)).To(Succeed())
				assertNoError()
				assertNoResult()
			})
		})
	})

	When("multiple vms have namespace/name information", func() {
		BeforeEach(func() {
			addNamespaceName(cluster1vm1, "my-namespace-1", "my-name-1")
			addNamespaceName(cluster1vm2, "my-namespace-2", "my-name-2")
		})
		Specify("the result channel should receive multiple results", func() {
			var (
				result1 watcher.Result
				result2 watcher.Result
			)

			Eventually(w.Result(), time.Second*3).Should(Receive(&result1))
			Eventually(w.Result(), time.Second*5).Should(Receive(&result2))

			// Assert that two results are signaled due to the VMs entering the
			// scope of the watcher.
			Expect([]watcher.Result{
				result1,
				result2,
			}).To(ConsistOf(
				watcher.Result{
					Namespace: "my-namespace-2",
					Name:      "my-name-2",
					Ref:       cluster1vm2.Reference(),
				},
				watcher.Result{
					Namespace: "my-namespace-1",
					Name:      "my-name-1",
					Ref:       cluster1vm1.Reference(),
				},
			))

			// Assert no more results are signaled.
			assertNoResult()

			// Assert no error either.
			assertNoError()
		})
	})

	When("a vm is destroyed", func() {
		BeforeEach(func() {
			addNamespaceName(cluster1vm1, "my-namespace-1", "my-name-1")
		})
		Specify("the result channel should not receive a result when the vm is deleted", func() {
			// Assert that a result is signaled due to the VM entering the
			// scope of the watcher.
			assertResult(cluster1vm1, "my-namespace-1", "my-name-1")

			// Delete the VM.
			t, err := cluster1vm1.Destroy(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(t.Wait(ctx)).To(Succeed())

			// Assert a result is not signaled as a result of the Delete
			// operation.
			assertNoResult()

			// Assert no error either.
			assertNoError()
		})
	})

	When("a vm is a member of a container not being watched", func() {
		Specify("no result or error should be received", func() {
			assertNoResult()
			assertNoError()
		})

		When("the container is added to the watcher", func() {
			JustBeforeEach(func() {
				Expect(watcher.Add(ctx, cluster2.Reference(), idStr(0))).To(Succeed())
			})
			When("the lookup function returns verified=false", func() {
				Specify("the result channel should receive a result", func() {
					// Assert that a result is signaled due to the VM entering the
					// scope of the watcher.
					//
					// Please note the result's namespaced name is not set on the
					// VM, but obtained from the lookup function.
					assertResult(cluster2vm1, "my-namespace-4", "my-name-1")

					// Assert no more results are signaled.
					assertNoResult()

					// Assert no error either.
					assertNoError()
				})
			})
			When("the lookup function returns verified=true", func() {
				BeforeEach(func() {
					lookupFnVerified = true
				})
				Specify("the result channel should not receive a result", func() {
					// Assert no result is signalled because the object entered
					// the watcher in a verified state.
					assertNoResult()

					// Assert no error either.
					assertNoError()
				})
			})

			When("the lookup function returns hasDeletionTimestamp=true", func() {
				BeforeEach(func() {
					lookupFnDeleted = true
				})
				Specify("the result channel should not receive a result", func() {
					// Assert no result is signalled because the object has a
					// deletion timestamp.
					assertNoResult()

					// Assert no error either.
					assertNoError()
				})
			})
		})
	})
})
