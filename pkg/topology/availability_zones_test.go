// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
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
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	poolMoID   = "pool-moid"
	folderMoID = "folder-moid"
)

var _ = Describe("Availability Zones", func() {
	var (
		ctx                       context.Context
		client                    ctrlclient.Client
		numberOfAvailabilityZones int
		numberOfNamespaces        int
	)

	BeforeEach(func() {
		ctx = pkgconfig.NewContextWithDefaultConfig()
		client = builder.NewFakeClient()
	})

	AfterEach(func() {
		ctx = nil
		client = nil
		numberOfAvailabilityZones = 0
		numberOfNamespaces = 0
	})

	JustBeforeEach(func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			obj := &topologyv1.AvailabilityZone{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("az-%d", i),
				},
				Spec: topologyv1.AvailabilityZoneSpec{
					ClusterComputeResourceMoIDs: []string{fmt.Sprintf("cluster-%d", i)},
					Namespaces:                  map[string]topologyv1.NamespaceInfo{},
				},
			}
			for j := 0; j < numberOfNamespaces; j++ {
				obj.Spec.Namespaces[fmt.Sprintf("ns-%d", j)] = topologyv1.NamespaceInfo{
					PoolMoIDs:  []string{poolMoID},
					FolderMoId: folderMoID,
				}
			}
			Expect(client.Create(ctx, obj)).To(Succeed())
		}
	})

	assertGetAvailabilityZonesSuccess := func() {
		zones, err := topology.GetAvailabilityZones(ctx, client)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, zones).To(HaveLen(numberOfAvailabilityZones))
	}

	assertGetAvailabilityZoneSuccess := func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			_, err := topology.GetAvailabilityZone(ctx, client, fmt.Sprintf("az-%d", i))
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
		}
	}

	assertGetAvailabilityZonesErrNoAvailabilityZones := func() {
		_, err := topology.GetAvailabilityZones(ctx, client)
		ExpectWithOffset(1, err).To(MatchError(topology.ErrNoAvailabilityZones))
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

	assertGetAvailabilityZoneEmptyNameErrNotFound := func() {
		_, err := topology.GetAvailabilityZone(ctx, client, "")
		ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
	}

	assertGetNamespaceFolderAndRPMoIDInvalidNameErrNotFound := func() {
		_, _, err := topology.GetNamespaceFolderAndRPMoID(ctx, client, "invalid", "ns-1")
		ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
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

	When("Availability zones do not exist", func() {
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
			Expect(err.Error()).To(ContainSubstring("failed to find zone for cluster MoID "))
		})

		It("returns expected zone name", func() {
			zoneName, err := topology.LookupZoneForClusterMoID(ctx, client, "cluster-2")
			Expect(err).ToNot(HaveOccurred())
			Expect(zoneName).To(Equal("az-2"))
		})
	})
})
