# Submit a Change

// TODO ([github.com/vmware-tanzu/vm-operator#99](https://github.com/vmware-tanzu/vm-operator/issues/99))

## Documentation

There are two types of documentation: source and markdown.

### Source Code

All source code should be documented in accordance with the [Go's documentation rules](http://blog.golang.org/godoc-documenting-go-code).

### Markdown

When creating or modifying the project's `README.md` file or any of the documentation in the `docs` directory, please keep the following rules in mind:

1. All links to internal resources should be relative.
1. All links to markdown files should include the file extension.

For example, the below link points to the `Quickstart` page:

<!--
Please note the following message appears when building the site documentation:

    INFO    -  Doc file 'start/contrib/submit-change.md' contains an absolute link '/start/quick.md', it was left as is. Did you mean '../quick.md'?

The above message may be safely ignored. It is caused by the below, intentional
error that demonstrates the *incorrect* method for defining links.
-->

&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
[/start/quick.md](/start/quick.md)

However, when the above link is followed when viewing this page directly from the Github repository instead of the generated site documentation, the link will return a 404.

While it's recommended that users view the generated site documentation instead of the source Markdown directly, we can still fix it so that the above link will work regardless. To fix the link, simply make it relative and add the Markdown file extension:

&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
[../quick.md](../quick.md)

Now the link will work regardless from where it's viewed.

## Style & Syntax

All source files should be linted. Any errors or warnings produced by the tools should be corrected before the source is committed. To lint the project, please run the following command:

```shell
make lint
```

The above command lints markdown, shell scripts, and Go sources.

Your IDE of choice likely has a plug-in that can utilize the golanglint-ci linter, and it will also constantly keep your Go sources up to date.

Another option is to use a client-side, pre-commit hook to ensure that the sources meet the required standards. For example, in the project's `.git/hooks` directory create a file called `pre-commit` and mark it as executable. Then paste the following content inside the file:

```shell
#!/bin/sh
make lint 1> /dev/null
```

The above script will execute prior to a Git commit operation, prior to even the commit message dialog. The script will invoke the `Makefile`'s `lint` target, formatting the sources. If the command returns a non-zero exit code, the commit operation will abort with the error.

## Code Coverage

All new work submitted to the project should have associated tests where applicable. If there is ever a question of whether or not a test is applicable then the answer is likely yes.

This project uses GitHub actions to add coverage to pull requests (PR). If a PR's coverage falls below 60%, the check fails and the PR will be declined until such time coverage is increased.

It's also possible to test the project locally while outputting the code coverage. On the command line, from the project's root directory, execute the following:

```shell
make coverage-full
```

## Commit Messages

Commit messages should follow the guide [5 Useful Tips For a Better Commit Message](https://robots.thoughtbot.com/5-useful-tips-for-a-better-commit-message).
The two primary rules to which to adhere are  

  1. Commit message subjects should not exceed 50 characters in total and should be followed by a blank line.

  2. The commit message's body should not have a width that exceeds 72 characters.

For example, the following commit has a very useful message that is succinct
without losing utility.

```text
commit e80c696939a03f26cd180934ba642a729b0d2941
Author: akutz <sakutz@gmail.com>
Date:   Tue Oct 20 23:47:36 2015 -0500

    Added --format,-f option for CLI

    This patch adds the flag '--format' or '-f' for the
    following CLI commands:

        * adapter instances
        * device [get]
        * snapshot [get]
        * snapshot copy
        * snapshot create
        * volume [get]
        * volume attach
        * volume create
        * volume map
        * volume mount
        * volume path

    The user can specify either '--format=yml|yaml|json' or
    '-f yml|yaml|json' in order to influence how the resulting,
    structured data is marshaled prior to being emitted to the console.
```

Please note that the output above is the full output for viewing a commit. However, because the above message adheres to the commit message rules, it's quite easy to show just the commit's subject:

```shell
$ git show e80c696939a03f26cd180934ba642a729b0d2941 --format="%s" -s
Added --format,-f option for CLI
```

It's also equally simple to print the commit's subject and body together:

```shell
$ git show e80c696939a03f26cd180934ba642a729b0d2941 --format="%s%n%n%b" -s
Added --format,-f option for CLI

This patch adds the flag '--format' or '-f' for the
following CLI commands:

    * adapter instances
    * device [get]
    * snapshot [get]
    * snapshot copy
    * snapshot create
    * volume [get]
    * volume attach
    * volume create
    * volume map
    * volume mount
    * volume path

The user can specify either '--format=yml|yaml|json' or
'-f yml|yaml|json' in order to influence how the resulting,
structured data is marshaled prior to being emitted to the console.
```

## Pull Requests

All developers are required to follow the [GitHub Flow model](https://guides.github.com/introduction/flow/) when proposing new features or even submitting fixes.

Please note that although not explicitly stated in the referenced GitHub Flow model, all work should occur on a __fork__ of this project, not from within a branch of this project itself.

Pull requests submitted to this project should adhere to the following guidelines:

* Branches should be rebased off of the upstream master prior to being opened as pull requests and again prior to merge. This is to ensure that the build system accounts for any changes that may only be detected during the build and test phase.

* Unless granted an exception a pull request should contain only a single commit. This is because features and patches should be atomic -- wholly shippable items that are either included in a release, or not. Please squash commits on a branch before opening a pull request. It is not a deal-breaker otherwise, but please be prepared to add a comment or explanation as to why you feel multiple commits are required.
