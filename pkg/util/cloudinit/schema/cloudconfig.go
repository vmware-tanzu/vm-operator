// This file was generated from JSON Schema using quicktype, do not modify it directly.
// To parse and unparse this JSON data, add this code to your project and do:
//
//    cloudconfig, err := UnmarshalCloudconfig(bytes)
//    bytes, err = cloudconfig.Marshal()

package schema

import (
	"bytes"
	"encoding/json"
	"errors"
)

func UnmarshalCloudconfig(data []byte) (Cloudconfig, error) {
	var r Cloudconfig
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *Cloudconfig) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type Cloudconfig struct {
	// If ``true``, will import the public SSH keys from the datasource's metadata to the user's
	// ``.ssh/authorized_keys`` file. Default: ``true``
	AllowPublicSSHKeys *bool               `json:"allow_public_ssh_keys,omitempty"`
	Ansible            *AnsibleClass       `json:"ansible,omitempty"`
	ApkRepos           *ApkRepos           `json:"apk_repos,omitempty"`
	Apt                *Apt                `json:"apt,omitempty"`
	AptPipelining      *AptPipeliningUnion `json:"apt_pipelining,omitempty"`
	// Default: ``false``.
	AptRebootIfRequired *bool `json:"apt_reboot_if_required,omitempty"`
	// Default: ``false``.
	AptUpdate *bool `json:"apt_update,omitempty"`
	// Default: ``false``.
	AptUpgrade *bool `json:"apt_upgrade,omitempty"`
	// The hash type to use when generating SSH fingerprints. Default: ``sha256``
	AuthkeyHash *string `json:"authkey_hash,omitempty"`
	// Opaque autoinstall schema definition for Ubuntu autoinstall. Full schema processed by
	// live-installer. See: https://ubuntu.com/server/docs/install/autoinstall-reference
	Autoinstall        *Autoinstall               `json:"autoinstall,omitempty"`
	Bootcmd            []Cmd                      `json:"bootcmd,omitempty"`
	ByobuByDefault     *ByobuByDefault            `json:"byobu_by_default,omitempty"`
	CACerts            *CACertsClass              `json:"ca-certs,omitempty"`
	CloudconfigCACerts *CACertsClass              `json:"ca_certs,omitempty"`
	Chef               *ChefClass                 `json:"chef,omitempty"`
	Chpasswd           *Chpasswd                  `json:"chpasswd,omitempty"`
	CloudConfigModules []CloudConfigModuleElement `json:"cloud_config_modules,omitempty"`
	CloudFinalModules  []CloudConfigModuleElement `json:"cloud_final_modules,omitempty"`
	CloudInitModules   []CloudConfigModuleElement `json:"cloud_init_modules,omitempty"`
	// If ``false``, the hostname file (e.g. /etc/hostname) will not be created if it does not
	// exist. On systems that use systemd, setting create_hostname_file to ``false`` will set
	// the hostname transiently. If ``true``, the hostname file will always be created and the
	// hostname will be set statically on systemd systems. Default: ``true``
	CreateHostnameFile *bool          `json:"create_hostname_file,omitempty"`
	DeviceAliases      *DeviceAliases `json:"device_aliases,omitempty"`
	// Set true to disable IPv4 routes to EC2 metadata. Default: ``false``
	DisableEc2Metadata *bool `json:"disable_ec2_metadata,omitempty"`
	// Disable root login. Default: ``true``
	DisableRoot *bool `json:"disable_root,omitempty"`
	// Disable root login options.  If ``disable_root_opts`` is specified and contains the
	// string ``$USER``, it will be replaced with the username of the default user. Default:
	// ``no-port-forwarding,no-agent-forwarding,no-X11-forwarding,command="echo 'Please login as
	// the user \"$USER\" rather than the user \"$DISABLE_USER\".';echo;sleep 10;exit 142"``
	DisableRootOpts *string         `json:"disable_root_opts,omitempty"`
	DiskSetup       *DiskSetupClass `json:"disk_setup,omitempty"`
	Drivers         *Drivers        `json:"drivers,omitempty"`
	Fan             *FanClass       `json:"fan,omitempty"`
	// The message to display at the end of the run
	FinalMessage *string `json:"final_message,omitempty"`
	// The fully qualified domain name to set
	//
	// Optional fully qualified domain name to use when updating ``/etc/hosts``. Preferred over
	// ``hostname`` if both are provided. In absence of ``hostname`` and ``fqdn`` in
	// cloud-config, the ``local-hostname`` value will be used from datasource metadata.
	FQDN     *string            `json:"fqdn,omitempty"`
	FSSetup  []FSSetup          `json:"fs_setup,omitempty"`
	Groups   *CloudconfigGroups `json:"groups,omitempty"`
	Growpart *Growpart          `json:"growpart,omitempty"`
	// An alias for ``grub_dpkg``
	GrubDpkg            map[string]interface{} `json:"grub-dpkg,omitempty"`
	CloudconfigGrubDpkg *GrubDpkgClass         `json:"grub_dpkg,omitempty"`
	// The hostname to set
	//
	// Hostname to set when rendering ``/etc/hosts``. If ``fqdn`` is set, the hostname extracted
	// from ``fqdn`` overrides ``hostname``.
	Hostname  *string         `json:"hostname,omitempty"`
	Keyboard  *KeyboardClass  `json:"keyboard,omitempty"`
	Landscape *LandscapeClass `json:"landscape,omitempty"`
	// The launch index for the specified cloud-config.
	LaunchIndex *int64 `json:"launch-index,omitempty"`
	// The locale to set as the system's locale (e.g. ar_PS)
	Locale *string `json:"locale,omitempty"`
	// The file in which to write the locale configuration (defaults to the distro's default
	// location)
	LocaleConfigfile *string   `json:"locale_configfile,omitempty"`
	Lxd              *LxdClass `json:"lxd,omitempty"`
	// Whether to manage ``/etc/hosts`` on the system. If ``true``, render the hosts file using
	// ``/etc/cloud/templates/hosts.tmpl`` replacing ``$hostname`` and ``$fdqn``. If
	// ``localhost``, append a ``127.0.1.1`` entry that resolves from FQDN and hostname every
	// boot. Default: ``false``
	ManageEtcHosts *ManageEtcHostsUnion `json:"manage_etc_hosts,omitempty"`
	// Whether to manage the resolv.conf file. ``resolv_conf`` block will be ignored unless this
	// is set to ``true``. Default: ``false``
	ManageResolvConf *bool             `json:"manage_resolv_conf,omitempty"`
	Mcollective      *McollectiveClass `json:"mcollective,omitempty"`
	MergeHow         *MergeHow         `json:"merge_how,omitempty"`
	MergeType        *MergeHow         `json:"merge_type,omitempty"`
	// Whether to migrate legacy cloud-init semaphores to new format. Default: ``true``
	Migrate *bool `json:"migrate,omitempty"`
	// Default mount configuration for any mount entry with less than 6 options provided. When
	// specified, 6 items are required and represent ``/etc/fstab`` entries. Default:
	// ``defaults,nofail,x-systemd.requires=cloud-init.service,_netdev``
	MountDefaultFields []*string `json:"mount_default_fields,omitempty"`
	// List of lists. Each inner list entry is a list of ``/etc/fstab`` mount declarations of
	// the format: [ fs_spec, fs_file, fs_vfstype, fs_mntops, fs-freq, fs_passno ]. A mount
	// declaration with less than 6 items will get remaining values from
	// ``mount_default_fields``. A mount declaration with only `fs_spec` and no `fs_file`
	// mountpoint will be skipped.
	Mounts [][]string `json:"mounts,omitempty"`
	// If true, SSH fingerprints will not be written. Default: ``false``
	NoSSHFingerprints *bool     `json:"no_ssh_fingerprints,omitempty"`
	NTP               *NTPClass `json:"ntp,omitempty"`
	Output            *Output   `json:"output,omitempty"`
	// Set ``true`` to reboot the system if required by presence of `/var/run/reboot-required`.
	// Default: ``false``
	PackageRebootIfRequired *bool `json:"package_reboot_if_required,omitempty"`
	// Set ``true`` to update packages. Happens before upgrade or install. Default: ``false``
	PackageUpdate *bool `json:"package_update,omitempty"`
	// Set ``true`` to upgrade packages. Happens before install. Default: ``false``
	PackageUpgrade *bool `json:"package_upgrade,omitempty"`
	// An array containing either a package specification, or an object consisting of a package
	// manager key having a package specification value . A package specification can be either
	// a package name or a list with two entries, the first being the package name and the
	// second being the specific package version to install.
	Packages []PackageElement `json:"packages,omitempty"`
	// Set the default user's password. Ignored if ``chpasswd`` ``list`` is used
	Password   *string         `json:"password,omitempty"`
	PhoneHome  *PhoneHomeClass `json:"phone_home,omitempty"`
	PowerState *PowerState     `json:"power_state,omitempty"`
	// If true, the fqdn will be used if it is set. If false, the hostname will be used. If
	// unset, the result is distro-dependent
	//
	// By default, it is distro-dependent whether cloud-init uses the short hostname or fully
	// qualified domain name when both ``local-hostname` and ``fqdn`` are both present in
	// instance metadata. When set ``true``, use fully qualified domain name if present as
	// hostname instead of short hostname. When set ``false``, use ``hostname`` config value if
	// present, otherwise fallback to ``fqdn``.
	PreferFQDNOverHostname *bool `json:"prefer_fqdn_over_hostname,omitempty"`
	// If true, the hostname will not be changed. Default: ``false``
	//
	// Do not update system hostname when ``true``. Default: ``false``.
	PreserveHostname *bool        `json:"preserve_hostname,omitempty"`
	Puppet           *PuppetClass `json:"puppet,omitempty"`
	RandomSeed       *RandomSeed  `json:"random_seed,omitempty"`
	Reporting        *Reporting   `json:"reporting,omitempty"`
	// Whether to resize the root partition. ``noblock`` will resize in the background. Default:
	// ``true``
	ResizeRootfs   *ResizeRootfsUnion   `json:"resize_rootfs,omitempty"`
	ResolvConf     *ResolvConfClass     `json:"resolv_conf,omitempty"`
	RhSubscription *RhSubscriptionClass `json:"rh_subscription,omitempty"`
	Rsyslog        *RsyslogClass        `json:"rsyslog,omitempty"`
	Runcmd         []RuncmdElement      `json:"runcmd,omitempty"`
	SaltMinion     *SaltMinionClass     `json:"salt_minion,omitempty"`
	Snap           *SnapClass           `json:"snap,omitempty"`
	Spacewalk      *SpacewalkClass      `json:"spacewalk,omitempty"`
	SSH            *SSHClass            `json:"ssh,omitempty"`
	// The SSH public keys to add ``.ssh/authorized_keys`` in the default user's home directory
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys,omitempty"`
	// Remove host SSH keys. This prevents re-use of a private host key from an image with
	// default host SSH keys. Default: ``true``
	SSHDeletekeys *bool `json:"ssh_deletekeys,omitempty"`
	// Avoid printing matching SSH fingerprints to the system console.
	SSHFPConsoleBlacklist []string `json:"ssh_fp_console_blacklist,omitempty"`
	// The SSH key types to generate. Default: ``[rsa, ecdsa, ed25519]``
	SSHGenkeytypes []SSHGenkeytype `json:"ssh_genkeytypes,omitempty"`
	SSHImportID    []string        `json:"ssh_import_id,omitempty"`
	// Avoid printing matching SSH key types to the system console.
	SSHKeyConsoleBlacklist []string `json:"ssh_key_console_blacklist,omitempty"`
	// A dictionary entries for the public and private host keys of each desired key type.
	// Entries in the ``ssh_keys`` config dict should have keys in the format ``<key
	// type>_private``, ``<key type>_public``, and, optionally, ``<key type>_certificate``, e.g.
	// ``rsa_private: <key>``, ``rsa_public: <key>``, and ``rsa_certificate: <key>``. Not all
	// key types have to be specified, ones left unspecified will not be used. If this config
	// option is used, then separate keys will not be automatically generated. In order to
	// specify multi-line private host keys and certificates, use YAML multi-line syntax.
	// **Note:** Your ssh keys might possibly be visible to unprivileged users on your system,
	// depending on your cloud's security model.
	SSHKeys            *SSHKeys            `json:"ssh_keys,omitempty"`
	SSHPublishHostkeys *SSHPublishHostkeys `json:"ssh_publish_hostkeys,omitempty"`
	// Sets whether or not to accept password authentication. ``true`` will enable password
	// auth. ``false`` will disable. Default: leave the value unchanged. In order for this
	// config to be applied, SSH may need to be restarted. On systemd systems, this restart will
	// only happen if the SSH service has already been started. On non-systemd systems, a
	// restart will be attempted regardless of the service state.
	SSHPwauth *GrubPCInstallDevicesEmpty `json:"ssh_pwauth,omitempty"`
	// If ``true``, will suppress the output of key generation to the console. Default: ``false``
	SSHQuietKeygen *bool `json:"ssh_quiet_keygen,omitempty"`
	Swap           *Swap `json:"swap,omitempty"`
	// The timezone to use as represented in /usr/share/zoneinfo
	Timezone        *string               `json:"timezone,omitempty"`
	UbuntuAdvantage *UbuntuAdvantageClass `json:"ubuntu_advantage,omitempty"`
	Updates         *Updates              `json:"updates,omitempty"`
	// The ``user`` dictionary values override the ``default_user`` configuration from
	// ``/etc/cloud/cloud.cfg``. The `user` dictionary keys supported for the default_user are
	// the same as the ``users`` schema.
	User       *CloudconfigUser `json:"user,omitempty"`
	Users      *Users           `json:"users,omitempty"`
	VendorData *VendorData      `json:"vendor_data,omitempty"`
	Version    interface{}      `json:"version,omitempty"`
	Wireguard  *WireguardClass  `json:"wireguard,omitempty"`
	WriteFiles []WriteFile      `json:"write_files,omitempty"`
	// The repo parts directory where individual yum repo config files will be written. Default:
	// ``/etc/yum.repos.d``
	YumRepoDir *string   `json:"yum_repo_dir,omitempty"`
	YumRepos   *YumRepos `json:"yum_repos,omitempty"`
	Zypper     *Zypper   `json:"zypper,omitempty"`
}

type AnsibleClass struct {
	// Sets the ANSIBLE_CONFIG environment variable. If set, overrides default config.
	AnsibleConfig *string `json:"ansible_config,omitempty"`
	Galaxy        *Galaxy `json:"galaxy,omitempty"`
	// The type of installation for ansible. It can be one of the following values:
	//
	// - ``distro``
	// - ``pip``
	InstallMethod *InstallMethod `json:"install_method,omitempty"`
	PackageName   *string        `json:"package_name,omitempty"`
	Pull          *Pull          `json:"pull,omitempty"`
	// User to run module commands as. If install_method: pip, the pip install runs as this user
	// as well.
	RunUser         *string          `json:"run_user,omitempty"`
	SetupController *SetupController `json:"setup_controller,omitempty"`
}

type Galaxy struct {
	Actions [][]string `json:"actions"`
}

type Pull struct {
	AcceptHostKey     *bool   `json:"accept_host_key,omitempty"`
	Checkout          *string `json:"checkout,omitempty"`
	Clean             *bool   `json:"clean,omitempty"`
	Connection        *string `json:"connection,omitempty"`
	Diff              *bool   `json:"diff,omitempty"`
	Full              *bool   `json:"full,omitempty"`
	ModuleName        *string `json:"module_name,omitempty"`
	ModulePath        *string `json:"module_path,omitempty"`
	PlaybookName      string  `json:"playbook_name"`
	PrivateKey        *string `json:"private_key,omitempty"`
	SCPExtraArgs      *string `json:"scp_extra_args,omitempty"`
	SFTPExtraArgs     *string `json:"sftp_extra_args,omitempty"`
	SkipTags          *string `json:"skip_tags,omitempty"`
	Sleep             *string `json:"sleep,omitempty"`
	SSHCommonArgs     *string `json:"ssh_common_args,omitempty"`
	Tags              *string `json:"tags,omitempty"`
	Timeout           *string `json:"timeout,omitempty"`
	URL               string  `json:"url"`
	VaultID           *string `json:"vault_id,omitempty"`
	VaultPasswordFile *string `json:"vault_password_file,omitempty"`
}

type SetupController struct {
	Repositories []Repository        `json:"repositories,omitempty"`
	RunAnsible   []RunAnsibleElement `json:"run_ansible,omitempty"`
}

type Repository struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

type RunAnsibleClass struct {
	Args                   *string  `json:"args,omitempty"`
	Background             *float64 `json:"background,omitempty"`
	BecomePasswordFile     *string  `json:"become_password_file,omitempty"`
	Check                  *bool    `json:"check,omitempty"`
	Connection             *string  `json:"connection,omitempty"`
	ConnectionPasswordFile *string  `json:"connection_password_file,omitempty"`
	Diff                   *bool    `json:"diff,omitempty"`
	ExtraVars              *string  `json:"extra_vars,omitempty"`
	Forks                  *float64 `json:"forks,omitempty"`
	Inventory              *string  `json:"inventory,omitempty"`
	ListHosts              *bool    `json:"list_hosts,omitempty"`
	ModuleName             *string  `json:"module_name,omitempty"`
	ModulePath             *string  `json:"module_path,omitempty"`
	PlaybookDir            *string  `json:"playbook_dir,omitempty"`
	PlaybookName           *string  `json:"playbook_name,omitempty"`
	Poll                   *float64 `json:"poll,omitempty"`
	PrivateKey             *string  `json:"private_key,omitempty"`
	SCPExtraArgs           *string  `json:"scp_extra_args,omitempty"`
	SFTPExtraArgs          *string  `json:"sftp_extra_args,omitempty"`
	SkipTags               *string  `json:"skip_tags,omitempty"`
	Sleep                  *string  `json:"sleep,omitempty"`
	SyntaxCheck            *bool    `json:"syntax_check,omitempty"`
	Tags                   *string  `json:"tags,omitempty"`
	Timeout                *float64 `json:"timeout,omitempty"`
	VaultID                *string  `json:"vault_id,omitempty"`
	VaultPasswordFile      *string  `json:"vault_password_file,omitempty"`
}

type ApkRepos struct {
	AlpineRepo *AlpineRepo `json:"alpine_repo,omitempty"`
	// The base URL of an Alpine repository containing unofficial packages
	LocalRepoBaseURL *string `json:"local_repo_base_url,omitempty"`
	// By default, cloud-init will generate a new repositories file ``/etc/apk/repositories``
	// based on any valid configuration settings specified within a apk_repos section of cloud
	// config. To disable this behavior and preserve the repositories file from the pristine
	// image, set ``preserve_repositories`` to ``true``.
	//
	// The ``preserve_repositories`` option overrides all other config keys that would alter
	// ``/etc/apk/repositories``.
	PreserveRepositories *bool `json:"preserve_repositories,omitempty"`
}

type AlpineRepo struct {
	// The base URL of an Alpine repository, or mirror, to download official packages from. If
	// not specified then it defaults to ``https://alpine.global.ssl.fastly.net/alpine``
	BaseURL *string `json:"base_url,omitempty"`
	// Whether to add the Community repo to the repositories file. By default the Community repo
	// is not included.
	CommunityEnabled *bool `json:"community_enabled,omitempty"`
	// Whether to add the Testing repo to the repositories file. By default the Testing repo is
	// not included. It is only recommended to use the Testing repo on a machine running the
	// ``Edge`` version of Alpine as packages installed from Testing may have dependencies that
	// conflict with those in non-Edge Main or Community repos.
	TestingEnabled *bool `json:"testing_enabled,omitempty"`
	// The Alpine version to use (e.g. ``v3.12`` or ``edge``)
	Version string `json:"version"`
}

type Apt struct {
	// All source entries in ``apt-sources`` that match regex in ``add_apt_repo_match`` will be
	// added to the system using ``add-apt-repository``. If ``add_apt_repo_match`` is not
	// specified, it defaults to ``^[\w-]+:\w``
	AddAptRepoMatch *string `json:"add_apt_repo_match,omitempty"`
	// Specify configuration for apt, such as proxy configuration. This configuration is
	// specified as a string. For multi-line APT configuration, make sure to follow YAML syntax.
	Conf *string `json:"conf,omitempty"`
	// Debconf additional configurations can be specified as a dictionary under the
	// ``debconf_selections`` config key, with each key in the dict representing a different set
	// of configurations. The value of each key must be a string containing all the debconf
	// configurations that must be applied. We will bundle all of the values and pass them to
	// ``debconf-set-selections``. Therefore, each value line must be a valid entry for
	// ``debconf-set-selections``, meaning that they must possess for distinct fields:
	//
	// ``pkgname question type answer``
	//
	// Where:
	//
	// - ``pkgname`` is the name of the package.
	// - ``question`` the name of the questions.
	// - ``type`` is the type of question.
	// - ``answer`` is the value used to answer the question.
	//
	// For example: ``ippackage ippackage/ip string 127.0.01``
	DebconfSelections *DebconfSelections `json:"debconf_selections,omitempty"`
	// Entries in the sources list can be disabled using ``disable_suites``, which takes a list
	// of suites to be disabled. If the string ``$RELEASE`` is present in a suite in the
	// ``disable_suites`` list, it will be replaced with the release name. If a suite specified
	// in ``disable_suites`` is not present in ``sources.list`` it will be ignored. For
	// convenience, several aliases are provided for`` disable_suites``:
	//
	// - ``updates`` => ``$RELEASE-updates``
	// - ``backports`` => ``$RELEASE-backports``
	// - ``security`` => ``$RELEASE-security``
	// - ``proposed`` => ``$RELEASE-proposed``
	// - ``release`` => ``$RELEASE``.
	//
	// When a suite is disabled using ``disable_suites``, its entry in ``sources.list`` is not
	// deleted; it is just commented out.
	DisableSuites []string `json:"disable_suites,omitempty"`
	// More convenient way to specify ftp APT proxy. ftp proxy url is specified in the format
	// ``ftp://[[user][:pass]@]host[:port]/``.
	FTPProxy *string `json:"ftp_proxy,omitempty"`
	// More convenient way to specify http APT proxy. http proxy url is specified in the format
	// ``http://[[user][:pass]@]host[:port]/``.
	HTTPProxy *string `json:"http_proxy,omitempty"`
	// More convenient way to specify https APT proxy. https proxy url is specified in the
	// format ``https://[[user][:pass]@]host[:port]/``.
	HTTPSProxy *string `json:"https_proxy,omitempty"`
	// By default, cloud-init will generate a new sources list in ``/etc/apt/sources.list.d``
	// based on any changes specified in cloud config. To disable this behavior and preserve the
	// sources list from the pristine image, set ``preserve_sources_list`` to ``true``.
	//
	// The ``preserve_sources_list`` option overrides all other config keys that would alter
	// ``sources.list`` or ``sources.list.d``, **except** for additional sources to be added to
	// ``sources.list.d``.
	PreserveSourcesList *bool `json:"preserve_sources_list,omitempty"`
	// The primary and security archive mirrors can be specified using the ``primary`` and
	// ``security`` keys, respectively. Both the ``primary`` and ``security`` keys take a list
	// of configs, allowing mirrors to be specified on a per-architecture basis. Each config is
	// a dictionary which must have an entry for ``arches``, specifying which architectures that
	// config entry is for. The keyword ``default`` applies to any architecture not explicitly
	// listed. The mirror url can be specified with the ``uri`` key, or a list of mirrors to
	// check can be provided in order, with the first mirror that can be resolved being
	// selected. This allows the same configuration to be used in different environment, with
	// different hosts used for a local APT mirror. If no mirror is provided by ``uri`` or
	// ``search``, ``search_dns`` may be used to search for dns names in the format
	// ``<distro>-mirror`` in each of the following:
	//
	// - fqdn of this host per cloud metadata,
	// - localdomain,
	// - domains listed in ``/etc/resolv.conf``.
	//
	// If there is a dns entry for ``<distro>-mirror``, then it is assumed that there is a
	// distro mirror at ``http://<distro>-mirror.<domain>/<distro>``. If the ``primary`` key is
	// defined, but not the ``security`` key, then then configuration for ``primary`` is also
	// used for ``security``. If ``search_dns`` is used for the ``security`` key, the search
	// pattern will be ``<distro>-security-mirror``.
	//
	// Each mirror may also specify a key to import via any of the following optional keys:
	//
	// - ``keyid``: a key to import via shortid or fingerprint.
	// - ``key``: a raw PGP key.
	// - ``keyserver``: alternate keyserver to pull ``keyid`` key from.
	//
	// If no mirrors are specified, or all lookups fail, then default mirrors defined in the
	// datasource are used. If none are present in the datasource either the following defaults
	// are used:
	//
	// - ``primary`` => ``http://archive.ubuntu.com/ubuntu``.
	// - ``security`` => ``http://security.ubuntu.com/ubuntu``
	Primary []PrimaryElement `json:"primary,omitempty"`
	// Alias for defining a http APT proxy.
	Proxy *string `json:"proxy,omitempty"`
	// Please refer to the primary config documentation
	Security []PrimaryElement `json:"security,omitempty"`
	// Source list entries can be specified as a dictionary under the ``sources`` config key,
	// with each key in the dict representing a different source file. The key of each source
	// entry will be used as an id that can be referenced in other config entries, as well as
	// the filename for the source's configuration under ``/etc/apt/sources.list.d``. If the
	// name does not end with ``.list``, it will be appended. If there is no configuration for a
	// key in ``sources``, no file will be written, but the key may still be referred to as an
	// id in other ``sources`` entries.
	//
	// Each entry under ``sources`` is a dictionary which may contain any of the following
	// optional keys:
	// - ``source``: a sources.list entry (some variable replacements apply).
	// - ``keyid``: a key to import via shortid or fingerprint.
	// - ``key``: a raw PGP key.
	// - ``keyserver``: alternate keyserver to pull ``keyid`` key from.
	// - ``filename``: specify the name of the list file.
	// - ``append``: If ``true``, append to sources file, otherwise overwrite it. Default:
	// ``true``.
	//
	// The ``source`` key supports variable replacements for the following strings:
	//
	// - ``$MIRROR``
	// - ``$PRIMARY``
	// - ``$SECURITY``
	// - ``$RELEASE``
	// - ``$KEY_FILE``
	Sources *Sources `json:"sources,omitempty"`
	// Specifies a custom template for rendering ``sources.list`` . If no ``sources_list``
	// template is given, cloud-init will use sane default. Within this template, the following
	// strings will be replaced with the appropriate values:
	//
	// - ``$MIRROR``
	// - ``$RELEASE``
	// - ``$PRIMARY``
	// - ``$SECURITY``
	// - ``$KEY_FILE``
	SourcesList *string `json:"sources_list,omitempty"`
}

// Debconf additional configurations can be specified as a dictionary under the
// “debconf_selections“ config key, with each key in the dict representing a different set
// of configurations. The value of each key must be a string containing all the debconf
// configurations that must be applied. We will bundle all of the values and pass them to
// “debconf-set-selections“. Therefore, each value line must be a valid entry for
// “debconf-set-selections“, meaning that they must possess for distinct fields:
//
// “pkgname question type answer“
//
// Where:
//
// - “pkgname“ is the name of the package.
// - “question“ the name of the questions.
// - “type“ is the type of question.
// - “answer“ is the value used to answer the question.
//
// For example: “ippackage ippackage/ip string 127.0.01“
type DebconfSelections struct {
}

// The primary and security archive mirrors can be specified using the “primary“ and
// “security“ keys, respectively. Both the “primary“ and “security“ keys take a list
// of configs, allowing mirrors to be specified on a per-architecture basis. Each config is
// a dictionary which must have an entry for “arches“, specifying which architectures that
// config entry is for. The keyword “default“ applies to any architecture not explicitly
// listed. The mirror url can be specified with the “uri“ key, or a list of mirrors to
// check can be provided in order, with the first mirror that can be resolved being
// selected. This allows the same configuration to be used in different environment, with
// different hosts used for a local APT mirror. If no mirror is provided by “uri“ or
// “search“, “search_dns“ may be used to search for dns names in the format
// “<distro>-mirror“ in each of the following:
//
// - fqdn of this host per cloud metadata,
// - localdomain,
// - domains listed in “/etc/resolv.conf“.
//
// If there is a dns entry for “<distro>-mirror“, then it is assumed that there is a
// distro mirror at “http://<distro>-mirror.<domain>/<distro>“. If the “primary“ key is
// defined, but not the “security“ key, then then configuration for “primary“ is also
// used for “security“. If “search_dns“ is used for the “security“ key, the search
// pattern will be “<distro>-security-mirror“.
//
// Each mirror may also specify a key to import via any of the following optional keys:
//
// - “keyid“: a key to import via shortid or fingerprint.
// - “key“: a raw PGP key.
// - “keyserver“: alternate keyserver to pull “keyid“ key from.
//
// If no mirrors are specified, or all lookups fail, then default mirrors defined in the
// datasource are used. If none are present in the datasource either the following defaults
// are used:
//
// - “primary“ => “http://archive.ubuntu.com/ubuntu“.
// - “security“ => “http://security.ubuntu.com/ubuntu“
type PrimaryElement struct {
	Arches    []string `json:"arches"`
	Key       *string  `json:"key,omitempty"`
	Keyid     *string  `json:"keyid,omitempty"`
	Keyserver *string  `json:"keyserver,omitempty"`
	Search    []string `json:"search,omitempty"`
	SearchDNS *bool    `json:"search_dns,omitempty"`
	URI       *string  `json:"uri,omitempty"`
}

// Source list entries can be specified as a dictionary under the “sources“ config key,
// with each key in the dict representing a different source file. The key of each source
// entry will be used as an id that can be referenced in other config entries, as well as
// the filename for the source's configuration under “/etc/apt/sources.list.d“. If the
// name does not end with “.list“, it will be appended. If there is no configuration for a
// key in “sources“, no file will be written, but the key may still be referred to as an
// id in other “sources“ entries.
//
// Each entry under “sources“ is a dictionary which may contain any of the following
// optional keys:
// - “source“: a sources.list entry (some variable replacements apply).
// - “keyid“: a key to import via shortid or fingerprint.
// - “key“: a raw PGP key.
// - “keyserver“: alternate keyserver to pull “keyid“ key from.
// - “filename“: specify the name of the list file.
// - “append“: If “true“, append to sources file, otherwise overwrite it. Default:
// “true“.
//
// The “source“ key supports variable replacements for the following strings:
//
// - “$MIRROR“
// - “$PRIMARY“
// - “$SECURITY“
// - “$RELEASE“
// - “$KEY_FILE“
type Sources struct {
}

// Opaque autoinstall schema definition for Ubuntu autoinstall. Full schema processed by
// live-installer. See: https://ubuntu.com/server/docs/install/autoinstall-reference
type Autoinstall struct {
	Version int64 `json:"version"`
}

type CACertsClass struct {
	RemoveDefaults *bool `json:"remove-defaults,omitempty"`
	// Remove default CA certificates if true. Default: ``false``
	CACertsRemoveDefaults *bool `json:"remove_defaults,omitempty"`
	// List of trusted CA certificates to add.
	Trusted []string `json:"trusted,omitempty"`
}

type ChefClass struct {
	// string that indicates if user accepts or not license related to some of chef products
	ChefLicense *string `json:"chef_license,omitempty"`
	// Optional path for client_cert. Default: ``/etc/chef/client.pem``.
	ClientKey *string `json:"client_key,omitempty"`
	// Create the necessary directories for chef to run. By default, it creates the following
	// directories:
	//
	// - ``/etc/chef``
	// - ``/var/log/chef``
	// - ``/var/lib/chef``
	// - ``/var/cache/chef``
	// - ``/var/backups/chef``
	// - ``/var/run/chef``
	Directories []string `json:"directories,omitempty"`
	// Specifies the location of the secret key used by chef to encrypt data items. By default,
	// this path is set to null, meaning that chef will have to look at the path
	// ``/etc/chef/encrypted_data_bag_secret`` for it.
	EncryptedDataBagSecret *string `json:"encrypted_data_bag_secret,omitempty"`
	// Specifies which environment chef will use. By default, it will use the ``_default``
	// configuration.
	Environment *string `json:"environment,omitempty"`
	// Set true if we should run or not run chef (defaults to false, unless a gem installed is
	// requested where this will then default to true).
	Exec *bool `json:"exec,omitempty"`
	// Specifies the location in which backup files are stored. By default, it uses the
	// ``/var/backups/chef`` location.
	FileBackupPath *string `json:"file_backup_path,omitempty"`
	// Specifies the location in which chef cache files will be saved. By default, it uses the
	// ``/var/cache/chef`` location.
	FileCachePath *string `json:"file_cache_path,omitempty"`
	// Path to write run_list and initial_attributes keys that should also be present in this
	// configuration, defaults to ``/etc/chef/firstboot.json``
	FirstbootPath *string `json:"firstboot_path,omitempty"`
	// If set to ``true``, forces chef installation, even if it is already installed.
	ForceInstall *bool `json:"force_install,omitempty"`
	// Specify a list of initial attributes used by the cookbooks.
	InitialAttributes map[string]interface{} `json:"initial_attributes,omitempty"`
	// The type of installation for chef. It can be one of the following values:
	//
	// - ``packages``
	// - ``gems``
	// - ``omnibus``
	InstallType *ChefInstallType `json:"install_type,omitempty"`
	// Specifies the location in which some chef json data is stored. By default, it uses the
	// ``/etc/chef/firstboot.json`` location.
	JSONAttribs *string `json:"json_attribs,omitempty"`
	// Defines the level of logging to be stored in the log file. By default this value is set
	// to ``:info``.
	LogLevel *string `json:"log_level,omitempty"`
	// Specifies the location of the chef log file. By default, the location is specified at
	// ``/var/log/chef/client.log``.
	LogLocation *string `json:"log_location,omitempty"`
	// The name of the node to run. By default, we will use th instance id as the node name.
	NodeName *string `json:"node_name,omitempty"`
	// Omnibus URL if chef should be installed through Omnibus. By default, it uses the
	// ``https://www.chef.io/chef/install.sh``.
	OmnibusURL *string `json:"omnibus_url,omitempty"`
	// The number of retries that will be attempted to reach the Omnibus URL. Default: ``5``.
	OmnibusURLRetries *int64 `json:"omnibus_url_retries,omitempty"`
	// Optional version string to require for omnibus install.
	OmnibusVersion *string `json:"omnibus_version,omitempty"`
	// The location in which a process identification number (pid) is saved. By default, it
	// saves in the ``/var/run/chef/client.pid`` location.
	PIDFile *string `json:"pid_file,omitempty"`
	// A run list for a first boot json.
	RunList []string `json:"run_list,omitempty"`
	// The URL for the chef server
	ServerURL *string `json:"server_url,omitempty"`
	// Show time in chef logs
	ShowTime *bool `json:"show_time,omitempty"`
	// Set the verify mode for HTTPS requests. We can have two possible values for this
	// parameter:
	//
	// - ``:verify_none``: No validation of SSL certificates.
	// - ``:verify_peer``: Validate all SSL certificates.
	//
	// By default, the parameter is set as ``:verify_none``.
	SSLVerifyMode *string `json:"ssl_verify_mode,omitempty"`
	// Optional string to be written to file validation_key. Special value ``system`` means set
	// use existing file.
	ValidationCERT *string `json:"validation_cert,omitempty"`
	// Optional path for validation_cert. default to ``/etc/chef/validation.pem``
	ValidationKey *string `json:"validation_key,omitempty"`
	// The name of the chef-validator key that Chef Infra Client uses to access the Chef Infra
	// Server during the initial Chef Infra Client run.
	ValidationName *string `json:"validation_name,omitempty"`
}

type Chpasswd struct {
	// Whether to expire all user passwords such that a password will need to be reset on the
	// user's next login. Default: ``true``
	Expire *bool `json:"expire,omitempty"`
	// List of ``username:password`` pairs. Each user will have the corresponding password set.
	// A password can be randomly generated by specifying ``RANDOM`` or ``R`` as a user's
	// password. A hashed password, created by a tool like ``mkpasswd``, can be specified. A
	// regex (``r'\$(1|2a|2y|5|6)(\$.+){2}'``) is used to determine if a password value should
	// be treated as a hash.
	List *ListUnion `json:"list,omitempty"`
	// This key represents a list of existing users to set passwords for. Each item under users
	// contains the following required keys: ``name`` and ``password`` or in the case of a
	// randomly generated password, ``name`` and ``type``. The ``type`` key has a default value
	// of ``hash``, and may alternatively be set to ``text`` or ``RANDOM``. Randomly generated
	// passwords may be insecure, use at your own risk.
	Users []UserClass `json:"users,omitempty"`
}

type UserClass struct {
	Name     string  `json:"name"`
	Type     *Type   `json:"type,omitempty"`
	Password *string `json:"password,omitempty"`
}

type GrubDpkgClass struct {
	// Whether to configure which device is used as the target for grub installation. Default:
	// ``true``
	Enabled *bool `json:"enabled,omitempty"`
	// Partition to use as target for grub installation. If unspecified, ``grub-probe`` of
	// ``/boot/efi`` will be used to find the partition
	GrubEFIInstallDevices *string `json:"grub-efi/install_devices,omitempty"`
	// Device to use as target for grub installation. If unspecified, ``grub-probe`` of
	// ``/boot`` will be used to find the device
	GrubPCInstallDevices *string `json:"grub-pc/install_devices,omitempty"`
	// Sets values for ``grub-pc/install_devices_empty``. If unspecified, will be set to
	// ``true`` if ``grub-pc/install_devices`` is empty, otherwise ``false``
	GrubPCInstallDevicesEmpty *GrubPCInstallDevicesEmpty `json:"grub-pc/install_devices_empty,omitempty"`
}

type DeviceAliases struct {
}

type DiskSetupClass struct {
}

type Drivers struct {
	Nvidia *Nvidia `json:"nvidia,omitempty"`
}

type Nvidia struct {
	// Do you accept the NVIDIA driver license?
	LicenseAccepted bool `json:"license-accepted"`
	// The version of the driver to install (e.g. "390", "410"). Default: latest version.
	Version *string `json:"version,omitempty"`
}

type FSSetup struct {
	// Optional command to run to create the filesystem. Can include string substitutions of the
	// other ``fs_setup`` config keys. This is only necessary if you need to override the
	// default command.
	Cmd *Cmd `json:"cmd,omitempty"`
	// Specified either as a path or as an alias in the format ``<alias name>.<y>`` where
	// ``<y>`` denotes the partition number on the device. If specifying device using the
	// ``<alias name>.<partition number>`` format, the value of ``partition`` will be
	// overwritten.
	Device *string `json:"device,omitempty"`
	// Optional options to pass to the filesystem creation command. Ignored if you using ``cmd``
	// directly.
	ExtraOpts *Cmd `json:"extra_opts,omitempty"`
	// Filesystem type to create. E.g., ``ext4`` or ``btrfs``
	Filesystem *string `json:"filesystem,omitempty"`
	// Label for the filesystem.
	Label *string `json:"label,omitempty"`
	// If ``true``, overwrite any existing filesystem. Using ``overwrite: true`` for filesystems
	// is **dangerous** and can lead to data loss, so double check the entry in ``fs_setup``.
	// Default: ``false``
	Overwrite *bool `json:"overwrite,omitempty"`
	// The partition can be specified by setting ``partition`` to the desired partition number.
	// The ``partition`` option may also be set to ``auto``, in which this module will search
	// for the existence of a filesystem matching the ``label``, ``filesystem`` and ``device``
	// of the ``fs_setup`` entry and will skip creating the filesystem if one is found. The
	// ``partition`` option may also be set to ``any``, in which case any filesystem that
	// matches ``filesystem`` and ``device`` will cause this module to skip filesystem creation
	// for the ``fs_setup`` entry, regardless of ``label`` matching or not. To write a
	// filesystem directly to a device, use ``partition: none``. ``partition: none`` will
	// **always** write the filesystem, even when the ``label`` and ``filesystem`` are matched,
	// and ``overwrite`` is ``false``.
	Partition *Partition `json:"partition,omitempty"`
	// Ignored unless ``partition`` is ``auto`` or ``any``. Default ``false``.
	ReplaceFS *string `json:"replace_fs,omitempty"`
}

type FanClass struct {
	// The fan configuration to use as a single multi-line string
	Config string `json:"config"`
	// The path to write the fan configuration to. Default: ``/etc/network/fan``
	ConfigPath *string `json:"config_path,omitempty"`
}

type GroupsClass struct {
}

type Growpart struct {
	// The devices to resize. Each entry can either be the path to the device's mountpoint in
	// the filesystem or a path to the block device in '/dev'. Default: ``[/]``
	Devices []string `json:"devices,omitempty"`
	// If ``true``, ignore the presence of ``/etc/growroot-disabled``. If ``false`` and the file
	// exists, then don't resize. Default: ``false``
	IgnoreGrowrootDisabled *bool `json:"ignore_growroot_disabled,omitempty"`
	// The utility to use for resizing. Default: ``auto``
	//
	// Possible options:
	//
	// * ``auto`` - Use any available utility
	//
	// * ``growpart`` - Use growpart utility
	//
	// * ``gpart`` - Use BSD gpart utility
	//
	// * ``off`` - Take no action
	Mode *ModeUnion `json:"mode,omitempty"`
}

type KeyboardClass struct {
	// Required. Keyboard layout. Corresponds to XKBLAYOUT.
	Layout string `json:"layout"`
	// Optional. Keyboard model. Corresponds to XKBMODEL. Default: ``pc105``.
	Model *string `json:"model,omitempty"`
	// Optional. Keyboard options. Corresponds to XKBOPTIONS.
	Options *string `json:"options,omitempty"`
	// Required for Alpine Linux, optional otherwise. Keyboard variant. Corresponds to
	// XKBVARIANT.
	Variant *string `json:"variant,omitempty"`
}

type LandscapeClass struct {
	Client Client `json:"client"`
}

type Client struct {
	// The account this computer belongs to.
	AccountName string `json:"account_name"`
	// The title of this computer.
	ComputerTitle string `json:"computer_title"`
	// The directory to store data files in. Default: ``/var/lib/land‐scape/client/``.
	DataPath *string `json:"data_path,omitempty"`
	// The URL of the HTTP proxy, if one is needed.
	HTTPProxy *string `json:"http_proxy,omitempty"`
	// The URL of the HTTPS proxy, if one is needed.
	HTTPSProxy *string `json:"https_proxy,omitempty"`
	// The log level for the client. Default: ``info``.
	LogLevel *LogLevel `json:"log_level,omitempty"`
	// The URL to perform lightweight exchange initiation with. Default:
	// ``https://landscape.canonical.com/ping``.
	PingURL *string `json:"ping_url,omitempty"`
	// The account-wide key used for registering clients.
	RegistrationKey *string `json:"registration_key,omitempty"`
	// Comma separated list of tag names to be sent to the server.
	Tags *string `json:"tags,omitempty"`
	// The Landscape server URL to connect to. Default:
	// ``https://landscape.canonical.com/message-system``.
	URL *string `json:"url,omitempty"`
}

type LxdClass struct {
	// LXD bridge configuration provided to setup the host lxd bridge. Can not be combined with
	// ``lxd.preseed``.
	Bridge *Bridge `json:"bridge,omitempty"`
	// LXD init configuration values to provide to `lxd init --auto` command. Can not be
	// combined with ``lxd.preseed``.
	Init *Init `json:"init,omitempty"`
	// Opaque LXD preseed YAML config passed via stdin to the command: lxd init --preseed. See:
	// https://documentation.ubuntu.com/lxd/en/latest/howto/initialize/#non-interactive-configuration
	// or lxd init --dump for viable config. Can not be combined with either ``lxd.init`` or
	// ``lxd.bridge``.
	Preseed *string `json:"preseed,omitempty"`
}

// LXD bridge configuration provided to setup the host lxd bridge. Can not be combined with
// “lxd.preseed“.
type Bridge struct {
	// Domain to advertise to DHCP clients and use for DNS resolution.
	Domain *string `json:"domain,omitempty"`
	// IPv4 address for the bridge. If set, ``ipv4_netmask`` key required.
	Ipv4Address *string `json:"ipv4_address,omitempty"`
	// First IPv4 address of the DHCP range for the network created. This value will combined
	// with ``ipv4_dhcp_last`` key to set LXC ``ipv4.dhcp.ranges``.
	Ipv4DHCPFirst *string `json:"ipv4_dhcp_first,omitempty"`
	// Last IPv4 address of the DHCP range for the network created. This value will combined
	// with ``ipv4_dhcp_first`` key to set LXC ``ipv4.dhcp.ranges``.
	Ipv4DHCPLast *string `json:"ipv4_dhcp_last,omitempty"`
	// Number of DHCP leases to allocate within the range. Automatically calculated based on
	// `ipv4_dhcp_first` and `ipv4_dchp_last` when unset.
	Ipv4DHCPLeases *int64 `json:"ipv4_dhcp_leases,omitempty"`
	// Set ``true`` to NAT the IPv4 traffic allowing for a routed IPv4 network. Default:
	// ``false``.
	Ipv4Nat *bool `json:"ipv4_nat,omitempty"`
	// Prefix length for the ``ipv4_address`` key. Required when ``ipv4_address`` is set.
	Ipv4Netmask *int64 `json:"ipv4_netmask,omitempty"`
	// IPv6 address for the bridge (CIDR notation). When set, ``ipv6_netmask`` key is required.
	// When absent, no IPv6 will be configured.
	Ipv6Address *string `json:"ipv6_address,omitempty"`
	// Whether to NAT. Default: ``false``.
	Ipv6Nat *bool `json:"ipv6_nat,omitempty"`
	// Prefix length for ``ipv6_address`` provided. Required when ``ipv6_address`` is set.
	Ipv6Netmask *int64 `json:"ipv6_netmask,omitempty"`
	// Whether to setup LXD bridge, use an existing bridge by ``name`` or create a new bridge.
	// `none` will avoid bridge setup, `existing` will configure lxd to use the bring matching
	// ``name`` and `new` will create a new bridge.
	Mode BridgeMode `json:"mode"`
	// Bridge MTU, defaults to LXD's default value
	MTU *int64 `json:"mtu,omitempty"`
	// Name of the LXD network bridge to attach or create. Default: ``lxdbr0``.
	Name *string `json:"name,omitempty"`
}

// LXD init configuration values to provide to `lxd init --auto` command. Can not be
// combined with “lxd.preseed“.
type Init struct {
	// IP address for LXD to listen on
	NetworkAddress *string `json:"network_address,omitempty"`
	// Network port to bind LXD to.
	NetworkPort *int64 `json:"network_port,omitempty"`
	// Storage backend to use. Default: ``dir``.
	StorageBackend *StorageBackend `json:"storage_backend,omitempty"`
	// Setup device based storage using DEVICE
	StorageCreateDevice *string `json:"storage_create_device,omitempty"`
	// Setup loop based storage with SIZE in GB
	StorageCreateLoop *int64 `json:"storage_create_loop,omitempty"`
	// Name of storage pool to use or create
	StoragePool *string `json:"storage_pool,omitempty"`
	// The password required to add new clients
	TrustPassword *string `json:"trust_password,omitempty"`
}

type McollectiveClass struct {
	Conf *McollectiveConf `json:"conf,omitempty"`
}

type McollectiveConf struct {
	// Optional value of server private certificate which will be written to
	// ``/etc/mcollective/ssl/server-private.pem``
	PrivateCERT *string `json:"private-cert,omitempty"`
	// Optional value of server public certificate which will be written to
	// ``/etc/mcollective/ssl/server-public.pem``
	PublicCERT *string `json:"public-cert,omitempty"`
}

type MergeHowElement struct {
	Name     Name      `json:"name"`
	Settings []Setting `json:"settings"`
}

type NTPClass struct {
	// List of CIDRs to allow
	Allow []string `json:"allow,omitempty"`
	// Configuration settings or overrides for the
	// ``ntp_client`` specified.
	Config *NTPConfig `json:"config,omitempty"`
	// Attempt to enable ntp clients if set to True.  If set
	// to False, ntp client will not be configured or
	// installed
	Enabled *bool `json:"enabled,omitempty"`
	// Name of an NTP client to use to configure system NTP.
	// When unprovided or 'auto' the default client preferred
	// by the distribution will be used. The following
	// built-in client names can be used to override existing
	// configuration defaults: chrony, ntp, openntpd,
	// ntpdate, systemd-timesyncd.
	NTPClient *string `json:"ntp_client,omitempty"`
	// List of ntp peers.
	Peers []string `json:"peers,omitempty"`
	// List of ntp pools. If both pools and servers are
	// empty, 4 default pool servers will be provided of
	// the format ``{0-3}.{distro}.pool.ntp.org``. NOTE:
	// for Alpine Linux when using the Busybox NTP client
	// this setting will be ignored due to the limited
	// functionality of Busybox's ntpd.
	Pools []string `json:"pools,omitempty"`
	// List of ntp servers. If both pools and servers are
	// empty, 4 default pool servers will be provided with
	// the format ``{0-3}.{distro}.pool.ntp.org``.
	Servers []string `json:"servers,omitempty"`
}

// Configuration settings or overrides for the
// “ntp_client“ specified.
type NTPConfig struct {
	// The executable name for the ``ntp_client``.
	// For example, ntp service ``check_exe`` is
	// 'ntpd' because it runs the ntpd binary.
	CheckExe *string `json:"check_exe,omitempty"`
	// The path to where the ``ntp_client``
	// configuration is written.
	Confpath *string `json:"confpath,omitempty"`
	// List of packages needed to be installed for the
	// selected ``ntp_client``.
	Packages []string `json:"packages,omitempty"`
	// The systemd or sysvinit service name used to
	// start and stop the ``ntp_client``
	// service.
	ServiceName *string `json:"service_name,omitempty"`
	// Inline template allowing users to customize their ``ntp_client`` configuration with the
	// use of the Jinja templating engine.
	// The template content should start with ``## template:jinja``.
	// Within the template, you can utilize any of the following ntp module config keys:
	// ``servers``, ``pools``, ``allow``, and ``peers``.
	// Each cc_ntp schema config key and expected value type is defined above.
	Template *string `json:"template,omitempty"`
}

type Output struct {
	All    *AllUnion `json:"all,omitempty"`
	Config *AllUnion `json:"config,omitempty"`
	Final  *AllUnion `json:"final,omitempty"`
	Init   *AllUnion `json:"init,omitempty"`
}

type AllClass struct {
	// A filepath operation configuration. A string containing a filepath and an optional
	// leading operator: '>', '>>' or '|'. Operators '>' and '>>' indicate whether to overwrite
	// or append to the file. The operator '|' redirects content to the command arguments
	// specified.
	Error *string `json:"error,omitempty"`
	// A filepath operation configuration. This is a string containing a filepath and an
	// optional leading operator: '>', '>>' or '|'. Operators '>' and '>>' indicate whether to
	// overwrite or append to the file. The operator '|' redirects content to the command
	// arguments specified.
	Output *string `json:"output,omitempty"`
}

type PackageClass struct {
	Apt  []Cmd `json:"apt,omitempty"`
	Snap []Cmd `json:"snap,omitempty"`
}

type PhoneHomeClass struct {
	// A list of keys to post or ``all``. Default: ``all``
	Post *PostUnion `json:"post,omitempty"`
	// The number of times to try sending the phone home data. Default: ``10``
	Tries *int64 `json:"tries,omitempty"`
	// The URL to send the phone home data to.
	URL string `json:"url"`
}

type PowerState struct {
	// Apply state change only if condition is met. May be boolean true (always met), false
	// (never met), or a command string or list to be executed. For command formatting, see the
	// documentation for ``cc_runcmd``. If exit code is 0, condition is met, otherwise not.
	// Default: ``true``
	Condition *Condition `json:"condition,omitempty"`
	// Time in minutes to delay after cloud-init has finished. Can be ``now`` or an integer
	// specifying the number of minutes to delay. Default: ``now``
	Delay *Delay `json:"delay,omitempty"`
	// Optional message to display to the user when the system is powering off or rebooting.
	Message *string `json:"message,omitempty"`
	// Must be one of ``poweroff``, ``halt``, or ``reboot``.
	Mode PowerStateMode `json:"mode"`
	// Time in seconds to wait for the cloud-init process to finish before executing shutdown.
	// Default: ``30``
	Timeout *int64 `json:"timeout,omitempty"`
}

type PuppetClass struct {
	// If ``install_type`` is ``aio``, change the url of the install script.
	AioInstallURL *string `json:"aio_install_url,omitempty"`
	// Whether to remove the puppetlabs repo after installation if ``install_type`` is ``aio``
	// Default: ``true``
	Cleanup *bool `json:"cleanup,omitempty"`
	// Puppet collection to install if ``install_type`` is ``aio``. This can be set to one of
	// ``puppet`` (rolling release), ``puppet6``, ``puppet7`` (or their nightly counterparts) in
	// order to install specific release streams.
	Collection *string `json:"collection,omitempty"`
	// Every key present in the conf object will be added to puppet.conf. As such, section names
	// should be one of: ``main``, ``server``, ``agent`` or ``user`` and keys should be valid
	// puppet configuration options. The configuration is specified as a dictionary containing
	// high-level ``<section>`` keys and lists of ``<key>=<value>`` pairs within each section.
	// The ``certname`` key supports string substitutions for ``%i`` and ``%f``, corresponding
	// to the instance id and fqdn of the machine respectively.
	//
	// ``ca_cert`` is a special case. It won't be added to puppet.conf. It holds the
	// puppetserver certificate in pem format. It should be a multi-line string (using the |
	// YAML notation for multi-line strings).
	Conf *PuppetConf `json:"conf,omitempty"`
	// The path to the puppet config file. Default depends on ``install_type``
	ConfFile *string `json:"conf_file,omitempty"`
	// create a ``csr_attributes.yaml`` file for CSR attributes and certificate extension
	// requests. See https://puppet.com/docs/puppet/latest/config_file_csr_attributes.html
	CsrAttributes *CsrAttributes `json:"csr_attributes,omitempty"`
	// The path to the puppet csr attributes file. Default depends on ``install_type``
	CsrAttributesPath *string `json:"csr_attributes_path,omitempty"`
	// Whether or not to run puppet after configuration finishes. A single manual run can be
	// triggered by setting ``exec`` to ``true``, and additional arguments can be passed to
	// ``puppet agent`` via the ``exec_args`` key (by default the agent will execute with the
	// ``--test`` flag). Default: ``false``
	Exec *bool `json:"exec,omitempty"`
	// A list of arguments to pass to 'puppet agent' if 'exec' is true Default: ``['--test']``
	ExecArgs []string `json:"exec_args,omitempty"`
	// Whether or not to install puppet. Setting to ``false`` will result in an error if puppet
	// is not already present on the system. Default: ``true``
	Install *bool `json:"install,omitempty"`
	// Valid values are ``packages`` and ``aio``. Agent packages from the puppetlabs
	// repositories can be installed by setting ``aio``. Based on this setting, the default
	// config/SSL/CSR paths will be adjusted accordingly. Default: ``packages``
	InstallType *PuppetInstallType `json:"install_type,omitempty"`
	// Name of the package to install if ``install_type`` is ``packages``. Default: ``puppet``
	PackageName *string `json:"package_name,omitempty"`
	// The path to the puppet SSL directory. Default depends on ``install_type``
	SSLDir *string `json:"ssl_dir,omitempty"`
	// By default, the puppet service will be automatically enabled after installation and set
	// to automatically start on boot. To override this in favor of manual puppet execution set
	// ``start_service`` to ``false``
	StartService *bool `json:"start_service,omitempty"`
	// Optional version to pass to the installer script or package manager. If unset, the latest
	// version from the repos will be installed.
	Version *string `json:"version,omitempty"`
}

// Every key present in the conf object will be added to puppet.conf. As such, section names
// should be one of: “main“, “server“, “agent“ or “user“ and keys should be valid
// puppet configuration options. The configuration is specified as a dictionary containing
// high-level “<section>“ keys and lists of “<key>=<value>“ pairs within each section.
// The “certname“ key supports string substitutions for “%i“ and “%f“, corresponding
// to the instance id and fqdn of the machine respectively.
//
// “ca_cert“ is a special case. It won't be added to puppet.conf. It holds the
// puppetserver certificate in pem format. It should be a multi-line string (using the |
// YAML notation for multi-line strings).
type PuppetConf struct {
	Agent  map[string]interface{} `json:"agent,omitempty"`
	CACERT *string                `json:"ca_cert,omitempty"`
	Main   map[string]interface{} `json:"main,omitempty"`
	Server map[string]interface{} `json:"server,omitempty"`
	User   map[string]interface{} `json:"user,omitempty"`
}

// create a “csr_attributes.yaml“ file for CSR attributes and certificate extension
// requests. See https://puppet.com/docs/puppet/latest/config_file_csr_attributes.html
type CsrAttributes struct {
	CustomAttributes  map[string]interface{} `json:"custom_attributes,omitempty"`
	ExtensionRequests map[string]interface{} `json:"extension_requests,omitempty"`
}

type RandomSeed struct {
	// Execute this command to seed random. The command will have RANDOM_SEED_FILE in its
	// environment set to the value of ``file`` above.
	Command []string `json:"command,omitempty"`
	// If true, and ``command`` is not available to be run then an exception is raised and
	// cloud-init will record failure. Otherwise, only debug error is mentioned. Default:
	// ``false``
	CommandRequired *bool `json:"command_required,omitempty"`
	// This data will be written to ``file`` before data from the datasource. When using a
	// multi-line value or specifying binary data, be sure to follow YAML syntax and use the
	// ``|`` and ``!binary`` YAML format specifiers when appropriate
	Data *string `json:"data,omitempty"`
	// Used to decode ``data`` provided. Allowed values are ``raw``, ``base64``, ``b64``,
	// ``gzip``, or ``gz``.  Default: ``raw``
	Encoding *RandomSeedEncoding `json:"encoding,omitempty"`
	// File to write random data to. Default: ``/dev/urandom``
	File *string `json:"file,omitempty"`
}

type Reporting struct {
}

type ResolvConfClass struct {
	// The domain to be added as ``domain`` line
	Domain *string `json:"domain,omitempty"`
	// A list of nameservers to use to be added as ``nameserver`` lines
	Nameservers []interface{} `json:"nameservers,omitempty"`
	// Key/value pairs of options to go under ``options`` heading. A unary option should be
	// specified as ``true``
	Options map[string]interface{} `json:"options,omitempty"`
	// A list of domains to be added ``search`` line
	Searchdomains []interface{} `json:"searchdomains,omitempty"`
	// A list of IP addresses to be added to ``sortlist`` line
	Sortlist []interface{} `json:"sortlist,omitempty"`
}

type RhSubscriptionClass struct {
	// The activation key to use. Must be used with ``org``. Should not be used with
	// ``username`` or ``password``
	ActivationKey *string `json:"activation-key,omitempty"`
	// A list of pools ids add to the subscription
	AddPool []string `json:"add-pool,omitempty"`
	// Whether to attach subscriptions automatically
	AutoAttach *bool `json:"auto-attach,omitempty"`
	// A list of repositories to disable
	DisableRepo []string `json:"disable-repo,omitempty"`
	// A list of repositories to enable
	EnableRepo []string `json:"enable-repo,omitempty"`
	// The organization number to use. Must be used with ``activation-key``. Should not be used
	// with ``username`` or ``password``
	Org *int64 `json:"org,omitempty"`
	// The password to use. Must be used with username. Should not be used with
	// ``activation-key`` or ``org``
	Password *string `json:"password,omitempty"`
	// Sets the baseurl in ``/etc/rhsm/rhsm.conf``
	RhsmBaseurl *string `json:"rhsm-baseurl,omitempty"`
	// Sets the serverurl in ``/etc/rhsm/rhsm.conf``
	ServerHostname *string `json:"server-hostname,omitempty"`
	// The service level to use when subscribing to RH repositories. ``auto-attach`` must be
	// true for this to be used
	ServiceLevel *string `json:"service-level,omitempty"`
	// The username to use. Must be used with password. Should not be used with
	// ``activation-key`` or ``org``
	Username *string `json:"username,omitempty"`
}

type RsyslogClass struct {
	// The executable name for the rsyslog daemon.
	// For example, ``rsyslogd``, or ``/opt/sbin/rsyslogd`` if the rsyslog binary is in an
	// unusual path. This is only used if ``install_rsyslog`` is ``true``. Default: ``rsyslogd``
	CheckExe *string `json:"check_exe,omitempty"`
	// The directory where rsyslog configuration files will be written. Default:
	// ``/etc/rsyslog.d``
	ConfigDir *string `json:"config_dir,omitempty"`
	// The name of the rsyslog configuration file. Default: ``20-cloud-config.conf``
	ConfigFilename *string `json:"config_filename,omitempty"`
	// Each entry in ``configs`` is either a string or an object. Each config entry contains a
	// configuration string and a file to write it to. For config entries that are an object,
	// ``filename`` sets the target filename and ``content`` specifies the config string to
	// write. For config entries that are only a string, the string is used as the config string
	// to write. If the filename to write the config to is not specified, the value of the
	// ``config_filename`` key is used. A file with the selected filename will be written inside
	// the directory specified by ``config_dir``.
	Configs []ConfigElement `json:"configs,omitempty"`
	// Install rsyslog. Default: ``false``
	InstallRsyslog *bool `json:"install_rsyslog,omitempty"`
	// List of packages needed to be installed for rsyslog. This is only used if
	// ``install_rsyslog`` is ``true``. Default: ``[rsyslog]``
	Packages []string `json:"packages,omitempty"`
	// Each key is the name for an rsyslog remote entry. Each value holds the contents of the
	// remote config for rsyslog. The config consists of the following parts:
	//
	// - filter for log messages (defaults to ``*.*``)
	//
	// - optional leading ``@`` or ``@@``, indicating udp and tcp respectively (defaults to
	// ``@``, for udp)
	//
	// - ipv4 or ipv6 hostname or address. ipv6 addresses must be in ``[::1]`` format, (e.g.
	// ``@[fd00::1]:514``)
	//
	// - optional port number (defaults to ``514``)
	//
	// This module will provide sane defaults for any part of the remote entry that is not
	// specified, so in most cases remote hosts can be specified just using ``<name>:
	// <address>``.
	Remotes map[string]interface{} `json:"remotes,omitempty"`
	// The command to use to reload the rsyslog service after the config has been updated. If
	// this is set to ``auto``, then an appropriate command for the distro will be used. This is
	// the default behavior. To manually set the command, use a list of command args (e.g.
	// ``[systemctl, restart, rsyslog]``).
	ServiceReloadCommand *ServiceReloadCommandUnion `json:"service_reload_command,omitempty"`
}

type ConfigConfig struct {
	Content  string  `json:"content"`
	Filename *string `json:"filename,omitempty"`
}

type SSHClass struct {
	// Set false to avoid printing SSH keys to system console. Default: ``true``.
	EmitKeysToConsole bool `json:"emit_keys_to_console"`
}

// A dictionary entries for the public and private host keys of each desired key type.
// Entries in the “ssh_keys“ config dict should have keys in the format “<key
// type>_private“, “<key type>_public“, and, optionally, “<key type>_certificate“, e.g.
// “rsa_private: <key>“, “rsa_public: <key>“, and “rsa_certificate: <key>“. Not all
// key types have to be specified, ones left unspecified will not be used. If this config
// option is used, then separate keys will not be automatically generated. In order to
// specify multi-line private host keys and certificates, use YAML multi-line syntax.
// **Note:** Your ssh keys might possibly be visible to unprivileged users on your system,
// depending on your cloud's security model.
type SSHKeys struct {
}

type SSHPublishHostkeys struct {
	// The SSH key types to ignore when publishing. Default: ``[]`` to publish all SSH key types
	Blacklist []string `json:"blacklist,omitempty"`
	// If true, will read host keys from ``/etc/ssh/*.pub`` and publish them to the datasource
	// (if supported). Default: ``true``
	Enabled *bool `json:"enabled,omitempty"`
}

type SaltMinionClass struct {
	// Configuration to be written to `config_dir`/minion
	Conf map[string]interface{} `json:"conf,omitempty"`
	// Directory to write config files to. Default: ``/etc/salt``
	ConfigDir *string `json:"config_dir,omitempty"`
	// Configuration to be written to `config_dir`/grains
	Grains map[string]interface{} `json:"grains,omitempty"`
	// Package name to install. Default: ``salt-minion``
	PkgName *string `json:"pkg_name,omitempty"`
	// Directory to write key files. Default: `config_dir`/pki/minion
	PKIDir *string `json:"pki_dir,omitempty"`
	// Private key to be used by salt minion
	PrivateKey *string `json:"private_key,omitempty"`
	// Public key to be used by the salt minion
	PublicKey *string `json:"public_key,omitempty"`
	// Service name to enable. Default: ``salt-minion``
	ServiceName *string `json:"service_name,omitempty"`
}

type SnapClass struct {
	// Properly-signed snap assertions which will run before and snap ``commands``.
	Assertions *Assertions `json:"assertions,omitempty"`
	// Snap commands to run on the target system
	Commands *Commands `json:"commands,omitempty"`
}

type SpacewalkClass struct {
	// The activation key to use when registering with Spacewalk
	ActivationKey *string `json:"activation_key,omitempty"`
	// The proxy to use when connecting to Spacewalk
	Proxy *string `json:"proxy,omitempty"`
	// The Spacewalk server to use
	Server *string `json:"server,omitempty"`
}

type Swap struct {
	// Path to the swap file to create
	Filename *string `json:"filename,omitempty"`
	// The maxsize in bytes of the swap file
	Maxsize *Size `json:"maxsize,omitempty"`
	// The size in bytes of the swap file, 'auto' or a human-readable size abbreviation of the
	// format <float_size><units> where units are one of B, K, M, G or T. **WARNING: Attempts to
	// use IEC prefixes in your configuration prior to cloud-init version 23.1 will result in
	// unexpected behavior. SI prefixes names (KB, MB) are required on pre-23.1 cloud-init,
	// however IEC values are used. In summary, assume 1KB == 1024B, not 1000B**
	Size *Size `json:"size,omitempty"`
}

type UbuntuAdvantageClass struct {
	// Configuration settings or override Ubuntu Advantage config.
	Config *UbuntuAdvantageConfig `json:"config,omitempty"`
	// Optional list of ubuntu-advantage services to enable. Any of: cc-eal, cis, esm-infra,
	// fips, fips-updates, livepatch. By default, a given contract token will automatically
	// enable a number of services, use this list to supplement which services should
	// additionally be enabled. Any service unavailable on a given Ubuntu release or unentitled
	// in a given contract will remain disabled. In Ubuntu Pro instances, if this list is given,
	// then only those services will be enabled, ignoring contract defaults. Passing beta
	// services here will cause an error.
	Enable []string `json:"enable,omitempty"`
	// Optional list of ubuntu-advantage beta services to enable. By default, a given contract
	// token will automatically enable a number of services, use this list to supplement which
	// services should additionally be enabled. Any service unavailable on a given Ubuntu
	// release or unentitled in a given contract will remain disabled. In Ubuntu Pro instances,
	// if this list is given, then only those services will be enabled, ignoring contract
	// defaults.
	EnableBeta []string `json:"enable_beta,omitempty"`
	// Ubuntu Advantage features.
	Features *Features `json:"features,omitempty"`
	// Contract token obtained from https://ubuntu.com/advantage to attach. Required for non-Pro
	// instances.
	Token *string `json:"token,omitempty"`
}

// Configuration settings or override Ubuntu Advantage config.
type UbuntuAdvantageConfig struct {
	// HTTP Proxy URL used for all APT repositories on a system or null to unset. Stored at
	// ``/etc/apt/apt.conf.d/90ubuntu-advantage-aptproxy``
	GlobalAptHTTPProxy *string `json:"global_apt_http_proxy,omitempty"`
	// HTTPS Proxy URL used for all APT repositories on a system or null to unset. Stored at
	// ``/etc/apt/apt.conf.d/90ubuntu-advantage-aptproxy``
	GlobalAptHTTPSProxy *string `json:"global_apt_https_proxy,omitempty"`
	// Ubuntu Advantage HTTP Proxy URL or null to unset.
	HTTPProxy *string `json:"http_proxy,omitempty"`
	// Ubuntu Advantage HTTPS Proxy URL or null to unset.
	HTTPSProxy *string `json:"https_proxy,omitempty"`
	// HTTP Proxy URL used only for Ubuntu Advantage APT repositories or null to unset. Stored
	// at ``/etc/apt/apt.conf.d/90ubuntu-advantage-aptproxy``
	UaAptHTTPProxy *string `json:"ua_apt_http_proxy,omitempty"`
	// HTTPS Proxy URL used only for Ubuntu Advantage APT repositories or null to unset. Stored
	// at ``/etc/apt/apt.conf.d/90ubuntu-advantage-aptproxy``
	UaAptHTTPSProxy *string `json:"ua_apt_https_proxy,omitempty"`
}

// Ubuntu Advantage features.
type Features struct {
	// Optional boolean for controlling if ua-auto-attach.service (in Ubuntu Pro instances) will
	// be attempted each boot. Default: ``false``
	DisableAutoAttach *bool `json:"disable_auto_attach,omitempty"`
}

type Updates struct {
	Network *Network `json:"network,omitempty"`
}

type Network struct {
	When []When `json:"when"`
}

type PurpleSchemaCloudConfigV1 struct {
	// Boolean set ``false`` to disable creation of specified user ``groups``. Default: ``true``.
	CreateGroups *bool `json:"create_groups,omitempty"`
	// List of doas rules to add for a user. doas or opendoas must be installed for rules to
	// take effect.
	Doas []string `json:"doas,omitempty"`
	// Optional. Date on which the user's account will be disabled. Default: ``null``
	Expiredate *string `json:"expiredate,omitempty"`
	// Optional comment about the user, usually a comma-separated string of real name and
	// contact information
	Gecos *string `json:"gecos,omitempty"`
	// Optional comma-separated string of groups to add the user to.
	Groups *UserGroups `json:"groups,omitempty"`
	// Hash of user password to be applied. This will be applied even if the user is
	// preexisting. To generate this hash, run: ``mkpasswd --method=SHA-512 --rounds=500000``.
	// **Note:** Your password might possibly be visible to unprivileged users on your system,
	// depending on your cloud's security model. Check if your cloud's IMDS server is visible
	// from an unprivileged user to evaluate risk.
	HashedPasswd *string `json:"hashed_passwd,omitempty"`
	// Optional home dir for user. Default: ``/home/<username>``
	Homedir *string `json:"homedir,omitempty"`
	// Optional string representing the number of days until the user is disabled.
	Inactive *string `json:"inactive,omitempty"`
	// Default: ``true``
	LockPasswd *bool `json:"lock-passwd,omitempty"`
	// Disable password login. Default: ``true``
	SchemaCloudConfigV1LockPasswd *bool `json:"lock_passwd,omitempty"`
	// The user's login name. Required otherwise user creation will be skipped for this user.
	Name *string `json:"name,omitempty"`
	// Do not create home directory. Default: ``false``
	NoCreateHome *bool `json:"no_create_home,omitempty"`
	// Do not initialize lastlog and faillog for user. Default: ``false``
	NoLogInit *bool `json:"no_log_init,omitempty"`
	// Do not create group named after user. Default: ``false``
	NoUserGroup *bool `json:"no_user_group,omitempty"`
	// Hash of user password applied when user does not exist. This will NOT be applied if the
	// user already exists. To generate this hash, run: ``mkpasswd --method=SHA-512
	// --rounds=500000`` **Note:** Your password might possibly be visible to unprivileged users
	// on your system, depending on your cloud's security model. Check if your cloud's IMDS
	// server is visible from an unprivileged user to evaluate risk.
	Passwd *string `json:"passwd,omitempty"`
	// Clear text of user password to be applied. This will be applied even if the user is
	// preexisting. **Note:** SSH keys or certificates are a safer choice for logging in to your
	// system. For local escalation, supplying a hashed password is a safer choice than plain
	// text. Your password might possibly be visible to unprivileged users on your system,
	// depending on your cloud's security model. An exposed plain text password is an immediate
	// security concern. Check if your cloud's IMDS server is visible from an unprivileged user
	// to evaluate risk.
	PlainTextPasswd *string `json:"plain_text_passwd,omitempty"`
	// Primary group for user. Default: ``<username>``
	PrimaryGroup *string `json:"primary_group,omitempty"`
	// SELinux user for user's login. Default: the default SELinux user.
	SelinuxUser *string `json:"selinux_user,omitempty"`
	// Path to the user's login shell. Default: the host system's default shell.
	Shell *string `json:"shell,omitempty"`
	// Specify an email address to create the user as a Snappy user through ``snap
	// create-user``. If an Ubuntu SSO account is associated with the address, username and SSH
	// keys will be requested from there.
	Snapuser *string `json:"snapuser,omitempty"`
	// List of SSH keys to add to user's authkeys file. Can not be combined with
	// ``ssh_redirect_user``
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys,omitempty"`
	// List of ssh ids to import for user. Can not be combined with ``ssh_redirect_user``. See
	// the man page[1] for more details. [1]
	// https://manpages.ubuntu.com/manpages/noble/en/man1/ssh-import-id.1.html
	SSHImportID []string `json:"ssh_import_id,omitempty"`
	// Boolean set to true to disable SSH logins for this user. When specified, all cloud
	// meta-data public SSH keys will be set up in a disabled state for this username. Any SSH
	// login as this username will timeout and prompt with a message to login instead as the
	// ``default_username`` for this instance. Default: ``false``. This key can not be combined
	// with ``ssh_import_id`` or ``ssh_authorized_keys``.
	SSHRedirectUser *bool `json:"ssh_redirect_user,omitempty"`
	Sudo            *Sudo `json:"sudo,omitempty"`
	// Optional. Create user as system user with no home directory. Default: ``false``.
	System *bool `json:"system,omitempty"`
	// The user's ID. Default value [system default]
	Uid *Uid `json:"uid,omitempty"`
}

type FluffySchemaCloudConfigV1 struct {
	// Boolean set ``false`` to disable creation of specified user ``groups``. Default: ``true``.
	CreateGroups *bool `json:"create_groups,omitempty"`
	// List of doas rules to add for a user. doas or opendoas must be installed for rules to
	// take effect.
	Doas []string `json:"doas,omitempty"`
	// Optional. Date on which the user's account will be disabled. Default: ``null``
	Expiredate *string `json:"expiredate,omitempty"`
	// Optional comment about the user, usually a comma-separated string of real name and
	// contact information
	Gecos *string `json:"gecos,omitempty"`
	// Optional comma-separated string of groups to add the user to.
	Groups *UserGroups `json:"groups,omitempty"`
	// Hash of user password to be applied. This will be applied even if the user is
	// preexisting. To generate this hash, run: ``mkpasswd --method=SHA-512 --rounds=500000``.
	// **Note:** Your password might possibly be visible to unprivileged users on your system,
	// depending on your cloud's security model. Check if your cloud's IMDS server is visible
	// from an unprivileged user to evaluate risk.
	HashedPasswd *string `json:"hashed_passwd,omitempty"`
	// Optional home dir for user. Default: ``/home/<username>``
	Homedir *string `json:"homedir,omitempty"`
	// Optional string representing the number of days until the user is disabled.
	Inactive *string `json:"inactive,omitempty"`
	// Default: ``true``
	LockPasswd *bool `json:"lock-passwd,omitempty"`
	// Disable password login. Default: ``true``
	SchemaCloudConfigV1LockPasswd *bool `json:"lock_passwd,omitempty"`
	// The user's login name. Required otherwise user creation will be skipped for this user.
	Name *string `json:"name,omitempty"`
	// Do not create home directory. Default: ``false``
	NoCreateHome *bool `json:"no_create_home,omitempty"`
	// Do not initialize lastlog and faillog for user. Default: ``false``
	NoLogInit *bool `json:"no_log_init,omitempty"`
	// Do not create group named after user. Default: ``false``
	NoUserGroup *bool `json:"no_user_group,omitempty"`
	// Hash of user password applied when user does not exist. This will NOT be applied if the
	// user already exists. To generate this hash, run: ``mkpasswd --method=SHA-512
	// --rounds=500000`` **Note:** Your password might possibly be visible to unprivileged users
	// on your system, depending on your cloud's security model. Check if your cloud's IMDS
	// server is visible from an unprivileged user to evaluate risk.
	Passwd *string `json:"passwd,omitempty"`
	// Clear text of user password to be applied. This will be applied even if the user is
	// preexisting. **Note:** SSH keys or certificates are a safer choice for logging in to your
	// system. For local escalation, supplying a hashed password is a safer choice than plain
	// text. Your password might possibly be visible to unprivileged users on your system,
	// depending on your cloud's security model. An exposed plain text password is an immediate
	// security concern. Check if your cloud's IMDS server is visible from an unprivileged user
	// to evaluate risk.
	PlainTextPasswd *string `json:"plain_text_passwd,omitempty"`
	// Primary group for user. Default: ``<username>``
	PrimaryGroup *string `json:"primary_group,omitempty"`
	// SELinux user for user's login. Default: the default SELinux user.
	SelinuxUser *string `json:"selinux_user,omitempty"`
	// Path to the user's login shell. Default: the host system's default shell.
	Shell *string `json:"shell,omitempty"`
	// Specify an email address to create the user as a Snappy user through ``snap
	// create-user``. If an Ubuntu SSO account is associated with the address, username and SSH
	// keys will be requested from there.
	Snapuser *string `json:"snapuser,omitempty"`
	// List of SSH keys to add to user's authkeys file. Can not be combined with
	// ``ssh_redirect_user``
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys,omitempty"`
	// List of ssh ids to import for user. Can not be combined with ``ssh_redirect_user``. See
	// the man page[1] for more details. [1]
	// https://manpages.ubuntu.com/manpages/noble/en/man1/ssh-import-id.1.html
	SSHImportID []string `json:"ssh_import_id,omitempty"`
	// Boolean set to true to disable SSH logins for this user. When specified, all cloud
	// meta-data public SSH keys will be set up in a disabled state for this username. Any SSH
	// login as this username will timeout and prompt with a message to login instead as the
	// ``default_username`` for this instance. Default: ``false``. This key can not be combined
	// with ``ssh_import_id`` or ``ssh_authorized_keys``.
	SSHRedirectUser *bool `json:"ssh_redirect_user,omitempty"`
	Sudo            *Sudo `json:"sudo,omitempty"`
	// Optional. Create user as system user with no home directory. Default: ``false``.
	System *bool `json:"system,omitempty"`
	// The user's ID. Default value [system default]
	Uid *Uid `json:"uid,omitempty"`
}

type VendorData struct {
	// Whether vendor data is enabled or not. Default: ``true``
	Enabled *GrubPCInstallDevicesEmpty `json:"enabled,omitempty"`
	// The command to run before any vendor scripts. Its primary use case is for profiling a
	// script, not to prevent its run
	Prefix *Prefix `json:"prefix,omitempty"`
}

type WireguardClass struct {
	Interfaces []Interface `json:"interfaces"`
	// List of shell commands to be executed as probes.
	Readinessprobe []string `json:"readinessprobe,omitempty"`
}

type Interface struct {
	// Path to configuration file of Wireguard interface
	ConfigPath *string `json:"config_path,omitempty"`
	// Wireguard interface configuration. Contains key, peer, ...
	Content *string `json:"content,omitempty"`
	// Name of the interface. Typically wgx (example: wg0)
	Name *string `json:"name,omitempty"`
}

type WriteFile struct {
	// Whether to append ``content`` to existing file if ``path`` exists. Default: ``false``.
	Append *bool `json:"append,omitempty"`
	// Optional content to write to the provided ``path``. When content is present and encoding
	// is not 'text/plain', decode the content prior to writing. Default: ``''``
	Content *string `json:"content,omitempty"`
	// Defer writing the file until 'final' stage, after users were created, and packages were
	// installed. Default: ``false``.
	Defer *bool `json:"defer,omitempty"`
	// Optional encoding type of the content. Default: ``text/plain``. No decoding is performed
	// by default. Supported encoding types are: gz, gzip, gz+base64, gzip+base64, gz+b64,
	// gzip+b64, b64, base64
	Encoding *WriteFileEncoding `json:"encoding,omitempty"`
	// Optional owner:group to chown on the file and new directories. Default: ``root:root``
	Owner *string `json:"owner,omitempty"`
	// Path of the file to which ``content`` is decoded and written
	Path string `json:"path"`
	// Optional file permissions to set on ``path`` represented as an octal string '0###'.
	// Default: ``0o644``
	Permissions *string `json:"permissions,omitempty"`
}

type YumRepos struct {
}

type Zypper struct {
	// Any supported zypo.conf key is written to ``/etc/zypp/zypp.conf``
	Config map[string]interface{} `json:"config,omitempty"`
	Repos  []Repo                 `json:"repos,omitempty"`
}

type Repo struct {
	// The base repositoy URL
	Baseurl string `json:"baseurl"`
	// The unique id of the repo, used when writing /etc/zypp/repos.d/<id>.repo.
	ID string `json:"id"`
}

// The type of installation for ansible. It can be one of the following values:
//
// - “distro“
// - “pip“
type InstallMethod string

const (
	Distro InstallMethod = "distro"
	Pip    InstallMethod = "pip"
)

// Optional command to run to create the filesystem. Can include string substitutions of the
// other “fs_setup“ config keys. This is only necessary if you need to override the
// default command.
//
// Optional options to pass to the filesystem creation command. Ignored if you using “cmd“
// directly.
//
// Properly-signed snap assertions which will run before and snap “commands“.
//
// # The SSH public key to import
//
// A filepath operation configuration. This is a string containing a filepath and an
// optional leading operator: '>', '>>' or '|'. Operators '>' and '>>' indicate whether to
// overwrite or append to the file. The operator '|' redirects content to the command
// arguments specified.
//
// A list specifying filepath operation configuration for stdout and stderror
type AptPipeliningEnum string

const (
	CloudconfigNone AptPipeliningEnum = "none"
	OS              AptPipeliningEnum = "os"
	Unchanged       AptPipeliningEnum = "unchanged"
)

type ByobuByDefault string

const (
	Disable       ByobuByDefault = "disable"
	DisableSystem ByobuByDefault = "disable-system"
	DisableUser   ByobuByDefault = "disable-user"
	Enable        ByobuByDefault = "enable"
	EnableSystem  ByobuByDefault = "enable-system"
	EnableUser    ByobuByDefault = "enable-user"
	System        ByobuByDefault = "system"
	User          ByobuByDefault = "user"
)

// The type of installation for chef. It can be one of the following values:
//
// - “packages“
// - “gems“
// - “omnibus“
type ChefInstallType string

const (
	Gems           ChefInstallType = "gems"
	Omnibus        ChefInstallType = "omnibus"
	PurplePackages ChefInstallType = "packages"
)

type Type string

const (
	Hash   Type = "hash"
	Random Type = "RANDOM"
	Text   Type = "text"
)

type CloudConfigModuleEnum string

const (
	Ansible                                        CloudConfigModuleEnum = "ansible"
	ApkConfigure                                   CloudConfigModuleEnum = "apk-configure"
	AptConfigure                                   CloudConfigModuleEnum = "apt-configure"
	AptPipelining                                  CloudConfigModuleEnum = "apt-pipelining"
	Bootcmd                                        CloudConfigModuleEnum = "bootcmd"
	Byobu                                          CloudConfigModuleEnum = "byobu"
	CACerts                                        CloudConfigModuleEnum = "ca-certs"
	Chef                                           CloudConfigModuleEnum = "chef"
	DisableEc2Metadata                             CloudConfigModuleEnum = "disable-ec2-metadata"
	DiskSetup                                      CloudConfigModuleEnum = "disk-setup"
	Fan                                            CloudConfigModuleEnum = "fan"
	FinalMessage                                   CloudConfigModuleEnum = "final-message"
	GrubDpkg                                       CloudConfigModuleEnum = "grub-dpkg"
	InstallHotplug                                 CloudConfigModuleEnum = "install-hotplug"
	Keyboard                                       CloudConfigModuleEnum = "keyboard"
	KeysToConsole                                  CloudConfigModuleEnum = "keys-to-console"
	Landscape                                      CloudConfigModuleEnum = "landscape"
	Locale                                         CloudConfigModuleEnum = "locale"
	Lxd                                            CloudConfigModuleEnum = "lxd"
	Mcollective                                    CloudConfigModuleEnum = "mcollective"
	Migrator                                       CloudConfigModuleEnum = "migrator"
	Mounts                                         CloudConfigModuleEnum = "mounts"
	NTP                                            CloudConfigModuleEnum = "ntp"
	PackageUpdateUpgradeInstall                    CloudConfigModuleEnum = "package-update-upgrade-install"
	PhoneHome                                      CloudConfigModuleEnum = "phone-home"
	PowerStateChange                               CloudConfigModuleEnum = "power-state-change"
	Puppet                                         CloudConfigModuleEnum = "puppet"
	ResetRmc                                       CloudConfigModuleEnum = "reset-rmc"
	Resizefs                                       CloudConfigModuleEnum = "resizefs"
	ResolvConf                                     CloudConfigModuleEnum = "resolv-conf"
	RhSubscription                                 CloudConfigModuleEnum = "rh-subscription"
	RightscaleUserdata                             CloudConfigModuleEnum = "rightscale-userdata"
	Rsyslog                                        CloudConfigModuleEnum = "rsyslog"
	Runcmd                                         CloudConfigModuleEnum = "runcmd"
	SSH                                            CloudConfigModuleEnum = "ssh"
	SSHAuthkeyFingerprints                         CloudConfigModuleEnum = "ssh-authkey-fingerprints"
	SSHImportID                                    CloudConfigModuleEnum = "ssh-import-id"
	SaltMinion                                     CloudConfigModuleEnum = "salt-minion"
	SchemaCloudConfigV1ApkConfigure                CloudConfigModuleEnum = "apk_configure"
	SchemaCloudConfigV1AptConfigure                CloudConfigModuleEnum = "apt_configure"
	SchemaCloudConfigV1AptPipelining               CloudConfigModuleEnum = "apt_pipelining"
	SchemaCloudConfigV1CACerts                     CloudConfigModuleEnum = "ca_certs"
	SchemaCloudConfigV1DisableEc2Metadata          CloudConfigModuleEnum = "disable_ec2_metadata"
	SchemaCloudConfigV1DiskSetup                   CloudConfigModuleEnum = "disk_setup"
	SchemaCloudConfigV1FinalMessage                CloudConfigModuleEnum = "final_message"
	SchemaCloudConfigV1Growpart                    CloudConfigModuleEnum = "growpart"
	SchemaCloudConfigV1GrubDpkg                    CloudConfigModuleEnum = "grub_dpkg"
	SchemaCloudConfigV1InstallHotplug              CloudConfigModuleEnum = "install_hotplug"
	SchemaCloudConfigV1KeysToConsole               CloudConfigModuleEnum = "keys_to_console"
	SchemaCloudConfigV1PackageUpdateUpgradeInstall CloudConfigModuleEnum = "package_update_upgrade_install"
	SchemaCloudConfigV1PhoneHome                   CloudConfigModuleEnum = "phone_home"
	SchemaCloudConfigV1PowerStateChange            CloudConfigModuleEnum = "power_state_change"
	SchemaCloudConfigV1ResetRmc                    CloudConfigModuleEnum = "reset_rmc"
	SchemaCloudConfigV1ResolvConf                  CloudConfigModuleEnum = "resolv_conf"
	SchemaCloudConfigV1RhSubscription              CloudConfigModuleEnum = "rh_subscription"
	SchemaCloudConfigV1RightscaleUserdata          CloudConfigModuleEnum = "rightscale_userdata"
	SchemaCloudConfigV1SSHAuthkeyFingerprints      CloudConfigModuleEnum = "ssh_authkey_fingerprints"
	SchemaCloudConfigV1SSHImportID                 CloudConfigModuleEnum = "ssh_import_id"
	SchemaCloudConfigV1SaltMinion                  CloudConfigModuleEnum = "salt_minion"
	SchemaCloudConfigV1ScriptsPerBoot              CloudConfigModuleEnum = "scripts_per_boot"
	SchemaCloudConfigV1ScriptsPerInstance          CloudConfigModuleEnum = "scripts_per_instance"
	SchemaCloudConfigV1ScriptsPerOnce              CloudConfigModuleEnum = "scripts_per_once"
	SchemaCloudConfigV1ScriptsUser                 CloudConfigModuleEnum = "scripts_user"
	SchemaCloudConfigV1ScriptsVendor               CloudConfigModuleEnum = "scripts_vendor"
	SchemaCloudConfigV1SeedRandom                  CloudConfigModuleEnum = "seed_random"
	SchemaCloudConfigV1SetHostname                 CloudConfigModuleEnum = "set_hostname"
	SchemaCloudConfigV1SetPasswords                CloudConfigModuleEnum = "set_passwords"
	SchemaCloudConfigV1UbuntuAdvantage             CloudConfigModuleEnum = "ubuntu_advantage"
	SchemaCloudConfigV1UbuntuAutoinstall           CloudConfigModuleEnum = "ubuntu_autoinstall"
	SchemaCloudConfigV1UbuntuDrivers               CloudConfigModuleEnum = "ubuntu_drivers"
	SchemaCloudConfigV1UpdateEtcHosts              CloudConfigModuleEnum = "update_etc_hosts"
	SchemaCloudConfigV1UpdateHostname              CloudConfigModuleEnum = "update_hostname"
	SchemaCloudConfigV1UsersGroups                 CloudConfigModuleEnum = "users_groups"
	SchemaCloudConfigV1WriteFiles                  CloudConfigModuleEnum = "write_files"
	SchemaCloudConfigV1WriteFilesDeferred          CloudConfigModuleEnum = "write_files_deferred"
	SchemaCloudConfigV1YumAddRepo                  CloudConfigModuleEnum = "yum_add_repo"
	SchemaCloudConfigV1ZypperAddRepo               CloudConfigModuleEnum = "zypper_add_repo"
	ScriptsPerBoot                                 CloudConfigModuleEnum = "scripts-per-boot"
	ScriptsPerInstance                             CloudConfigModuleEnum = "scripts-per-instance"
	ScriptsPerOnce                                 CloudConfigModuleEnum = "scripts-per-once"
	ScriptsUser                                    CloudConfigModuleEnum = "scripts-user"
	ScriptsVendor                                  CloudConfigModuleEnum = "scripts-vendor"
	SeedRandom                                     CloudConfigModuleEnum = "seed-random"
	SetHostname                                    CloudConfigModuleEnum = "set-hostname"
	SetPasswords                                   CloudConfigModuleEnum = "set-passwords"
	Snap                                           CloudConfigModuleEnum = "snap"
	Spacewalk                                      CloudConfigModuleEnum = "spacewalk"
	Timezone                                       CloudConfigModuleEnum = "timezone"
	UbuntuAdvantage                                CloudConfigModuleEnum = "ubuntu-advantage"
	UbuntuAutoinstall                              CloudConfigModuleEnum = "ubuntu-autoinstall"
	UbuntuDrivers                                  CloudConfigModuleEnum = "ubuntu-drivers"
	UpdateEtcHosts                                 CloudConfigModuleEnum = "update-etc-hosts"
	UpdateHostname                                 CloudConfigModuleEnum = "update-hostname"
	UsersGroups                                    CloudConfigModuleEnum = "users-groups"
	Wireguard                                      CloudConfigModuleEnum = "wireguard"
	WriteFiles                                     CloudConfigModuleEnum = "write-files"
	WriteFilesDeferred                             CloudConfigModuleEnum = "write-files-deferred"
	YumAddRepo                                     CloudConfigModuleEnum = "yum-add-repo"
	ZypperAddRepo                                  CloudConfigModuleEnum = "zypper-add-repo"
)

// The partition can be specified by setting “partition“ to the desired partition number.
// The “partition“ option may also be set to “auto“, in which this module will search
// for the existence of a filesystem matching the “label“, “filesystem“ and “device“
// of the “fs_setup“ entry and will skip creating the filesystem if one is found. The
// “partition“ option may also be set to “any“, in which case any filesystem that
// matches “filesystem“ and “device“ will cause this module to skip filesystem creation
// for the “fs_setup“ entry, regardless of “label“ matching or not. To write a
// filesystem directly to a device, use “partition: none“. “partition: none“ will
// **always** write the filesystem, even when the “label“ and “filesystem“ are matched,
// and “overwrite“ is “false“.
//
// Optional command to run to create the filesystem. Can include string substitutions of the
// other “fs_setup“ config keys. This is only necessary if you need to override the
// default command.
//
// Optional options to pass to the filesystem creation command. Ignored if you using “cmd“
// directly.
//
// Properly-signed snap assertions which will run before and snap “commands“.
//
// # The SSH public key to import
//
// A filepath operation configuration. This is a string containing a filepath and an
// optional leading operator: '>', '>>' or '|'. Operators '>' and '>>' indicate whether to
// overwrite or append to the file. The operator '|' redirects content to the command
// arguments specified.
//
// A list specifying filepath operation configuration for stdout and stderror
type Partition string

const (
	Any           Partition = "any"
	PartitionAuto Partition = "auto"
	PartitionNone Partition = "none"
)

type ModeMode string

const (
	Gpart        ModeMode = "gpart"
	ModeAuto     ModeMode = "auto"
	ModeGrowpart ModeMode = "growpart"
	Off          ModeMode = "off"
)

// The log level for the client. Default: “info“.
type LogLevel string

const (
	Critical LogLevel = "critical"
	Debug    LogLevel = "debug"
	Error    LogLevel = "error"
	Info     LogLevel = "info"
	Warning  LogLevel = "warning"
)

// Whether to setup LXD bridge, use an existing bridge by “name“ or create a new bridge.
// `none` will avoid bridge setup, `existing` will configure lxd to use the bring matching
// “name“ and `new` will create a new bridge.
type BridgeMode string

const (
	Existing BridgeMode = "existing"
	ModeNone BridgeMode = "none"
	New      BridgeMode = "new"
)

// Storage backend to use. Default: “dir“.
type StorageBackend string

const (
	Btrfs StorageBackend = "btrfs"
	Dir   StorageBackend = "dir"
	LVM   StorageBackend = "lvm"
	Zfs   StorageBackend = "zfs"
)

type ManageEtcHostsEnum string

const (
	Localhost ManageEtcHostsEnum = "localhost"
	Template  ManageEtcHostsEnum = "template"
)

type Name string

const (
	Dict Name = "dict"
	List Name = "list"
	Str  Name = "str"
)

type Setting string

const (
	AllowDelete  Setting = "allow_delete"
	Append       Setting = "append"
	NoReplace    Setting = "no_replace"
	Prepend      Setting = "prepend"
	RecurseArray Setting = "recurse_array"
	RecurseDict  Setting = "recurse_dict"
	RecurseList  Setting = "recurse_list"
	RecurseStr   Setting = "recurse_str"
	Replace      Setting = "replace"
)

type PostElement string

const (
	FQDN          PostElement = "fqdn"
	Hostname      PostElement = "hostname"
	InstanceID    PostElement = "instance_id"
	PubKeyEcdsa   PostElement = "pub_key_ecdsa"
	PubKeyEd25519 PostElement = "pub_key_ed25519"
	PubKeyRSA     PostElement = "pub_key_rsa"
)

type PurplePost string

const (
	All PurplePost = "all"
)

// Must be one of “poweroff“, “halt“, or “reboot“.
type PowerStateMode string

const (
	Halt     PowerStateMode = "halt"
	Poweroff PowerStateMode = "poweroff"
	Reboot   PowerStateMode = "reboot"
)

// Valid values are “packages“ and “aio“. Agent packages from the puppetlabs
// repositories can be installed by setting “aio“. Based on this setting, the default
// config/SSL/CSR paths will be adjusted accordingly. Default: “packages“
type PuppetInstallType string

const (
	Aio            PuppetInstallType = "aio"
	FluffyPackages PuppetInstallType = "packages"
)

// Used to decode “data“ provided. Allowed values are “raw“, “base64“, “b64“,
// “gzip“, or “gz“.  Default: “raw“
type RandomSeedEncoding string

const (
	PurpleB64    RandomSeedEncoding = "b64"
	PurpleBase64 RandomSeedEncoding = "base64"
	PurpleGz     RandomSeedEncoding = "gz"
	PurpleGzip   RandomSeedEncoding = "gzip"
	Raw          RandomSeedEncoding = "raw"
)

type ResizeRootfsEnum string

const (
	Noblock ResizeRootfsEnum = "noblock"
)

type ServiceReloadCommandEnum string

const (
	ServiceReloadCommandAuto ServiceReloadCommandEnum = "auto"
)

type SSHGenkeytype string

const (
	Ecdsa   SSHGenkeytype = "ecdsa"
	Ed25519 SSHGenkeytype = "ed25519"
	RSA     SSHGenkeytype = "rsa"
)

type When string

const (
	Boot            When = "boot"
	BootLegacy      When = "boot-legacy"
	BootNewInstance When = "boot-new-instance"
	Hotplug         When = "hotplug"
)

// Optional encoding type of the content. Default: “text/plain“. No decoding is performed
// by default. Supported encoding types are: gz, gzip, gz+base64, gzip+base64, gz+b64,
// gzip+b64, b64, base64
type WriteFileEncoding string

const (
	FluffyB64    WriteFileEncoding = "b64"
	FluffyBase64 WriteFileEncoding = "base64"
	FluffyGz     WriteFileEncoding = "gz"
	FluffyGzip   WriteFileEncoding = "gzip"
	GzB64        WriteFileEncoding = "gz+b64"
	GzBase64     WriteFileEncoding = "gz+base64"
	GzipB64      WriteFileEncoding = "gzip+b64"
	GzipBase64   WriteFileEncoding = "gzip+base64"
	TextPlain    WriteFileEncoding = "text/plain"
)

type RunAnsibleElement struct {
	AnythingArray   []interface{}
	Bool            *bool
	Double          *float64
	Integer         *int64
	RunAnsibleClass *RunAnsibleClass
	String          *string
}

func (x *RunAnsibleElement) UnmarshalJSON(data []byte) error {
	x.AnythingArray = nil
	x.RunAnsibleClass = nil
	var c RunAnsibleClass
	object, err := unmarshalUnion(data, &x.Integer, &x.Double, &x.Bool, &x.String, true, &x.AnythingArray, true, &c, false, nil, false, nil, true)
	if err != nil {
		return err
	}
	if object {
		x.RunAnsibleClass = &c
	}
	return nil
}

func (x *RunAnsibleElement) MarshalJSON() ([]byte, error) {
	return marshalUnion(x.Integer, x.Double, x.Bool, x.String, x.AnythingArray != nil, x.AnythingArray, x.RunAnsibleClass != nil, x.RunAnsibleClass, false, nil, false, nil, true)
}

type AptPipeliningUnion struct {
	Bool    *bool
	Enum    *AptPipeliningEnum
	Integer *int64
}

func (x *AptPipeliningUnion) UnmarshalJSON(data []byte) error {
	x.Enum = nil
	object, err := unmarshalUnion(data, &x.Integer, nil, &x.Bool, nil, false, nil, false, nil, false, nil, true, &x.Enum, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *AptPipeliningUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(x.Integer, nil, x.Bool, nil, false, nil, false, nil, false, nil, x.Enum != nil, x.Enum, false)
}

// Optional command to run to create the filesystem. Can include string substitutions of the
// other “fs_setup“ config keys. This is only necessary if you need to override the
// default command.
//
// Optional options to pass to the filesystem creation command. Ignored if you using “cmd“
// directly.
//
// Snap commands to run on the target system
type Cmd struct {
	String      *string
	StringArray []string
}

func (x *Cmd) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.StringArray, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Cmd) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.StringArray != nil, x.StringArray, false, nil, false, nil, false, nil, false)
}

// List of “username:password“ pairs. Each user will have the corresponding password set.
// A password can be randomly generated by specifying “RANDOM“ or “R“ as a user's
// password. A hashed password, created by a tool like “mkpasswd“, can be specified. A
// regex (“r'\$(1|2a|2y|5|6)(\$.+){2}'“) is used to determine if a password value should
// be treated as a hash.
type ListUnion struct {
	String      *string
	StringArray []string
}

func (x *ListUnion) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.StringArray, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *ListUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.StringArray != nil, x.StringArray, false, nil, false, nil, false, nil, false)
}

type CloudConfigModuleElement struct {
	AnythingArray []interface{}
	Enum          *CloudConfigModuleEnum
}

func (x *CloudConfigModuleElement) UnmarshalJSON(data []byte) error {
	x.AnythingArray = nil
	x.Enum = nil
	object, err := unmarshalUnion(data, nil, nil, nil, nil, true, &x.AnythingArray, false, nil, false, nil, true, &x.Enum, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *CloudConfigModuleElement) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, nil, x.AnythingArray != nil, x.AnythingArray, false, nil, false, nil, x.Enum != nil, x.Enum, false)
}

type GrubPCInstallDevicesEmpty struct {
	Bool   *bool
	String *string
}

func (x *GrubPCInstallDevicesEmpty) UnmarshalJSON(data []byte) error {
	object, err := unmarshalUnion(data, nil, nil, &x.Bool, &x.String, false, nil, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *GrubPCInstallDevicesEmpty) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, x.Bool, x.String, false, nil, false, nil, false, nil, false, nil, false)
}

type CloudconfigGroups struct {
	AnythingArray []interface{}
	GroupsClass   *GroupsClass
	String        *string
}

func (x *CloudconfigGroups) UnmarshalJSON(data []byte) error {
	x.AnythingArray = nil
	x.GroupsClass = nil
	var c GroupsClass
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.AnythingArray, true, &c, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
		x.GroupsClass = &c
	}
	return nil
}

func (x *CloudconfigGroups) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.AnythingArray != nil, x.AnythingArray, x.GroupsClass != nil, x.GroupsClass, false, nil, false, nil, false)
}

// The utility to use for resizing. Default: “auto“
//
// Possible options:
//
// * “auto“ - Use any available utility
//
// * “growpart“ - Use growpart utility
//
// * “gpart“ - Use BSD gpart utility
//
// * “off“ - Take no action
type ModeUnion struct {
	Bool *bool
	Enum *ModeMode
}

func (x *ModeUnion) UnmarshalJSON(data []byte) error {
	x.Enum = nil
	object, err := unmarshalUnion(data, nil, nil, &x.Bool, nil, false, nil, false, nil, false, nil, true, &x.Enum, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *ModeUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, x.Bool, nil, false, nil, false, nil, false, nil, x.Enum != nil, x.Enum, false)
}

// Whether to manage “/etc/hosts“ on the system. If “true“, render the hosts file using
// “/etc/cloud/templates/hosts.tmpl“ replacing “$hostname“ and “$fdqn“. If
// “localhost“, append a “127.0.1.1“ entry that resolves from FQDN and hostname every
// boot. Default: “false“
type ManageEtcHostsUnion struct {
	Bool *bool
	Enum *ManageEtcHostsEnum
}

func (x *ManageEtcHostsUnion) UnmarshalJSON(data []byte) error {
	x.Enum = nil
	object, err := unmarshalUnion(data, nil, nil, &x.Bool, nil, false, nil, false, nil, false, nil, true, &x.Enum, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *ManageEtcHostsUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, x.Bool, nil, false, nil, false, nil, false, nil, x.Enum != nil, x.Enum, false)
}

type MergeHow struct {
	MergeHowElementArray []MergeHowElement
	String               *string
}

func (x *MergeHow) UnmarshalJSON(data []byte) error {
	x.MergeHowElementArray = nil
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.MergeHowElementArray, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *MergeHow) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.MergeHowElementArray != nil, x.MergeHowElementArray, false, nil, false, nil, false, nil, false)
}

type AllUnion struct {
	AllClass    *AllClass
	String      *string
	StringArray []string
}

func (x *AllUnion) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	x.AllClass = nil
	var c AllClass
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.StringArray, true, &c, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
		x.AllClass = &c
	}
	return nil
}

func (x *AllUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.StringArray != nil, x.StringArray, x.AllClass != nil, x.AllClass, false, nil, false, nil, false)
}

type PackageElement struct {
	PackageClass *PackageClass
	String       *string
	StringArray  []string
}

func (x *PackageElement) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	x.PackageClass = nil
	var c PackageClass
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.StringArray, true, &c, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
		x.PackageClass = &c
	}
	return nil
}

func (x *PackageElement) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.StringArray != nil, x.StringArray, x.PackageClass != nil, x.PackageClass, false, nil, false, nil, false)
}

// A list of keys to post or “all“. Default: “all“
type PostUnion struct {
	Enum      *PurplePost
	EnumArray []PostElement
}

func (x *PostUnion) UnmarshalJSON(data []byte) error {
	x.EnumArray = nil
	x.Enum = nil
	object, err := unmarshalUnion(data, nil, nil, nil, nil, true, &x.EnumArray, false, nil, false, nil, true, &x.Enum, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *PostUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, nil, x.EnumArray != nil, x.EnumArray, false, nil, false, nil, x.Enum != nil, x.Enum, false)
}

// Apply state change only if condition is met. May be boolean true (always met), false
// (never met), or a command string or list to be executed. For command formatting, see the
// documentation for “cc_runcmd“. If exit code is 0, condition is met, otherwise not.
// Default: “true“
type Condition struct {
	AnythingArray []interface{}
	Bool          *bool
	String        *string
}

func (x *Condition) UnmarshalJSON(data []byte) error {
	x.AnythingArray = nil
	object, err := unmarshalUnion(data, nil, nil, &x.Bool, &x.String, true, &x.AnythingArray, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Condition) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, x.Bool, x.String, x.AnythingArray != nil, x.AnythingArray, false, nil, false, nil, false, nil, false)
}

// Time in minutes to delay after cloud-init has finished. Can be “now“ or an integer
// specifying the number of minutes to delay. Default: “now“
type Delay struct {
	Integer *int64
	String  *string
}

func (x *Delay) UnmarshalJSON(data []byte) error {
	object, err := unmarshalUnion(data, &x.Integer, nil, nil, &x.String, false, nil, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Delay) MarshalJSON() ([]byte, error) {
	return marshalUnion(x.Integer, nil, nil, x.String, false, nil, false, nil, false, nil, false, nil, false)
}

// Whether to resize the root partition. “noblock“ will resize in the background. Default:
// “true“
type ResizeRootfsUnion struct {
	Bool *bool
	Enum *ResizeRootfsEnum
}

func (x *ResizeRootfsUnion) UnmarshalJSON(data []byte) error {
	x.Enum = nil
	object, err := unmarshalUnion(data, nil, nil, &x.Bool, nil, false, nil, false, nil, false, nil, true, &x.Enum, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *ResizeRootfsUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, x.Bool, nil, false, nil, false, nil, false, nil, x.Enum != nil, x.Enum, false)
}

type ConfigElement struct {
	ConfigConfig *ConfigConfig
	String       *string
}

func (x *ConfigElement) UnmarshalJSON(data []byte) error {
	x.ConfigConfig = nil
	var c ConfigConfig
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, false, nil, true, &c, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
		x.ConfigConfig = &c
	}
	return nil
}

func (x *ConfigElement) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, false, nil, x.ConfigConfig != nil, x.ConfigConfig, false, nil, false, nil, false)
}

// The command to use to reload the rsyslog service after the config has been updated. If
// this is set to “auto“, then an appropriate command for the distro will be used. This is
// the default behavior. To manually set the command, use a list of command args (e.g.
// “[systemctl, restart, rsyslog]“).
type ServiceReloadCommandUnion struct {
	Enum        *ServiceReloadCommandEnum
	StringArray []string
}

func (x *ServiceReloadCommandUnion) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	x.Enum = nil
	object, err := unmarshalUnion(data, nil, nil, nil, nil, true, &x.StringArray, false, nil, false, nil, true, &x.Enum, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *ServiceReloadCommandUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, nil, x.StringArray != nil, x.StringArray, false, nil, false, nil, x.Enum != nil, x.Enum, false)
}

type RuncmdElement struct {
	String      *string
	StringArray []string
}

func (x *RuncmdElement) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.StringArray, false, nil, false, nil, false, nil, true)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *RuncmdElement) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.StringArray != nil, x.StringArray, false, nil, false, nil, false, nil, true)
}

// Properly-signed snap assertions which will run before and snap “commands“.
type Assertions struct {
	StringArray []string
	StringMap   map[string]string
}

func (x *Assertions) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	x.StringMap = nil
	object, err := unmarshalUnion(data, nil, nil, nil, nil, true, &x.StringArray, false, nil, true, &x.StringMap, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Assertions) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, nil, x.StringArray != nil, x.StringArray, false, nil, x.StringMap != nil, x.StringMap, false, nil, false)
}

// Snap commands to run on the target system
type Commands struct {
	UnionArray []Cmd
	UnionMap   map[string]*Cmd
}

func (x *Commands) UnmarshalJSON(data []byte) error {
	x.UnionArray = nil
	x.UnionMap = nil
	object, err := unmarshalUnion(data, nil, nil, nil, nil, true, &x.UnionArray, false, nil, true, &x.UnionMap, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Commands) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, nil, x.UnionArray != nil, x.UnionArray, false, nil, x.UnionMap != nil, x.UnionMap, false, nil, false)
}

// The maxsize in bytes of the swap file
//
// The size in bytes of the swap file, 'auto' or a human-readable size abbreviation of the
// format <float_size><units> where units are one of B, K, M, G or T. **WARNING: Attempts to
// use IEC prefixes in your configuration prior to cloud-init version 23.1 will result in
// unexpected behavior. SI prefixes names (KB, MB) are required on pre-23.1 cloud-init,
// however IEC values are used. In summary, assume 1KB == 1024B, not 1000B**
type Size struct {
	Integer *int64
	String  *string
}

func (x *Size) UnmarshalJSON(data []byte) error {
	object, err := unmarshalUnion(data, &x.Integer, nil, nil, &x.String, false, nil, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Size) MarshalJSON() ([]byte, error) {
	return marshalUnion(x.Integer, nil, nil, x.String, false, nil, false, nil, false, nil, false, nil, false)
}

// The “user“ dictionary values override the “default_user“ configuration from
// “/etc/cloud/cloud.cfg“. The `user` dictionary keys supported for the default_user are
// the same as the “users“ schema.
type CloudconfigUser struct {
	PurpleSchemaCloudConfigV1 *PurpleSchemaCloudConfigV1
	String                    *string
}

func (x *CloudconfigUser) UnmarshalJSON(data []byte) error {
	x.PurpleSchemaCloudConfigV1 = nil
	var c PurpleSchemaCloudConfigV1
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, false, nil, true, &c, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
		x.PurpleSchemaCloudConfigV1 = &c
	}
	return nil
}

func (x *CloudconfigUser) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, false, nil, x.PurpleSchemaCloudConfigV1 != nil, x.PurpleSchemaCloudConfigV1, false, nil, false, nil, false)
}

// Optional comma-separated string of groups to add the user to.
type UserGroups struct {
	AnythingMap map[string]interface{}
	String      *string
	StringArray []string
}

func (x *UserGroups) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	x.AnythingMap = nil
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.StringArray, false, nil, true, &x.AnythingMap, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *UserGroups) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.StringArray != nil, x.StringArray, false, nil, x.AnythingMap != nil, x.AnythingMap, false, nil, false)
}

type Sudo struct {
	Bool   *bool
	String *string
}

func (x *Sudo) UnmarshalJSON(data []byte) error {
	object, err := unmarshalUnion(data, nil, nil, &x.Bool, &x.String, false, nil, false, nil, false, nil, false, nil, true)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Sudo) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, x.Bool, x.String, false, nil, false, nil, false, nil, false, nil, true)
}

// The command to run before any vendor scripts. Its primary use case is for profiling a
// script, not to prevent its run
//
// The user's ID. Default value [system default]
type Uid struct {
	Integer *int64
	String  *string
}

func (x *Uid) UnmarshalJSON(data []byte) error {
	object, err := unmarshalUnion(data, &x.Integer, nil, nil, &x.String, false, nil, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Uid) MarshalJSON() ([]byte, error) {
	return marshalUnion(x.Integer, nil, nil, x.String, false, nil, false, nil, false, nil, false, nil, false)
}

type Users struct {
	AnythingMap map[string]interface{}
	String      *string
	UnionArray  []UsersUser
}

func (x *Users) UnmarshalJSON(data []byte) error {
	x.UnionArray = nil
	x.AnythingMap = nil
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.UnionArray, false, nil, true, &x.AnythingMap, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Users) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.UnionArray != nil, x.UnionArray, false, nil, x.AnythingMap != nil, x.AnythingMap, false, nil, false)
}

type UsersUser struct {
	FluffySchemaCloudConfigV1 *FluffySchemaCloudConfigV1
	String                    *string
	StringArray               []string
}

func (x *UsersUser) UnmarshalJSON(data []byte) error {
	x.StringArray = nil
	x.FluffySchemaCloudConfigV1 = nil
	var c FluffySchemaCloudConfigV1
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.StringArray, true, &c, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
		x.FluffySchemaCloudConfigV1 = &c
	}
	return nil
}

func (x *UsersUser) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.StringArray != nil, x.StringArray, x.FluffySchemaCloudConfigV1 != nil, x.FluffySchemaCloudConfigV1, false, nil, false, nil, false)
}

// The command to run before any vendor scripts. Its primary use case is for profiling a
// script, not to prevent its run
type Prefix struct {
	String     *string
	UnionArray []Uid
}

func (x *Prefix) UnmarshalJSON(data []byte) error {
	x.UnionArray = nil
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, true, &x.UnionArray, false, nil, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *Prefix) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, x.UnionArray != nil, x.UnionArray, false, nil, false, nil, false, nil, false)
}

func unmarshalUnion(data []byte, pi **int64, pf **float64, pb **bool, ps **string, haveArray bool, pa interface{}, haveObject bool, pc interface{}, haveMap bool, pm interface{}, haveEnum bool, pe interface{}, nullable bool) (bool, error) {
	if pi != nil {
		*pi = nil
	}
	if pf != nil {
		*pf = nil
	}
	if pb != nil {
		*pb = nil
	}
	if ps != nil {
		*ps = nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return false, err
	}

	switch v := tok.(type) {
	case json.Number:
		if pi != nil {
			i, err := v.Int64()
			if err == nil {
				*pi = &i
				return false, nil
			}
		}
		if pf != nil {
			f, err := v.Float64()
			if err == nil {
				*pf = &f
				return false, nil
			}
			return false, errors.New("Unparsable number")
		}
		return false, errors.New("Union does not contain number")
	case float64:
		return false, errors.New("Decoder should not return float64")
	case bool:
		if pb != nil {
			*pb = &v
			return false, nil
		}
		return false, errors.New("Union does not contain bool")
	case string:
		if haveEnum {
			return false, json.Unmarshal(data, pe)
		}
		if ps != nil {
			*ps = &v
			return false, nil
		}
		return false, errors.New("Union does not contain string")
	case nil:
		if nullable {
			return false, nil
		}
		return false, errors.New("Union does not contain null")
	case json.Delim:
		if v == '{' {
			if haveObject {
				return true, json.Unmarshal(data, pc)
			}
			if haveMap {
				return false, json.Unmarshal(data, pm)
			}
			return false, errors.New("Union does not contain object")
		}
		if v == '[' {
			if haveArray {
				return false, json.Unmarshal(data, pa)
			}
			return false, errors.New("Union does not contain array")
		}
		return false, errors.New("Cannot handle delimiter")
	}
	return false, errors.New("Cannot unmarshal union")

}

func marshalUnion(pi *int64, pf *float64, pb *bool, ps *string, haveArray bool, pa interface{}, haveObject bool, pc interface{}, haveMap bool, pm interface{}, haveEnum bool, pe interface{}, nullable bool) ([]byte, error) {
	if pi != nil {
		return json.Marshal(*pi)
	}
	if pf != nil {
		return json.Marshal(*pf)
	}
	if pb != nil {
		return json.Marshal(*pb)
	}
	if ps != nil {
		return json.Marshal(*ps)
	}
	if haveArray {
		return json.Marshal(pa)
	}
	if haveObject {
		return json.Marshal(pc)
	}
	if haveMap {
		return json.Marshal(pm)
	}
	if haveEnum {
		return json.Marshal(pe)
	}
	if nullable {
		return json.Marshal(nil)
	}
	return nil, errors.New("Union must not be null")
}
