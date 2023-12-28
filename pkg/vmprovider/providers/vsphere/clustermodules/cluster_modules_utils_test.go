// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustermodules_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/clustermodules"
)

var _ = Describe("FindClusterModuleUUID", func() {
	const (
		groupName1, groupName2   = "groupName1", "groupName2"
		moduleUUID1, moduleUUID2 = "uuid1", "uuid2"
	)

	var (
		resourcePolicy           *vmopv1.VirtualMachineSetResourcePolicy
		clusterRef1, clusterRef2 types.ManagedObjectReference
	)

	BeforeEach(func() {
		resourcePolicy = &vmopv1.VirtualMachineSetResourcePolicy{
			Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModules: []vmopv1.ClusterModuleSpec{
					{
						GroupName: groupName1,
					},
					{
						GroupName: groupName2,
					},
				},
			},
		}

		clusterRef1 = types.ManagedObjectReference{Value: "dummy-cluster1"}
		clusterRef2 = types.ManagedObjectReference{Value: "dummy-cluster2"}
	})

	Context("FaultDomains FSS is disabled", func() {

		Context("GroupName does not exist", func() {
			It("Returns expected values", func() {
				idx, uuid := clustermodules.FindClusterModuleUUID("does-not-exist", clusterRef1, resourcePolicy)
				Expect(idx).To(Equal(-1))
				Expect(uuid).To(BeEmpty())
			})
		})

		Context("GroupName exists", func() {
			BeforeEach(func() {
				resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules,
					vmopv1.ClusterModuleStatus{
						GroupName:  groupName1,
						ModuleUuid: moduleUUID1,
					},
					vmopv1.ClusterModuleStatus{
						GroupName:   groupName2,
						ModuleUuid:  moduleUUID2,
						ClusterMoID: "this should be ignored",
					},
				)
			})

			It("Returns expected entry", func() {
				idx, uuid := clustermodules.FindClusterModuleUUID(groupName1, clusterRef1, resourcePolicy)
				Expect(idx).To(Equal(0))
				Expect(uuid).To(Equal(moduleUUID1))

				idx, uuid = clustermodules.FindClusterModuleUUID(groupName2, clusterRef1, resourcePolicy)
				Expect(idx).To(Equal(1))
				Expect(uuid).To(Equal(moduleUUID2))
			})
		})

	})

	Context("FaultDomains FSS is enabled", func() {
		var (
			oldFaultDomainsFunc func() bool
		)

		BeforeEach(func() {
			oldFaultDomainsFunc = lib.IsWcpFaultDomainsFSSEnabled
			lib.IsWcpFaultDomainsFSSEnabled = func() bool { return true }
		})

		AfterEach(func() {
			lib.IsWcpFaultDomainsFSSEnabled = oldFaultDomainsFunc
		})

		Context("GroupName does not exist", func() {
			It("Returns expected values", func() {
				idx, uuid := clustermodules.FindClusterModuleUUID("does-not-exist", clusterRef1, resourcePolicy)
				Expect(idx).To(Equal(-1))
				Expect(uuid).To(BeEmpty())
			})
		})

		Context("GroupName exists", func() {
			BeforeEach(func() {
				resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules,
					vmopv1.ClusterModuleStatus{
						GroupName:   groupName1,
						ModuleUuid:  moduleUUID1,
						ClusterMoID: clusterRef1.Value,
					},
					vmopv1.ClusterModuleStatus{
						GroupName:   groupName2,
						ModuleUuid:  moduleUUID2,
						ClusterMoID: clusterRef1.Value,
					},
				)
			})

			It("Returns expected entry", func() {
				idx, uuid := clustermodules.FindClusterModuleUUID(groupName1, clusterRef1, resourcePolicy)
				Expect(idx).To(Equal(0))
				Expect(uuid).To(Equal(moduleUUID1))

				idx, uuid = clustermodules.FindClusterModuleUUID(groupName2, clusterRef1, resourcePolicy)
				Expect(idx).To(Equal(1))
				Expect(uuid).To(Equal(moduleUUID2))
			})
		})

		Context("Matches by cluster reference", func() {
			BeforeEach(func() {
				resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules,
					vmopv1.ClusterModuleStatus{
						GroupName:   groupName1,
						ModuleUuid:  moduleUUID1,
						ClusterMoID: clusterRef1.Value,
					},
					vmopv1.ClusterModuleStatus{
						GroupName:   groupName1,
						ModuleUuid:  moduleUUID2,
						ClusterMoID: clusterRef2.Value,
					},
				)
			})

			It("Returns expected entry", func() {
				idx, uuid := clustermodules.FindClusterModuleUUID(groupName1, clusterRef1, resourcePolicy)
				Expect(idx).To(Equal(0))
				Expect(uuid).To(Equal(moduleUUID1))

				idx, uuid = clustermodules.FindClusterModuleUUID(groupName1, clusterRef2, resourcePolicy)
				Expect(idx).To(Equal(1))
				Expect(uuid).To(Equal(moduleUUID2))
			})
		})
	})
})
