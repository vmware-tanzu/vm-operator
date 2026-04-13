
# Local E2E test runs

This section describes how to setup your local environment to run the
VM Service E2E tests against a remote environment. 

## Prepare Nimbus Testbed

### 1. Create Content Library
From the VC UI, 
- Left pane > Content Libraries
- Create
    - Name `vmservice`
    - Subscription URL: `https://wp-content-pstg.broadcom.com/vmsvc/lib.json`
    - Wait for library sync to complete

### 2. Storage Classes

The e2e tests require two storage policies to be created beforehand:
- `wcpglobal_storage_profile` exists or create one
- `worker-storagepolicy` 

Testbeds created using the `dev-integ-vds` already have the `wcpglobal_storage_profile`, 
so you would need to only create the `worker-storagepolicy`.

#### Option 1: Create missing policy

Run the following script which creates the `worker-storagepolicy` for you.
```bash
GOVC_PASSWORD="${SSH_PASSWORD}" ./hack/managestoragepolicy.sh create "${VCSA_IP}"
```

#### Option 2: From VC UI

- Left pane > Policies and Profiles
- VM Storage Policies from the left lane
- Confirm `wcpglobal_storage_profile` exists or create one
    - K8s compliant name must be `wcpglobal-storage-profile`
- Create `worker-storagepolicy` 
    - K8s compliant name must be `worker-storagepolicy`
- Add both policies to any namespace in the Supervisor cluster. 
This will add the storage policy CRD to the cluster which is a cluster resource.
    - Supervisor > Select any namespace
    - Storage > Edit button
    - Select both storage policies

## Running tests

### Option 1 (VDS Testbeds)

You can use the `configure-local.sh` script to configure your local shell to run the
E2E tests if you are using a VDS testbed. The script downloads the kubeconfig 
from the WCP Control Plane node, places it in `~/.kube/wcp-config`, and exports 
the required environment variables.

You can run the tests by following,

```bash
export TEST_TAGS="vmservice"        # or a different tag you want to run.
export TEST_FOCUS="<test focus>"    # the tests you want to execute.
export TEST_SKIP=""                 # optional: if you want to skip specific tests.

source ./hack/configure-local.sh <testbed info json local path>
make wcp-vmservice-e2e
```

### Option 2 (Non VDS)

Do the following to prepare your local shell session

```bash
cp <kubeconfig path> ~/.kube/wcp-config
export KUBECONFIG=~/.kube/wcp-config

export VCSA_IP="<VC IP>"
export SSH_PASSWORD="<VC Password>"
export VCSA_PASSWORD="${SSH_PASSWORD}"
export NETWORK="vds"                # vds or nsx.
```

Run the tests

```bash
export TEST_TAGS="vmservice"        # or a different tag you want to run.
export TEST_FOCUS="<test focus>"    # the tests you want to execute.
export TEST_SKIP=""                 # optional: if you want to skip specific tests.

make wcp-vmservice-e2e
```