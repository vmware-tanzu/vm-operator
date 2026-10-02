// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Secret struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// GetSecretYamlCloudConfig returns a Secret whose user-data is a plaintext
// cloud-init configuration, the same content as GetConfigMapYamlGOSC.
func GetSecretYamlCloudConfig(secret Secret) []byte {
	return ToYAML(&corev1.Secret{
		TypeMeta:   typeMeta("v1", "Secret"),
		ObjectMeta: secretObjectMeta(secret),
		StringData: map[string]string{
			"user-data": goscCloudConfig,
		},
	})
}

// seedDataCloudConfig is the cloud-init user-data of
// GetSecretYamlCloudConfigSeedData.
const seedDataCloudConfig = `#cloud-config
ssh_pwauth: true
users:
  - name: vmware
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: false
    # Password set to Admin!23
    passwd: '$1$salt$SOC33fVbA/ZxeIwD5yw1u1'
    shell: /bin/bash
write_files:
  # Seeds random data on the boot disk and on the first non-boot disk
  # and records its checksums in /var/lib/vmop-seed.sha256, so a
  # restore test can prove the data came back. The data disk is only
  # formatted when it is blank, so a re-run never wipes restored data.
  # The seed is owned by the vmware user so the test can check and
  # delete it without sudo, which some images do not ship. A restored
  # VM may get a new instance ID, which makes cloud-init run this
  # again, so it never re-seeds a disk that already holds a seed:
  # new data and checksums would hide a restore that lost the data.
  - path: /usr/local/bin/vmop-seed.sh
    permissions: '0755'
    content: |
      #!/bin/bash
      [ -f /var/lib/vmop-seed.done ] && exit 0
      set -eux
      ROOTDISK=$(lsblk -no PKNAME "$(findmnt -no SOURCE /)")
      DATA=$(lsblk -dn -o NAME,TYPE | awk -v r="$ROOTDISK" \
        '$2=="disk" && $1!=r {print $1; exit}')
      blkid "/dev/$DATA" || mkfs.ext4 -F -L vmopdata "/dev/$DATA"
      mkdir -p /mnt/data
      grep -q vmopdata /etc/fstab || \
        echo "LABEL=vmopdata /mnt/data ext4 defaults,nofail 0 2" \
        >> /etc/fstab
      mountpoint -q /mnt/data || mount /mnt/data
      mkdir -p /var/lib/vmop-seed
      head -c 8M /dev/urandom > /var/lib/vmop-seed/boot.bin
      echo "boot $(date +%s%N)" > /var/lib/vmop-seed/boot.txt
      head -c 8M /dev/urandom > /mnt/data/data.bin
      echo "data $(date +%s%N)" > /mnt/data/data.txt
      sha256sum /var/lib/vmop-seed/boot.* /mnt/data/data.* \
        > /var/lib/vmop-seed.sha256
      chown -R vmware /var/lib/vmop-seed /mnt/data
      sync
      touch /var/lib/vmop-seed.done
runcmd:
  - [/usr/local/bin/vmop-seed.sh]
`

// GetSecretYamlCloudConfigSeedData returns a cloud-config Secret that, in
// addition to the default user, seeds checksummed random data on the boot disk
// and on the first data disk.
func GetSecretYamlCloudConfigSeedData(secret Secret) []byte {
	return ToYAML(&corev1.Secret{
		TypeMeta:   typeMeta("v1", "Secret"),
		ObjectMeta: secretObjectMeta(secret),
		StringData: map[string]string{
			"user-data": seedDataCloudConfig,
		},
	})
}

// GetSecretYamlInlineCloudInitData returns a Secret with inline data
// referenced by cloud-init bootstrap secret key selectors.
func GetSecretYamlInlineCloudInitData(secret Secret) []byte {
	return ToYAML(&corev1.Secret{
		TypeMeta:   typeMeta("v1", "Secret"),
		ObjectMeta: secretObjectMeta(secret),
		StringData: map[string]string{
			// Password set to Admin!23
			"vmsvc-pwd": `$1$salt$SOC33fVbA/ZxeIwD5yw1u1`,
			"hello":     "Hello World!",
		},
	})
}

// GetSecretYamlInlineSysprepData returns a Secret with inline data
// referenced by sysprep bootstrap secret key selectors.
func GetSecretYamlInlineSysprepData(secret Secret) []byte {
	return ToYAML(&corev1.Secret{
		TypeMeta:   typeMeta("v1", "Secret"),
		ObjectMeta: secretObjectMeta(secret),
		StringData: map[string]string{
			"vmsvc-pwd": "vmware",
		},
	})
}

// GetSecretYamlOvfEnv returns a Secret whose user-data is the same
// cloud-init configuration as GetSecretYamlCloudConfig, but base64-encoded,
// as vSphere delivers it through the guestinfo.ovfEnv OVF property.
func GetSecretYamlOvfEnv(secret Secret) []byte {
	return ToYAML(&corev1.Secret{
		TypeMeta:   typeMeta("v1", "Secret"),
		ObjectMeta: secretObjectMeta(secret),
		StringData: map[string]string{
			"user-data": base64.StdEncoding.EncodeToString([]byte(goscCloudConfig)),
		},
	})
}

// GetSecretYamlVAppConfig returns a Secret whose values are vApp property
// template expressions evaluated by VM Operator's guest customization
// engine, not by this package. They are literal strings here.
func GetSecretYamlVAppConfig(secret Secret) []byte {
	return ToYAML(&corev1.Secret{
		TypeMeta:   typeMeta("v1", "Secret"),
		ObjectMeta: secretObjectMeta(secret),
		StringData: map[string]string{
			"nameservers":        `{{ V1alpha1_FormatNameservers 2 "," }}`,
			"hostname":           `{{ .V1alpha1.VM.Name }}`,
			"management_ip":      `{{ V1alpha1_FirstIP }}`,
			"management_gateway": `{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}`,
		},
	})
}

// GetSecretYamlSysprepConfig returns a Secret whose unattend value is a
// sysprep answer file with template expressions evaluated by VM Operator's
// guest customization engine, not by this package. It is a literal string
// here.
func GetSecretYamlSysprepConfig(secret Secret) []byte {
	return ToYAML(&corev1.Secret{
		TypeMeta:   typeMeta("v1", "Secret"),
		ObjectMeta: secretObjectMeta(secret),
		StringData: map[string]string{
			"unattend": sysprepUnattend,
		},
	})
}

const sysprepUnattend = `<?xml version="1.0" encoding="UTF-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend">
  <settings pass="specialize">
    <component name="Microsoft-Windows-TCPIP" processorArchitecture="amd64"
      publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS"
      xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
      xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <Interfaces>
        <Interface wcm:action="add">
          <Ipv4Settings>
            <DhcpEnabled>false</DhcpEnabled>
          </Ipv4Settings>
          <Ipv6Settings>
            <DhcpEnabled>false</DhcpEnabled>
          </Ipv6Settings>
          <Identifier>{{ V1alpha1_FirstNicMacAddr }}</Identifier>
          <UnicastIpAddresses>
            <IpAddress wcm:action="add" wcm:keyValue="1">{{ V1alpha1_FirstIP }}</IpAddress>
          </UnicastIpAddresses>
          <Routes>
            <Route wcm:action="add">
              <Identifier>0</Identifier>
              <Metric>10</Metric>
              <NextHopAddress>{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}</NextHopAddress>
              <Prefix>{{ V1alpha1_SubnetMask V1alpha1_FirstIP }}</Prefix>
            </Route>
          </Routes>
        </Interface>
      </Interfaces>
    </component>
    <component name="Microsoft-Windows-DNS-Client" processorArchitecture="amd64"
      publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS"
      xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
      xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <Interfaces>
        <Interface wcm:action="add">
          <Identifier>{{ V1alpha1_FirstNicMacAddr }}</Identifier>
          <DNSServerSearchOrder> {{ range .V1alpha1.Net.Nameservers }} <IpAddress
              wcm:action="add"
              wcm:keyValue="{{.}}"></IpAddress> {{ end }} </DNSServerSearchOrder>
        </Interface>
      </Interfaces>
    </component>
    <component name="Microsoft-Windows-Deployment" processorArchitecture="amd64"
      publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS"
      xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
      xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <RunSynchronous>
        <RunSynchronousCommand wcm:action="add">
          <Path>C:\sysprep\guestcustutil.exe restoreMountedDevices</Path>
          <Order>1</Order>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Path>C:\sysprep\guestcustutil.exe flagComplete</Path>
          <Order>2</Order>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Path>C:\sysprep\guestcustutil.exe deleteContainingFolder</Path>
          <Order>3</Order>
        </RunSynchronousCommand>
      </RunSynchronous>
    </component>
  </settings>
</unattend>
`

func secretObjectMeta(secret Secret) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}
}
