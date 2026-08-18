package v1alpha5

import (
	"testing"

	. "github.com/onsi/gomega"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

func TestVirtualMachineSnapshotConversion(t *testing.T) {
	t.Run("status.disks round-trip conversion", func(t *testing.T) {
		g := NewWithT(t)

		hub := &vmopv1.VirtualMachineSnapshot{
			Status: vmopv1.VirtualMachineSnapshotStatus{
				Disks: []vmopv1.VirtualMachineSnapshotDiskStatus{
					{
						ID:                     "disk-1",
						ChangedBlockTrackingID: "cbt-1",
					},
					{
						ID:                     "disk-2",
						ChangedBlockTrackingID: "cbt-2",
					},
				},
			},
		}

		spoke := &vmopv1a5.VirtualMachineSnapshot{}

		// Convert from v1alpha6 to v1alpha5
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())

		// Disks should be preserved in annotations
		g.Expect(spoke.Annotations).NotTo(BeEmpty())

		// Convert back from v1alpha5 to v1alpha6
		hub2 := &vmopv1.VirtualMachineSnapshot{}
		g.Expect(spoke.ConvertTo(hub2)).To(Succeed())

		// Disks should be restored
		g.Expect(hub2.Status.Disks).To(HaveLen(2))
		g.Expect(hub2.Status.Disks[0].ID).To(Equal("disk-1"))
		g.Expect(hub2.Status.Disks[0].ChangedBlockTrackingID).To(Equal("cbt-1"))
		g.Expect(hub2.Status.Disks[1].ID).To(Equal("disk-2"))
		g.Expect(hub2.Status.Disks[1].ChangedBlockTrackingID).To(Equal("cbt-2"))
	})
}
