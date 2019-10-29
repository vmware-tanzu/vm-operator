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
				Expect(err).To(HaveOccurred())
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

		Context("when updating VirtualMachineVolumeStatus", func() {
			It("the orphan VirtualMachineVolumeStatus should be deleted", func() {
				vm = generateDefaultVirtualMachine()
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())
				err = cnsVolumeProvider.client.Create(context.TODO(), generateAttachedCnsNodeVmAttachment(vm.Name, vm.Spec.Volumes[0].Name, vm.Namespace))
				Expect(err).NotTo(HaveOccurred())

				// Update the vm spec to remove the volumes, so there will be orphan volumes under vm.status.volumes
				vm.Spec.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumes{}
				err = cnsVolumeProvider.client.Update(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())

				vmRetrieved := &vmoperatorv1alpha1.VirtualMachine{}
				err = cnsVolumeProvider.client.Get(context.TODO(), client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace}, vmRetrieved)
				Expect(err).NotTo(HaveOccurred())
				err = cnsVolumeProvider.UpdateVmVolumesStatus(context.TODO(), vmRetrieved)
				Expect(err).NotTo(HaveOccurred())

				vmRetrieved = &vmoperatorv1alpha1.VirtualMachine{}
				err = cnsVolumeProvider.client.Get(context.TODO(), client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace}, vmRetrieved)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vmRetrieved.Status.Volumes)).To(Equal(0))

			})
		})
	})

	Describe("Unit tests for CreateCnsNodeVmAttachments()", func() {
		Context("when creating CnsNodeVmAttachment", func() {
			It("should not return error if there are new volumes added", func() {
				vm = generateDefaultVirtualMachine()
				vm.Status.Volumes = []vmoperatorv1alpha1.VirtualMachineVolumeStatus{}
				err := cnsVolumeProvider.client.Create(context.TODO(), vm)
				Expect(err).NotTo(HaveOccurred())

				virtualMachineVolumesAdded, _ := GetVmVolumesToProcess(vm)
				err = cnsVolumeProvider.AttachVolumes(context.TODO(), vm, virtualMachineVolumesAdded)
				Expect(err).NotTo(HaveOccurred())

				vmRetrieved := &vmoperatorv1alpha1.VirtualMachine{}
				err = cnsVolumeProvider.client.Get(context.TODO(), client.ObjectKey{Name: vm.Name, Namespace: vm.Namespace}, vmRetrieved)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vmRetrieved.Status.Volumes)).To(Equal(1))

				cnsNodeVmAttachmentList := &cnsv1alpha1.CnsNodeVmAttachmentList{}
				err = cnsVolumeProvider.client.List(context.TODO(), &client.ListOptions{Namespace: vm.Namespace}, cnsNodeVmAttachmentList)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cnsNodeVmAttachmentList.Items)).To(Equal(1))
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
