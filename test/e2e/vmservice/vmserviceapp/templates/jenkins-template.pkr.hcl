# A Packer template to deploy a VM-Service VM using the vsphere-supervisor builder.
# It installs Jenkins and runs a sample hello-world job in the deployed VM.

# VM-Service source VM configs.
variable "image_name" {
  type = string
}
variable "class_name" {
  type = string
}
variable "storage_class" {
  type = string
}
variable "source_name" {
  type = string
  default = null
}
variable "bootstrap_provider" {
  type = string
  default = "CloudInit"
}
variable "bootstrap_data_file" {
  type = string
  default = null
}

# Supervisor cluster configs.
variable "kubeconfig_path" {
  type = string
  default = null
}
variable "supervisor_namespace" {
  type = string
  default = null
}

# SSH connection configs.
variable "communicator" {
  type = string
  default = "ssh"
}
variable "ssh_username" {
  type = string
  default = "packer"
}
variable "ssh_password" {
  type = string
  default = "packer"
  sensitive = true
}
variable "ssh_bastion_host" {
  type = string
  default = null
}
variable "ssh_bastion_username" {
  type = string
  default = null
}
variable "ssh_bastion_password" {
  type = string
  default = null
  sensitive = true
}

# Whether to keep the created source VM after the build.
variable "keep_input_artifact" {
  type = bool
  default = false
}

# VM publishing configs.
variable "publish_location_name" {
  type = string
  default = null
}
variable "publish_image_name" {
  type = string
  default = null
}

source "vsphere-supervisor" "vm" {
  kubeconfig_path = "${var.kubeconfig_path}"
  supervisor_namespace = "${var.supervisor_namespace}"
  class_name = "${var.class_name}"
  image_name = "${var.image_name}"
  source_name = "${var.source_name}"
  storage_class = "${var.storage_class}"
  bootstrap_provider = "${var.bootstrap_provider}"
  bootstrap_data_file = "${var.bootstrap_data_file}"
  communicator = "${var.communicator}"
  ssh_username = "${var.ssh_username}"
  ssh_password = "${var.ssh_password}"
  ssh_bastion_host = "${var.ssh_bastion_host}"
  ssh_bastion_username = "${var.ssh_bastion_username}"
  ssh_bastion_password = "${var.ssh_bastion_password}"
  keep_input_artifact = "${var.keep_input_artifact}"
  publish_location_name = "${var.publish_location_name}"
  publish_image_name = "${var.publish_image_name}"
}

build {
  sources = ["source.vsphere-supervisor.vm"]

  # Jenkins job configuration file.
  provisioner "file" {
    destination = "/tmp/sample-job.xml"
    content = <<EOF
<?xml version='1.1' encoding='UTF-8'?>
<project>
  <description>A sample job</description>
  <builders>
    <hudson.tasks.Shell>
      <command>echo "Hello VM-Service from Jenkins"</command>
    </hudson.tasks.Shell>
  </builders>
</project>
EOF
  }

  # Jenkins root URL. The CLI refuses to connect until one is configured.
  provisioner "file" {
    destination = "/tmp/jenkins.model.JenkinsLocationConfiguration.xml"
    content = <<EOF
<?xml version='1.1' encoding='UTF-8'?>
<jenkins.model.JenkinsLocationConfiguration>
  <jenkinsUrl>http://localhost:8080/</jenkinsUrl>
</jenkins.model.JenkinsLocationConfiguration>
EOF
  }

  provisioner "shell" {
    inline = [
      # Display the commands being executed.
      "set -x",

      # On failure, dump enough guest state to tell a slow download apart from a slow guest.
      "trap 'rc=$?; if [ $rc -ne 0 ]; then sudo systemctl status jenkins --no-pager || true; sudo journalctl -u jenkins --no-pager -n 200 || true; uptime || true; vmstat 1 5 || true; df -h || true; fi' EXIT",

      # Remove outdated Kubernetes apt repository if present (no longer has a Release file).
      "sudo rm -f /etc/apt/sources.list.d/kubernetes.list",

      # Sometimes apt-get uses IPv6 and causes failure, force to use IPv4 address.
      # Bound each fetch so a stalled mirror fails fast instead of hanging the build.
      "APT='sudo apt-get -qq -o Acquire::ForceIPv4=true -o Acquire::http::Timeout=60 -o Acquire::https::Timeout=60 -o Acquire::Retries=3'",
      "$APT update",
      "$APT install -y ca-certificates openjdk-21-jre-headless wget",

      # Install Jenkins from a fixed version .deb package directly (avoids GPG key issues with apt repository).
      # Use a current LTS: old releases stay on only a few mirrors and downloads of them can stall for hours.
      # If the mirror picked by get.jenkins.io stalls, fall back to archives.jenkins.io, which keeps every release.
      "for site in https://get.jenkins.io https://archives.jenkins.io; do timeout 300 wget -nv --timeout=60 --tries=2 -O /tmp/jenkins.deb $site/debian-stable/jenkins_2.580.1_all.deb && break; done",

      # The first start can exceed systemd's default 90s on a slow guest, so allow more time.
      "sudo mkdir -p /etc/systemd/system/jenkins.service.d",
      "printf '[Service]\\nTimeoutStartSec=600\\n' | sudo tee /etc/systemd/system/jenkins.service.d/timeout.conf",

      # Installing the local .deb through apt also pulls in its dependencies (e.g. net-tools).
      "$APT install -y /tmp/jenkins.deb",

      # Set the Jenkins URL, which the CLI requires, and restart Jenkins so it is read.
      # The package creates /var/lib/jenkins and the jenkins user, so do this after installing it.
      "sudo install -o jenkins -g jenkins -m 0644 /tmp/jenkins.model.JenkinsLocationConfiguration.xml /var/lib/jenkins/",
      "sudo systemctl restart jenkins",
      # Wait up to 10 minutes for Jenkins to be fully up and running.
      "i=0; until curl -s -o /dev/null -w '%%{http_code}' http://localhost:8080/ | grep -qE '^(200|403)$'; do i=$((i+1)); [ $i -lt 120 ] || exit 1; sleep 5; done",

      "export JENKINS_URL=http://localhost:8080/",
      "export USER=admin",
      "export PASSWORD=$(sudo cat /var/lib/jenkins/secrets/initialAdminPassword)",
      # Download Jenkins CLI to create and check job status.
      "wget -nv --timeout=60 --tries=3 -O /tmp/jenkins-cli.jar $JENKINS_URL/jnlpJars/jenkins-cli.jar",
      # Create a new job from the above sample-job.xml file.
      "java -jar /tmp/jenkins-cli.jar -s $JENKINS_URL -auth $USER:$PASSWORD create-job sample-job < /tmp/sample-job.xml",
      # Build and wait for a successful completion of the job.
      "java -jar /tmp/jenkins-cli.jar -s $JENKINS_URL -auth $USER:$PASSWORD build sample-job -s -v",
    ]
  }
}
