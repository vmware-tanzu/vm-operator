/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import "fmt"

/*
 * Configuration for a Vsphere VM Provider instance.  Contains information enabling integration with a backend
 * vSphere instance for VM management.
 */
type VSphereVmProviderConfig struct {
	VcUser       string
	VcPassword   string
	VcIP         string
	VcUrl        string
	Datacenter   string
	ResourcePool string
	Folder       string
	Datastore    string
}

func NewVsphereVmProviderConfig() *VSphereVmProviderConfig {

	vcUser := "Administrator@vsphere.local"
	//vcPassword := "Admin!23"
	vcPassword := "Vmoperator1!"
	vcIp := "10.161.164.210"

	vcUrl := fmt.Sprintf("https://%s:%s@%s", vcUser, vcPassword, vcIp)
	return &VSphereVmProviderConfig{
		VcUser:       vcUser,
		VcPassword:   vcPassword,
		VcIP:         vcIp,
		VcUrl:        vcUrl,
		Datacenter:   "Datacenter",
		ResourcePool: "Resources",
		Folder:       "vm",
		Datastore:    "datastore1",
	}
}
