# VirtualMachineWebConsoleRequest

The `VirtualMachineWebConsoleRequest` API provides secure, one-time web console access to deployed VirtualMachines. This resource enables users to establish encrypted web console connections through the vSphere infrastructure, allowing direct interaction with VM consoles via a web browser.

## Overview

The VirtualMachineWebConsoleRequest creates a temporary, authenticated web console session that allows users to:

- Access VM console directly through a web interface
- Interact with VMs during boot, troubleshooting, or maintenance
- Establish secure connections without requiring direct vSphere client access
- Use time-limited tickets that automatically expire for security

The web console connection uses the VMware Web Console (WebMKS) protocol, which provides a secure WebSocket-based connection to the VM's console through the vSphere infrastructure.

## How It Works

The web console request workflow involves several components:

1. **Request Creation**: User creates a VirtualMachineWebConsoleRequest with a public key
2. **Ticket Generation**: VM Operator requests a WebMKS ticket from vSphere
3. **Encryption**: The ticket URL is encrypted using the provided public key
4. **Proxy Resolution**: The system determines the appropriate proxy address for access
5. **Web Access**: Users access the console through the web console proxy service

### Architecture Components

- **VM Operator Controller**: Processes web console requests and manages ticket lifecycle
- **vSphere WebMKS**: Provides the underlying console access mechanism
- **Web Console Proxy**: Routes and validates web console connections
- **Validation Service**: Ensures request authenticity and authorization

## API Reference

### VirtualMachineWebConsoleRequestSpec

The spec defines the desired state for a web console request:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the VirtualMachine in the same namespace |
| `publicKey` | string | Yes | RSA OAEP public key in X.509 PEM format for encrypting the response |

#### Public Key Requirements

The `publicKey` field must contain:
- RSA public key in PKCS#1 format
- X.509 PEM encoding with "PUBLIC KEY" header
- Minimum 2048-bit key length recommended
- Used for RSA-OAEP encryption with SHA-512

### VirtualMachineWebConsoleRequestStatus

The status contains the observed state and connection information:

| Field | Type | Description |
|-------|------|-------------|
| `response` | string | Encrypted WebMKS ticket URL (base64-encoded) |
| `expiryTime` | metav1.Time | When the web console access expires |
| `proxyAddr` | string | Proxy address for accessing the web console |

#### Proxy Address Format

The `proxyAddr` field supports various formats:

**DNS Names:**
- `host.example.com`
- `host.example.com:6443`

**IPv4 Addresses:**
- `192.168.1.100`
- `192.168.1.100:6443`

**IPv6 Addresses:**
- `2001:db8::1`
- `[2001:db8::1]:6443`

## Usage Examples

### Basic Web Console Request

```yaml
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineWebConsoleRequest
metadata:
  name: my-vm-console
  namespace: my-namespace
spec:
  name: my-virtual-machine
  publicKey: |
    -----BEGIN PUBLIC KEY-----
    MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA1234567890abcdef...
    -----END PUBLIC KEY-----
```

### Generating RSA Key Pair

To generate the required RSA key pair:

```bash
# Generate private key
openssl genrsa -out private.pem 2048

# Extract public key
openssl rsa -in private.pem -pubout -out public.pem

# Use public.pem content in the publicKey field
```

### Decrypting the Response

Once the request is processed, decrypt the response:

```python
import base64
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa, padding

# Load private key
with open('private.pem', 'rb') as f:
    private_key = serialization.load_pem_private_key(f.read(), password=None)

# Decrypt the response
encrypted_data = base64.b64decode(response)
decrypted_url = private_key.decrypt(
    encrypted_data,
    padding.OAEP(
        mgf=padding.MGF1(algorithm=hashes.SHA512()),
        algorithm=hashes.SHA512(),
        label=None
    )
)

console_url = decrypted_url.decode('utf-8')
print(f"Web console URL: {console_url}")
```

### Complete Workflow Example

```bash
# 1. Create the web console request
kubectl apply -f - <<EOF
apiVersion: vmoperator.vmware.com/v1alpha4
kind: VirtualMachineWebConsoleRequest
metadata:
  name: debug-console
  namespace: production
spec:
  name: web-server-vm
  publicKey: |
    -----BEGIN PUBLIC KEY-----
    MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...
    -----END PUBLIC KEY-----
EOF

# 2. Wait for the request to be processed
kubectl wait --for=condition=Ready \
  virtualmachinewebconsolerequest/debug-console \
  --namespace=production \
  --timeout=60s

# 3. Get the encrypted response
kubectl get virtualmachinewebconsolerequest debug-console \
  --namespace=production \
  -o jsonpath='{.status.response}'

# 4. Get proxy address
kubectl get virtualmachinewebconsolerequest debug-console \
  --namespace=production \
  -o jsonpath='{.status.proxyAddr}'
```

## Web Console Access

### Accessing Through Proxy

The web console is accessed through the proxy service using the following URL pattern:

```
https://<proxyAddr>/vm/web-console/proxy?host=<vm-host>&port=<vm-port>&ticket=<ticket>&uuid=<request-uuid>&namespace=<namespace>
```

Where:
- `proxyAddr`: From the status field
- `host` and `port`: Extracted from decrypted WebMKS URL
- `ticket`: Extracted from decrypted WebMKS URL
- `uuid`: The VirtualMachineWebConsoleRequest UID
- `namespace`: The request's namespace

### Security Validation

The proxy service validates each request by:

1. **Parameter Validation**: Ensures all required parameters are present and valid
2. **UUID Verification**: Confirms the request UUID exists and is authorized
3. **Namespace Authorization**: Validates namespace access permissions
4. **Ticket Validation**: Verifies the WebMKS ticket with vSphere

## Configuration and Management

### Request Lifecycle

1. **Creation**: VirtualMachineWebConsoleRequest is created
2. **Processing**: Controller validates VM exists and generates WebMKS ticket
3. **Encryption**: Ticket URL is encrypted with provided public key
4. **Ready**: Status is populated with encrypted response and proxy address
5. **Expiration**: Request automatically expires after configured timeout (default: 120 seconds)

### Automatic Cleanup

Web console requests are automatically cleaned up:
- Requests expire after 2 minutes by default
- Expired requests are removed by the controller
- Owner references ensure cleanup when parent VM is deleted

### Resource Limits

Consider these limits when using web console requests:
- One active request per VM recommended
- Tickets have limited lifetime (120 seconds default)
- Proxy connections are rate-limited
- Maximum concurrent connections may be restricted

## Troubleshooting

### Common Issues

**Request Stuck in Pending State:**
- Verify the target VirtualMachine exists and is running
- Check VM Operator controller logs for errors
- Ensure vSphere connectivity is working

**Invalid Public Key Error:**
- Verify public key is in correct PEM format
- Ensure key uses PKCS#1 RSA format
- Check key has appropriate length (2048+ bits)

**Proxy Connection Failed:**
- Verify proxy address is accessible
- Check network connectivity to vSphere
- Validate WebMKS ticket hasn't expired

**Decryption Errors:**
- Ensure private key matches the public key used
- Verify decryption algorithm matches (RSA-OAEP with SHA-512)
- Check base64 decoding of encrypted response

### Debugging Commands

```bash
# Check request status
kubectl describe virtualmachinewebconsolerequest <name> -n <namespace>

# View controller logs
kubectl logs -n vmware-system-vmop \
  deployment/vmware-system-vmop-controller-manager

# Check proxy service status
kubectl get service -n kube-system kube-apiserver-lb-svc

# Validate VM status
kubectl get virtualmachine <vm-name> -n <namespace>
```

### Log Analysis

Monitor these log sources for troubleshooting:
- VM Operator controller logs for request processing
- Web console validator service logs for validation issues
- Nginx proxy logs for connection problems
- vSphere logs for WebMKS ticket generation

## Security Considerations

### Encryption

- All web console URLs are encrypted using RSA-OAEP
- Private keys should be stored securely and never shared
- Use strong RSA keys (2048+ bits) for adequate security

### Access Control

- Web console requests are namespace-scoped
- Users need appropriate RBAC permissions to create requests
- Proxy validates each connection attempt

### Network Security

- Web console traffic is encrypted end-to-end
- Proxy service provides additional security layer
- Consider network policies for additional isolation

### Best Practices

1. **Key Management**: Use secure key generation and storage
2. **Time Limits**: Don't extend ticket expiration times unnecessarily
3. **Monitoring**: Log and monitor web console access
4. **Cleanup**: Remove unused requests promptly
5. **Network**: Use network policies to restrict proxy access

## API Versions

The VirtualMachineWebConsoleRequest API is available in multiple versions:

- **v1alpha1**: Legacy `WebConsoleRequest` (deprecated)
- **v1alpha2**: `VirtualMachineWebConsoleRequest` (legacy)
- **v1alpha3**: `VirtualMachineWebConsoleRequest` (legacy)
- **v1alpha4**: `VirtualMachineWebConsoleRequest` (current, storage version)

Use v1alpha4 for new deployments as it's the current storage version and provides the most stable API surface.

## Related Resources

- [VirtualMachine](vm.md) - The target VM for console access
- [VirtualMachineService](vm-service.md) - Network services for VMs
- [VM Networking](../networking/README.md) - Network configuration concepts
