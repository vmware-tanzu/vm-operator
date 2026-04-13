// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package csi

import (
	"context"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	cnsoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// WaitForBatchAttachVolumesToBeAttached waits for the CnsNodeVMBatchAttachment
// resource to exist and for the specified volumes to be marked as attached.
func WaitForBatchAttachVolumesToBeAttached(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string,
	volumeNames []string,
) bool {
	return Eventually(func() bool {
		batchAttachment, err := utils.GetCnsNodeVMBatchAttachment(ctx, client, ns, name)
		if err != nil {
			e2eframework.Logf("error getting CnsNodeVMBatchAttachment. retry due to: %v", err)
			return false
		}

		batchAttachmentVolAttached := make(map[string]bool)

		for _, volStatus := range batchAttachment.Status.VolumeStatus {
			volName := volStatus.Name
			if volName == "" {
				e2eframework.Logf("volume name is empty. retrying")
				return false
			}

			// Check if volume is attached by looking at conditions
			for _, condition := range volStatus.PersistentVolumeClaim.Conditions {
				if condition.Type == cnsoperatorv1alpha1.ConditionAttached {
					if condition.Status == metav1.ConditionTrue {
						batchAttachmentVolAttached[volName] = true
					} else {
						batchAttachmentVolAttached[volName] = false
					}

					break
				}
			}
		}

		volumeNamesSet := make(map[string]bool)
		for _, volumeName := range volumeNames {
			volumeNamesSet[volumeName] = true

			attached, ok := batchAttachmentVolAttached[volumeName]
			if !ok {
				e2eframework.Logf("volume %s is not found in the batch attachment. retrying", volumeName)
				return false
			}

			if !attached {
				e2eframework.Logf("volume %s is not attached. retrying", volumeName)
				return false
			}
		}

		// Verify that the volume name lists are exactly equal.
		if len(volumeNamesSet) != len(batchAttachmentVolAttached) {
			e2eframework.Logf("volume names count not equal. retrying: %d != %d",
				len(volumeNamesSet), len(batchAttachmentVolAttached))

			return false
		}

		for volumeName := range batchAttachmentVolAttached {
			if !volumeNamesSet[volumeName] {
				e2eframework.Logf("volume names are not equal. extra volume in batch attachment: %s", volumeName)
				return false
			}
		}

		return true
	},
		config.GetIntervals("default", "wait-virtual-machine-cns-node-batch-attachment-creation")...).
		Should(BeTrue(), "Timed out waiting for CnsNodeVMBatchAttachment %s/%s to exist", ns, name)
}

func WaitForCnsNodeVMBatchAttachmentToBeDeleted(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string,
) {
	Eventually(func(g Gomega) {
		_, err := utils.GetCnsNodeVMBatchAttachment(ctx, client, ns, name)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}, config.GetIntervals("default", "wait-virtual-machine-cns-node-batch-attachment-deletion")...).
		Should(Succeed(), "Timed out waiting for CnsNodeVMBatchAttachment %s/%s to be deleted", ns, name)
}
