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

### Preview Changes

Once documentation is written, it is important to see how it looks in order to review it. There are a few ways to preview the VM Operator project documentation:

#### Preview with Pull Request

First, and perhaps the simplest way to preview documentation is by opening a pull request. If any changes are detected under the `./docs` folder, a link is automatically added at the bottom of a pull request's description that points to a temporary, public URL that hosts the documentation from the pull request. For example, at the bottom of the description of pull request that added this section, [vmware-tanzu/vm-operator/pull#190](https://github.com/vmware-tanzu/vm-operator/pull/190), the following was added automatically:

> ----
> :books: Documentation preview :books:: https://vm-operator--190.org.readthedocs.build/en/190/

This option makes it trivial for pull request reviewers to verify doc updates. Additionally, because the process used to build documentation for a pull request is the same as the one for `main`, this option is a great way to assert that changes to files such as `mkdocs.yml` or `.readthedocs.yaml`, have not broken the ability to build the documentation.

#### Preview Locally

Understandably, some people would like to preview changes prior to opening a pull request. There are two ways to build and preview the documentation locally:

* With Python3

    ```shell
    make docs-serve-python
    ```

* With Docker

    ```shell
    make docs-server-docker
    ```

Both of the above options have their strengths and weaknesses. Using Python3 is faster, but adds the complexity of knowing how to debug issues with a local Python3 installation if any should arise. The Docker option is more portable, but only if Docker is already installed. After choosing a command and running it:

* The documentation is available at [http://127.0.0.1:8000/vmware-tanzu/vm-operator/](http://127.0.0.1:8000/vmware-tanzu/vm-operator/).
* The local server will automatically reload if any changes are detected to content under the `./docs` folder. 

!!! note "Docker and `0.0.0.0`"

    If the Docker option is selected to preview the documentation locally, the last line of the output will be `Serving on http://0.0.0.0:8000/vmware-tanzu/vm-operator/`. However, the site is still accessed via [http://127.0.0.1:8000/vmware-tanzu/vm-operator/](http://127.0.0.1:8000/vmware-tanzu/vm-operator/). So what gives with the `0.0.0.0`?

    The IP address `0.0.0.0` instructs the web server to bind the port over which the content is accessed to all, available IP addresses. Normally `mkdocs` just binds the web server port to `127.0.0.1`, but if that was used inside of the container, the web server would not be accessible via the web browser on the system where Docker is running.
