// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustermodules_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/clustermodules"
)

var _ = Describe("FindClusterModuleUUID", func() {
	const (
		groupName1, groupName2   = "groupName1", "groupName2"
		moduleUUID1, moduleUUID2 = "uuid1", "uuid2"
	)

	var (
		ctx                      context.Context
		resourcePolicy           *vmopv1.VirtualMachineSetResourcePolicy
		clusterRef1, clusterRef2 types.ManagedObjectReference
	)

	BeforeEach(func() {
		ctx = pkgconfig.NewContext()

		resourcePolicy = &vmopv1.VirtualMachineSetResourcePolicy{
			Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModuleGroups: []string{groupName1, groupName2},
			},
		}

		clusterRef1 = types.ManagedObjectReference{Value: "dummy-cluster1"}
		clusterRef2 = types.ManagedObjectReference{Value: "dummy-cluster2"}
	})

	AfterEach(func() {
		ctx = nil
	})

	Context("GroupName does not exist", func() {
		It("Returns expected values", func() {
			idx, uuid := clustermodules.FindClusterModuleUUID(ctx, "does-not-exist", clusterRef1, resourcePolicy)
			Expect(idx).To(Equal(-1))
			Expect(uuid).To(BeEmpty())
		})
	})

	Context("GroupName exists", func() {
		BeforeEach(func() {
			resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules,
				vmopv1.VSphereClusterModuleStatus{
					GroupName:   groupName1,
					ModuleUuid:  moduleUUID1,
					ClusterMoID: clusterRef1.Value,
				},
				vmopv1.VSphereClusterModuleStatus{
					GroupName:   groupName2,
					ModuleUuid:  moduleUUID2,
					ClusterMoID: clusterRef1.Value,
				},
			)
		})

		It("Returns expected entry", func() {
			idx, uuid := clustermodules.FindClusterModuleUUID(ctx, groupName1, clusterRef1, resourcePolicy)
			Expect(idx).To(Equal(0))
			Expect(uuid).To(Equal(moduleUUID1))

			idx, uuid = clustermodules.FindClusterModuleUUID(ctx, groupName2, clusterRef1, resourcePolicy)
			Expect(idx).To(Equal(1))
			Expect(uuid).To(Equal(moduleUUID2))
		})
	})

	Context("Matches by cluster reference", func() {
		BeforeEach(func() {
			resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules,
				vmopv1.VSphereClusterModuleStatus{
					GroupName:   groupName1,
					ModuleUuid:  moduleUUID1,
					ClusterMoID: clusterRef1.Value,
				},
				vmopv1.VSphereClusterModuleStatus{
					GroupName:   groupName1,
					ModuleUuid:  moduleUUID2,
					ClusterMoID: clusterRef2.Value,
				},
			)
		})

		It("Returns expected entry", func() {
			idx, uuid := clustermodules.FindClusterModuleUUID(ctx, groupName1, clusterRef1, resourcePolicy)
			Expect(idx).To(Equal(0))
			Expect(uuid).To(Equal(moduleUUID1))

			idx, uuid = clustermodules.FindClusterModuleUUID(ctx, groupName1, clusterRef2, resourcePolicy)
			Expect(idx).To(Equal(1))
			Expect(uuid).To(Equal(moduleUUID2))
		})
	})
})
