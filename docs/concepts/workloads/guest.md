# Customizing a Guest

The ability to deploy a virtual machine with Kubernetes is nice, but one of the values of VM Operator is its support for popular bootstrap providers such as Cloud-Init, Sysprep, and vAppConfig. This page reviews these bootstrap providers to help inform when to select one over the other.

## Bootstrap Providers

There are a number of methods that may be used to bootstrap a virtual machine's (VM) guest operating system:

| Provider                    | Supported    | Network Config                | Linux | Windows | Description |
|-----------------------------|:------------:|-------------------------------|:-----:|:-------:|-------------|
| [Cloud-Init](#cloud-init)   |       ✓      | [Cloud-Init Network v2](https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html) |   ✓   |     ✓    | The industry standard, multi-distro method for cross-platform, cloud instance initialization with modern, VM images |
| [Sysprep](#sysprep)         |       ✓      | [Guest OS Customization](https://vdc-download.vmware.com/vmwb-repository/dcr-public/c476b64b-c93c-4b21-9d76-be14da0148f9/04ca12ad-59b9-4e1c-8232-fd3d4276e52c/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.customization.Specification.html) (GOSC) |       |     ✓    | Microsoft Sysprep is used by VMware to customize Windows images on first-boot |
| [vAppConfig](#vappconfig)   |       ✓      | Bespoke                       |   ✓   |         | For images with bespoke, bootstrap engines driven by vAppConfig properties |
| [OvfEnv](#ovfenv)           | _deprecated_ | [Guest OS Customization](https://vdc-download.vmware.com/vmwb-repository/dcr-public/c476b64b-c93c-4b21-9d76-be14da0148f9/04ca12ad-59b9-4e1c-8232-fd3d4276e52c/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.customization.Specification.html) (GOSC) |   ✓   |         | A combination of GOSC and Cloud-Init user-data |
| [ExtraConfig](#extraconfig) | _deprecated_ | GOSC                          |   ✓   |         | For images with bespoke, bootstrap engines that rely on Guest Info data |

!!! note "`ConfigMap` or `Secret`"

    The choice of a `ConfigMap` or `Secret` resource in no way impacts the choice or behavior of the selected bootstrap provider. When VM Operator was first released, the only way to store bootstrap data was via a `ConfigMap` resource. While this still works, it is not recommended as data stored in a `ConfigMap` is not encrypted at rest. Instead, it is recommended users switch to using `Secret` resources for storing bootstrap data. Aside from how the data is stored in etcd, the following two resources are effectively identical and serve the same purpose:

    === "CloudConfig in a ConfigMap"

        ``` yaml
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name:      my-vm-bootstrap-data
          namespace: my-namespace
        data:
          user-data: |
            #cloud-config
            users:
            - default
        ```

    === "CloudConfig in a Secret"

        ``` yaml
        apiVersion: v1
        kind: Secret
        metadata:
          name:      my-vm-bootstrap-data
          namespace: my-namespace
        stringData:
          user-data: |
            #cloud-config
            users:
            - default
        ```
## Supported

### Cloud-Init

[Cloud-Init](https://cloudinit.readthedocs.io/en/latest/) is widely recognized as the de facto method for bootstrapping modern VM instances on hyperscalers, including Tanzu with VM Operator. For example, the following resources may be used to deploy a VM and bootstrap its guest with Cloud-Init to:

* add a custom user
* execute commands on boot
* write files

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha1
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    small
      imageName:    ubuntu-2210
      storageClass: iscsi
      vmMetadata:
        transport: CloudInit
        secretName: my-vm-bootstrap-data
    ```

=== "CloudConfig"

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name:      my-vm-bootstrap-data
      namespace: my-namespace
    stringData:
      user-data: |
        #cloud-config
        users:
        - default
        - name: akutz
          primary_group: akutz
          groups: users
          ssh_authorized_keys:
          - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
        runcmd:
        - "ls /"
        - [ "ls", "-a", "-l", "/" ]
        write_files:
        - path: /etc/my-plaintext
          permissions: '0644'
          owner: root:root
          content: |
            Hello, world.
    ```

The data in the above `Secret` is called the Cloud-Init _Cloud Config_. For more information on the Cloud-Init Cloud Config format, please see its [official documentation](https://cloudinit.readthedocs.io/en/latest/reference/examples.html).

!!! note "Windows and Cloud-Init"

    It is possible to use the Cloud-Init bootstrap provider to deploy a Windows image if it contains [Cloudbase-Init](https://cloudbase.it/cloudbase-init/), the Windows port of Cloud-Init.

### Sysprep

Microsoft originally designed Sysprep as a means to prepare a deployed system for use as a template. It was such a useful tool, that VMware utilized it as the means to customize a VM with a Windows guest.

!!! note "Sysprep State"

    Deploying Windows images that have not completed their previous Sysprep operation could cause the Guest OS customization to fail. Therefore, it is important to ensure that the image is sealed correctly and in a clean state when using Sysprep. For more information on this issue, please refer to [this article](https://kb.vmware.com/s/article/86350).

#### Minimal Config

The following YAML may be used to bootstrap a Windows image with minimal information. For proper network configuration and Guest OS Customization (GOSC) completion, Sysprep unattend data requires a template for providing network info and `RunSynchronousCommand` to record GOSC status. Both components are essential for Windows Vista and later versions.

!!! note "Product Key"

    Please note the image would have to be using a Volume License SKU as the product ID is not provided in the following Sysprep configuration. See the "Activate Windows" section below for more information.

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha1
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    small
      imageName:    windows11
      storageClass: iscsi
      vmMetadata:
        transport: Sysprep
        secretName: my-vm-bootstrap-data
    ```

=== "SysprepConfig"

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: my-vm-bootstrap-data
      namespace: my-ns
    stringData:
      unattend: |
        <?xml version="1.0" encoding="UTF-8"?>
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
    ```

#### Activate Windows

The following XML can be used to supply a product key to activate Windows:

```xml
<settings pass="windowsPE">
  <component name="Microsoft-Windows-Setup" processorArchitecture="amd64"
    publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS"
    xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <UserData>
      <AcceptEula>true</AcceptEula>
      <FullName>akutz</FullName>
      <Organization>VMware</Organization>
      <ProductKey>
        <Key>1234-5678-9abc-defg</Key>
        <WillShowUI>Never</WillShowUI>
      </ProductKey>
    </UserData>
  </component>
</settings>
```

#### User Accounts

The following XML can be used to update Administrator password and create a new local user account:

```xml
<settings pass="oobeSystem">
  <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64"
    publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS"
    xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <UserAccounts>
      <AdministratorPassword>
        <Value>FakePassword</Value>
        <PlainText>true</PlainText>
      </AdministratorPassword>
      <LocalAccounts>
        <LocalAccount wcm:action="add">
          <Name>sdiliyaer</Name>
          <Password>
            <Value>vmware</Value>
            <PlainText>true</PlainText>
          </Password>
          <Group>Administrators</Group>
          <DisplayName>sdiliyaer</DisplayName>
          <Description>Local administrator account</Description>
        </LocalAccount>
      </LocalAccounts>
    </UserAccounts>
  </component>
</settings>
```

#### Automate OOBE

The following XML can be used to prevent appearing the Windows OOBE screen:

```xml
<settings pass="oobeSystem">
  <component name="Microsoft-Windows-International-Core" processorArchitecture="amd64"
    publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <InputLocale>0409:00000409</InputLocale>
    <SystemLocale>en-US</SystemLocale>
    <UserLocale>en-US</UserLocale>
    <UILanguage>en-US</UILanguage>
  </component>
  <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64"
    publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS"
    xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <OOBE>
      <HideEULAPage>true</HideEULAPage>
      <HideLocalAccountScreen>true</HideLocalAccountScreen>
      <HideOEMRegistrationScreen>true</HideOEMRegistrationScreen>
      <HideOnlineAccountScreens>true</HideOnlineAccountScreens>
      <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
      <ProtectYourPC>3</ProtectYourPC>
      <SkipMachineOOBE>true</SkipMachineOOBE>
      <SkipUserOOBE>true</SkipUserOOBE>
    </OOBE>
    <TimeZone>Central Standard Time</TimeZone>
  </component>
</settings>
```

For more information on Sysprep, please refer to Microsoft's [official documentation](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/).

### vAppConfig

The vAppConfig bootstrap method is useful for legacy, VM images that rely on bespoke, boot-time processes that leverage vAppConfig properties for customizing a guest.

To illustrate, the following YAML can be utilized to deploy a VirtualMachine and bootstrap OVF properties that define the network information:

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha1
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    small
      imageName:    my-vm-image
      storageClass: my-storage-class
      vmMetadata:
        transport: vAppConfig
        secretName: my-vm-bootstrap-template
    ```

=== "vAppConfig"

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: my-vm-bootstrap-template
      namespace: my-namespace
    stringData:
      nameservers: "{{ (index .V1alpha1.Net.Nameservers 0) }}"
      hostname: "{{ .V1alpha1.VM.Name }} "
      management_ip: "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"
      management_gateway: "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}"
    ```

=== "vAppConfig with supporting template"

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
    name: my-secret
    namespace: test-ns
    stringData:
    # see more details on below Supporting Template Queries section
    nameservers: "{{ V1alpha1_FormatNameservers 2 \",\" }}"
    management_ip: "{{ V1alpha1_FormatIP \"192.168.1.10\" \"255.255.255.0\" }}"
    hostname: "{{ .V1alpha1.VM.Name }} "  
    management_gateway: "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}"
    ```

For more information on vAppConfig, please refer to [tutorial/deploy-vm/vappconfig](https://vm-operator.readthedocs.io/en/stable/tutorials/deploy-vm/vappconfig/).
## Deprecated

The following bootstrap providers are still available, but they are deprecated and are not recommended.

### OvfEnv

The `OvfEnv` method combines VMware's Guest OS Customization (GOSC) for bootstrapping a guest's network and the Cloud-Init, OVF data source to supply a Cloud-Init Cloud Config.

!!! warning "Deprecation Notice"

    The `OvfEnv` transport has been deprecated in favor of the `CloudInit` provider. Apart from the reasons outlined in the [Cloud-Init](#cloud-init) section on why one would want to use Cloud-Init, the `OvfEnv` provider's reliance on the GOSC APIs for bootstrapping the guest's network and Cloud-Init for additional customizations resulted in a race condition. VMware Tools would reboot the guest to satisfy the GOSC network configuration while Cloud-Init was still running. Thus any image that used `OvfEnv` needed to have a special fix applied to prevent Cloud-Init from running on first-boot. This method is removed in v1alpha2, and any consumers still relying on this provider should switch to Cloud-Init.

The following resources may be used to deploy a VM and bootstrap the guest's network with GOSC and then leverage Cloud-Init to:

* add a custom user
* execute commands on boot
* write files

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha1
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    small
      imageName:    ubuntu-2210
      storageClass: iscsi
      vmMetadata:
        transport: OvfEnv
        configMapName: my-vm-bootstrap-data
    ```

=== "CloudConfig"

    ``` yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name:      my-vm-bootstrap-data
      namespace: my-namespace
    data:
      user-data: I2Nsb3VkLWNvbmZpZwp1c2VyczoKLSBkZWZhdWx0Ci0gbmFtZTogYWt1dHoKICBwcmltYXJ5X2dyb3VwOiBha3V0egogIGdyb3VwczogdXNlcnMKICBzc2hfYXV0aG9yaXplZF9rZXlzOgogIC0gc3NoLXJzYSBBQUFBQjNOemFDMXljMkVBQUFBREFRQUJBQUFCQVFEU0w3dVdHai4uLgpydW5jbWQ6Ci0gImxzIC8iCi0gWyAibHMiLCAiLWEiLCAiLWwiLCAiLyIgXQp3cml0ZV9maWxlczoKLSBwYXRoOiAvZXRjL215LXBsYWludGV4dAogIHBlcm1pc3Npb25zOiAnMDY0NCcKICBvd25lcjogcm9vdDpyb290CiAgY29udGVudDogfAogICAgSGVsbG8sIHdvcmxkLg==
    ```

    !!! note "Base64 Encoded User Data"

        Unlike the `CloudInit` transport which accepts user data as either plain-text or base64-encoded, the `OvfEnv` provider requires the base64-encoding. The base64-encoded value above decodes to:

        ```yaml
        #cloud-config
        users:
        - default
        - name: akutz
          primary_group: akutz
          groups: users
          ssh_authorized_keys:
          - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
        runcmd:
        - "ls /"
        - [ "ls", "-a", "-l", "/" ]
        write_files:
        - path: /etc/my-plaintext
          permissions: '0644'
          owner: root:root
          content: |
            Hello, world.
        ```

### ExtraConfig

The `ExtraConfig` provider combined VMware's Guest OS Customization (GOSC) for bootstrapping a guest's network allowed clients to set any `guestinfo` key they wanted in order to influence a guest's bootstrap engine.

!!! warning "Deprecation Notice"

    When Tanzu Kubernetes was first released, the Cluster API provider that depended upon VM Operator used the `ExtraConfig` provider for supplying bootstrap information. This method was never intended for wide use, and Tanzu now uses Cloud-Init anyway. To that end, this provider is no longer supported and will be removed in v1alpha2. Any consumers still relying on this provider should switch to Cloud-Init.
