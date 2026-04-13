// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmservicee2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice/devops"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice/viadmin"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice/virtualmachine"
)

var _ = Describe("Testing VM Services", Label("devops"), Label("viadmin"), Label("virtualmachine"), Label("vmservice"), func() {
	Context("VI-ADMIN-VM-CLASS", func() {
		viadmin.VIAdminVMClassSpec(context.TODO(), func() viadmin.VIAdminVMClassSpecInput {
			return viadmin.VIAdminVMClassSpecInput{
				ClusterProxy: svClusterProxy,
				Config:       config,
				WCPClient:    wcpClient,
			}
		})
	})

	Context("VI-ADMIN-CL", func() {
		viadmin.VIAdminCLSpec(context.TODO(), func() viadmin.VIAdminCLSpecInput {
			return viadmin.VIAdminCLSpecInput{
				ClusterProxy:   svClusterProxy,
				Config:         config,
				WCPClient:      wcpClient,
				ArtifactFolder: artifactFolder,
				SkipCleanup:    skipCleanup,
			}
		})
	})

	Context("VI-ADMIN-RegisterVM", func() {
		viadmin.VIAdminRegisterVMSpec(context.TODO(), func() viadmin.VIAdminRegisterVMSpecInput {
			return viadmin.VIAdminRegisterVMSpecInput{
				ClusterProxy:     svClusterProxy,
				Config:           config,
				WCPClient:        wcpClient,
				WCPNamespaceName: wcpNamespaceName,
				LinuxVMName:      linuxVMName,
			}
		})
	})

	Context("DEVOPS-NS", func() {
		devops.DevOpsSpec(context.TODO(), func() devops.DevOpsSpecInput {
			return devops.DevOpsSpecInput{
				ClusterProxy:     svClusterProxy,
				Config:           config,
				WCPClient:        wcpClient,
				WCPNamespaceName: wcpNamespaceName,
			}
		})
	})

	Context("VIRTUAL-MACHINE", func() {
		Context("VM-LCM", func() {
			virtualmachine.VMSpec(context.TODO(), func() virtualmachine.VMSpecInput {
				return virtualmachine.VMSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					WCPClient:        wcpClient,
					ArtifactFolder:   artifactFolder,
					SkipCleanup:      skipCleanup,
					WCPNamespaceName: wcpNamespaceName,
					LinuxVMName:      linuxVMName,
				}
			})
		})

		Context("VM-LONGEVITY", func() {
			virtualmachine.VMLongevitySpec(context.TODO(), func() virtualmachine.VMLongevityInput {
				return virtualmachine.VMLongevityInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					WCPClient:        wcpClient,
					ArtifactFolder:   artifactFolder,
					SkipCleanup:      skipCleanup,
					WCPNamespaceName: wcpNamespaceName,
				}
			})
		})

		// Introduce vGPU in the context description so that we can skip vGPU related tests in TEST_SKIP

		Context("VM-GUEST-CUSTOMIZATION", func() {
			virtualmachine.VMGOSCSpec(context.TODO(), func() virtualmachine.VMGOSCSpecInput {
				return virtualmachine.VMGOSCSpecInput{
					ClusterProxy:        svClusterProxy,
					Config:              config,
					WCPClient:           wcpClient,
					ArtifactFolder:      artifactFolder,
					WCPNamespaceName:    wcpNamespaceName,
					WindowsServerVMName: windowsServerVMName,
				}
			})
		})

		Context("VM-MULTIPLE-CLUSTER", func() {
			virtualmachine.VMMultipleClusterSpec(context.TODO(), func() virtualmachine.VMMultipleClusterInput {
				return virtualmachine.VMMultipleClusterInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					ArtifactFolder:   artifactFolder,
					SkipCleanup:      skipCleanup,
					WCPNamespaceName: wcpNamespaceName,
				}
			})
		})

		Context("VM-PUBLISH-REQUEST", func() {
			virtualmachine.VMPublishRequestSpec(context.TODO(), func() virtualmachine.VMPublishRequestSpecInput {
				return virtualmachine.VMPublishRequestSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					WCPClient:        wcpClient,
					ArtifactFolder:   artifactFolder,
					SkipCleanup:      skipCleanup,
					WCPNamespaceName: wcpNamespaceName,
					LinuxVMName:      linuxVMName,
				}
			})
		})

		Context("VM-GROUP-PUBLISH-REQUEST", func() {
			virtualmachine.VMGroupPublishRequestSpec(context.TODO(), func() virtualmachine.VMGroupPublishRequestSpecInput {
				return virtualmachine.VMGroupPublishRequestSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					WCPClient:        wcpClient,
					ArtifactFolder:   artifactFolder,
					WCPNamespaceName: wcpNamespaceName,
				}
			})
		})

		Context("VM-WEB-CONSOLE-REQUEST", func() {
			virtualmachine.VMWebConsoleRequestSpec(context.TODO(), func() virtualmachine.VMWebConsoleRequestSpecInput {
				return virtualmachine.VMWebConsoleRequestSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					ArtifactFolder:   artifactFolder,
					SkipCleanup:      skipCleanup,
					WCPNamespaceName: wcpNamespaceName,
					LinuxVMName:      linuxVMName,
				}
			})
		})

		Context("VM-VPC", func() {
			virtualmachine.VMVPCSpec(context.TODO(), func() virtualmachine.VMVPCSpecInput {
				return virtualmachine.VMVPCSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					WCPClient:        wcpClient,
					ArtifactFolder:   artifactFolder,
					WCPNamespaceName: wcpNamespaceName,
				}
			})
		})

		Context("VM-ENCRYPTION", func() {
			virtualmachine.VMEncryptionSpec(context.TODO(), func() virtualmachine.VMEncryptionInput {
				return virtualmachine.VMEncryptionInput{
					ClusterProxy:   svClusterProxy,
					Config:         config,
					WCPClient:      wcpClient,
					ArtifactFolder: artifactFolder,
				}
			})
		})

		Context("VM-NETWORKING", func() {
			virtualmachine.VMNetworkSpec(context.TODO(), func() virtualmachine.VMNetworkSpecInput {
				return virtualmachine.VMNetworkSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					ArtifactFolder:   artifactFolder,
					WCPNamespaceName: wcpNamespaceName,
				}
			})
		})

		Context("VM-GROUP", func() {
			virtualmachine.VMGroupSpec(context.TODO(), func() virtualmachine.VMGroupSpecInput {
				return virtualmachine.VMGroupSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					WCPClient:        wcpClient,
					ArtifactFolder:   artifactFolder,
					WCPNamespaceName: wcpNamespaceName,
				}
			})
		})

		Context("VM-SNAPSHOT", func() {
			virtualmachine.VMSnapshotSpec(context.TODO(), func() virtualmachine.VMSnapshotSpecInput {
				return virtualmachine.VMSnapshotSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					WCPClient:        wcpClient,
					ArtifactFolder:   artifactFolder,
					WCPNamespaceName: wcpNamespaceName,
				}
			})
		})

		Context("VM-HARDWARE", func() {
			virtualmachine.VMHardwareSpec(context.TODO(), func() virtualmachine.VMHardwareSpecInput {
				return virtualmachine.VMHardwareSpecInput{
					ClusterProxy:     svClusterProxy,
					Config:           config,
					WCPClient:        wcpClient,
					ArtifactFolder:   artifactFolder,
					WCPNamespaceName: wcpNamespaceName,
				}
			})
		})
	})
})
