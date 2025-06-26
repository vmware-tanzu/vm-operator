# Tutorials

The Tutorials section provides step-by-step guides and practical examples for common VM Operator tasks. These tutorials are designed to help you learn by doing, walking you through real-world scenarios from basic VM deployment to advanced troubleshooting and application deployment.

Each tutorial includes detailed explanations, complete YAML examples, and command-line instructions that you can follow along with in your own environment.

## Getting Started

* [Deploy a VM](./deploy-vm/README.md)

    Learn the fundamentals of deploying virtual machines with VM Operator, including understanding the VirtualMachine API, bootstrap providers, and various deployment methods

## Application Deployment

* [Deploy Applications](./deploy-apps/README.md)

    Practical guides for deploying different types of applications and services on VMs managed by VM Operator

## Troubleshooting

* [Troubleshooting](./troubleshooting/README.md)

    Step-by-step troubleshooting procedures for common issues, including VM deployment problems, networking issues, and using web console access for debugging

## What You'll Learn

These tutorials cover essential VM Operator workflows:

- **VM Deployment**: Understanding VirtualMachine resources, classes, images, and storage configuration
- **Bootstrap Configuration**: Using Cloud-Init, Sysprep, and vAppConfig for guest OS customization  
- **Storage Management**: Working with persistent volumes, storage classes, and ISO mounting
- **Networking**: Configuring VM networking and troubleshooting connectivity issues
- **Web Console Access**: Getting console sessions for debugging and maintenance
- **Application Deployment**: Deploying and managing applications on VM Operator VMs
- **Publishing VMs**: Creating reusable VM images from deployed instances

## Prerequisites

Before starting these tutorials, you should have:

- A Kubernetes cluster with VM Operator installed
- Access to vSphere infrastructure (for vSphere Supervisor environments)  
- Basic familiarity with Kubernetes concepts and `kubectl`
- Appropriate RBAC permissions for VM Operator resources

## Tutorial Structure

Each tutorial follows a consistent structure:

1. **Overview**: What you'll accomplish and learn
2. **Prerequisites**: Required setup and permissions
3. **Step-by-step Instructions**: Detailed walkthrough with examples
4. **Verification**: How to confirm successful completion
5. **Troubleshooting**: Common issues and solutions
6. **What's Next**: Related tutorials and advanced topics

Start with the [Deploy a VM](./deploy-vm/README.md) tutorial if you're new to VM Operator, then explore specific topics based on your needs.
