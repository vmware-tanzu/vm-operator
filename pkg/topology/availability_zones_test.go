// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
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
		wcpFaultDomainsFssEnabled bool
		numberOfAvailabilityZones int
		numberOfNamespaces        int
		oldFaultDomainsFunc       func() bool
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = builder.NewFakeClient()
		oldFaultDomainsFunc = lib.IsWcpFaultDomainsFSSEnabled
	})

	AfterEach(func() {
		ctx = nil
		client = nil
		wcpFaultDomainsFssEnabled = false
		numberOfAvailabilityZones = 0
		numberOfNamespaces = 0
		lib.IsWcpFaultDomainsFSSEnabled = oldFaultDomainsFunc
	})

	JustBeforeEach(func() {
		lib.IsWcpFaultDomainsFSSEnabled = func() bool {
			return wcpFaultDomainsFssEnabled
		}

		for i := 0; i < numberOfAvailabilityZones; i++ {
			obj := &topologyv1.AvailabilityZone{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("az-%d", i),
				},
				Spec: topologyv1.AvailabilityZoneSpec{
					Namespaces: map[string]topologyv1.NamespaceInfo{},
				},
			}
			if wcpFaultDomainsFssEnabled {
				for j := 0; j < numberOfNamespaces; j++ {
					obj.Spec.Namespaces[fmt.Sprintf("ns-%d", j)] = topologyv1.NamespaceInfo{
						PoolMoId:   poolMoID,
						FolderMoId: folderMoID,
					}
				}
			}
			Expect(client.Create(ctx, obj)).To(Succeed())
		}

		if !wcpFaultDomainsFssEnabled {
			for i := 0; i < numberOfNamespaces; i++ {
				obj := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("ns-%d", i),
						Annotations: map[string]string{
							topology.NamespaceRPAnnotationKey:     poolMoID,
							topology.NamespaceFolderAnnotationKey: folderMoID,
						},
					},
				}
				Expect(client.Create(ctx, obj)).To(Succeed())
			}
		}
	})

	assertDefaultZoneNamespaces := func(zone topologyv1.AvailabilityZone, name string) {
		nsName := ctrlclient.ObjectKey{Name: name}
		ExpectWithOffset(2, zone.Spec.Namespaces).To(HaveKey(nsName.Name))
		ExpectWithOffset(2, client.Get(ctx, nsName, &corev1.Namespace{})).To(Succeed())
		ExpectWithOffset(2, zone.Spec.Namespaces[nsName.Name].PoolMoId).To(Equal(poolMoID))
		ExpectWithOffset(2, zone.Spec.Namespaces[nsName.Name].FolderMoId).To(Equal(folderMoID))
	}

	assertGetAvailabilityZonesDefaultZone := func() {
		zones, err := topology.GetAvailabilityZones(ctx, client)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, zones).To(HaveLen(1))
		ExpectWithOffset(1, zones[0].Name).To(Equal(topology.DefaultAvailabilityZoneName))
		ExpectWithOffset(1, zones[0].Spec.Namespaces).To(HaveLen(numberOfNamespaces))
		for i := 0; i < numberOfNamespaces; i++ {
			assertDefaultZoneNamespaces(zones[0], fmt.Sprintf("ns-%d", i))
		}
	}
	assertGetAvailabilityZoneDefaultZone := func() {
		zone, err := topology.GetAvailabilityZone(ctx, client, topology.DefaultAvailabilityZoneName)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, zone.Name).To(Equal(topology.DefaultAvailabilityZoneName))
		ExpectWithOffset(1, zone.Spec.Namespaces).To(HaveLen(numberOfNamespaces))
		for i := 0; i < numberOfNamespaces; i++ {
			assertDefaultZoneNamespaces(zone, fmt.Sprintf("ns-%d", i))
		}
	}

	// Same assertions as assertGetAvailabilityZoneDefaultZone() above.
	assertGetAvailabilityEmptyZoneDefaultZone := func() {
		zone, err := topology.GetAvailabilityZone(ctx, client, "")
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, zone.Name).To(Equal(topology.DefaultAvailabilityZoneName))
		ExpectWithOffset(1, zone.Spec.Namespaces).To(HaveLen(numberOfNamespaces))
		for i := 0; i < numberOfNamespaces; i++ {
			assertDefaultZoneNamespaces(zone, fmt.Sprintf("ns-%d", i))
		}
	}

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

	assertGetNamespaceRPAndFolderInvalidNameErrNotFound := func() {
		_, _, err := topology.GetNamespaceRPAndFolder(ctx, client, "invalid", "ns-1")
		ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
	}
	assertGetNamespaceRPAndFolderSuccessForAZ := func(azName string) {
		for i := 0; i < numberOfNamespaces; i++ {
			rp, folder, err := topology.GetNamespaceRPAndFolder(ctx, client, azName, fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(2, err).ToNot(HaveOccurred())
			ExpectWithOffset(2, rp).To(Equal(poolMoID))
			ExpectWithOffset(2, folder).To(Equal(folderMoID))
		}
	}
	assertGetNamespaceRPAndFolderSuccess := func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			assertGetNamespaceRPAndFolderSuccessForAZ(fmt.Sprintf("az-%d", i))
		}
	}
	assertGetNamespaceRPAndFolderSuccessNoZone := func() {
		assertGetNamespaceRPAndFolderSuccessForAZ("")
	}
	assertGetNamespaceRPAndFolderSuccessDefaultZone := func() {
		assertGetNamespaceRPAndFolderSuccessForAZ(topology.DefaultAvailabilityZoneName)
	}
	assertGetNamespaceRPAndFolderInvalidAZErrNotFound := func() {
		for i := 0; i < numberOfNamespaces; i++ {
			_, _, err := topology.GetNamespaceRPAndFolder(ctx, client, "invalid", fmt.Sprintf("ns-%d", i))
			ExpectWithOffset(1, apierrors.IsNotFound(err)).To(BeTrue())
		}
	}
	assertGetNamespaceRPAndFolderInvalidNamespaceErrNotFound := func() {
		for i := 0; i < numberOfAvailabilityZones; i++ {
			azName := fmt.Sprintf("az-%d", i)
			_, _, err := topology.GetNamespaceRPAndFolder(ctx, client, azName, "invalid")
			ExpectWithOffset(1, err).To(
				MatchError(fmt.Errorf("availability zone %s missing info for namespace %s", azName, "invalid")))
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
			Context("WCP_FaultDomains=enabled", func() {
				BeforeEach(func() {
					wcpFaultDomainsFssEnabled = true
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
				Context("GetNamespaceRPAndFolder", func() {
					Context("With an invalid AvailabilityZone name", assertGetNamespaceRPAndFolderInvalidAZErrNotFound)
					Context("With a valid AvailabilityZone name", func() {
						It("Should return the RP and Folder resources", assertGetNamespaceRPAndFolderSuccess)
					})
					Context("With an invalid AvailabilityZone name", func() {
						It("Should return an apierrors.NotFound error", assertGetNamespaceRPAndFolderInvalidNameErrNotFound)
					})
					Context("With an invalid Namespace name", func() {
						It("Should return an missing info error", assertGetNamespaceRPAndFolderInvalidNamespaceErrNotFound)
					})
				})
			})
			Context("WCP_FaultDomains=disabled", func() {
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
						It("Should return the Default AvailabilityZone resource", assertGetAvailabilityEmptyZoneDefaultZone)
					})
				})
				Context("GetNamespaceRPAndFolder", func() {
					Context("With an invalid AvailabilityZone name", assertGetNamespaceRPAndFolderInvalidAZErrNotFound)
					Context("With the default AvailabilityZone name", func() {
						It("Should return the RP and Folder resources", assertGetNamespaceRPAndFolderSuccessDefaultZone)
					})
					Context("With an empty AvailabilityZone name", func() {
						It("Should return the RP and Folder resources", assertGetNamespaceRPAndFolderSuccessNoZone)
					})
					Context("With an invalid Namespace name", func() {
						It("Should return an missing info error", assertGetNamespaceRPAndFolderInvalidNamespaceErrNotFound)
					})
				})
			})
		})
		When("DevOps Namespaces do not exist", func() {
			Context("WCP_FaultDomains=enabled", func() {
				BeforeEach(func() {
					wcpFaultDomainsFssEnabled = true
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
			})
			Context("WCP_FaultDomains=disabled", func() {
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
						It("Should return the Default AvailabilityZone resource", assertGetAvailabilityEmptyZoneDefaultZone)
					})
				})
			})
		})
	})

	When("Availability zones do not exist", func() {
		When("DevOps Namespaces exist", func() {
			BeforeEach(func() {
				numberOfNamespaces = 3
			})
			Context("WCP_FaultDomains=enabled", func() {
				BeforeEach(func() {
					wcpFaultDomainsFssEnabled = true
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
			Context("WCP_FaultDomains=disabled", func() {
				Context("GetAvailabilityZones", func() {
					It("Should return a single AvailabilityZone resource "+
						"with the Namespaces field populated from the "+
						"DevOps Namespace resources", assertGetAvailabilityZonesDefaultZone)
				})
				Context("GetAvailabilityZone", func() {
					Context("With topology.DefaultAvailabilityZone", func() {
						It("Should return a single AvailabilityZone resource "+
							"with the Namespaces field populated from the "+
							"DevOps Namespace resources", assertGetAvailabilityZoneDefaultZone)
					})
					Context("With an invalid AvailabilityZone name", func() {
						It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneInvalidNameErrNotFound)
					})
					Context("With an empty AvailabilityZone name", func() {
						It("Should return the Default AvailabilityZone resource", assertGetAvailabilityEmptyZoneDefaultZone)
					})
				})
			})
		})
		When("DevOps Namespaces do not exist", func() {
			Context("WCP_FaultDomains=enabled", func() {
				BeforeEach(func() {
					wcpFaultDomainsFssEnabled = true
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
			Context("WCP_FaultDomains=disabled", func() {
				Context("GetAvailabilityZones", func() {
					It("Should return a single AvailabilityZone resource "+
						"with the Namespaces field populated from the "+
						"DevOps Namespace resources", assertGetAvailabilityZonesDefaultZone)
				})
				Context("GetAvailabilityZone", func() {
					Context("With topology.DefaultAvailabilityZone", func() {
						It("Should return a single AvailabilityZone resource "+
							"with the Namespaces field populated from the "+
							"DevOps Namespace resources", assertGetAvailabilityZoneDefaultZone)
					})
					Context("With an invalid AvailabilityZone name", func() {
						It("Should return an apierrors.NotFound error", assertGetAvailabilityZoneInvalidNameErrNotFound)
					})
					Context("With an empty AvailabilityZone name", func() {
						It("Should return the Default AvailabilityZone resource", assertGetAvailabilityEmptyZoneDefaultZone)
					})
				})
			})
		})
	})
})
