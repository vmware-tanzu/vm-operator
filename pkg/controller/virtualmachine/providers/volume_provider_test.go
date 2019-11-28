// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package providers

import (
	"context"

	ptr "github.com/kubernetes/utils/pointer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	cnsv1alpha1 "gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	virtualMachineName          = "volumeProviderTestVirtualMachine"
	virtualMachineNamespace     = "default"
	virtualMachineKind          = "VirtualMachine"
	virtualMachineClassName     = "small"
	virtualMachineImageName     = "capw-vm-image"
	virtualMachinePowerState    = "poweredOn"
	newVirtualMachineVolumeName = "newVirtualMachineVolumeName"
	attributeFirstClassDiskUUID = "diskUUID"
)

var _ = Describe("Volume Provider", func() {
	var (
		cnsVolumeProvider              *cnsVolumeProvider
		vm                             *vmoperatorv1alpha1.VirtualMachine
		s                              *runtime.Scheme
		seededSupervisorClusterObjects []runtime.Object
		supervisorClusterClient        client.Client
	)

	BeforeSuite(func() {
		// Set up the scheme.
		s = scheme.Scheme
		err := apis.AddToScheme(s)
		Expect(err).NotTo(HaveOccurred())
		err = cnsv1alpha1.SchemeBuilder.AddToScheme(s)
		Expect(err).NotTo(HaveOccurred())
		// Just set up empty objects by default, let the individual tests override this.
		seededSupervisorClusterObjects = []runtime.Object{}
	})
	JustBeforeEach(func() {
		// Reset each of the clients.
		// Unpack the initial objects into the fake clients.
		supervisorClusterClient = fake.NewFakeClientWithScheme(s, seededSupervisorClusterObjects...)
		cnsVolumeProvider = CnsVolumeProvider(supervisorClusterClient)
	})

	Describe("Unit tests for GetVmVolumesToProcess()", func() {
		Context("when new volumes under VirtualMachine object has been added", func() {
			It("should return the new set of volume object keys to be added and empty set of volume object keys to be deleted", func() {
				vm = generateDefaultVirtualMachine()
				vm.Spec.Volumes = append(vm.Spec.Volumes, generateVirtualMachineVolumes(newVirtualMachineVolumeName))
				vmVolumeToAdd, vmVolumeToDelete := GetVmVolumesToProcess(vm)
				Expect(len(vmVolumeToAdd)).To(Equal(1))
				Expect(vmVolumeToAdd[client.ObjectKey{Name: newVirtualMachineVolumeName, Namespace: virtualMachineNamespace}]).To(BeTrue())
				Expect(len(vmVolumeToDelete)).To(Equal(0))
			})
		})

		Context("when a volume under VirtualMachine object has been deleted", func() {
			It("should return an empty set of volume object keys to be added and a set of volume object keys to be deleted", func() {
				vm = generateDefaultVirtualMachine()
				vm.Spec.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumes{}
				vmVolumeToAdd, vmVolumeToDelete := GetVmVolumesToProcess(vm)
				Expect(len(vmVolumeToAdd)).To(Equal(0))
				Expect(len(vmVolumeToDelete)).To(Equal(1))
				Expect(vmVolumeToDelete[client.ObjectKey{Name: "alpha", Namespace: virtualMachineNamespace}]).To(BeTrue())
			})
		})

		Context("when a new volume under VirtualMachine object has been added, and an old volume has been deleted", func() {
			It("should return a set of volume object keys to be added and a set of volume object keys to be deleted", func() {
				vm = generateDefaultVirtualMachine()
				vm.Spec.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumes{
					generateVirtualMachineVolumes("beta"),
				}
				vmVolumeToAdd, vmVolumeToDelete := GetVmVolumesToProcess(vm)
				Expect(len(vmVolumeToAdd)).To(Equal(1))
				Expect(vmVolumeToAdd[client.ObjectKey{Name: "beta", Namespace: virtualMachineNamespace}]).To(BeTrue())
				Expect(len(vmVolumeToDelete)).To(Equal(1))
				Expect(vmVolumeToDelete[client.ObjectKey{Name: "alpha", Namespace: virtualMachineNamespace}]).To(BeTrue())
			})
		})
	})

	Describe("Unit tests for UpdateVmVolumesStatus()", func() {
		Context("when updating VirtualMachineVolumeStatus by checking the corresponding CnsNodeVmAttachment", func() {
			It("should return error if no CnsNodeVmAttachment could be found", func() {
				vm = generateDefaultVirtualMachine()
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())
				err = cnsVolumeProvider.UpdateVmVolumesStatus(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when updating VirtualMachineVolumeStatus by checking the corresponding CnsNodeVmAttachment", func() {
			It("should not return error if there is corresponding CnsNodeVmAttachment exits", func() {
				vm = generateDefaultVirtualMachine()
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())
				err = cnsVolumeProvider.client.Create(context.TODO(), generateAttachedCnsNodeVmAttachment(vm.Name, vm.Spec.Volumes[0].Name, vm.Namespace))
				Expect(err).NotTo(HaveOccurred())
				err = cnsVolumeProvider.UpdateVmVolumesStatus(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("if volume has been added into spec.volume, but the CnsNodeVmAttachment has not been created", func() {
			It("UpdateVmVolumesStatus() should not add the volume status", func() {
				vm = generateDefaultVirtualMachine()
				vm.Spec.Volumes = append(vm.Spec.Volumes, generateVirtualMachineVolumes("beta"))
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())

				generatedCnsNodeVmAttachment := generateAttachedCnsNodeVmAttachment(vm.Name, vm.Spec.Volumes[0].Name, vm.Namespace)
				err = cnsVolumeProvider.client.Create(context.TODO(), generatedCnsNodeVmAttachment)
				Expect(err).NotTo(HaveOccurred())

				err = cnsVolumeProvider.UpdateVmVolumesStatus(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vm.Status.Volumes)).To(Equal(1))
			})
		})

		Context("if volume has been deleted from spec.volume, but the CnsNodeVmAttachment has not been delete yet", func() {
			It("UpdateVmVolumesStatus() should not delete the volume status", func() {
				vm = generateDefaultVirtualMachine()
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())
				generatedCnsNodeVmAttachment := generateAttachedCnsNodeVmAttachment(vm.Name, vm.Spec.Volumes[0].Name, vm.Namespace)
				err = cnsVolumeProvider.client.Create(context.TODO(), generatedCnsNodeVmAttachment)
				Expect(err).NotTo(HaveOccurred())

				// Update the vm spec to remove the volumes, so there will be orphan volumes under vm.status.volumes
				vm.Spec.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumes{}
				err = cnsVolumeProvider.client.Update(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())

				// Update CnsNodeVmAttachment status to simulate the detach
				generatedCnsNodeVmAttachment.Status.Error = "Unable to detach the volume!"
				err = cnsVolumeProvider.client.Status().Update(context.TODO(), generatedCnsNodeVmAttachment)
				Expect(err).NotTo(HaveOccurred())

				vmRetrieved := &vmoperatorv1alpha1.VirtualMachine{}
				err = cnsVolumeProvider.client.Get(context.TODO(), client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace}, vmRetrieved)
				Expect(err).NotTo(HaveOccurred())
				err = cnsVolumeProvider.UpdateVmVolumesStatus(context.TODO(), vmRetrieved)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vmRetrieved.Status.Volumes)).To(Equal(1))
				Expect(vmRetrieved.Status.Volumes[0].Error).To(Equal("Unable to detach the volume!"))

			})
		})
	})

	Describe("Unit tests for AttachVolumes()", func() {
		Context("when a new volume is added", func() {
			It("should have no error, a new CnsNodeVmAttachment gets created, and VirtualMachineVolumeStatus is set to default value", func() {
				vm = generateDefaultVirtualMachine()
				vm.Status.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumeStatus{}
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())

				virtualMachineVolumesAdded, _ := GetVmVolumesToProcess(vm)
				err = cnsVolumeProvider.AttachVolumes(context.TODO(), vm, virtualMachineVolumesAdded)
				Expect(err).NotTo(HaveOccurred())

				cnsNodeVmAttachmentList := &cnsv1alpha1.CnsNodeVmAttachmentList{}
				err = cnsVolumeProvider.client.List(context.TODO(), &client.ListOptions{Namespace: vm.Namespace}, cnsNodeVmAttachmentList)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cnsNodeVmAttachmentList.Items)).To(Equal(1))

				for _, vmVolumeStatus := range vm.Status.Volumes {
					Expect(vmVolumeStatus.Error).To(Equal(""))
					Expect(vmVolumeStatus.DiskUuid).To(Equal(""))
					Expect(vmVolumeStatus.Attached).To(BeFalse())
				}
			})
		})

		Context("when multiple volumes are added", func() {
			It("should create multiple CnsNodeVmAttachment instances", func() {
				vm = generateDefaultVirtualMachine()
				vm.Spec.Volumes = append(vm.Spec.Volumes, generateVirtualMachineVolumes("beta"))
				// Remove default vm.status.volumes
				vm.Status.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumeStatus{}
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())

				virtualMachineVolumesAdded, _ := GetVmVolumesToProcess(vm)
				Expect(len(virtualMachineVolumesAdded)).To(Equal(2))

				err = cnsVolumeProvider.AttachVolumes(context.TODO(), vm, virtualMachineVolumesAdded)
				Expect(err).NotTo(HaveOccurred())

				// There should be multiple CnsNodeVmAttachment instances get created
				cnsNodeVmAttachmentList := &cnsv1alpha1.CnsNodeVmAttachmentList{}
				err = cnsVolumeProvider.client.List(context.TODO(), &client.ListOptions{}, cnsNodeVmAttachmentList)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cnsNodeVmAttachmentList.Items)).To(Equal(2))
			})
		})

		Context("when there is an existing CnsNodeVmAttachment instance", func() {
			It("should not return error, no additional CnsNodeVmAttachment instance gets created, and VirtualMachineVolumeStatus is set properly", func() {
				vm = generateDefaultVirtualMachine()
				vm.Spec.Volumes = append(vm.Spec.Volumes, generateVirtualMachineVolumes(newVirtualMachineVolumeName))
				vm.Status.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumeStatus{}
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())

				// Only create one CnsNodeVmAttachment instance with "attached": true
				virtualMachineVolumesAdded, _ := GetVmVolumesToProcess(vm)
				for virtualMachineVolume := range virtualMachineVolumesAdded {
					if virtualMachineVolume.Name == newVirtualMachineVolumeName {
						cnsNodeVmAttachment := generateAttachedCnsNodeVmAttachment(vm.Name, virtualMachineVolume.Name, virtualMachineVolume.Namespace)
						err = cnsVolumeProvider.client.Create(context.TODO(), cnsNodeVmAttachment)
						Expect(err).NotTo(HaveOccurred())
						break
					}
				}

				err = cnsVolumeProvider.AttachVolumes(context.TODO(), vm, virtualMachineVolumesAdded)
				Expect(err).NotTo(HaveOccurred())

				cnsNodeVmAttachmentList := &cnsv1alpha1.CnsNodeVmAttachmentList{}
				err = cnsVolumeProvider.client.List(context.TODO(), &client.ListOptions{Namespace: vm.Namespace}, cnsNodeVmAttachmentList)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cnsNodeVmAttachmentList.Items)).To(Equal(2))

				// Expect only the matched virtualMachineVolumeStatus has "attached": true
				for _, virtualMachineVolumeStatus := range vm.Status.Volumes {
					if virtualMachineVolumeStatus.Name == newVirtualMachineVolumeName {
						Expect(virtualMachineVolumeStatus.Attached).To(BeTrue())
					} else {
						Expect(virtualMachineVolumeStatus.Attached).To(BeFalse())
					}
				}
			})
		})

		Context("when no new volumes need to be added", func() {
			It("should have no error", func() {
				vm = generateDefaultVirtualMachine()
				vm.Spec.Volumes = nil
				vm.Status.Volumes = nil
				virtualMachineVolumesAdded, _ := GetVmVolumesToProcess(vm)
				Expect(len(virtualMachineVolumesAdded)).To(Equal(0))

				err := cnsVolumeProvider.AttachVolumes(context.TODO(), vm, virtualMachineVolumesAdded)
				Expect(err).NotTo(HaveOccurred())

			})
		})

	})

	Describe("Unit tests for DetachVolumes()", func() {
		Context("when a volume needs to be detached", func() {
			It("should have no error if the respective CnsNodeVmAttachment instance does not exist", func() {
				vm = generateDefaultVirtualMachine()
				vm.Spec.Volumes = nil
				_, virtualMachineVolumesToDetach := GetVmVolumesToProcess(vm)
				Expect(len(virtualMachineVolumesToDetach)).To(Equal(1))

				err := cnsVolumeProvider.DetachVolumes(context.TODO(), vm, virtualMachineVolumesToDetach)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when a volume need to be detached", func() {
			It("should have no error returned and the respective CnsNodeVmAttachment instance is deleted", func() {
				vm = generateDefaultVirtualMachine()
				cnsNodeVmAttachment := generateAttachedCnsNodeVmAttachment(vm.Name, vm.Spec.Volumes[0].Name, vm.Namespace)
				err := cnsVolumeProvider.client.Create(context.TODO(), cnsNodeVmAttachment)
				Expect(err).NotTo(HaveOccurred())

				vm.Spec.Volumes = nil
				_, virtualMachineVolumesToDetach := GetVmVolumesToProcess(vm)
				Expect(len(virtualMachineVolumesToDetach)).To(Equal(1))

				err = cnsVolumeProvider.DetachVolumes(context.TODO(), vm, virtualMachineVolumesToDetach)
				Expect(err).NotTo(HaveOccurred())

				cnsNodeVmAttachmentReceived := &cnsv1alpha1.CnsNodeVmAttachment{}
				err = cnsVolumeProvider.client.Get(context.TODO(),
					client.ObjectKey{Name: cnsNodeVmAttachment.Name, Namespace: cnsNodeVmAttachment.Namespace},
					cnsNodeVmAttachmentReceived,
				)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("when no volumes need to be detached", func() {
			It("should have no error returned", func() {
				vm = generateDefaultVirtualMachine()
				_, virtualMachineVolumesToDetach := GetVmVolumesToProcess(vm)
				Expect(len(virtualMachineVolumesToDetach)).To(Equal(0))

				err := cnsVolumeProvider.DetachVolumes(context.TODO(), vm, virtualMachineVolumesToDetach)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})

func generateAttachedCnsNodeVmAttachment(vmName string, vmVolumeName string, vmVolumeNamespace string) *cnsv1alpha1.CnsNodeVmAttachment {
	cnsNodeVmAttachment := &cnsv1alpha1.CnsNodeVmAttachment{}
	cnsNodeVmAttachment.SetName(constructCnsNodeVmAttachmentName(vmName, vmVolumeName))
	cnsNodeVmAttachment.SetNamespace(vmVolumeNamespace)
	cnsNodeVmAttachment.Spec.VolumeName = vmVolumeName
	cnsNodeVmAttachment.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         cnsNodeVmAttachmentOwnerRefVersion,
			Controller:         ptr.BoolPtr(true),
			BlockOwnerDeletion: ptr.BoolPtr(true),
			Kind:               vmoperatorv1alpha1.VirtualMachine{}.TypeMeta.Kind,
			Name:               vmName,
		},
	}
	cnsNodeVmAttachment.Status = cnsv1alpha1.CnsNodeVmAttachmentStatus{
		Attached: true,
		Error:    "",
		AttachmentMetadata: map[string]string{
			attributeFirstClassDiskUUID: "123-123-123-123",
		},
	}

	return cnsNodeVmAttachment
}

func generateVirtualMachineVolumes(name string) vmoperatorv1alpha1.VirtualMachineVolumes {
	return vmoperatorv1alpha1.VirtualMachineVolumes{
		Name: name,
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: "pvc-claim" + name,
			ReadOnly:  false,
		},
	}
}

func generateVirtualMachineVolumeStatus(name string) vmoperatorv1alpha1.VirtualMachineVolumeStatus {
	return vmoperatorv1alpha1.VirtualMachineVolumeStatus{
		Name:     name,
		Attached: false,
		Error:    "",
	}
}

func generateDefaultVirtualMachine() *vmoperatorv1alpha1.VirtualMachine {
	return &vmoperatorv1alpha1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			Kind: virtualMachineKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtualMachineName,
			Namespace: virtualMachineNamespace,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSpec{
			ClassName:  virtualMachineClassName,
			ImageName:  virtualMachineImageName,
			PowerState: virtualMachinePowerState,
			Volumes: []vmoperatorv1alpha1.VirtualMachineVolumes{
				generateVirtualMachineVolumes("alpha"),
			},
		},
		Status: vmoperatorv1alpha1.VirtualMachineStatus{
			Volumes: []vmoperatorv1alpha1.VirtualMachineVolumeStatus{
				generateVirtualMachineVolumeStatus("alpha"),
			},
		},
	}
}
