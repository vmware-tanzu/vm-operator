# Contributing to VM Operator

An introduction to contributing to the VM Operator project

---

The VM Operator project welcomes, and depends, on contributions from developers and users in the open source community. Contributions can be made in a number of ways, a few examples are:

- Code patches via pull requests
- Documentation improvements
- Bug reports and patch reviews

## Reporting an Issue

Please include as much detail as you can. This includes:

* The VM Operator version
* The name and verison of the platform on which VM Operator is installed
* A set of logs with debug-logging enabled that show the problem

## Testing the Development Version

If you want to just install and try out the latest development version of VM Operator, you may:

1. Build the container image:

    ```shell
    make docker-build
    ```

1. Save the image to a tar file:

    ```shell
    docker save <IMAGE> ><IMAGE>.tar
    ```

1. Upload the tar file to each Supervisor control plane node:

    `// TODO(akutz)`

1. Load the image into the ContainerD namespace `k8s.io` on each Supervisor control plane node:

    `// TODO(akutz)`

1. Update the VM Operator `Deployment` to reference the custom image:

    `// TODO(akutz)`

## Running the tests

To run the tests, please run the following command(s):

```bash
make -j1 test test-integration
```

## Submitting Pull Requests

Once you are happy with your changes or you are ready for some feedback, push it to your fork and send a pull request. For a change to be accepted it will most likely need to have tests and documentation if it is a new feature.
