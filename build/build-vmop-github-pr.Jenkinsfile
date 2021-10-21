/*******************************************************************************
* Build Github VMOperator Pull Requests
* 
* This pipeline polls github vmoperator for pull request changes (new PR commitIDs) and builds them
* 
 ******************************************************************************/

// The label used to determine how to provision a Butler instance
def String NODE_LABEL = "${env.NODE_LABEL}"
// The path to a custom workspace. 
def String customWorkspaceStr = "workspace/${env.JOB_NAME}/${env.BUILD_NUMBER}"
// The commitID to build from
commitID = ''
// Github url for vm-operator
github_url = 'https://github.com/vmware-tanzu/vm-operator.git'
// The ID of the SSH private key credentials to use.
def String credentialsId = "${env.CREDENTIALS_ID}"
// polling schedule
def String pollSCM_Schedule = "${env.POLLSCM_SCHEDULE}"
// vm_operator build job name for sandbox and precommit tests
def String vmopBuildJobName = "${env.VMOP_BUILD}"
// sandbox output json to archive 
def sbFileName = 'sandbox-build-output.json'
// branchSpecifier is the branches or commitID that needs to be checked out
def branchSpecifier = ''
// List of userIDs for check label
String[] users = "${env.PRIV_USERS}".split(",")
// Sandbox build server url
def sbServerUrl = "${env.SB_SERVER}"

// Optional github commitID specified to build from 
if (params.COMMIT_ID != "") {
    commitID = params.COMMIT_ID
    branchSpecifier = params.COMMIT_ID
} else {
    // Look for all pr branches and build the newest one compared to the workspace
    branchSpecifier = 'origin/pr/*'
}



void setBuildStatus(String message, String state) {
  step([
      $class: "GitHubCommitStatusSetter",
      reposSource: [$class: "ManuallyEnteredRepositorySource", url: github_url],
      contextSource: [$class: "ManuallyEnteredCommitContextSource", context: "ci/jenkins/build-status"],
      commitShaSource: [$class: "ManuallyEnteredShaSource", sha: commitID],
      errorHandlers: [[$class: "ChangingBuildStatusErrorHandler", result: "UNSTABLE"]],
      statusResultSource: [ $class: "ConditionalStatusResultSource", results: [[$class: "AnyBuildResult", message: message, state: state]] ]
  ]);
}

pipeline {
  agent {
    node {
            label "${NODE_LABEL}"
            customWorkspace "${customWorkspaceStr}"
    }
  }

  triggers {
  pollSCM pollSCM_Schedule
  }

  options {
    skipDefaultCheckout true
  }

  stages {
    stage('checkout') {
      steps {
        // checkout scm and fetch pull request branches
        checkout scm: [$class: 'GitSCM',
        branches: [[name: branchSpecifier]],
        extensions: [[$class: 'LocalBranch', localBranch: "**"]],
        userRemoteConfigs: [[url: github_url,
                            credentialsId: credentialsId,
                            refspec: '+refs/pull/*/head:refs/remotes/origin/pr/*']],
        ], poll: true
        
        script {
            currentBuild.description = ""
            if (params.COMMIT_ID == "") {
                // Save HEAD commitID
                commitID = sh (script: 'git rev-parse HEAD', returnStdout: true).trim()
            }
            print "GIT sha1: ${commitID}"
            def commitUrl = "https://github.com/vmware-tanzu/vm-operator/commit/${commitID}"
            // Add commitID and url to build description
            currentBuild.description += "<br>Git sha1: <a href='${commitUrl}'>${commitID}</a>"
            // Add commit Message to build description
            def commitMessage = sh (script: 'git log --format=format:%s -1', returnStdout: true).trim()
            currentBuild.description += "<br>${commitMessage}"
        }
      }
    }
    
    stage('checking PR for testing label') {
        steps {
            script {
                // Gate PRs with a build label (ok-to-test) in the commit message
                // Mentioning a COMMIT_ID to build will result in a detached branch and
                // Pull request numbers cannot be fetched without the branch name
                if (params.CHECK_BUILD_LABEL && params.COMMIT_ID == "") { 
                    def branchName = sh (script: "git rev-parse --abbrev-ref HEAD", returnStdout: true).trim()
                    print "branch to build: ${branchName}"
    
                    def prNumber = sh (script: "echo $branchName | sed -e 's.pr/..g'", returnStdout: true).trim()
                    
                    sh "curl 'https://api.github.com/repos/vmware-tanzu/vm-operator/issues/${prNumber}/comments' > issue-comments.json"
                    commentsJson = readJSON file: "issue-comments.json"
                    print(commentsJson)
                    
                    def labelExists = false
                    for (def c : commentsJson) {
                        if (c['body'] == 'ok-to-test' && users.contains(c['user']['login'])) {
                            echo "'ok-to-test' found. performing build..."
                            labelExists = true
                            break
                        } 
                    }
                    
                    if (!labelExists) {
                        currentBuild.result = 'ABORTED'
                        error("'ok-to-test' not found in git commit message. aborting build...")
                    }
                }
            }
        }
    }
    
    stage('Build and test') {
        steps {
            script {
                def vmopBuild = build job: vmopBuildJobName,
                propagate: true,
                wait: true,
                parameters: [
                    string(name: 'CUSTOM_GIT_URL', value: github_url),
                    string(name: 'CUSTOM_GIT_REF', value: commitID)
                ]
                
                if (vmopBuild.result != "SUCCESS") {
                    error("Sandbox build failed: ${JENKINS_URL}/job/${vmopBuildJobName}/${vmopBuild.number}")
                }
                
                copyArtifacts filter: '**/*', flatten: true, fingerprintArtifacts: true, projectName: vmopBuildJobName, selector: specific("${vmopBuild.number}")
                archiveArtifacts sbFileName
                
                // Extract the SB build number to display it nicely
                if (fileExists(sbFileName)) {
                    def buildJson = readJSON file: sbFileName
                    sbBuildId = "sb-${buildJson.buildid}"
                    def sbBuildUrl = "${sbServerUrl}/sb/${buildJson.buildid}"
                    currentBuild.description += "<br><a href='${sbBuildUrl}'>${sbBuildId}</a>"
                }
            }
        }
    }
  }
  
  post {
    success {
        setBuildStatus("Build succeeded", "SUCCESS");
    }
    failure {
        setBuildStatus("Build failed", "FAILURE");
    }
    aborted {
        echo "Aborted"
    }
  }
}
