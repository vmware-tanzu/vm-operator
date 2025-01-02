// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	poolMoID   = "pool-moid"
	folderMoID = "folder-moid"
)

var _ = Describe("Availability Zones and Zones", func() {
	var (
		ctx                       context.Context
		client                    ctrlclient.Client
		numberOfAvailabilityZones int
		numberOfZonesPerNamespace int
		numberOfNamespaces        int
		specCCRID                 bool
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		client = builder.NewFakeClient()
		specCCRID = false
	})

	AfterEach(func() {
		ctx = nil
		client = nil
		numberOfAvailabilityZones = 0
		numberOfZonesPerNamespace = 0
		numberOfNamespaces = 0
	})

	JustBeforeEach(func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			obj := &topologyv1.AvailabilityZone{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("az-%d", i),
				},
				Spec: topologyv1.AvailabilityZoneSpec{
					Namespaces: map[string]topologyv1.NamespaceInfo{},
				},
			}
			if specCCRID {
				obj.Spec.ClusterComputeResourceMoId = fmt.Sprintf("cluster-%d", i)
			} else {
				obj.Spec.ClusterComputeResourceMoIDs = []string{fmt.Sprintf("cluster-%d", i)}
			}
			for j := 0; j < numberOfNamespaces; j++ {
				obj.Spec.Namespaces[fmt.Sprintf("ns-%d", j)] = topologyv1.NamespaceInfo{
					PoolMoIDs:  []string{poolMoID},
					FolderMoId: folderMoID,
				}
			}
			Expect(client.Create(ctx, obj)).To(Succeed())
		}
		for i := 0; i < numberOfNamespaces; i++ {
			for j := 0; j < numberOfZonesPerNamespace; j++ {
				obj := &topologyv1.Zone{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fmt.Sprintf("ns-%d", i),
						Name:      fmt.Sprintf("zone-%d", j),
					},
					Spec: topologyv1.ZoneSpec{
						ManagedVMs: topologyv1.VSphereEntityInfo{
							PoolMoIDs:  []string{poolMoID},
							FolderMoID: folderMoID,
						},
					},
				}
				Expect(client.Create(ctx, obj)).To(Succeed())
			}
		}
	})

	assertGetAvailabilityZonesSuccess := func() {
		zones, err := topology.GetAvailabilityZones(ctx, client)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, zones).To(HaveLen(numberOfAvailabilityZones))
	}

	assertGetZonesSuccess := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			zones, err := topology.GetZones(ctx, client, fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, zones).To(HaveLen(numberOfZonesPerNamespace))
		}
	}

	assertGetAvailabilityZoneSuccess := func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			_, err := topology.GetAvailabilityZone(ctx, client, fmt.Sprintf("az-%d", i))
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
		}
	}

	assertGetZoneSuccess := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			for j := 0; j < numberOfZonesPerNamespace; j++ {
				_, err := topology.GetZone(ctx, client, fmt.Sprintf("zone-%d", j), fmt.Sprintf("ns-%d", i))
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
			}
		}
	}

	assertGetAvailabilityZonesErrNoAvailabilityZones := func() {
		_, err := topology.GetAvailabilityZones(ctx, client)
		ExpectWithOffset(1, err).To(MatchError(topology.ErrNoAvailabilityZones))
	}

	assertGetZonesErrNoZones := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			_, err := topology.GetZones(ctx, client, fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(1, err).To(MatchError(topology.ErrNoZones))
		}
	}

	assertGetAvailabilityZoneValidNamesErrNotFound := func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			_, err := topology.GetAvailabilityZone(ctx, client, fmt.Sprintf("az-%d", i))
			ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
		}
	}

	assertGetAvailabilityZoneInvalidNameErrNotFound := func() {
		_, err := topology.GetAvailabilityZone(ctx, client, "invalid")
		ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
	}

	assertGetZoneInvalidNameErrNotFound := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			_, err := topology.GetZone(ctx, client, "invalid", fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
		}
	}

	assertGetAvailabilityZoneEmptyNameErrNotFound := func() {
		_, err := topology.GetAvailabilityZone(ctx, client, "")
		ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
	}

	assertGetZoneEmptyNameErrNotFound := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			_, err := topology.GetZone(ctx, client, "", fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
		}
	}

	assertGetNamespaceFolderAndRPMoIDInvalidNameErrNotFound := func() {
		_, _, err := topology.GetNamespaceFolderAndRPMoID(ctx, client, "invalid", "ns-1")
		ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
	}

	assertGetNamespaceFolderAndRPMoIDZoneToDelete := func() {
		obj := &topologyv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:  "ns-1",
				Name:       "zone-to-delete",
				Finalizers: []string{"test"},
			},
		}
		err := client.Create(ctx, obj)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		_, err = topology.GetZone(ctx, client, "zone-to-delete", "ns-1")
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		err = client.Delete(ctx, obj)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		_, _, err = topology.GetNamespaceFolderAndRPMoID(ctx, client, "zone-to-delete", "ns-1")
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
	}

	assertGetNamespaceFolderAndRPMoIDSuccessForAZ := func(azName string) {
		for i := 0; i < numberOfNamespaces; i++ {
			folder, rp, err := topology.GetNamespaceFolderAndRPMoID(ctx, client, azName, fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(2, err).ToNot(HaveOccurred())
			ExpectWithOffset(2, rp).To(Equal(poolMoID))
			ExpectWithOffset(2, folder).To(Equal(folderMoID))
		}
	}

	assertGetNamespaceFolderAndRPMoIDSuccess := func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			assertGetNamespaceFolderAndRPMoIDSuccessForAZ(fmt.Sprintf("az-%d", i))
		}
		for i := 0; i < numberOfZonesPerNamespace; i++ {
			assertGetNamespaceFolderAndRPMoIDSuccessForAZ(fmt.Sprintf("zone-%d", i))
		}
	}

	assertGetNamespaceFolderAndRPMoIDInvalidAZErrNotFound := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			_, _, err := topology.GetNamespaceFolderAndRPMoID(ctx, client, "invalid", fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
		}
	}

	assertGetNamespaceFolderAndRPMoIDInvalidNamespaceErrNotFound := func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			azName := fmt.Sprintf("az-%d", i)
			_, _, err := topology.GetNamespaceFolderAndRPMoID(ctx, client, azName, "invalid")
			ExpectWithOffset(1, err).To(
				MatchError(fmt.Errorf("availability zone %q missing info for namespace %s", azName, "invalid")))
		}
	}

	assertGetNamespaceFolderAndRPMoIDsInvalidNamespaceNoErr := func() {
		folder, rpMoIDs, err := topology.GetNamespaceFolderAndRPMoIDs(ctx, client, "invalid")
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, rpMoIDs).To(BeEmpty())
		ExpectWithOffset(1, folder).To(BeEmpty())
	}

	assertGetNamespaceFolderAndRPMoIDsSuccess := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			folder, rpMoIDs, err := topology.GetNamespaceFolderAndRPMoIDs(ctx, client, fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, rpMoIDs).To(Not(BeEmpty()))
			ExpectWithOffset(1, folder).To(Equal(folderMoID))
		}
	}

	assertGetNamespaceFolderMoIDInvalidNamespaceErrNotFound := func() {
		_, err := topology.GetNamespaceFolderMoID(ctx, client, "invalid")
		ExpectWithOffset(1, err).To(HaveOccurred())
	}

	assertGetNamespaceFolderMoIDSuccess := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			folder, err := topology.GetNamespaceFolderMoID(ctx, client, fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, folder).To(Equal(folderMoID))
		}
	}

	When("Two AvailabilityZone resources exist", func() {
		BeforeEach(func() {
			numberOfAvailabilityZones = 2
		})
		When("Three DevOps Namespace resources exist", func() {
			BeforeEach(func() {
				numberOfNamespaces = 3
			})
			Context("GetAvailabilityZones", func() {
				It("Should return the two AvailabilityZone resources", assertGetAvailabilityZonesSuccess)
			})
			Context("GetAvailabilityZone", func() {
				Context("With a valid AvailabilityZone name", func() {
					It("Should return the AvailabilityZone resource", assertGetAvailabilityZoneSuccess)
				})
				Context("With an invalid AvailabilityZone name", func() {
					It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneInvalidNameErrNotFound)
				})
				Context("With an empty AvailabilityZone name", func() {
					It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneEmptyNameErrNotFound)
				})
			})
			Context("GetNamespaceFolderAndRPMoID", func() {
				Context("With an invalid AvailabilityZone name", assertGetNamespaceFolderAndRPMoIDInvalidAZErrNotFound)
				Context("With a valid AvailabilityZone name", func() {
					It("Should return the RP and Folder resources", assertGetNamespaceFolderAndRPMoIDSuccess)
				})
				Context("With an invalid AvailabilityZone name", func() {
					It("Should return an apierrors.NotFound error", assertGetNamespaceFolderAndRPMoIDInvalidNameErrNotFound)
				})
				Context("With an invalid Namespace name", func() {
					It("Should return an missing info error", assertGetNamespaceFolderAndRPMoIDInvalidNamespaceErrNotFound)
				})
			})
			Context("GetNamespaceFolderMoID", func() {
				Context("With an invalid Namespace name", func() {
					It("Should return NotFound", assertGetNamespaceFolderMoIDInvalidNamespaceErrNotFound)
				})
				Context("With a valid Namespace name", func() {
					It("Should return the RP and Folder resources", assertGetNamespaceFolderMoIDSuccess)
				})
			})
			Context("GetNamespaceFolderAndRPMoIDs", func() {
				Context("With an invalid Namespace name", func() {
					It("Should return NotFound", assertGetNamespaceFolderAndRPMoIDsInvalidNamespaceNoErr)
				})
				Context("With a valid Namespace name", func() {
					It("Should return the RP and Folder resources", assertGetNamespaceFolderAndRPMoIDsSuccess)
				})
			})
		})
		When("DevOps Namespaces do not exist", func() {
			Context("GetAvailabilityZones", func() {
				It("Should return the two AvailabilityZone resources", assertGetAvailabilityZonesSuccess)
			})
			Context("GetAvailabilityZone", func() {
				Context("With a valid AvailabilityZone name", func() {
					It("Should return the AvailabilityZone resource", assertGetAvailabilityZoneSuccess)
				})
				Context("With an invalid AvailabilityZone name", func() {
					It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneInvalidNameErrNotFound)
				})
				Context("With an empty AvailabilityZone name", func() {
					It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneEmptyNameErrNotFound)
				})
			})
		})
	})

	When("Two Zone resources exist per namespace", func() {
		BeforeEach(func() {
			numberOfZonesPerNamespace = 2
			pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
				config.Features.WorkloadDomainIsolation = true
			})
		})
		When("Three DevOps Namespace resources exist", func() {
			BeforeEach(func() {
				numberOfNamespaces = 3
			})
			Context("GetZones", func() {
				It("Should return the two Zone resources per namespace", assertGetZonesSuccess)
			})
			Context("GetZone", func() {
				Context("With a valid Zone name", func() {
					It("Should return the AvailabilityZone resource", assertGetZoneSuccess)
				})
				Context("With an invalid Zone name", func() {
					It("Should return an apierrors.NotFound error", assertGetZoneInvalidNameErrNotFound)
				})
				Context("With an empty Zone name", func() {
					It("Should return an apierrors.NotFound error", assertGetZoneEmptyNameErrNotFound)
				})
			})
			Context("GetNamespaceFolderAndRPMoID", func() {
				Context("With an invalid Zone name", assertGetNamespaceFolderAndRPMoIDInvalidAZErrNotFound)
				Context("With a valid Zone name", func() {
					It("Should return the RP and Folder resources", assertGetNamespaceFolderAndRPMoIDSuccess)
				})
				Context("With an invalid Zone name", func() {
					It("Should return an apierrors.NotFound error", assertGetNamespaceFolderAndRPMoIDInvalidNameErrNotFound)
				})
				Context("With an Zone that is being deleted", func() {
					It("Should return no error", assertGetNamespaceFolderAndRPMoIDZoneToDelete)
				})
				Context("With an invalid Namespace name", func() {
					It("Should return an error", assertGetNamespaceFolderAndRPMoIDInvalidNamespaceErrNotFound)
				})
			})
			Context("GetNamespaceFolderMoID", func() {
				Context("With an invalid Namespace name", func() {
					It("Should return NotFound", assertGetNamespaceFolderMoIDInvalidNamespaceErrNotFound)
				})
				Context("With a valid Namespace name", func() {
					It("Should return the RP and Folder resources", assertGetNamespaceFolderMoIDSuccess)
				})
			})
			Context("GetNamespaceFolderAndRPMoIDs", func() {
				Context("With an invalid Namespace name", func() {
					It("Should return NotFound", assertGetNamespaceFolderAndRPMoIDsInvalidNamespaceNoErr)
				})
				Context("With a valid Namespace name", func() {
					It("Should return the RP and Folder resources", assertGetNamespaceFolderAndRPMoIDsSuccess)
				})
			})
		})
	})

	When("Availability zones and zones do not exist", func() {
		When("DevOps Namespaces exist", func() {
			BeforeEach(func() {
				numberOfNamespaces = 3
			})
			Context("GetAvailabilityZones", func() {
				It("Should return an ErrNoAvailabilityZones", assertGetAvailabilityZonesErrNoAvailabilityZones)
			})
			Context("GetAvailabilityZone", func() {
				Context("With topology.DefaultAvailabilityZone", func() {
					It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneValidNamesErrNotFound)
					Context("With an invalid AvailabilityZone name", func() {
						It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneInvalidNameErrNotFound)
					})
					Context("With an empty AvailabilityZone name", func() {
						It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneEmptyNameErrNotFound)
					})
				})
			})
			Context("GetZones", func() {
				It("Should return an ErrNoZones", assertGetZonesErrNoZones)
			})
			Context("GetZone", func() {
				Context("With an invalid Zone name", func() {
					It("Should return an apierrors.NotFound error", assertGetZoneInvalidNameErrNotFound)
				})
				Context("With an empty Zone name", func() {
					It("Should return an apierrors.NotFound error", assertGetZoneEmptyNameErrNotFound)
				})
			})
		})

		When("DevOps Namespaces do not exist", func() {
			Context("GetAvailabilityZones", func() {
				It("Should return an ErrNoAvailabilityZones", assertGetAvailabilityZonesErrNoAvailabilityZones)
			})
			Context("GetAvailabilityZone", func() {
				Context("With topology.DefaultAvailabilityZone", func() {
					It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneValidNamesErrNotFound)
					Context("With an invalid AvailabilityZone name", func() {
						It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneInvalidNameErrNotFound)
					})
					Context("With an empty AvailabilityZone name", func() {
						It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneEmptyNameErrNotFound)
					})
				})
			})
		})
	})

	Context("LookupZoneForClusterMoID", func() {
		BeforeEach(func() {
			numberOfAvailabilityZones = 3
		})

		It("returns error when cluster is not found", func() {
			_, err := topology.LookupZoneForClusterMoID(ctx, client, "cluster-42")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to find availability zone for cluster MoID "))
		})

		When("spec.ClusterComputeResourceMoId", func() {
			BeforeEach(func() {
				specCCRID = true
			})
			It("returns expected zone name", func() {
				name, err := topology.LookupZoneForClusterMoID(ctx, client, "cluster-0")
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("az-0"))
			})
		})
		When("spec.ClusterComputeResourceMoIIDs", func() {
			BeforeEach(func() {
				specCCRID = false
			})
			It("returns expected zone name", func() {
				name, err := topology.LookupZoneForClusterMoID(ctx, client, "cluster-0")
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("az-0"))
			})
		})

		When("there are no availability zones", func() {
			BeforeEach(func() {
				numberOfAvailabilityZones = 0
			})
			It("returns error when cluster is not found", func() {
				_, err := topology.LookupZoneForClusterMoID(ctx, client, "cluster-42")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no availability zones"))
			})
		})
	})
})
