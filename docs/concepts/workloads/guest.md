# Customizing a Guest

The ability to deploy a virtual machine with Kubernetes is nice, but one of the values of VM Operator is its support for popular bootstrap providers such as Cloud-Init, Sysprep, and vAppConfig. This page reviews these bootstrap providers to help inform when to select one over the other.

## Bootstrap Providers

There are a number of methods that may be used to bootstrap a virtual machine's (VM) guest operating system:

| Provider                    | Network Config   | Linux  | Windows | Description |
|-----------------------------|------------------|:------:|:-------:|-------------|
| [Cloud-Init](#cloud-init)   | [Cloud-Init Network v2](https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html) |   ✓   |     ✓    | The industry standard, multi-distro method for cross-platform, cloud instance initialization with modern, VM images |
| [LinuxPrep](#linuxprep)     | [Guest OS Customization](https://vdc-download.vmware.com/vmwb-repository/dcr-public/c476b64b-c93c-4b21-9d76-be14da0148f9/04ca12ad-59b9-4e1c-8232-fd3d4276e52c/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.customization.Specification.html) (GOSC) |    ✓   |         | LinuxPrep is used by VMware to customize Linux images on first-boot or at runtime |
| [Sysprep](#sysprep)         | [Guest OS Customization](https://vdc-download.vmware.com/vmwb-repository/dcr-public/c476b64b-c93c-4b21-9d76-be14da0148f9/04ca12ad-59b9-4e1c-8232-fd3d4276e52c/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.customization.Specification.html) (GOSC) |       |     ✓    | Microsoft Sysprep is used by VMware to customize Windows images on first-boot |
| [vAppConfig](#vappconfig)   | Bespoke                       |   ✓   |         | For images with bespoke, bootstrap engines driven by vAppConfig properties |

## Cloud-Init

[Cloud-Init](https://cloudinit.readthedocs.io/en/latest/) is widely recognized as the de facto method for bootstrapping modern VM instances on hyperscalers, including Tanzu with VM Operator.

!!! note "Windows and Cloud-Init"

    It is possible to use the Cloud-Init bootstrap provider to deploy a Windows image if it contains [Cloudbase-Init](https://cloudbase.it/cloudbase-init/), the Windows port of Cloud-Init.

### InstanceID

The `cloudInit.instanceID` field defaults to the VM's `spec.biosUUID`. The value of this field can be changed when a VM is restored from backup for example, to trigger cloud-init based network initialization.

### Inline Cloud Config

The `VirtualMachine` API directly supports specifying a Cloud-Init [cloud config](https://cloudinit.readthedocs.io/en/latest/reference/examples.html) for bootstrapping:

* users
* commands
* files

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha3
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    my-vm-class
      imageName:    vmi-0a0044d7c690bcbea
      storageClass: my-storage-class
      bootstrap:
        cloudInit:
          cloudConfig:
            users:
            - name: jdoe
              primary_group: jdoe
              groups: users
              passwd:
                name: my-vm-bootstrap-data
                key:  jdoe-passwd
              ssh_authorized_keys:
              - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
            runcmd:
            - "ls /"
            - [ "ls", "-a", "-l", "/" ]
            write_files:
            - path: /etc/my-plain-text
              permissions: '0644'
              owner: root:root
              content: |
                Hello, world.
            - path: /etc/my-secret-data
              permissions: '0644'
              owner: root:root
              content:
                name: my-vm-bootstrap-data
                key:  etc-my-secret-data
    ```

=== "Secret"

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name:      my-vm-bootstrap-data
      namespace: my-namespace
    stringData:
      jdoe-passwd: my-password
      etc-my-secret-data: |
        My super secret message.
    ```

The data in the above `Secret` is referenced by portions of the `VirtualMachine` resource's inline cloud config.

#### Schema Differences

Please note, there are a few differences between the inline Cloud Config and the official format. Please refer to [`./api/v1alpha2/cloudinit/cloudconfig.go`](https://github.com/vmware-tanzu/vm-operator/blob/main/api/v1alpha2/cloudinit/cloudconfig.go) for up-to-date information on how the inline Cloud Config compares to the official format.

##### Default User

The inline `users` list may not include the keyword `default` to indicate the default user should remain active, ex:

```yaml
users:
- default
- name: jdoe
  primary_group: jdoe
  ...
```

Instead, please use the `defaultUserEnabled` property which is a sibling of `users`:

```yaml
defaultUserEnabled: true
users:
- name: jdoe
  primary_group: jdoe
  ...
```

Please note it is only necessary to use `defaultUserEnabled` when the `users` list is non-empty and the default users should still be enabled.

##### Secret Key Selector

Some values cannot be specified directly so as to disallow the inclusion of sensitive data directly in the `VirtualMachine` resource. Instead, these field values are `SecretKeySelector` data types, specifying the name of a `Secret` resource and name in said resource of the key that contains the value. These fields include:

* `users[].hashed_passwd`
* `users[].passwd`
* `write_files[].content`

For example, here is how a user would specify a password:

```yaml
users:
- name: jdoe
  primary_group: jdoe
  groups: users
  passwd:
    name: my-vm-bootstrap-data
    key:  jdoe-passwd
  ssh_authorized_keys:
  - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
```

##### Dynamic Properties

Despite the rules and conventions for Kubernetes custom resource definitions (CRD), there are also some nice tricks in place to try and retain many of the dynamic properties from the official Cloud Config format. For example, the `write_files[].content` field _may_ specify a field from a `Secret` resource when the data is sensitive, but the field may also directly specify the file's contents as well:

```yaml
write_files:
- path: /etc/my-plain-text
  permissions: '0644'
  owner: root:root
  content: |
    Hello, world.
- path: /etc/my-secret-data
  permissions: '0644'
  owner: root:root
  content:
    name: my-vm-bootstrap-data
    key:  etc-my-secret-data
```

Another such field is `runcmd`, a list of the commands to run at first boot. Normally this would be specified as the following in a CRD:

```yaml
runcmd:
- ls -al
- echo Hello, world.
```

However, just like in the official format, the inline run commands may be specified as a string or a list of strings, ex:

```yaml
runcmd:
- ls -al
- - echo
  - Hello, world.
```

### Raw Cloud Config

When more advanced configurations are required, a raw cloud config may be used via a `Secret` resource:

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha3
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    my-vm-class
      imageName:    vmi-0a0044d7c690bcbea
      storageClass: my-storage-class
      bootstrap:
        cloudInit:
          rawCloudConfig:
            key:  my-vm-bootstrap-data
            name: user-data
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
        - name: jdoe
          primary_group: jdoe
          groups: users
          passwd: my-password
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
        - path: /etc/my-secret-data
          permissions: '0644'
          owner: root:root
          content: |
            My super secret message.
    ```

## LinuxPrep

If using Linux and Cloud-Init is not an option, try the LinuxPrep bootstrap provider, which uses VMware tools to bootstrap a Linux guest operating system. It has minimal configuration options, but it supports a wide-range of Linux distributions. The following YAML may be used to bootstrap a guest using LinuxPrep:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name:      my-vm
  namespace: my-namespace
spec:
  className:    my-vm-class
  imageName:    vmi-0a0044d7c690bcbea
  storageClass: my-storage-class
  bootstrap:
    linuxPrep:
      hardwareClockIsUTC: true
      timeZone: America/Chicago
```

## Sysprep

Microsoft originally designed Sysprep as a means to prepare a deployed system for use as a template. It was such a useful tool, VMware utilized it as the means to customize a VM with a Windows guest.

!!! note "Sysprep State"

    Deploying Windows images that have not completed their previous Sysprep operation could cause the Guest OS customization to fail. Therefore, it is important to ensure that the image is sealed correctly and in a clean state when using Sysprep. For more information on this issue, please refer to [this article](https://kb.vmware.com/s/article/86350).

### Inline Sysprep

It is possible to specify the sysprep configuration inline with the `VirtualMachine` API:

#### Volume License SKU

The following YAML may be used to bootstrap a Windows image that uses the volume license SKU and does not require a product ID:

``` yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name:      my-vm
  namespace: my-namespace
spec:
  className:    my-vm-class
  imageName:    vmi-0a0044d7c690bcbea
  storageClass: my-storage-class
  bootstrap:
    sysprep:
      sysprep: {}
```

#### Product ID

The following may be used to bootstrap a Windows image that requires a product ID to activate Windows:

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha3
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    my-vm-class
      imageName:    vmi-0a0044d7c690bcbea
      storageClass: my-storage-class
      bootstrap:
        sysprep:
          sysprep:
            userData:
              fullName: John Doe
              orgName:  Lost and Still Lost
              productID:
                name: my-vm-bootstrap-data
                key:  product-id
    ```

=== "Secret"

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name:      my-vm-bootstrap-data
      namespace: my-namespace
    stringData:
      product-id: "0123456789..."
    ```

### Raw Sysprep

Sometimes it is necessary to provide the sysprep [answers file](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/) directly, in which case the raw sysprep option is available.

#### Minimal Config

The following YAML may be used to bootstrap a Windows image with minimal information. For proper network configuration and Guest OS Customization (GOSC) completion, Sysprep unattend data requires a template for providing network info and `RunSynchronousCommand` to record GOSC status. Both components are essential for Windows Vista and later versions.

!!! note "Product Key"

    Please note the image would have to be using a Volume License SKU as the product ID is not provided in the following Sysprep configuration. Please the [_Activate Windows_](#activate-windows) section below for more information.

=== "VirtualMachine"

    ``` yaml
    apiVersion: vmoperator.vmware.com/v1alpha3
    kind: VirtualMachine
    metadata:
      name:      my-vm
      namespace: my-namespace
    spec:
      className:    my-vm-class
      imageName:    vmi-0a0044d7c690bcbea
      storageClass: my-storage-class
      bootstrap:
        sysprep:
          rawSysprep:
            name: my-vm-bootstrap-data
            key:  unattend
    ```

=== "SysprepConfig"

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: my-vm-bootstrap-data
      namespace: my-namespace
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
      <FullName>jdoe</FullName>
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
        <Value>my-password</Value>
        <PlainText>true</PlainText>
      </AdministratorPassword>
      <LocalAccounts>
        <LocalAccount wcm:action="add">
          <Name>jdoe</Name>
          <Password>
            <Value>vmware</Value>
            <PlainText>true</PlainText>
          </Password>
          <Group>Administrators</Group>
          <DisplayName>jdoe</DisplayName>
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

## vAppConfig

The vAppConfig bootstrap method is useful for legacy, VM images that rely on bespoke, boot-time processes that leverage vAppConfig properties for customizing a guest.

To illustrate, the following YAML can be utilized to deploy a VirtualMachine and bootstrap OVF properties that define the network information:

``` yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name:      my-vm
  namespace: my-namespace
spec:
  className:    my-vm-class
  imageName:    vmi-0a0044d7c690bcbea
  storageClass: my-storage-class
  bootstrap:
    vAppConfig:
      properties:
      - key: nameservers
        value:
          value: "{{ (index .V1alpha3.Net.Nameservers 0) }}"
      - key: management_ip
        value:
          value: "{{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) }}"
      - key: hostname
        value:
          value: "{{ .V1alpha3.VM.Name }}"
      - key: management_gateway
        value:
          value: "{{ (index .V1alpha3.Net.Devices 0).Gateway4 }}"
```

### Templating

Properties are templated according to the Golang [`text/template`](https://pkg.go.dev/text/template) package. Please refer to Go's documentation for a full understanding of how to construct template queries.

#### Input Object

The object provided to the template engine is the [`VirtualMachineTemplate`](https://github.com/vmware-tanzu/vm-operator/blob/b01fdba17310289be0f0144dad26758fa50b11ce/api/v1alpha2/virtualmachinetempl_types.go#L37-L50) data structure.

#### Pre-Defined Functions

The following table lists the functions VM Operator defines and passes into the template engine to make it easier to construct the information required for vApp properties:

!!! note "`V1alpha1` and `V1alpha2` Prefixed Template Functions"

    Please note the template functions beginning with `V1alpha1` and `V1alpha2` are still supported, but users are encouraged to switch to the `V1alpha3` variants.

| Query name | Signature | Description |
| -------- | -------- | -------- |
| V1alpha3_FirstIP | `func () string` | Get the first, non-loopback IP address (formatted with network length) from the first NIC. |
| V1alpha3_FirstIPFromNIC | `func (index int) string` | Get the first, non-loopback IP address (formatted with network length) from the n'th NIC. If the specified index is out-of-bounds, the template string is not parsed. |
| V1alpha3_FormatIP | `func (IP string, netmask string) string` | This function may be used to format an IP address with or without a network prefix length. If the provided netmask is empty, then the IP address returned does not include a network length. If the provided netmask is non-empty, then it must be either a length, ex. `/24`, or decimal notation, ex. `255.255.255.0`. |
| V1alpha3_FirstNicMacAddr | `func() (string, error)` | Get the MAC address from the first NIC. |
| V1alpha3_FormatNameservers| `func (count int, delimiter string) string` | Format the first occurred count of nameservers with the provided delimiter. Specify a negative number to include all nameservers. |
| V1alpha3_IP | `func(IP string) string` | Format an IP address with the default netmask CIDR. If the specified IP is invalid, the template string is not parsed. |
| V1alpha3_IPsFromNIC | `func (index int) []string` | List all IPs, formatted with the network length, from the n'th NIC. If the specified index is out-of-bounds, the template string is not parsed. |
| V1alpha3_SubnetMask | `func(cidr string) (string, error)` | Get a subnet mask from an IP address formatted with a network length. |

## Deprecated

The following bootstrap providers are still available, but they are deprecated and are not recommended.

### OvfEnv

The `OvfEnv` method combines VMware's Guest OS Customization (GOSC) for bootstrapping a guest's network and the Cloud-Init, OVF data source to supply a Cloud-Init Cloud Config.

!!! warning "Deprecation Notice"

    The `OvfEnv` transport has been deprecated in favor of the `CloudInit` provider. Apart from the reasons outlined in the [Cloud-Init](#cloud-init) section on why one would want to use Cloud-Init, the `OvfEnv` provider's reliance on the GOSC APIs for bootstrapping the guest's network and Cloud-Init for additional customizations resulted in a race condition. VMware Tools would reboot the guest to satisfy the GOSC network configuration while Cloud-Init was still running. Thus any image that used `OvfEnv` needed to have a special fix applied to prevent Cloud-Init from running on first-boot. This method is removed in v1alpha2, and any consumers still relying on this provider should switch to Cloud-Init.

### ExtraConfig

The `ExtraConfig` provider combined VMware's Guest OS Customization (GOSC) for bootstrapping a guest's network allowed clients to set any `guestinfo` key they wanted in order to influence a guest's bootstrap engine.

!!! warning "Deprecation Notice"

    When Tanzu Kubernetes was first released, the Cluster API provider that depended upon VM Operator used the `ExtraConfig` provider for supplying bootstrap information. This method was never intended for wide use, and Tanzu now uses Cloud-Init anyway. To that end, this provider is no longer supported and is removed in v1alpha2. Any consumers still relying on this provider should switch to Cloud-Init.
