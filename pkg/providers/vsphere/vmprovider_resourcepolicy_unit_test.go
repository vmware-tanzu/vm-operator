// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

var _ = Describe("vSphereVMProvider.reconcileClusterModules", func() {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = vmopv1.AddToScheme(scheme)

	DescribeTable("Should reconcile and ", func(
		computeClusterReferences []vimtypes.ManagedObjectReference,
		resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy,
		wantClusterModuleStatus []vmopv1.VSphereClusterModuleStatus,
		objs []client.Object,
		existingModules map[string]fakeClusterModule,
	) {
		clusterModProvider := &fakeClusterModuleProvider{
			modules: existingModules,
		}
		vs := &vSphereVMProvider{
			k8sClient: fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&vmopv1.VirtualMachineSetResourcePolicy{}).
				WithObjects(objs...).Build(),
		}

		Expect(vs.reconcileClusterModules(ctx, clusterModProvider, computeClusterReferences, resourcePolicy)).ToNot(HaveOccurred())
		Expect(resourcePolicy.Status.ClusterModules).To(BeEquivalentTo(wantClusterModuleStatus))
		Expect(clusterModProvider.modules).To(HaveLen(len(resourcePolicy.Status.ClusterModules)))
	},
		Entry("not create a cluster module when having no clusterref",
			[]vimtypes.ManagedObjectReference{},
			&vmopv1.VirtualMachineSetResourcePolicy{Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModuleGroups: []string{"zero"},
			}},
			nil,
			[]client.Object{},
			map[string]fakeClusterModule{},
		),
		Entry("create a cluster module when having a clusterref",
			[]vimtypes.ManagedObjectReference{{Value: "first"}},
			&vmopv1.VirtualMachineSetResourcePolicy{Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModuleGroups: []string{"zero"},
			}},
			[]vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "created-0", ClusterMoID: "first"},
			},
			[]client.Object{},
			map[string]fakeClusterModule{},
		),
		Entry("create multiple modules when having multiple clusterrefs",
			[]vimtypes.ManagedObjectReference{
				{Value: "first"},
				{Value: "second"},
			},
			&vmopv1.VirtualMachineSetResourcePolicy{Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModuleGroups: []string{"zero", "one"},
			}},
			[]vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "created-0", ClusterMoID: "first"},
				{GroupName: "one", ModuleUuid: "created-1", ClusterMoID: "first"},
				{GroupName: "zero", ModuleUuid: "created-2", ClusterMoID: "second"},
				{GroupName: "one", ModuleUuid: "created-3", ClusterMoID: "second"},
			},
			[]client.Object{},
			map[string]fakeClusterModule{},
		),
		Entry("preserve existing cluster modules when not having changes",
			[]vimtypes.ManagedObjectReference{
				{Value: "first"},
				{Value: "second"},
			},
			&vmopv1.VirtualMachineSetResourcePolicy{
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{ClusterModuleGroups: []string{"zero", "one"}},
				Status: vmopv1.VirtualMachineSetResourcePolicyStatus{ClusterModules: []vmopv1.VSphereClusterModuleStatus{
					{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
					{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
					{GroupName: "zero", ModuleUuid: "pre-existing-2", ClusterMoID: "second"},
					{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
				}},
			},
			[]vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
				{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
				{GroupName: "zero", ModuleUuid: "pre-existing-2", ClusterMoID: "second"},
				{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
			},
			[]client.Object{},
			map[string]fakeClusterModule{
				"pre-existing-0": {clusterRef: "first"},
				"pre-existing-1": {clusterRef: "first"},
				"pre-existing-2": {clusterRef: "second"},
				"pre-existing-3": {clusterRef: "second"},
			},
		),
		Entry("delete cluster modules when cluster reference got removed",
			[]vimtypes.ManagedObjectReference{
				{Value: "first"},
			},
			&vmopv1.VirtualMachineSetResourcePolicy{
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{ClusterModuleGroups: []string{"zero", "one"}},
				Status: vmopv1.VirtualMachineSetResourcePolicyStatus{ClusterModules: []vmopv1.VSphereClusterModuleStatus{
					{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
					{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
					{GroupName: "zero", ModuleUuid: "pre-existing-2", ClusterMoID: "second"},
					{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
				}},
			},
			[]vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
				{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
			},
			[]client.Object{},
			map[string]fakeClusterModule{
				"pre-existing-0": {clusterRef: "first"},
				"pre-existing-1": {clusterRef: "first"},
				"pre-existing-2": {clusterRef: "second"},
				"pre-existing-3": {clusterRef: "second"},
			},
		),
		Entry("delete cluster modules when group got removed got removed",
			[]vimtypes.ManagedObjectReference{
				{Value: "first"},
				{Value: "second"},
			},
			&vmopv1.VirtualMachineSetResourcePolicy{
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{ClusterModuleGroups: []string{"one"}},
				Status: vmopv1.VirtualMachineSetResourcePolicyStatus{ClusterModules: []vmopv1.VSphereClusterModuleStatus{
					{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
					{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
					{GroupName: "zero", ModuleUuid: "pre-existing-2", ClusterMoID: "second"},
					{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
				}},
			},
			[]vmopv1.VSphereClusterModuleStatus{
				{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
				{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
			},
			[]client.Object{},
			map[string]fakeClusterModule{
				"pre-existing-0": {clusterRef: "first"},
				"pre-existing-1": {clusterRef: "first"},
				"pre-existing-2": {clusterRef: "second"},
				"pre-existing-3": {clusterRef: "second"},
			},
		),
		Entry("recreate a cluster module when it does not exist anymore",
			[]vimtypes.ManagedObjectReference{
				{Value: "first"},
			},
			&vmopv1.VirtualMachineSetResourcePolicy{
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{ClusterModuleGroups: []string{"zero"}},
				Status: vmopv1.VirtualMachineSetResourcePolicyStatus{ClusterModules: []vmopv1.VSphereClusterModuleStatus{
					{GroupName: "zero", ModuleUuid: "not-existing-anymore-0", ClusterMoID: "first"},
				}},
			},
			[]vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "created-0", ClusterMoID: "first"},
			},
			[]client.Object{},
			map[string]fakeClusterModule{},
		),
	)
})

type fakeClusterModule struct {
	clusterRef string
}

type fakeClusterModuleProvider struct {
	modules map[string]fakeClusterModule
	nextID  int
}

func (p *fakeClusterModuleProvider) CreateModule(_ context.Context, clusterRef vimtypes.ManagedObjectReference) (string, error) {
	u := fmt.Sprintf("created-%d", p.nextID)
	p.nextID++
	p.modules[u] = fakeClusterModule{
		clusterRef: clusterRef.Value,
	}
	return u, nil
}

func (p *fakeClusterModuleProvider) DeleteModule(ctx context.Context, moduleID string) error {
	delete(p.modules, moduleID)
	return nil
}

func (p *fakeClusterModuleProvider) DoesModuleExist(ctx context.Context, moduleID string, cluster vimtypes.ManagedObjectReference) (bool, error) {
	got, ok := p.modules[moduleID]
	if !ok {
		return false, nil
	}
	return got.clusterRef == cluster.Value, nil
}

func (p *fakeClusterModuleProvider) IsMoRefModuleMember(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) (bool, error) {
	return false, errors.New("unimplemented")
}

func (p *fakeClusterModuleProvider) AddMoRefToModule(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) error {
	return errors.New("unimplemented")
}

func (p *fakeClusterModuleProvider) RemoveMoRefFromModule(ctx context.Context, moduleID string, moRef vimtypes.ManagedObjectReference) error {
	return errors.New("unimplemented")
}
