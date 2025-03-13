// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

func Test_vSphereVMProvider_reconcileClusterModules(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = vmopv1.AddToScheme(scheme)

	tests := []struct {
		name                     string
		objs                     []client.Object
		existingModules          map[string]fakeClusterModule
		computeClusterReferences []vimtypes.ManagedObjectReference
		resourcePolicy           *vmopv1.VirtualMachineSetResourcePolicy
		wantClusterModuleStatus  []vmopv1.VSphereClusterModuleStatus
		wantErr                  bool
	}{
		{
			name:                     "don't create a module when having no clusterref",
			objs:                     []client.Object{},
			existingModules:          map[string]fakeClusterModule{},
			computeClusterReferences: []vimtypes.ManagedObjectReference{},
			resourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModuleGroups: []string{"zero"},
			}},
			wantClusterModuleStatus: nil,
			wantErr:                 false,
		},
		{
			name:                     "create a module when having a clusterref",
			objs:                     []client.Object{},
			existingModules:          map[string]fakeClusterModule{},
			computeClusterReferences: []vimtypes.ManagedObjectReference{{Value: "first"}},
			resourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModuleGroups: []string{"zero"},
			}},
			wantClusterModuleStatus: []vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "created-0", ClusterMoID: "first"},
			},
			wantErr: false,
		},
		{
			name:            "create multiple modules when having multiple clusterrefs",
			objs:            []client.Object{},
			existingModules: map[string]fakeClusterModule{},
			computeClusterReferences: []vimtypes.ManagedObjectReference{
				{Value: "first"},
				{Value: "second"},
			},
			resourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModuleGroups: []string{"zero", "one"},
			}},
			wantClusterModuleStatus: []vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "created-0", ClusterMoID: "first"},
				{GroupName: "one", ModuleUuid: "created-1", ClusterMoID: "first"},
				{GroupName: "zero", ModuleUuid: "created-2", ClusterMoID: "second"},
				{GroupName: "one", ModuleUuid: "created-3", ClusterMoID: "second"},
			},
			wantErr: false,
		},
		{
			name: "preserve when no changes",
			objs: []client.Object{},
			existingModules: map[string]fakeClusterModule{
				"pre-existing-0": {clusterRef: "first"},
				"pre-existing-1": {clusterRef: "first"},
				"pre-existing-2": {clusterRef: "second"},
				"pre-existing-3": {clusterRef: "second"},
			},
			computeClusterReferences: []vimtypes.ManagedObjectReference{
				{Value: "first"},
				{Value: "second"},
			},
			resourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{ClusterModuleGroups: []string{"zero", "one"}},
				Status: vmopv1.VirtualMachineSetResourcePolicyStatus{ClusterModules: []vmopv1.VSphereClusterModuleStatus{
					{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
					{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
					{GroupName: "zero", ModuleUuid: "pre-existing-2", ClusterMoID: "second"},
					{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
				}},
			},
			wantClusterModuleStatus: []vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
				{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
				{GroupName: "zero", ModuleUuid: "pre-existing-2", ClusterMoID: "second"},
				{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
			},
			wantErr: false,
		},
		{
			name: "delete cluster modules when cluster reference got removed",
			objs: []client.Object{},
			existingModules: map[string]fakeClusterModule{
				"pre-existing-0": {clusterRef: "first"},
				"pre-existing-1": {clusterRef: "first"},
				"pre-existing-2": {clusterRef: "second"},
				"pre-existing-3": {clusterRef: "second"},
			},
			computeClusterReferences: []vimtypes.ManagedObjectReference{
				{Value: "first"},
			},
			resourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{ClusterModuleGroups: []string{"zero", "one"}},
				Status: vmopv1.VirtualMachineSetResourcePolicyStatus{ClusterModules: []vmopv1.VSphereClusterModuleStatus{
					{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
					{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
					{GroupName: "zero", ModuleUuid: "pre-existing-2", ClusterMoID: "second"},
					{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
				}},
			},
			wantClusterModuleStatus: []vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
				{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
			},
			wantErr: false,
		},
		{
			name: "delete cluster modules when group got removed got removed",
			objs: []client.Object{},
			existingModules: map[string]fakeClusterModule{
				"pre-existing-0": {clusterRef: "first"},
				"pre-existing-1": {clusterRef: "first"},
				"pre-existing-2": {clusterRef: "second"},
				"pre-existing-3": {clusterRef: "second"},
			},
			computeClusterReferences: []vimtypes.ManagedObjectReference{
				{Value: "first"},
				{Value: "second"},
			},
			resourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{ClusterModuleGroups: []string{"one"}},
				Status: vmopv1.VirtualMachineSetResourcePolicyStatus{ClusterModules: []vmopv1.VSphereClusterModuleStatus{
					{GroupName: "zero", ModuleUuid: "pre-existing-0", ClusterMoID: "first"},
					{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
					{GroupName: "zero", ModuleUuid: "pre-existing-2", ClusterMoID: "second"},
					{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
				}},
			},
			wantClusterModuleStatus: []vmopv1.VSphereClusterModuleStatus{
				{GroupName: "one", ModuleUuid: "pre-existing-1", ClusterMoID: "first"},
				{GroupName: "one", ModuleUuid: "pre-existing-3", ClusterMoID: "second"},
			},
			wantErr: false,
		},
		{
			name:            "create cluster module when it does not exist anymore",
			objs:            []client.Object{},
			existingModules: map[string]fakeClusterModule{},
			computeClusterReferences: []vimtypes.ManagedObjectReference{
				{Value: "first"},
			},
			resourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{
				Spec: vmopv1.VirtualMachineSetResourcePolicySpec{ClusterModuleGroups: []string{"zero"}},
				Status: vmopv1.VirtualMachineSetResourcePolicyStatus{ClusterModules: []vmopv1.VSphereClusterModuleStatus{
					{GroupName: "zero", ModuleUuid: "not-existing-anymore-0", ClusterMoID: "first"},
				}},
			},
			wantClusterModuleStatus: []vmopv1.VSphereClusterModuleStatus{
				{GroupName: "zero", ModuleUuid: "created-0", ClusterMoID: "first"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			clusterModProvider := &fakeClusterModuleProvider{
				modules: tt.existingModules,
			}
			vs := &vSphereVMProvider{
				k8sClient: fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&vmopv1.VirtualMachineSetResourcePolicy{}).
					WithObjects(tt.objs...).Build(),
			}

			g.Expect(vs.reconcileClusterModules(ctx, clusterModProvider, tt.computeClusterReferences, tt.resourcePolicy)).ToNot(HaveOccurred())
			g.Expect(tt.resourcePolicy.Status.ClusterModules).To(BeEquivalentTo(tt.wantClusterModuleStatus))
			g.Expect(clusterModProvider.modules).To(HaveLen(len(tt.resourcePolicy.Status.ClusterModules)))
		})
	}
}

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
