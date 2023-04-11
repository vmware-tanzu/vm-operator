# Writing Documentation

This page outlines how to write documentation for VM Operator.

## Go Docs

// TODO ([github.com/vmware-tanzu/vm-operator#94](https://github.com/vmware-tanzu/vm-operator/issues/94))

## Project Docs

The project documentation lives in the [./docs](${{ config.repo_url }}/tree/main/docs) directory and is written in Markdown. This section reviews the project documentation structure and examples for writing high-quality documentation.

### Structure

The project documentation adheres to a specific structure under the `docs` directory:

```
docs
|-- README.md
|-- SECTION1
|   |-- SECTION1/README.md
|   |-- SECTION1/topic1.md
|   |-- SECTION1/topic2.md
|-- SECTION2
    |-- SECTION2/README.md
    |-- SECTION2/topic1.md
    |-- SUBSECTION1
        |-- SECTION2/SUBSECTION1/README.md
        |-- SECTION2/SUBSECTION1/topic1.md
        |-- SECTION2/SUBSECTION1/topic2.md
        |-- SECTION2/SUBSECTION1/topic3.md
```

* The `docs` directory represents the _root_ section

* The file layout is based on sections, with each section getting its own directory.

* Do not modify the top-level sections unless you have cleared it with the project leadership. These are: `Getting Started`, `Concepts`, `Tutorials`, and `Reference`.

* Every section should have a `README.md` that summarizes the contents of the section. It does not matter if the section has a single topic in it, if there's a section, then it must have a `README.md`. Besides good organization, there is another reason this is important that will be reviewed later.

* The documentation will not appear unless added to the `nav` section in the project's [`mkdocs.yml`](${{ config.repo_url }}/tree/main/mkdocs.yml) file, ex.:

    ```yaml title="mkdocs.yml"
    nav:
    - Home: README.md
    - Getting Started:
      - start/README.md
      - Quickstart: start/quick.md
      - Talk to Us: start/help.md
      - Contribute:
        - start/contrib/README.md
        - Suggest a Change: start/contrib/suggest-change.md
        - Report an Issue: start/contrib/report-issue.md
        - Submit a Change: start/contrib/submit-change.md
      - About:
        - start/about/README.md
        - Roadmap: start/about/roadmap.md
        - Release Notes: start/about/release-notes.md
        - License: start/about/license.md
    - Concepts:
      - concepts/README.md
      - Workloads:
        - concepts/workloads/README.md
        - VirtualMachine: concepts/workloads/vm.md
        - VirtualMachineClass: concepts/workloads/vm-class.md
        - WebConsoleRequest: concepts/workloads/vm-web-console.md
        - Guest Customization: concepts/workloads/guest.md
      - Images:
        - concepts/images/README.md
        - VirtualMachineImage: concepts/images/vm-image.md
        - Publish a VM Image: concepts/images/pub-vm-image.md
      - Services & Networking:
        - concepts/services-networking/README.md
        - VirtualMachineService: concepts/services-networking/vm-service.md
        - Guest Network Config: concepts/services-networking/guest-net-config.md
    - Tutorials:
      - tutorials/README.md
      - Deploy VM:
        - tutorials/deploy-vm/README.md
        - With Cloud-Init: tutorials/deploy-vm/cloudinit.md
        - With vAppConfig: tutorials/deploy-vm/vappconfig.md
      - Troubleshooting:
        - tutorials/troubleshooting/README.md
        - Get a Console Session: tutorials/troubleshooting/get-console-session.md
    - Reference:
      - ref/README.md
      - API:
        - ref/api/README.md
        - v1alpha1: ref/api/v1alpha1.md
      - Configuration:
        - ref/config/README.md
        - Manager Pod: ref/config/manager.md
      - Project:
        - ref/proj/README.md
        - Build from Source: ref/proj/build.md
        - Create a Release: ref/proj/release.md
        - Writing Documentation: ref/proj/docs.md
    ```

### Examples

The guidelines for writing project documentation are as follows:

* The documentation is based on the Kubernetes documentation, so if you have a question, the first place to look for answers and inspiration is [https://kubernetes.io/docs/](https://kubernetes.io/docs/).

* Samples for the level of detail and quality associated with this project's documentation may also be found inside this project as well:

    * [Example of a section `README.md`](../../concepts/workloads/README.md)
    * [Example of a concept](../../concepts/workloads/vm.md)
    * [Example of a tutorial](../../tutorials/deploy-vm/cloudinit.md)

