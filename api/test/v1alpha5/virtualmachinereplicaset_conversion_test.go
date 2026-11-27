// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

func TestVirtualMachineReplicaSetConversion(t *testing.T) {
	newHub := func() *vmopv1.VirtualMachineReplicaSet {
		return &vmopv1.VirtualMachineReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "replicaset"},
			Spec: vmopv1.VirtualMachineReplicaSetSpec{
				Replicas: ptr.To(int32(1)),
				Template: vmopv1.VirtualMachineTemplateSpec{
					Spec: vmopv1.VirtualMachineSpec{
						Volumes: []vmopv1.VirtualMachineVolume{
							{
								Name: "snapshot-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
										Name: "snapshot", DiskID: "disk",
									},
								},
							},
							{
								Name: "pvc-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "claim",
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}

	t.Run("snapshot and PVC volumes round trip", func(t *testing.T) {
		g := NewWithT(t)
		hub := newHub()
		spoke := &vmopv1a5.VirtualMachineReplicaSet{}
		after := &vmopv1.VirtualMachineReplicaSet{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(apiequality.Semantic.DeepEqual(after, hub)).To(BeTrue(), cmp.Diff(hub, after))
	})

	t.Run("preserves edits through v1alpha5 and restores sources by name", func(t *testing.T) {
		g := NewWithT(t)
		hub := newHub()
		spoke := &vmopv1a5.VirtualMachineReplicaSet{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		spoke.Spec.Replicas = ptr.To(int32(3))
		volumes := spoke.Spec.Template.Spec.Volumes
		volumes[0], volumes[1] = volumes[1], volumes[0]
		volumes[0].PersistentVolumeClaim.ClaimName = "updated-claim"
		after := &vmopv1.VirtualMachineReplicaSet{}
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(*after.Spec.Replicas).To(Equal(int32(3)))
		g.Expect(after.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal("updated-claim"))
		g.Expect(after.Spec.Template.Spec.Volumes[0].VirtualMachineSnapshotDisk).To(BeNil())
		g.Expect(after.Spec.Template.Spec.Volumes[1].VirtualMachineSnapshotDisk).
			To(Equal(hub.Spec.Template.Spec.Volumes[0].VirtualMachineSnapshotDisk))
	})

	t.Run("does not restore removed volumes", func(t *testing.T) {
		g := NewWithT(t)
		spoke := &vmopv1a5.VirtualMachineReplicaSet{}
		g.Expect(spoke.ConvertFrom(newHub())).To(Succeed())
		spoke.Spec.Template.Spec.Volumes = spoke.Spec.Template.Spec.Volumes[1:]
		after := &vmopv1.VirtualMachineReplicaSet{}
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(after.Spec.Template.Spec.Volumes).To(HaveLen(1))
		g.Expect(after.Spec.Template.Spec.Volumes[0].Name).To(Equal("pvc-volume"))
	})

	t.Run("converts without preserved hub data", func(t *testing.T) {
		g := NewWithT(t)
		spoke := &vmopv1a5.VirtualMachineReplicaSet{
			Spec: vmopv1a5.VirtualMachineReplicaSetSpec{Replicas: ptr.To(int32(2))},
		}
		after := &vmopv1.VirtualMachineReplicaSet{}
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(*after.Spec.Replicas).To(Equal(int32(2)))
	})

	t.Run("rejects malformed preserved hub data", func(t *testing.T) {
		g := NewWithT(t)
		spoke := &vmopv1a5.VirtualMachineReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{utilconversion.AnnotationKey: "{"}},
		}
		g.Expect(spoke.ConvertTo(&vmopv1.VirtualMachineReplicaSet{})).NotTo(Succeed())
	})

	t.Run("list conversion preserves snapshot sources", func(t *testing.T) {
		g := NewWithT(t)
		hub := &vmopv1.VirtualMachineReplicaSetList{
			ListMeta: metav1.ListMeta{ResourceVersion: "42"},
			Items:    []vmopv1.VirtualMachineReplicaSet{*newHub()},
		}
		spoke := &vmopv1a5.VirtualMachineReplicaSetList{}
		after := &vmopv1.VirtualMachineReplicaSetList{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(apiequality.Semantic.DeepEqual(after, hub)).To(BeTrue(), cmp.Diff(hub, after))
	})
}
