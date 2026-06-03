// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "strings"

// VirtualMachineGuestOSFamily represents a possible guest family type.
// It corresponds to vim.vm.GuestOsDescriptor.GuestOsFamily.
type VirtualMachineGuestOSFamily string

const (
	// VirtualMachineGuestOSFamilyDarwin indicates an Apple macOS/Darwin guest OS.
	VirtualMachineGuestOSFamilyDarwin VirtualMachineGuestOSFamily = "Darwin"

	// VirtualMachineGuestOSFamilyLinux indicates a Linux guest OS.
	VirtualMachineGuestOSFamilyLinux VirtualMachineGuestOSFamily = "Linux"

	// VirtualMachineGuestOSFamilyNetware indicates a Novell NetWare guest OS.
	VirtualMachineGuestOSFamilyNetware VirtualMachineGuestOSFamily = "Netware"

	// VirtualMachineGuestOSFamilyOther indicates a guest OS not covered by any
	// specific family.
	VirtualMachineGuestOSFamilyOther VirtualMachineGuestOSFamily = "Other"

	// VirtualMachineGuestOSFamilySolaris indicates a Sun/Oracle Solaris guest OS.
	VirtualMachineGuestOSFamilySolaris VirtualMachineGuestOSFamily = "Solaris"

	// VirtualMachineGuestOSFamilyWindows indicates a Microsoft Windows guest OS.
	VirtualMachineGuestOSFamilyWindows VirtualMachineGuestOSFamily = "Windows"
)

// ToVimType returns the Vim identifier for the VirtualMachineGuestOSFamily.
func (t VirtualMachineGuestOSFamily) ToVimType() string {
	switch t {
	case VirtualMachineGuestOSFamilyDarwin:
		return "darwinGuestFamily"
	case VirtualMachineGuestOSFamilyLinux:
		return "linuxGuest"
	case VirtualMachineGuestOSFamilyNetware:
		return "netwareGuest"
	case VirtualMachineGuestOSFamilyOther:
		return "otherGuestFamily"
	case VirtualMachineGuestOSFamilySolaris:
		return "solarisGuest"
	case VirtualMachineGuestOSFamilyWindows:
		return "windowsGuest"
	}
	return string(t)
}

// FromVimType returns the VirtualMachineGuestOSFamily from the Vim
// identifier.
func (t *VirtualMachineGuestOSFamily) FromVimType(s string) {
	switch s {
	case "darwinGuestFamily":
		*t = VirtualMachineGuestOSFamilyDarwin
	case "linuxGuest":
		*t = VirtualMachineGuestOSFamilyLinux
	case "netwareGuest":
		*t = VirtualMachineGuestOSFamilyNetware
	case "otherGuestFamily":
		*t = VirtualMachineGuestOSFamilyOther
	case "solarisGuest":
		*t = VirtualMachineGuestOSFamilySolaris
	case "windowsGuest":
		*t = VirtualMachineGuestOSFamilyWindows
	}
	*t = VirtualMachineGuestOSFamily(s)
}

// VirtualMachineGuestOSIdentifier identifies a guest operating system.
// It corresponds to vim.vm.GuestOsDescriptor.GuestOsIdentifier.
type VirtualMachineGuestOSIdentifier string

const (
	// Windows

	// VirtualMachineGuestOSIdentifierDOS indicates MS-DOS.
	VirtualMachineGuestOSIdentifierDOS VirtualMachineGuestOSIdentifier = "DOS"

	// VirtualMachineGuestOSIdentifierWin31 indicates Windows 3.1.
	VirtualMachineGuestOSIdentifierWin31 VirtualMachineGuestOSIdentifier = "Win31"

	// VirtualMachineGuestOSIdentifierWin95 indicates Windows 95.
	VirtualMachineGuestOSIdentifierWin95 VirtualMachineGuestOSIdentifier = "Win95"

	// VirtualMachineGuestOSIdentifierWin98 indicates Windows 98.
	VirtualMachineGuestOSIdentifierWin98 VirtualMachineGuestOSIdentifier = "Win98"

	// VirtualMachineGuestOSIdentifierWinMe indicates Windows Millennium
	// Edition.
	VirtualMachineGuestOSIdentifierWinMe VirtualMachineGuestOSIdentifier = "WinMe"

	// VirtualMachineGuestOSIdentifierWinNT indicates Windows NT 4.
	VirtualMachineGuestOSIdentifierWinNT VirtualMachineGuestOSIdentifier = "WinNT"

	// VirtualMachineGuestOSIdentifierWin2000Pro indicates Windows 2000
	// Professional.
	VirtualMachineGuestOSIdentifierWin2000Pro VirtualMachineGuestOSIdentifier = "Win2000Pro"

	// VirtualMachineGuestOSIdentifierWin2000Serv indicates Windows 2000
	// Server.
	VirtualMachineGuestOSIdentifierWin2000Serv VirtualMachineGuestOSIdentifier = "Win2000Serv"

	// VirtualMachineGuestOSIdentifierWin2000AdvServ indicates Windows 2000
	// Advanced Server.
	VirtualMachineGuestOSIdentifierWin2000AdvServ VirtualMachineGuestOSIdentifier = "Win2000AdvServ"

	// VirtualMachineGuestOSIdentifierWinXPHome indicates Windows XP Home
	// Edition.
	VirtualMachineGuestOSIdentifierWinXPHome VirtualMachineGuestOSIdentifier = "WinXPHome"

	// VirtualMachineGuestOSIdentifierWinXPPro indicates Windows XP
	// Professional.
	VirtualMachineGuestOSIdentifierWinXPPro VirtualMachineGuestOSIdentifier = "WinXPPro"

	// VirtualMachineGuestOSIdentifierWinXPPro64 indicates Windows XP
	// Professional Edition (64 bit).
	VirtualMachineGuestOSIdentifierWinXPPro64 VirtualMachineGuestOSIdentifier = "WinXPPro64"

	// VirtualMachineGuestOSIdentifierWinNetWeb indicates Windows Server
	// 2003, Web Edition.
	VirtualMachineGuestOSIdentifierWinNetWeb VirtualMachineGuestOSIdentifier = "WinNetWeb"

	// VirtualMachineGuestOSIdentifierWinNetStandard indicates Windows
	// Server 2003, Standard Edition.
	VirtualMachineGuestOSIdentifierWinNetStandard VirtualMachineGuestOSIdentifier = "WinNetStandard"

	// VirtualMachineGuestOSIdentifierWinNetEnterprise indicates Windows
	// Server 2003, Enterprise Edition.
	VirtualMachineGuestOSIdentifierWinNetEnterprise VirtualMachineGuestOSIdentifier = "WinNetEnterprise"

	// VirtualMachineGuestOSIdentifierWinNetDatacenter indicates Windows
	// Server 2003, Datacenter Edition.
	VirtualMachineGuestOSIdentifierWinNetDatacenter VirtualMachineGuestOSIdentifier = "WinNetDatacenter"

	// VirtualMachineGuestOSIdentifierWinNetBusiness indicates Windows
	// Small Business Server 2003.
	VirtualMachineGuestOSIdentifierWinNetBusiness VirtualMachineGuestOSIdentifier = "WinNetBusiness"

	// VirtualMachineGuestOSIdentifierWinNetStandard64 indicates Windows
	// Server 2003, Standard Edition (64 bit).
	VirtualMachineGuestOSIdentifierWinNetStandard64 VirtualMachineGuestOSIdentifier = "WinNetStandard64"

	// VirtualMachineGuestOSIdentifierWinNetEnterprise64 indicates Windows
	// Server 2003, Enterprise Edition (64 bit).
	VirtualMachineGuestOSIdentifierWinNetEnterprise64 VirtualMachineGuestOSIdentifier = "WinNetEnterprise64"

	// VirtualMachineGuestOSIdentifierWinLonghorn indicates Windows
	// Longhorn.
	VirtualMachineGuestOSIdentifierWinLonghorn VirtualMachineGuestOSIdentifier = "WinLonghorn"

	// VirtualMachineGuestOSIdentifierWinLonghorn64 indicates Windows
	// Longhorn (64 bit).
	VirtualMachineGuestOSIdentifierWinLonghorn64 VirtualMachineGuestOSIdentifier = "WinLonghorn64"

	// VirtualMachineGuestOSIdentifierWinNetDatacenter64 indicates Windows
	// Server 2003, Datacenter Edition (64 bit).
	VirtualMachineGuestOSIdentifierWinNetDatacenter64 VirtualMachineGuestOSIdentifier = "WinNetDatacenter64"

	// VirtualMachineGuestOSIdentifierWinVista indicates Windows Vista.
	VirtualMachineGuestOSIdentifierWinVista VirtualMachineGuestOSIdentifier = "WinVista"

	// VirtualMachineGuestOSIdentifierWinVista64 indicates Windows Vista
	// (64 bit).
	VirtualMachineGuestOSIdentifierWinVista64 VirtualMachineGuestOSIdentifier = "WinVista64"

	// VirtualMachineGuestOSIdentifierWindows7 indicates Windows 7.
	VirtualMachineGuestOSIdentifierWindows7 VirtualMachineGuestOSIdentifier = "Windows7"

	// VirtualMachineGuestOSIdentifierWindows7x64 indicates Windows 7
	// (64 bit).
	VirtualMachineGuestOSIdentifierWindows7x64 VirtualMachineGuestOSIdentifier = "Windows7x64"

	// VirtualMachineGuestOSIdentifierWindows7Server64 indicates Windows
	// Server 2008 R2 (64 bit).
	VirtualMachineGuestOSIdentifierWindows7Server64 VirtualMachineGuestOSIdentifier = "Windows7Server64"

	// VirtualMachineGuestOSIdentifierWindows8 indicates Windows 8.
	VirtualMachineGuestOSIdentifierWindows8 VirtualMachineGuestOSIdentifier = "Windows8"

	// VirtualMachineGuestOSIdentifierWindows8x64 indicates Windows 8
	// (64 bit).
	VirtualMachineGuestOSIdentifierWindows8x64 VirtualMachineGuestOSIdentifier = "Windows8x64"

	// VirtualMachineGuestOSIdentifierWindows8Server64 indicates Windows
	// Server 2012 (64 bit).
	VirtualMachineGuestOSIdentifierWindows8Server64 VirtualMachineGuestOSIdentifier = "Windows8Server64"

	// VirtualMachineGuestOSIdentifierWindows9 indicates Windows 10.
	VirtualMachineGuestOSIdentifierWindows9 VirtualMachineGuestOSIdentifier = "Windows9"

	// VirtualMachineGuestOSIdentifierWindows9x64 indicates Windows 10
	// (64 bit).
	VirtualMachineGuestOSIdentifierWindows9x64 VirtualMachineGuestOSIdentifier = "Windows9x64"

	// VirtualMachineGuestOSIdentifierWindows9Server64 indicates Windows
	// Server 2016 (64 bit).
	VirtualMachineGuestOSIdentifierWindows9Server64 VirtualMachineGuestOSIdentifier = "Windows9Server64"

	// VirtualMachineGuestOSIdentifierWindows11x64 indicates Windows 11.
	VirtualMachineGuestOSIdentifierWindows11x64 VirtualMachineGuestOSIdentifier = "Windows11x64"

	// VirtualMachineGuestOSIdentifierWindows12x64 indicates Windows 12.
	VirtualMachineGuestOSIdentifierWindows12x64 VirtualMachineGuestOSIdentifier = "Windows12x64"

	// VirtualMachineGuestOSIdentifierWindowsHyperV indicates Windows
	// Hyper-V.
	VirtualMachineGuestOSIdentifierWindowsHyperV VirtualMachineGuestOSIdentifier = "WindowsHyperV"

	// VirtualMachineGuestOSIdentifierWindows2019Serverx64 indicates
	// Windows Server 2019.
	VirtualMachineGuestOSIdentifierWindows2019Serverx64 VirtualMachineGuestOSIdentifier = "Windows2019Serverx64"

	// VirtualMachineGuestOSIdentifierWindows2019ServerNextx64 indicates
	// Windows Server 2022.
	VirtualMachineGuestOSIdentifierWindows2019ServerNextx64 VirtualMachineGuestOSIdentifier = "Windows2019ServerNextx64"

	// VirtualMachineGuestOSIdentifierWindows2022ServerNextx64 indicates
	// Windows Server 2025.
	VirtualMachineGuestOSIdentifierWindows2022ServerNextx64 VirtualMachineGuestOSIdentifier = "Windows2022ServerNextx64"

	// FreeBSD

	// VirtualMachineGuestOSIdentifierFreeBSD indicates FreeBSD.
	VirtualMachineGuestOSIdentifierFreeBSD VirtualMachineGuestOSIdentifier = "FreeBSD"

	// VirtualMachineGuestOSIdentifierFreeBSD64 indicates FreeBSD x64.
	VirtualMachineGuestOSIdentifierFreeBSD64 VirtualMachineGuestOSIdentifier = "FreeBSD64"

	// VirtualMachineGuestOSIdentifierFreeBSD11 indicates FreeBSD 11.
	VirtualMachineGuestOSIdentifierFreeBSD11 VirtualMachineGuestOSIdentifier = "FreeBSD11"

	// VirtualMachineGuestOSIdentifierFreeBSD11x64 indicates FreeBSD 11
	// (64 bit).
	VirtualMachineGuestOSIdentifierFreeBSD11x64 VirtualMachineGuestOSIdentifier = "FreeBSD11x64"

	// VirtualMachineGuestOSIdentifierFreeBSD12 indicates FreeBSD 12.
	VirtualMachineGuestOSIdentifierFreeBSD12 VirtualMachineGuestOSIdentifier = "FreeBSD12"

	// VirtualMachineGuestOSIdentifierFreeBSD12x64 indicates FreeBSD 12
	// (64 bit).
	VirtualMachineGuestOSIdentifierFreeBSD12x64 VirtualMachineGuestOSIdentifier = "FreeBSD12x64"

	// VirtualMachineGuestOSIdentifierFreeBSD13 indicates FreeBSD 13.
	VirtualMachineGuestOSIdentifierFreeBSD13 VirtualMachineGuestOSIdentifier = "FreeBSD13"

	// VirtualMachineGuestOSIdentifierFreeBSD13x64 indicates FreeBSD 13
	// (64 bit).
	VirtualMachineGuestOSIdentifierFreeBSD13x64 VirtualMachineGuestOSIdentifier = "FreeBSD13x64"

	// VirtualMachineGuestOSIdentifierFreeBSD14 indicates FreeBSD 14.
	VirtualMachineGuestOSIdentifierFreeBSD14 VirtualMachineGuestOSIdentifier = "FreeBSD14"

	// VirtualMachineGuestOSIdentifierFreeBSD14x64 indicates FreeBSD 14
	// (64 bit).
	VirtualMachineGuestOSIdentifierFreeBSD14x64 VirtualMachineGuestOSIdentifier = "FreeBSD14x64"

	// VirtualMachineGuestOSIdentifierFreeBSD15 indicates FreeBSD 15.
	VirtualMachineGuestOSIdentifierFreeBSD15 VirtualMachineGuestOSIdentifier = "FreeBSD15"

	// VirtualMachineGuestOSIdentifierFreeBSD15x64 indicates FreeBSD 15
	// (64 bit).
	VirtualMachineGuestOSIdentifierFreeBSD15x64 VirtualMachineGuestOSIdentifier = "FreeBSD15x64"

	// Red Hat

	// VirtualMachineGuestOSIdentifierRedHat indicates Red Hat Linux 2.1.
	VirtualMachineGuestOSIdentifierRedHat VirtualMachineGuestOSIdentifier = "RedHat"

	// VirtualMachineGuestOSIdentifierRHEL2 indicates Red Hat Enterprise
	// Linux 2.
	VirtualMachineGuestOSIdentifierRHEL2 VirtualMachineGuestOSIdentifier = "RHEL2"

	// VirtualMachineGuestOSIdentifierRHEL3 indicates Red Hat Enterprise
	// Linux 3.
	VirtualMachineGuestOSIdentifierRHEL3 VirtualMachineGuestOSIdentifier = "RHEL3"

	// VirtualMachineGuestOSIdentifierRHEL3x64 indicates Red Hat Enterprise
	// Linux 3 (64 bit).
	VirtualMachineGuestOSIdentifierRHEL3x64 VirtualMachineGuestOSIdentifier = "RHEL3x64"

	// VirtualMachineGuestOSIdentifierRHEL4 indicates Red Hat Enterprise
	// Linux 4.
	VirtualMachineGuestOSIdentifierRHEL4 VirtualMachineGuestOSIdentifier = "RHEL4"

	// VirtualMachineGuestOSIdentifierRHEL4x64 indicates Red Hat Enterprise
	// Linux 4 (64 bit).
	VirtualMachineGuestOSIdentifierRHEL4x64 VirtualMachineGuestOSIdentifier = "RHEL4x64"

	// VirtualMachineGuestOSIdentifierRHEL5 indicates Red Hat Enterprise
	// Linux 5.
	VirtualMachineGuestOSIdentifierRHEL5 VirtualMachineGuestOSIdentifier = "RHEL5"

	// VirtualMachineGuestOSIdentifierRHEL5x64 indicates Red Hat Enterprise
	// Linux 5 (64 bit).
	VirtualMachineGuestOSIdentifierRHEL5x64 VirtualMachineGuestOSIdentifier = "RHEL5x64"

	// VirtualMachineGuestOSIdentifierRHEL6 indicates Red Hat Enterprise
	// Linux 6.
	VirtualMachineGuestOSIdentifierRHEL6 VirtualMachineGuestOSIdentifier = "RHEL6"

	// VirtualMachineGuestOSIdentifierRHEL6x64 indicates Red Hat Enterprise
	// Linux 6 (64 bit).
	VirtualMachineGuestOSIdentifierRHEL6x64 VirtualMachineGuestOSIdentifier = "RHEL6x64"

	// VirtualMachineGuestOSIdentifierRHEL7 indicates Red Hat Enterprise
	// Linux 7.
	VirtualMachineGuestOSIdentifierRHEL7 VirtualMachineGuestOSIdentifier = "RHEL7"

	// VirtualMachineGuestOSIdentifierRHEL7x64 indicates Red Hat Enterprise
	// Linux 7 (64 bit).
	VirtualMachineGuestOSIdentifierRHEL7x64 VirtualMachineGuestOSIdentifier = "RHEL7x64"

	// VirtualMachineGuestOSIdentifierRHEL8x64 indicates Red Hat Enterprise
	// Linux 8 (64 bit).
	VirtualMachineGuestOSIdentifierRHEL8x64 VirtualMachineGuestOSIdentifier = "RHEL8x64"

	// VirtualMachineGuestOSIdentifierRHEL9x64 indicates Red Hat Enterprise
	// Linux 9 (64 bit).
	VirtualMachineGuestOSIdentifierRHEL9x64 VirtualMachineGuestOSIdentifier = "RHEL9x64"

	// VirtualMachineGuestOSIdentifierRHEL10x64 indicates Red Hat Enterprise
	// Linux 10 (64 bit).
	VirtualMachineGuestOSIdentifierRHEL10x64 VirtualMachineGuestOSIdentifier = "RHEL10x64"

	// CentOS

	// VirtualMachineGuestOSIdentifierCentOS indicates CentOS 4/5.
	VirtualMachineGuestOSIdentifierCentOS VirtualMachineGuestOSIdentifier = "CentOS"

	// VirtualMachineGuestOSIdentifierCentOS64 indicates CentOS 4/5
	// (64 bit).
	VirtualMachineGuestOSIdentifierCentOS64 VirtualMachineGuestOSIdentifier = "CentOS64"

	// VirtualMachineGuestOSIdentifierCentOS6 indicates CentOS 6.
	VirtualMachineGuestOSIdentifierCentOS6 VirtualMachineGuestOSIdentifier = "CentOS6"

	// VirtualMachineGuestOSIdentifierCentOS6x64 indicates CentOS 6
	// (64 bit).
	VirtualMachineGuestOSIdentifierCentOS6x64 VirtualMachineGuestOSIdentifier = "CentOS6x64"

	// VirtualMachineGuestOSIdentifierCentOS7 indicates CentOS 7.
	VirtualMachineGuestOSIdentifierCentOS7 VirtualMachineGuestOSIdentifier = "CentOS7"

	// VirtualMachineGuestOSIdentifierCentOS7x64 indicates CentOS 7
	// (64 bit).
	VirtualMachineGuestOSIdentifierCentOS7x64 VirtualMachineGuestOSIdentifier = "CentOS7x64"

	// VirtualMachineGuestOSIdentifierCentOS8x64 indicates CentOS 8
	// (64 bit).
	VirtualMachineGuestOSIdentifierCentOS8x64 VirtualMachineGuestOSIdentifier = "CentOS8x64"

	// VirtualMachineGuestOSIdentifierCentOS9x64 indicates CentOS 9
	// (64 bit).
	VirtualMachineGuestOSIdentifierCentOS9x64 VirtualMachineGuestOSIdentifier = "CentOS9x64"

	// Oracle Linux

	// VirtualMachineGuestOSIdentifierOracleLinux indicates Oracle Linux
	// 4/5.
	VirtualMachineGuestOSIdentifierOracleLinux VirtualMachineGuestOSIdentifier = "OracleLinux"

	// VirtualMachineGuestOSIdentifierOracleLinux64 indicates Oracle Linux
	// 4/5 (64 bit).
	VirtualMachineGuestOSIdentifierOracleLinux64 VirtualMachineGuestOSIdentifier = "OracleLinux64"

	// VirtualMachineGuestOSIdentifierOracleLinux6 indicates Oracle Linux 6.
	VirtualMachineGuestOSIdentifierOracleLinux6 VirtualMachineGuestOSIdentifier = "OracleLinux6"

	// VirtualMachineGuestOSIdentifierOracleLinux6x64 indicates Oracle Linux
	// 6 (64 bit).
	VirtualMachineGuestOSIdentifierOracleLinux6x64 VirtualMachineGuestOSIdentifier = "OracleLinux6x64"

	// VirtualMachineGuestOSIdentifierOracleLinux7 indicates Oracle Linux 7.
	VirtualMachineGuestOSIdentifierOracleLinux7 VirtualMachineGuestOSIdentifier = "OracleLinux7"

	// VirtualMachineGuestOSIdentifierOracleLinux7x64 indicates Oracle Linux
	// 7 (64 bit).
	VirtualMachineGuestOSIdentifierOracleLinux7x64 VirtualMachineGuestOSIdentifier = "OracleLinux7x64"

	// VirtualMachineGuestOSIdentifierOracleLinux8x64 indicates Oracle Linux
	// 8 (64 bit).
	VirtualMachineGuestOSIdentifierOracleLinux8x64 VirtualMachineGuestOSIdentifier = "OracleLinux8x64"

	// VirtualMachineGuestOSIdentifierOracleLinux9x64 indicates Oracle Linux
	// 9 (64 bit).
	VirtualMachineGuestOSIdentifierOracleLinux9x64 VirtualMachineGuestOSIdentifier = "OracleLinux9x64"

	// VirtualMachineGuestOSIdentifierOracleLinux10x64 indicates Oracle
	// Linux 10 (64 bit).
	VirtualMachineGuestOSIdentifierOracleLinux10x64 VirtualMachineGuestOSIdentifier = "OracleLinux10x64"

	// SUSE

	// VirtualMachineGuestOSIdentifierSUSE indicates SUSE Linux.
	VirtualMachineGuestOSIdentifierSUSE VirtualMachineGuestOSIdentifier = "SUSE"

	// VirtualMachineGuestOSIdentifierSUSE64 indicates SUSE Linux (64 bit).
	VirtualMachineGuestOSIdentifierSUSE64 VirtualMachineGuestOSIdentifier = "SUSE64"

	// VirtualMachineGuestOSIdentifierSLES indicates SUSE Linux Enterprise
	// Server 9.
	VirtualMachineGuestOSIdentifierSLES VirtualMachineGuestOSIdentifier = "SLES"

	// VirtualMachineGuestOSIdentifierSLES64 indicates SUSE Linux Enterprise
	// Server 9 (64 bit).
	VirtualMachineGuestOSIdentifierSLES64 VirtualMachineGuestOSIdentifier = "SLES64"

	// VirtualMachineGuestOSIdentifierSLES10 indicates SUSE Linux Enterprise
	// Server 10.
	VirtualMachineGuestOSIdentifierSLES10 VirtualMachineGuestOSIdentifier = "SLES10"

	// VirtualMachineGuestOSIdentifierSLES10x64 indicates SUSE Linux
	// Enterprise Server 10 (64 bit).
	VirtualMachineGuestOSIdentifierSLES10x64 VirtualMachineGuestOSIdentifier = "SLES10x64"

	// VirtualMachineGuestOSIdentifierSLES11 indicates SUSE Linux Enterprise
	// Server 11.
	VirtualMachineGuestOSIdentifierSLES11 VirtualMachineGuestOSIdentifier = "SLES11"

	// VirtualMachineGuestOSIdentifierSLES11x64 indicates SUSE Linux
	// Enterprise Server 11 (64 bit).
	VirtualMachineGuestOSIdentifierSLES11x64 VirtualMachineGuestOSIdentifier = "SLES11x64"

	// VirtualMachineGuestOSIdentifierSLES12 indicates SUSE Linux Enterprise
	// Server 12.
	VirtualMachineGuestOSIdentifierSLES12 VirtualMachineGuestOSIdentifier = "SLES12"

	// VirtualMachineGuestOSIdentifierSLES12x64 indicates SUSE Linux
	// Enterprise Server 12 (64 bit).
	VirtualMachineGuestOSIdentifierSLES12x64 VirtualMachineGuestOSIdentifier = "SLES12x64"

	// VirtualMachineGuestOSIdentifierSLES15x64 indicates SUSE Linux
	// Enterprise Server 15 (64 bit).
	VirtualMachineGuestOSIdentifierSLES15x64 VirtualMachineGuestOSIdentifier = "SLES15x64"

	// VirtualMachineGuestOSIdentifierSLES16x64 indicates SUSE Linux
	// Enterprise Server 16 (64 bit).
	VirtualMachineGuestOSIdentifierSLES16x64 VirtualMachineGuestOSIdentifier = "SLES16x64"

	// Novell

	// VirtualMachineGuestOSIdentifierNLD9 indicates Novell Linux Desktop 9.
	VirtualMachineGuestOSIdentifierNLD9 VirtualMachineGuestOSIdentifier = "NLD9"

	// VirtualMachineGuestOSIdentifierOES indicates Open Enterprise Server.
	VirtualMachineGuestOSIdentifierOES VirtualMachineGuestOSIdentifier = "OES"

	// VirtualMachineGuestOSIdentifierSJDS indicates Sun Java Desktop System.
	VirtualMachineGuestOSIdentifierSJDS VirtualMachineGuestOSIdentifier = "SJDS"

	// Mandriva / Mandrake

	// VirtualMachineGuestOSIdentifierMandrake indicates Mandrake Linux.
	VirtualMachineGuestOSIdentifierMandrake VirtualMachineGuestOSIdentifier = "Mandrake"

	// VirtualMachineGuestOSIdentifierMandriva indicates Mandriva Linux.
	VirtualMachineGuestOSIdentifierMandriva VirtualMachineGuestOSIdentifier = "Mandriva"

	// VirtualMachineGuestOSIdentifierMandriva64 indicates Mandriva Linux
	// (64 bit).
	VirtualMachineGuestOSIdentifierMandriva64 VirtualMachineGuestOSIdentifier = "Mandriva64"

	// TurboLinux

	// VirtualMachineGuestOSIdentifierTurboLinux indicates Turbolinux.
	VirtualMachineGuestOSIdentifierTurboLinux VirtualMachineGuestOSIdentifier = "TurboLinux"

	// VirtualMachineGuestOSIdentifierTurboLinux64 indicates Turbolinux
	// (64 bit).
	VirtualMachineGuestOSIdentifierTurboLinux64 VirtualMachineGuestOSIdentifier = "TurboLinux64"

	// Ubuntu

	// VirtualMachineGuestOSIdentifierUbuntu indicates Ubuntu Linux.
	VirtualMachineGuestOSIdentifierUbuntu VirtualMachineGuestOSIdentifier = "Ubuntu"

	// VirtualMachineGuestOSIdentifierUbuntu64 indicates Ubuntu Linux
	// (64 bit).
	VirtualMachineGuestOSIdentifierUbuntu64 VirtualMachineGuestOSIdentifier = "Ubuntu64"

	// Debian

	// VirtualMachineGuestOSIdentifierDebian4 indicates Debian GNU/Linux 4.
	VirtualMachineGuestOSIdentifierDebian4 VirtualMachineGuestOSIdentifier = "Debian4"

	// VirtualMachineGuestOSIdentifierDebian4x64 indicates Debian GNU/Linux
	// 4 (64 bit).
	VirtualMachineGuestOSIdentifierDebian4x64 VirtualMachineGuestOSIdentifier = "Debian4x64"

	// VirtualMachineGuestOSIdentifierDebian5 indicates Debian GNU/Linux 5.
	VirtualMachineGuestOSIdentifierDebian5 VirtualMachineGuestOSIdentifier = "Debian5"

	// VirtualMachineGuestOSIdentifierDebian5x64 indicates Debian GNU/Linux
	// 5 (64 bit).
	VirtualMachineGuestOSIdentifierDebian5x64 VirtualMachineGuestOSIdentifier = "Debian5x64"

	// VirtualMachineGuestOSIdentifierDebian6 indicates Debian GNU/Linux 6.
	VirtualMachineGuestOSIdentifierDebian6 VirtualMachineGuestOSIdentifier = "Debian6"

	// VirtualMachineGuestOSIdentifierDebian6x64 indicates Debian GNU/Linux
	// 6 (64 bit).
	VirtualMachineGuestOSIdentifierDebian6x64 VirtualMachineGuestOSIdentifier = "Debian6x64"

	// VirtualMachineGuestOSIdentifierDebian7 indicates Debian GNU/Linux 7.
	VirtualMachineGuestOSIdentifierDebian7 VirtualMachineGuestOSIdentifier = "Debian7"

	// VirtualMachineGuestOSIdentifierDebian7x64 indicates Debian GNU/Linux
	// 7 (64 bit).
	VirtualMachineGuestOSIdentifierDebian7x64 VirtualMachineGuestOSIdentifier = "Debian7x64"

	// VirtualMachineGuestOSIdentifierDebian8 indicates Debian GNU/Linux 8.
	VirtualMachineGuestOSIdentifierDebian8 VirtualMachineGuestOSIdentifier = "Debian8"

	// VirtualMachineGuestOSIdentifierDebian8x64 indicates Debian GNU/Linux
	// 8 (64 bit).
	VirtualMachineGuestOSIdentifierDebian8x64 VirtualMachineGuestOSIdentifier = "Debian8x64"

	// VirtualMachineGuestOSIdentifierDebian9 indicates Debian GNU/Linux 9.
	VirtualMachineGuestOSIdentifierDebian9 VirtualMachineGuestOSIdentifier = "Debian9"

	// VirtualMachineGuestOSIdentifierDebian9x64 indicates Debian GNU/Linux
	// 9 (64 bit).
	VirtualMachineGuestOSIdentifierDebian9x64 VirtualMachineGuestOSIdentifier = "Debian9x64"

	// VirtualMachineGuestOSIdentifierDebian10 indicates Debian GNU/Linux
	// 10.
	VirtualMachineGuestOSIdentifierDebian10 VirtualMachineGuestOSIdentifier = "Debian10"

	// VirtualMachineGuestOSIdentifierDebian10x64 indicates Debian GNU/Linux
	// 10 (64 bit).
	VirtualMachineGuestOSIdentifierDebian10x64 VirtualMachineGuestOSIdentifier = "Debian10x64"

	// VirtualMachineGuestOSIdentifierDebian11 indicates Debian GNU/Linux
	// 11.
	VirtualMachineGuestOSIdentifierDebian11 VirtualMachineGuestOSIdentifier = "Debian11"

	// VirtualMachineGuestOSIdentifierDebian11x64 indicates Debian GNU/Linux
	// 11 (64 bit).
	VirtualMachineGuestOSIdentifierDebian11x64 VirtualMachineGuestOSIdentifier = "Debian11x64"

	// VirtualMachineGuestOSIdentifierDebian12 indicates Debian GNU/Linux
	// 12.
	VirtualMachineGuestOSIdentifierDebian12 VirtualMachineGuestOSIdentifier = "Debian12"

	// VirtualMachineGuestOSIdentifierDebian12x64 indicates Debian GNU/Linux
	// 12 (64 bit).
	VirtualMachineGuestOSIdentifierDebian12x64 VirtualMachineGuestOSIdentifier = "Debian12x64"

	// VirtualMachineGuestOSIdentifierDebian13 indicates Debian GNU/Linux
	// 13.
	VirtualMachineGuestOSIdentifierDebian13 VirtualMachineGuestOSIdentifier = "Debian13"

	// VirtualMachineGuestOSIdentifierDebian13x64 indicates Debian GNU/Linux
	// 13 (64 bit).
	VirtualMachineGuestOSIdentifierDebian13x64 VirtualMachineGuestOSIdentifier = "Debian13x64"

	// Asianux

	// VirtualMachineGuestOSIdentifierAsianux3 indicates Asianux Server 3.
	VirtualMachineGuestOSIdentifierAsianux3 VirtualMachineGuestOSIdentifier = "Asianux3"

	// VirtualMachineGuestOSIdentifierAsianux3x64 indicates Asianux Server
	// 3 (64 bit).
	VirtualMachineGuestOSIdentifierAsianux3x64 VirtualMachineGuestOSIdentifier = "Asianux3x64"

	// VirtualMachineGuestOSIdentifierAsianux4 indicates Asianux Server 4.
	VirtualMachineGuestOSIdentifierAsianux4 VirtualMachineGuestOSIdentifier = "Asianux4"

	// VirtualMachineGuestOSIdentifierAsianux4x64 indicates Asianux Server
	// 4 (64 bit).
	VirtualMachineGuestOSIdentifierAsianux4x64 VirtualMachineGuestOSIdentifier = "Asianux4x64"

	// VirtualMachineGuestOSIdentifierAsianux5x64 indicates Asianux Server
	// 5 (64 bit).
	VirtualMachineGuestOSIdentifierAsianux5x64 VirtualMachineGuestOSIdentifier = "Asianux5x64"

	// VirtualMachineGuestOSIdentifierAsianux7x64 indicates Asianux Server
	// 7 (64 bit).
	VirtualMachineGuestOSIdentifierAsianux7x64 VirtualMachineGuestOSIdentifier = "Asianux7x64"

	// VirtualMachineGuestOSIdentifierAsianux8x64 indicates Asianux Server
	// 8 (64 bit).
	VirtualMachineGuestOSIdentifierAsianux8x64 VirtualMachineGuestOSIdentifier = "Asianux8x64"

	// VirtualMachineGuestOSIdentifierAsianux9x64 indicates Asianux Server
	// 9 (64 bit).
	VirtualMachineGuestOSIdentifierAsianux9x64 VirtualMachineGuestOSIdentifier = "Asianux9x64"

	// VirtualMachineGuestOSIdentifierMiracleLinux64 indicates MIRACLE LINUX
	// (64 bit).
	VirtualMachineGuestOSIdentifierMiracleLinux64 VirtualMachineGuestOSIdentifier = "MiracleLinux64"

	// VirtualMachineGuestOSIdentifierPardus64 indicates Pardus (64 bit).
	VirtualMachineGuestOSIdentifierPardus64 VirtualMachineGuestOSIdentifier = "Pardus64"

	// OpenSUSE

	// VirtualMachineGuestOSIdentifierOpenSUSE indicates OpenSUSE Linux.
	VirtualMachineGuestOSIdentifierOpenSUSE VirtualMachineGuestOSIdentifier = "OpenSUSE"

	// VirtualMachineGuestOSIdentifierOpenSUSE64 indicates OpenSUSE Linux
	// (64 bit).
	VirtualMachineGuestOSIdentifierOpenSUSE64 VirtualMachineGuestOSIdentifier = "OpenSUSE64"

	// Fedora

	// VirtualMachineGuestOSIdentifierFedora indicates Fedora Linux.
	VirtualMachineGuestOSIdentifierFedora VirtualMachineGuestOSIdentifier = "Fedora"

	// VirtualMachineGuestOSIdentifierFedora64 indicates Fedora Linux
	// (64 bit).
	VirtualMachineGuestOSIdentifierFedora64 VirtualMachineGuestOSIdentifier = "Fedora64"

	// CoreOS / Photon

	// VirtualMachineGuestOSIdentifierCoreOS64 indicates CoreOS Linux
	// (64 bit).
	VirtualMachineGuestOSIdentifierCoreOS64 VirtualMachineGuestOSIdentifier = "CoreOS64"

	// VirtualMachineGuestOSIdentifierVMwarePhoton64 indicates VMware Photon
	// (64 bit).
	VirtualMachineGuestOSIdentifierVMwarePhoton64 VirtualMachineGuestOSIdentifier = "VMwarePhoton64"

	// Other Linux

	// VirtualMachineGuestOSIdentifierOther24xLinux indicates Linux 2.4.x
	// Kernel.
	VirtualMachineGuestOSIdentifierOther24xLinux VirtualMachineGuestOSIdentifier = "Other24xLinux"

	// VirtualMachineGuestOSIdentifierOther26xLinux indicates Linux 2.6.x
	// Kernel.
	VirtualMachineGuestOSIdentifierOther26xLinux VirtualMachineGuestOSIdentifier = "Other26xLinux"

	// VirtualMachineGuestOSIdentifierOtherLinux indicates Linux 2.2.x
	// Kernel.
	VirtualMachineGuestOSIdentifierOtherLinux VirtualMachineGuestOSIdentifier = "OtherLinux"

	// VirtualMachineGuestOSIdentifierOther3xLinux indicates Linux 3.x
	// Kernel.
	VirtualMachineGuestOSIdentifierOther3xLinux VirtualMachineGuestOSIdentifier = "Other3xLinux"

	// VirtualMachineGuestOSIdentifierOther4xLinux indicates Linux 4.x
	// Kernel.
	VirtualMachineGuestOSIdentifierOther4xLinux VirtualMachineGuestOSIdentifier = "Other4xLinux"

	// VirtualMachineGuestOSIdentifierOther5xLinux indicates Linux 5.x
	// Kernel.
	VirtualMachineGuestOSIdentifierOther5xLinux VirtualMachineGuestOSIdentifier = "Other5xLinux"

	// VirtualMachineGuestOSIdentifierOther6xLinux indicates Linux 6.x
	// Kernel.
	VirtualMachineGuestOSIdentifierOther6xLinux VirtualMachineGuestOSIdentifier = "Other6xLinux"

	// VirtualMachineGuestOSIdentifierOther7xLinux indicates Linux 7.x
	// Kernel.
	VirtualMachineGuestOSIdentifierOther7xLinux VirtualMachineGuestOSIdentifier = "Other7xLinux"

	// VirtualMachineGuestOSIdentifierGenericLinux indicates a generic Linux
	// guest.
	VirtualMachineGuestOSIdentifierGenericLinux VirtualMachineGuestOSIdentifier = "GenericLinux"

	// VirtualMachineGuestOSIdentifierOther24xLinux64 indicates Linux 2.4.x
	// Kernel (64 bit).
	VirtualMachineGuestOSIdentifierOther24xLinux64 VirtualMachineGuestOSIdentifier = "Other24xLinux64"

	// VirtualMachineGuestOSIdentifierOther26xLinux64 indicates Linux 2.6.x
	// Kernel (64 bit).
	VirtualMachineGuestOSIdentifierOther26xLinux64 VirtualMachineGuestOSIdentifier = "Other26xLinux64"

	// VirtualMachineGuestOSIdentifierOther3xLinux64 indicates Linux 3.x
	// Kernel (64 bit).
	VirtualMachineGuestOSIdentifierOther3xLinux64 VirtualMachineGuestOSIdentifier = "Other3xLinux64"

	// VirtualMachineGuestOSIdentifierOther4xLinux64 indicates Linux 4.x
	// Kernel (64 bit).
	VirtualMachineGuestOSIdentifierOther4xLinux64 VirtualMachineGuestOSIdentifier = "Other4xLinux64"

	// VirtualMachineGuestOSIdentifierOther5xLinux64 indicates Linux 5.x
	// Kernel (64 bit).
	VirtualMachineGuestOSIdentifierOther5xLinux64 VirtualMachineGuestOSIdentifier = "Other5xLinux64"

	// VirtualMachineGuestOSIdentifierOther6xLinux64 indicates Linux 6.x
	// Kernel (64 bit).
	VirtualMachineGuestOSIdentifierOther6xLinux64 VirtualMachineGuestOSIdentifier = "Other6xLinux64"

	// VirtualMachineGuestOSIdentifierOther7xLinux64 indicates Linux 7.x
	// Kernel (64 bit).
	VirtualMachineGuestOSIdentifierOther7xLinux64 VirtualMachineGuestOSIdentifier = "Other7xLinux64"

	// VirtualMachineGuestOSIdentifierOtherLinux64 indicates Linux 2.2.x
	// Kernel (64 bit).
	VirtualMachineGuestOSIdentifierOtherLinux64 VirtualMachineGuestOSIdentifier = "OtherLinux64"

	// Solaris

	// VirtualMachineGuestOSIdentifierSolaris6 indicates Solaris 6.
	VirtualMachineGuestOSIdentifierSolaris6 VirtualMachineGuestOSIdentifier = "Solaris6"

	// VirtualMachineGuestOSIdentifierSolaris7 indicates Solaris 7.
	VirtualMachineGuestOSIdentifierSolaris7 VirtualMachineGuestOSIdentifier = "Solaris7"

	// VirtualMachineGuestOSIdentifierSolaris8 indicates Solaris 8.
	VirtualMachineGuestOSIdentifierSolaris8 VirtualMachineGuestOSIdentifier = "Solaris8"

	// VirtualMachineGuestOSIdentifierSolaris9 indicates Solaris 9.
	VirtualMachineGuestOSIdentifierSolaris9 VirtualMachineGuestOSIdentifier = "Solaris9"

	// VirtualMachineGuestOSIdentifierSolaris10 indicates Solaris 10.
	VirtualMachineGuestOSIdentifierSolaris10 VirtualMachineGuestOSIdentifier = "Solaris10"

	// VirtualMachineGuestOSIdentifierSolaris10x64 indicates Solaris 10
	// (64 bit).
	VirtualMachineGuestOSIdentifierSolaris10x64 VirtualMachineGuestOSIdentifier = "Solaris10x64"

	// VirtualMachineGuestOSIdentifierSolaris11x64 indicates Solaris 11
	// (64 bit).
	VirtualMachineGuestOSIdentifierSolaris11x64 VirtualMachineGuestOSIdentifier = "Solaris11x64"

	// Chinese Linux distributions

	// VirtualMachineGuestOSIdentifierFusionOS64 indicates Fusion OS
	// (64 bit).
	VirtualMachineGuestOSIdentifierFusionOS64 VirtualMachineGuestOSIdentifier = "FusionOS64"

	// VirtualMachineGuestOSIdentifierProLinux64 indicates Pro Linux
	// (64 bit).
	VirtualMachineGuestOSIdentifierProLinux64 VirtualMachineGuestOSIdentifier = "ProLinux64"

	// VirtualMachineGuestOSIdentifierKylinLinux64 indicates Kylin Linux
	// (64 bit).
	VirtualMachineGuestOSIdentifierKylinLinux64 VirtualMachineGuestOSIdentifier = "KylinLinux64"

	// OS/2 and eComStation

	// VirtualMachineGuestOSIdentifierOS2 indicates OS/2.
	VirtualMachineGuestOSIdentifierOS2 VirtualMachineGuestOSIdentifier = "OS2"

	// VirtualMachineGuestOSIdentifierEComStation indicates eComStation 1.x.
	VirtualMachineGuestOSIdentifierEComStation VirtualMachineGuestOSIdentifier = "EComStation"

	// VirtualMachineGuestOSIdentifierEComStation2 indicates eComStation 2.x.
	VirtualMachineGuestOSIdentifierEComStation2 VirtualMachineGuestOSIdentifier = "EComStation2"

	// NetWare

	// VirtualMachineGuestOSIdentifierNetWare4 indicates Novell NetWare 4.
	VirtualMachineGuestOSIdentifierNetWare4 VirtualMachineGuestOSIdentifier = "NetWare4"

	// VirtualMachineGuestOSIdentifierNetWare5 indicates Novell NetWare 5.1.
	VirtualMachineGuestOSIdentifierNetWare5 VirtualMachineGuestOSIdentifier = "NetWare5"

	// VirtualMachineGuestOSIdentifierNetWare6 indicates Novell NetWare 6.x.
	VirtualMachineGuestOSIdentifierNetWare6 VirtualMachineGuestOSIdentifier = "NetWare6"

	// SCO

	// VirtualMachineGuestOSIdentifierOpenServer5 indicates SCO OpenServer 5.
	VirtualMachineGuestOSIdentifierOpenServer5 VirtualMachineGuestOSIdentifier = "OpenServer5"

	// VirtualMachineGuestOSIdentifierOpenServer6 indicates SCO OpenServer 6.
	VirtualMachineGuestOSIdentifierOpenServer6 VirtualMachineGuestOSIdentifier = "OpenServer6"

	// VirtualMachineGuestOSIdentifierUnixWare7 indicates SCO UnixWare 7.
	VirtualMachineGuestOSIdentifierUnixWare7 VirtualMachineGuestOSIdentifier = "UnixWare7"

	// macOS / Darwin

	// VirtualMachineGuestOSIdentifierDarwin indicates Apple macOS 10.5.
	VirtualMachineGuestOSIdentifierDarwin VirtualMachineGuestOSIdentifier = "Darwin"

	// VirtualMachineGuestOSIdentifierDarwin64 indicates Apple macOS 10.5
	// (64 bit).
	VirtualMachineGuestOSIdentifierDarwin64 VirtualMachineGuestOSIdentifier = "Darwin64"

	// VirtualMachineGuestOSIdentifierDarwin10 indicates Apple macOS 10.6.
	VirtualMachineGuestOSIdentifierDarwin10 VirtualMachineGuestOSIdentifier = "Darwin10"

	// VirtualMachineGuestOSIdentifierDarwin10x64 indicates Apple macOS
	// 10.6 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin10x64 VirtualMachineGuestOSIdentifier = "Darwin10x64"

	// VirtualMachineGuestOSIdentifierDarwin11 indicates Apple macOS 10.7.
	VirtualMachineGuestOSIdentifierDarwin11 VirtualMachineGuestOSIdentifier = "Darwin11"

	// VirtualMachineGuestOSIdentifierDarwin11x64 indicates Apple macOS
	// 10.7 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin11x64 VirtualMachineGuestOSIdentifier = "Darwin11x64"

	// VirtualMachineGuestOSIdentifierDarwin12x64 indicates Apple macOS
	// 10.8 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin12x64 VirtualMachineGuestOSIdentifier = "Darwin12x64"

	// VirtualMachineGuestOSIdentifierDarwin13x64 indicates Apple macOS
	// 10.9 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin13x64 VirtualMachineGuestOSIdentifier = "Darwin13x64"

	// VirtualMachineGuestOSIdentifierDarwin14x64 indicates Apple macOS
	// 10.10 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin14x64 VirtualMachineGuestOSIdentifier = "Darwin14x64"

	// VirtualMachineGuestOSIdentifierDarwin15x64 indicates Apple macOS
	// 10.11 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin15x64 VirtualMachineGuestOSIdentifier = "Darwin15x64"

	// VirtualMachineGuestOSIdentifierDarwin16x64 indicates Apple macOS
	// 10.12 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin16x64 VirtualMachineGuestOSIdentifier = "Darwin16x64"

	// VirtualMachineGuestOSIdentifierDarwin17x64 indicates Apple macOS
	// 10.13 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin17x64 VirtualMachineGuestOSIdentifier = "Darwin17x64"

	// VirtualMachineGuestOSIdentifierDarwin18x64 indicates Apple macOS
	// 10.14 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin18x64 VirtualMachineGuestOSIdentifier = "Darwin18x64"

	// VirtualMachineGuestOSIdentifierDarwin19x64 indicates Apple macOS
	// 10.15 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin19x64 VirtualMachineGuestOSIdentifier = "Darwin19x64"

	// VirtualMachineGuestOSIdentifierDarwin20x64 indicates Apple macOS
	// 11 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin20x64 VirtualMachineGuestOSIdentifier = "Darwin20x64"

	// VirtualMachineGuestOSIdentifierDarwin21x64 indicates Apple macOS
	// 12 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin21x64 VirtualMachineGuestOSIdentifier = "Darwin21x64"

	// VirtualMachineGuestOSIdentifierDarwin22x64 indicates Apple macOS
	// 13 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin22x64 VirtualMachineGuestOSIdentifier = "Darwin22x64"

	// VirtualMachineGuestOSIdentifierDarwin23x64 indicates Apple macOS
	// 14 (64 bit).
	VirtualMachineGuestOSIdentifierDarwin23x64 VirtualMachineGuestOSIdentifier = "Darwin23x64"

	// VMkernel / ESX

	// VirtualMachineGuestOSIdentifierVMkernel indicates VMware ESX 4.x.
	VirtualMachineGuestOSIdentifierVMkernel VirtualMachineGuestOSIdentifier = "VMkernel"

	// VirtualMachineGuestOSIdentifierVMkernel5 indicates VMware ESXi 5.x.
	VirtualMachineGuestOSIdentifierVMkernel5 VirtualMachineGuestOSIdentifier = "VMkernel5"

	// VirtualMachineGuestOSIdentifierVMkernel6 indicates VMware ESXi 6.x.
	VirtualMachineGuestOSIdentifierVMkernel6 VirtualMachineGuestOSIdentifier = "VMkernel6"

	// VirtualMachineGuestOSIdentifierVMkernel65 indicates VMware ESXi 6.5.
	VirtualMachineGuestOSIdentifierVMkernel65 VirtualMachineGuestOSIdentifier = "VMkernel65"

	// VirtualMachineGuestOSIdentifierVMkernel7 indicates VMware ESXi 7.x.
	VirtualMachineGuestOSIdentifierVMkernel7 VirtualMachineGuestOSIdentifier = "VMkernel7"

	// VirtualMachineGuestOSIdentifierVMkernel8 indicates VMware ESXi 8.x.
	VirtualMachineGuestOSIdentifierVMkernel8 VirtualMachineGuestOSIdentifier = "VMkernel8"

	// VirtualMachineGuestOSIdentifierVMkernel9 indicates VMware ESXi 9.x.
	VirtualMachineGuestOSIdentifierVMkernel9 VirtualMachineGuestOSIdentifier = "VMkernel9"

	// Amazon Linux

	// VirtualMachineGuestOSIdentifierAmazonLinux2x64 indicates Amazon Linux
	// 2 (64 bit).
	VirtualMachineGuestOSIdentifierAmazonLinux2x64 VirtualMachineGuestOSIdentifier = "AmazonLinux2x64"

	// VirtualMachineGuestOSIdentifierAmazonLinux3x64 indicates Amazon Linux
	// 3 (64 bit).
	VirtualMachineGuestOSIdentifierAmazonLinux3x64 VirtualMachineGuestOSIdentifier = "AmazonLinux3x64"

	// CRX

	// VirtualMachineGuestOSIdentifierCRXPod1 indicates CRX Pod 1.
	VirtualMachineGuestOSIdentifierCRXPod1 VirtualMachineGuestOSIdentifier = "CRXPod1"

	// VirtualMachineGuestOSIdentifierCRXSys1 indicates CRX Sys 1.
	VirtualMachineGuestOSIdentifierCRXSys1 VirtualMachineGuestOSIdentifier = "CRXSys1"

	// Rocky Linux / AlmaLinux

	// VirtualMachineGuestOSIdentifierRockyLinux64 indicates Rocky Linux
	// (64 bit).
	VirtualMachineGuestOSIdentifierRockyLinux64 VirtualMachineGuestOSIdentifier = "RockyLinux64"

	// VirtualMachineGuestOSIdentifierAlmaLinux64 indicates AlmaLinux
	// (64 bit).
	VirtualMachineGuestOSIdentifierAlmaLinux64 VirtualMachineGuestOSIdentifier = "AlmaLinux64"

	// Other

	// VirtualMachineGuestOSIdentifierOther indicates Other Operating System.
	VirtualMachineGuestOSIdentifierOther VirtualMachineGuestOSIdentifier = "Other"

	// VirtualMachineGuestOSIdentifierOther64 indicates Other Operating
	// System (64 bit).
	VirtualMachineGuestOSIdentifierOther64 VirtualMachineGuestOSIdentifier = "Other64"
)

// ToVimType returns the vSphere API identifier string for the
// VirtualMachineGuestOSIdentifier.
func (t VirtualMachineGuestOSIdentifier) ToVimType() string {
	s := string(t)
	switch {
	case s == "DOS",
		strings.HasPrefix(s, "Win"):
		return toVimTypeWindows(t)
	case strings.HasPrefix(s, "FreeBSD"):
		return toVimTypeFreeBSD(t)
	case strings.HasPrefix(s, "Solaris"):
		return toVimTypeSolaris(t)
	case strings.HasPrefix(s, "Darwin"):
		return toVimTypeDarwin(t)
	case strings.HasPrefix(s, "VMkernel"):
		return toVimTypeVMkernel(t)
	case strings.HasPrefix(s, "NetWare"):
		return toVimTypeNetWare(t)
	case strings.HasPrefix(s, "OpenServer"),
		strings.HasPrefix(s, "UnixWare"):
		return toVimTypeSCO(t)
	case s == "OS2",
		strings.HasPrefix(s, "EComStation"):
		return toVimTypeOS2(t)
	case strings.HasPrefix(s, "CRX"):
		return toVimTypeCRX(t)
	case s == "Other",
		s == "Other64":
		return toVimTypeOtherOS(t)
	default:
		return toVimTypeLinux(t)
	}
}

// toVimTypeWindows handles DOS and all Windows variants.
func toVimTypeWindows(t VirtualMachineGuestOSIdentifier) string {
	s := string(t)
	switch {
	case s == "DOS":
		return "dosGuest"
	case strings.HasPrefix(s, "Windows"):
		return toVimTypeWindowsModern(t)
	default:
		return toVimTypeWindowsLegacy(t)
	}
}

// toVimTypeWindowsLegacy handles Win3.1 through WinVista.
func toVimTypeWindowsLegacy(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierWin31:
		return "win31Guest"
	case VirtualMachineGuestOSIdentifierWin95:
		return "win95Guest"
	case VirtualMachineGuestOSIdentifierWin98:
		return "win98Guest"
	case VirtualMachineGuestOSIdentifierWinMe:
		return "winMeGuest"
	case VirtualMachineGuestOSIdentifierWinNT:
		return "winNTGuest"
	case VirtualMachineGuestOSIdentifierWin2000Pro:
		return "win2000ProGuest"
	case VirtualMachineGuestOSIdentifierWin2000Serv:
		return "win2000ServGuest"
	case VirtualMachineGuestOSIdentifierWin2000AdvServ:
		return "win2000AdvServGuest"
	case VirtualMachineGuestOSIdentifierWinXPHome:
		return "winXPHomeGuest"
	case VirtualMachineGuestOSIdentifierWinXPPro:
		return "winXPProGuest"
	case VirtualMachineGuestOSIdentifierWinXPPro64:
		return "winXPPro64Guest"
	case VirtualMachineGuestOSIdentifierWinNetWeb:
		return "winNetWebGuest"
	case VirtualMachineGuestOSIdentifierWinNetStandard:
		return "winNetStandardGuest"
	case VirtualMachineGuestOSIdentifierWinNetEnterprise:
		return "winNetEnterpriseGuest"
	case VirtualMachineGuestOSIdentifierWinNetDatacenter:
		return "winNetDatacenterGuest"
	case VirtualMachineGuestOSIdentifierWinNetBusiness:
		return "winNetBusinessGuest"
	case VirtualMachineGuestOSIdentifierWinNetStandard64:
		return "winNetStandard64Guest"
	case VirtualMachineGuestOSIdentifierWinNetEnterprise64:
		return "winNetEnterprise64Guest"
	case VirtualMachineGuestOSIdentifierWinLonghorn:
		return "winLonghornGuest"
	case VirtualMachineGuestOSIdentifierWinLonghorn64:
		return "winLonghorn64Guest"
	case VirtualMachineGuestOSIdentifierWinNetDatacenter64:
		return "winNetDatacenter64Guest"
	case VirtualMachineGuestOSIdentifierWinVista:
		return "winVistaGuest"
	case VirtualMachineGuestOSIdentifierWinVista64:
		return "winVista64Guest"
	}
	return string(t)
}

// toVimTypeWindowsModern handles Windows 7 through Windows Server 2025.
func toVimTypeWindowsModern(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierWindows7:
		return "windows7Guest"
	case VirtualMachineGuestOSIdentifierWindows7x64:
		return "windows7_64Guest"
	case VirtualMachineGuestOSIdentifierWindows7Server64:
		return "windows7Server64Guest"
	case VirtualMachineGuestOSIdentifierWindows8:
		return "windows8Guest"
	case VirtualMachineGuestOSIdentifierWindows8x64:
		return "windows8_64Guest"
	case VirtualMachineGuestOSIdentifierWindows8Server64:
		return "windows8Server64Guest"
	case VirtualMachineGuestOSIdentifierWindows9:
		return "windows9Guest"
	case VirtualMachineGuestOSIdentifierWindows9x64:
		return "windows9_64Guest"
	case VirtualMachineGuestOSIdentifierWindows9Server64:
		return "windows9Server64Guest"
	case VirtualMachineGuestOSIdentifierWindows11x64:
		return "windows11_64Guest"
	case VirtualMachineGuestOSIdentifierWindows12x64:
		return "windows12_64Guest"
	case VirtualMachineGuestOSIdentifierWindowsHyperV:
		return "windowsHyperVGuest"
	case VirtualMachineGuestOSIdentifierWindows2019Serverx64:
		return "windows2019srv_64Guest"
	case VirtualMachineGuestOSIdentifierWindows2019ServerNextx64:
		return "windows2019srvNext_64Guest"
	case VirtualMachineGuestOSIdentifierWindows2022ServerNextx64:
		return "windows2022srvNext_64Guest"
	}
	return string(t)
}

func toVimTypeFreeBSD(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierFreeBSD:
		return "freebsdGuest"
	case VirtualMachineGuestOSIdentifierFreeBSD64:
		return "freebsd64Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD11:
		return "freebsd11Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD11x64:
		return "freebsd11_64Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD12:
		return "freebsd12Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD12x64:
		return "freebsd12_64Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD13:
		return "freebsd13Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD13x64:
		return "freebsd13_64Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD14:
		return "freebsd14Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD14x64:
		return "freebsd14_64Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD15:
		return "freebsd15Guest"
	case VirtualMachineGuestOSIdentifierFreeBSD15x64:
		return "freebsd15_64Guest"
	}
	return string(t)
}

// toVimTypeLinux dispatches to a distro-specific helper.
func toVimTypeLinux(t VirtualMachineGuestOSIdentifier) string {
	s := string(t)
	switch {
	case s == "RedHat",
		strings.HasPrefix(s, "RHEL"):
		return toVimTypeLinuxRHEL(t)
	case strings.HasPrefix(s, "CentOS"):
		return toVimTypeLinuxCentOS(t)
	case strings.HasPrefix(s, "OracleLinux"):
		return toVimTypeLinuxOracle(t)
	case strings.HasPrefix(s, "SUSE"),
		strings.HasPrefix(s, "SLES"),
		strings.HasPrefix(s, "OpenSUSE"):
		return toVimTypeLinuxSUSE(t)
	case strings.HasPrefix(s, "Debian"):
		return toVimTypeLinuxDebian(t)
	case strings.HasPrefix(s, "Asianux"),
		strings.HasPrefix(s, "MiracleLinux"),
		strings.HasPrefix(s, "Pardus"):
		return toVimTypeLinuxAsianux(t)
	case strings.HasPrefix(s, "Other"),
		s == "GenericLinux":
		return toVimTypeLinuxOtherKernel(t)
	default:
		return toVimTypeLinuxMisc(t)
	}
}

func toVimTypeLinuxRHEL(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierRedHat:
		return "redhatGuest"
	case VirtualMachineGuestOSIdentifierRHEL2:
		return "rhel2Guest"
	case VirtualMachineGuestOSIdentifierRHEL3:
		return "rhel3Guest"
	case VirtualMachineGuestOSIdentifierRHEL3x64:
		return "rhel3_64Guest"
	case VirtualMachineGuestOSIdentifierRHEL4:
		return "rhel4Guest"
	case VirtualMachineGuestOSIdentifierRHEL4x64:
		return "rhel4_64Guest"
	case VirtualMachineGuestOSIdentifierRHEL5:
		return "rhel5Guest"
	case VirtualMachineGuestOSIdentifierRHEL5x64:
		return "rhel5_64Guest"
	case VirtualMachineGuestOSIdentifierRHEL6:
		return "rhel6Guest"
	case VirtualMachineGuestOSIdentifierRHEL6x64:
		return "rhel6_64Guest"
	case VirtualMachineGuestOSIdentifierRHEL7:
		return "rhel7Guest"
	case VirtualMachineGuestOSIdentifierRHEL7x64:
		return "rhel7_64Guest"
	case VirtualMachineGuestOSIdentifierRHEL8x64:
		return "rhel8_64Guest"
	case VirtualMachineGuestOSIdentifierRHEL9x64:
		return "rhel9_64Guest"
	case VirtualMachineGuestOSIdentifierRHEL10x64:
		return "rhel10_64Guest"
	}
	return string(t)
}

func toVimTypeLinuxCentOS(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierCentOS:
		return "centosGuest"
	case VirtualMachineGuestOSIdentifierCentOS64:
		return "centos64Guest"
	case VirtualMachineGuestOSIdentifierCentOS6:
		return "centos6Guest"
	case VirtualMachineGuestOSIdentifierCentOS6x64:
		return "centos6_64Guest"
	case VirtualMachineGuestOSIdentifierCentOS7:
		return "centos7Guest"
	case VirtualMachineGuestOSIdentifierCentOS7x64:
		return "centos7_64Guest"
	case VirtualMachineGuestOSIdentifierCentOS8x64:
		return "centos8_64Guest"
	case VirtualMachineGuestOSIdentifierCentOS9x64:
		return "centos9_64Guest"
	}
	return string(t)
}

func toVimTypeLinuxOracle(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierOracleLinux:
		return "oracleLinuxGuest"
	case VirtualMachineGuestOSIdentifierOracleLinux64:
		return "oracleLinux64Guest"
	case VirtualMachineGuestOSIdentifierOracleLinux6:
		return "oracleLinux6Guest"
	case VirtualMachineGuestOSIdentifierOracleLinux6x64:
		return "oracleLinux6_64Guest"
	case VirtualMachineGuestOSIdentifierOracleLinux7:
		return "oracleLinux7Guest"
	case VirtualMachineGuestOSIdentifierOracleLinux7x64:
		return "oracleLinux7_64Guest"
	case VirtualMachineGuestOSIdentifierOracleLinux8x64:
		return "oracleLinux8_64Guest"
	case VirtualMachineGuestOSIdentifierOracleLinux9x64:
		return "oracleLinux9_64Guest"
	case VirtualMachineGuestOSIdentifierOracleLinux10x64:
		return "oracleLinux10_64Guest"
	}
	return string(t)
}

// toVimTypeLinuxSUSE handles SUSE, SLES, and OpenSUSE.
func toVimTypeLinuxSUSE(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierSUSE:
		return "suseGuest"
	case VirtualMachineGuestOSIdentifierSUSE64:
		return "suse64Guest"
	case VirtualMachineGuestOSIdentifierSLES:
		return "slesGuest"
	case VirtualMachineGuestOSIdentifierSLES64:
		return "sles64Guest"
	case VirtualMachineGuestOSIdentifierSLES10:
		return "sles10Guest"
	case VirtualMachineGuestOSIdentifierSLES10x64:
		return "sles10_64Guest"
	case VirtualMachineGuestOSIdentifierSLES11:
		return "sles11Guest"
	case VirtualMachineGuestOSIdentifierSLES11x64:
		return "sles11_64Guest"
	case VirtualMachineGuestOSIdentifierSLES12:
		return "sles12Guest"
	case VirtualMachineGuestOSIdentifierSLES12x64:
		return "sles12_64Guest"
	case VirtualMachineGuestOSIdentifierSLES15x64:
		return "sles15_64Guest"
	case VirtualMachineGuestOSIdentifierSLES16x64:
		return "sles16_64Guest"
	case VirtualMachineGuestOSIdentifierOpenSUSE:
		return "opensuseGuest"
	case VirtualMachineGuestOSIdentifierOpenSUSE64:
		return "opensuse64Guest"
	}
	return string(t)
}

func toVimTypeLinuxDebian(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierDebian4:
		return "debian4Guest"
	case VirtualMachineGuestOSIdentifierDebian4x64:
		return "debian4_64Guest"
	case VirtualMachineGuestOSIdentifierDebian5:
		return "debian5Guest"
	case VirtualMachineGuestOSIdentifierDebian5x64:
		return "debian5_64Guest"
	case VirtualMachineGuestOSIdentifierDebian6:
		return "debian6Guest"
	case VirtualMachineGuestOSIdentifierDebian6x64:
		return "debian6_64Guest"
	case VirtualMachineGuestOSIdentifierDebian7:
		return "debian7Guest"
	case VirtualMachineGuestOSIdentifierDebian7x64:
		return "debian7_64Guest"
	case VirtualMachineGuestOSIdentifierDebian8:
		return "debian8Guest"
	case VirtualMachineGuestOSIdentifierDebian8x64:
		return "debian8_64Guest"
	case VirtualMachineGuestOSIdentifierDebian9:
		return "debian9Guest"
	case VirtualMachineGuestOSIdentifierDebian9x64:
		return "debian9_64Guest"
	case VirtualMachineGuestOSIdentifierDebian10:
		return "debian10Guest"
	case VirtualMachineGuestOSIdentifierDebian10x64:
		return "debian10_64Guest"
	case VirtualMachineGuestOSIdentifierDebian11:
		return "debian11Guest"
	case VirtualMachineGuestOSIdentifierDebian11x64:
		return "debian11_64Guest"
	case VirtualMachineGuestOSIdentifierDebian12:
		return "debian12Guest"
	case VirtualMachineGuestOSIdentifierDebian12x64:
		return "debian12_64Guest"
	case VirtualMachineGuestOSIdentifierDebian13:
		return "debian13Guest"
	case VirtualMachineGuestOSIdentifierDebian13x64:
		return "debian13_64Guest"
	}
	return string(t)
}

// toVimTypeLinuxAsianux handles Asianux, MIRACLE LINUX, and Pardus.
func toVimTypeLinuxAsianux(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierAsianux3:
		return "asianux3Guest"
	case VirtualMachineGuestOSIdentifierAsianux3x64:
		return "asianux3_64Guest"
	case VirtualMachineGuestOSIdentifierAsianux4:
		return "asianux4Guest"
	case VirtualMachineGuestOSIdentifierAsianux4x64:
		return "asianux4_64Guest"
	case VirtualMachineGuestOSIdentifierAsianux5x64:
		return "asianux5_64Guest"
	case VirtualMachineGuestOSIdentifierAsianux7x64:
		return "asianux7_64Guest"
	case VirtualMachineGuestOSIdentifierAsianux8x64:
		return "asianux8_64Guest"
	case VirtualMachineGuestOSIdentifierAsianux9x64:
		return "asianux9_64Guest"
	case VirtualMachineGuestOSIdentifierMiracleLinux64:
		return "miraclelinux_64Guest"
	case VirtualMachineGuestOSIdentifierPardus64:
		return "pardus_64Guest"
	}
	return string(t)
}

// toVimTypeLinuxOtherKernel handles generic/other Linux kernel variants.
func toVimTypeLinuxOtherKernel(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierOther24xLinux:
		return "other24xLinuxGuest"
	case VirtualMachineGuestOSIdentifierOther26xLinux:
		return "other26xLinuxGuest"
	case VirtualMachineGuestOSIdentifierOtherLinux:
		return "otherLinuxGuest"
	case VirtualMachineGuestOSIdentifierOther3xLinux:
		return "other3xLinuxGuest"
	case VirtualMachineGuestOSIdentifierOther4xLinux:
		return "other4xLinuxGuest"
	case VirtualMachineGuestOSIdentifierOther5xLinux:
		return "other5xLinuxGuest"
	case VirtualMachineGuestOSIdentifierOther6xLinux:
		return "other6xLinuxGuest"
	case VirtualMachineGuestOSIdentifierOther7xLinux:
		return "other7xLinuxGuest"
	case VirtualMachineGuestOSIdentifierGenericLinux:
		return "genericLinuxGuest"
	case VirtualMachineGuestOSIdentifierOther24xLinux64:
		return "other24xLinux64Guest"
	case VirtualMachineGuestOSIdentifierOther26xLinux64:
		return "other26xLinux64Guest"
	case VirtualMachineGuestOSIdentifierOther3xLinux64:
		return "other3xLinux64Guest"
	case VirtualMachineGuestOSIdentifierOther4xLinux64:
		return "other4xLinux64Guest"
	case VirtualMachineGuestOSIdentifierOther5xLinux64:
		return "other5xLinux64Guest"
	case VirtualMachineGuestOSIdentifierOther6xLinux64:
		return "other6xLinux64Guest"
	case VirtualMachineGuestOSIdentifierOther7xLinux64:
		return "other7xLinux64Guest"
	case VirtualMachineGuestOSIdentifierOtherLinux64:
		return "otherLinux64Guest"
	}
	return string(t)
}

// toVimTypeLinuxMisc handles the remaining Linux distributions.
func toVimTypeLinuxMisc(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	// Novell
	case VirtualMachineGuestOSIdentifierNLD9:
		return "nld9Guest"
	case VirtualMachineGuestOSIdentifierOES:
		return "oesGuest"
	case VirtualMachineGuestOSIdentifierSJDS:
		return "sjdsGuest"
	// Mandriva / Mandrake
	case VirtualMachineGuestOSIdentifierMandrake:
		return "mandrakeGuest"
	case VirtualMachineGuestOSIdentifierMandriva:
		return "mandrivaGuest"
	case VirtualMachineGuestOSIdentifierMandriva64:
		return "mandriva64Guest"
	// TurboLinux
	case VirtualMachineGuestOSIdentifierTurboLinux:
		return "turboLinuxGuest"
	case VirtualMachineGuestOSIdentifierTurboLinux64:
		return "turboLinux64Guest"
	// Ubuntu
	case VirtualMachineGuestOSIdentifierUbuntu:
		return "ubuntuGuest"
	case VirtualMachineGuestOSIdentifierUbuntu64:
		return "ubuntu64Guest"
	// Fedora
	case VirtualMachineGuestOSIdentifierFedora:
		return "fedoraGuest"
	case VirtualMachineGuestOSIdentifierFedora64:
		return "fedora64Guest"
	// CoreOS / Photon
	case VirtualMachineGuestOSIdentifierCoreOS64:
		return "coreos64Guest"
	case VirtualMachineGuestOSIdentifierVMwarePhoton64:
		return "vmwarePhoton64Guest"
	// Chinese Linux distributions
	case VirtualMachineGuestOSIdentifierFusionOS64:
		return "fusionos_64Guest"
	case VirtualMachineGuestOSIdentifierProLinux64:
		return "prolinux_64Guest"
	case VirtualMachineGuestOSIdentifierKylinLinux64:
		return "kylinlinux_64Guest"
	// Amazon Linux
	case VirtualMachineGuestOSIdentifierAmazonLinux2x64:
		return "amazonlinux2_64Guest"
	case VirtualMachineGuestOSIdentifierAmazonLinux3x64:
		return "amazonlinux3_64Guest"
	// Rocky Linux / AlmaLinux
	case VirtualMachineGuestOSIdentifierRockyLinux64:
		return "rockylinux_64Guest"
	case VirtualMachineGuestOSIdentifierAlmaLinux64:
		return "almalinux_64Guest"
	}
	return string(t)
}

func toVimTypeSolaris(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierSolaris6:
		return "solaris6Guest"
	case VirtualMachineGuestOSIdentifierSolaris7:
		return "solaris7Guest"
	case VirtualMachineGuestOSIdentifierSolaris8:
		return "solaris8Guest"
	case VirtualMachineGuestOSIdentifierSolaris9:
		return "solaris9Guest"
	case VirtualMachineGuestOSIdentifierSolaris10:
		return "solaris10Guest"
	case VirtualMachineGuestOSIdentifierSolaris10x64:
		return "solaris10_64Guest"
	case VirtualMachineGuestOSIdentifierSolaris11x64:
		return "solaris11_64Guest"
	}
	return string(t)
}

func toVimTypeDarwin(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierDarwin:
		return "darwinGuest"
	case VirtualMachineGuestOSIdentifierDarwin64:
		return "darwin64Guest"
	case VirtualMachineGuestOSIdentifierDarwin10:
		return "darwin10Guest"
	case VirtualMachineGuestOSIdentifierDarwin10x64:
		return "darwin10_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin11:
		return "darwin11Guest"
	case VirtualMachineGuestOSIdentifierDarwin11x64:
		return "darwin11_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin12x64:
		return "darwin12_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin13x64:
		return "darwin13_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin14x64:
		return "darwin14_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin15x64:
		return "darwin15_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin16x64:
		return "darwin16_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin17x64:
		return "darwin17_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin18x64:
		return "darwin18_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin19x64:
		return "darwin19_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin20x64:
		return "darwin20_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin21x64:
		return "darwin21_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin22x64:
		return "darwin22_64Guest"
	case VirtualMachineGuestOSIdentifierDarwin23x64:
		return "darwin23_64Guest"
	}
	return string(t)
}

func toVimTypeVMkernel(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierVMkernel:
		return "vmkernelGuest"
	case VirtualMachineGuestOSIdentifierVMkernel5:
		return "vmkernel5Guest"
	case VirtualMachineGuestOSIdentifierVMkernel6:
		return "vmkernel6Guest"
	case VirtualMachineGuestOSIdentifierVMkernel65:
		return "vmkernel65Guest"
	case VirtualMachineGuestOSIdentifierVMkernel7:
		return "vmkernel7Guest"
	case VirtualMachineGuestOSIdentifierVMkernel8:
		return "vmkernel8Guest"
	case VirtualMachineGuestOSIdentifierVMkernel9:
		return "vmkernel9Guest"
	}
	return string(t)
}

func toVimTypeNetWare(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierNetWare4:
		return "netware4Guest"
	case VirtualMachineGuestOSIdentifierNetWare5:
		return "netware5Guest"
	case VirtualMachineGuestOSIdentifierNetWare6:
		return "netware6Guest"
	}
	return string(t)
}

func toVimTypeSCO(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierOpenServer5:
		return "openServer5Guest"
	case VirtualMachineGuestOSIdentifierOpenServer6:
		return "openServer6Guest"
	case VirtualMachineGuestOSIdentifierUnixWare7:
		return "unixWare7Guest"
	}
	return string(t)
}

func toVimTypeOS2(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierOS2:
		return "os2Guest"
	case VirtualMachineGuestOSIdentifierEComStation:
		return "eComStationGuest"
	case VirtualMachineGuestOSIdentifierEComStation2:
		return "eComStation2Guest"
	}
	return string(t)
}

func toVimTypeCRX(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierCRXPod1:
		return "crxPod1Guest"
	case VirtualMachineGuestOSIdentifierCRXSys1:
		return "crxSys1Guest"
	}
	return string(t)
}

func toVimTypeOtherOS(t VirtualMachineGuestOSIdentifier) string {
	switch t {
	case VirtualMachineGuestOSIdentifierOther:
		return "otherGuest"
	case VirtualMachineGuestOSIdentifierOther64:
		return "otherGuest64"
	}
	return string(t)
}

// FromVimType sets the VirtualMachineGuestOSIdentifier from the vSphere API
// identifier string.
func (t *VirtualMachineGuestOSIdentifier) FromVimType(s string) {
	switch {
	case s == "dosGuest",
		strings.HasPrefix(s, "win"):
		fromVimTypeWindows(t, s)
	case strings.HasPrefix(s, "freebsd"):
		fromVimTypeFreeBSD(t, s)
	case strings.HasPrefix(s, "solaris"):
		fromVimTypeSolaris(t, s)
	case strings.HasPrefix(s, "darwin"):
		fromVimTypeDarwin(t, s)
	case strings.HasPrefix(s, "vmkernel"):
		fromVimTypeVMkernel(t, s)
	case strings.HasPrefix(s, "netware"):
		fromVimTypeNetWare(t, s)
	case strings.HasPrefix(s, "openServer"),
		strings.HasPrefix(s, "unixWare"):
		fromVimTypeSCO(t, s)
	case s == "os2Guest",
		strings.HasPrefix(s, "eComStation"):
		fromVimTypeOS2(t, s)
	case strings.HasPrefix(s, "crx"):
		fromVimTypeCRX(t, s)
	case s == "otherGuest",
		s == "otherGuest64":
		fromVimTypeOtherOS(t, s)
	default:
		fromVimTypeLinux(t, s)
	}
}

// fromVimTypeWindows handles DOS and all Windows variants.
func fromVimTypeWindows(t *VirtualMachineGuestOSIdentifier, s string) {
	switch {
	case s == "dosGuest":
		*t = VirtualMachineGuestOSIdentifierDOS
	case strings.HasPrefix(s, "windows"):
		fromVimTypeWindowsModern(t, s)
	default:
		fromVimTypeWindowsLegacy(t, s)
	}
}

// fromVimTypeWindowsLegacy handles win3.1 through winVista.
func fromVimTypeWindowsLegacy(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "win31Guest":
		*t = VirtualMachineGuestOSIdentifierWin31
	case "win95Guest":
		*t = VirtualMachineGuestOSIdentifierWin95
	case "win98Guest":
		*t = VirtualMachineGuestOSIdentifierWin98
	case "winMeGuest":
		*t = VirtualMachineGuestOSIdentifierWinMe
	case "winNTGuest":
		*t = VirtualMachineGuestOSIdentifierWinNT
	case "win2000ProGuest":
		*t = VirtualMachineGuestOSIdentifierWin2000Pro
	case "win2000ServGuest":
		*t = VirtualMachineGuestOSIdentifierWin2000Serv
	case "win2000AdvServGuest":
		*t = VirtualMachineGuestOSIdentifierWin2000AdvServ
	case "winXPHomeGuest":
		*t = VirtualMachineGuestOSIdentifierWinXPHome
	case "winXPProGuest":
		*t = VirtualMachineGuestOSIdentifierWinXPPro
	case "winXPPro64Guest":
		*t = VirtualMachineGuestOSIdentifierWinXPPro64
	case "winNetWebGuest":
		*t = VirtualMachineGuestOSIdentifierWinNetWeb
	case "winNetStandardGuest":
		*t = VirtualMachineGuestOSIdentifierWinNetStandard
	case "winNetEnterpriseGuest":
		*t = VirtualMachineGuestOSIdentifierWinNetEnterprise
	case "winNetDatacenterGuest":
		*t = VirtualMachineGuestOSIdentifierWinNetDatacenter
	case "winNetBusinessGuest":
		*t = VirtualMachineGuestOSIdentifierWinNetBusiness
	case "winNetStandard64Guest":
		*t = VirtualMachineGuestOSIdentifierWinNetStandard64
	case "winNetEnterprise64Guest":
		*t = VirtualMachineGuestOSIdentifierWinNetEnterprise64
	case "winLonghornGuest":
		*t = VirtualMachineGuestOSIdentifierWinLonghorn
	case "winLonghorn64Guest":
		*t = VirtualMachineGuestOSIdentifierWinLonghorn64
	case "winNetDatacenter64Guest":
		*t = VirtualMachineGuestOSIdentifierWinNetDatacenter64
	case "winVistaGuest":
		*t = VirtualMachineGuestOSIdentifierWinVista
	case "winVista64Guest":
		*t = VirtualMachineGuestOSIdentifierWinVista64
	}
}

// fromVimTypeWindowsModern handles windows7 through windows2022.
func fromVimTypeWindowsModern(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "windows7Guest":
		*t = VirtualMachineGuestOSIdentifierWindows7
	case "windows7_64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows7x64
	case "windows7Server64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows7Server64
	case "windows8Guest":
		*t = VirtualMachineGuestOSIdentifierWindows8
	case "windows8_64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows8x64
	case "windows8Server64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows8Server64
	case "windows9Guest":
		*t = VirtualMachineGuestOSIdentifierWindows9
	case "windows9_64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows9x64
	case "windows9Server64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows9Server64
	case "windows11_64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows11x64
	case "windows12_64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows12x64
	case "windowsHyperVGuest":
		*t = VirtualMachineGuestOSIdentifierWindowsHyperV
	case "windows2019srv_64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows2019Serverx64
	case "windows2019srvNext_64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows2019ServerNextx64
	case "windows2022srvNext_64Guest":
		*t = VirtualMachineGuestOSIdentifierWindows2022ServerNextx64
	}
}

func fromVimTypeFreeBSD(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "freebsdGuest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD
	case "freebsd64Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD64
	case "freebsd11Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD11
	case "freebsd11_64Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD11x64
	case "freebsd12Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD12
	case "freebsd12_64Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD12x64
	case "freebsd13Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD13
	case "freebsd13_64Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD13x64
	case "freebsd14Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD14
	case "freebsd14_64Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD14x64
	case "freebsd15Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD15
	case "freebsd15_64Guest":
		*t = VirtualMachineGuestOSIdentifierFreeBSD15x64
	}
}

// fromVimTypeLinux dispatches to a distro-specific helper.
func fromVimTypeLinux(t *VirtualMachineGuestOSIdentifier, s string) {
	switch {
	case s == "redhatGuest",
		strings.HasPrefix(s, "rhel"):
		fromVimTypeLinuxRHEL(t, s)
	case strings.HasPrefix(s, "centos"):
		fromVimTypeLinuxCentOS(t, s)
	case strings.HasPrefix(s, "oracle"):
		fromVimTypeLinuxOracle(t, s)
	case strings.HasPrefix(s, "suse"),
		strings.HasPrefix(s, "sles"),
		strings.HasPrefix(s, "opensuse"):
		fromVimTypeLinuxSUSE(t, s)
	case strings.HasPrefix(s, "debian"):
		fromVimTypeLinuxDebian(t, s)
	case strings.HasPrefix(s, "asianux"),
		strings.HasPrefix(s, "miraclelinux"),
		strings.HasPrefix(s, "pardus"):
		fromVimTypeLinuxAsianux(t, s)
	case strings.HasPrefix(s, "other"),
		s == "genericLinuxGuest":
		fromVimTypeLinuxOtherKernel(t, s)
	default:
		fromVimTypeLinuxMisc(t, s)
	}
}

func fromVimTypeLinuxRHEL(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "redhatGuest":
		*t = VirtualMachineGuestOSIdentifierRedHat
	case "rhel2Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL2
	case "rhel3Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL3
	case "rhel3_64Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL3x64
	case "rhel4Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL4
	case "rhel4_64Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL4x64
	case "rhel5Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL5
	case "rhel5_64Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL5x64
	case "rhel6Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL6
	case "rhel6_64Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL6x64
	case "rhel7Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL7
	case "rhel7_64Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL7x64
	case "rhel8_64Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL8x64
	case "rhel9_64Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL9x64
	case "rhel10_64Guest":
		*t = VirtualMachineGuestOSIdentifierRHEL10x64
	}
}

func fromVimTypeLinuxCentOS(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "centosGuest":
		*t = VirtualMachineGuestOSIdentifierCentOS
	case "centos64Guest":
		*t = VirtualMachineGuestOSIdentifierCentOS64
	case "centos6Guest":
		*t = VirtualMachineGuestOSIdentifierCentOS6
	case "centos6_64Guest":
		*t = VirtualMachineGuestOSIdentifierCentOS6x64
	case "centos7Guest":
		*t = VirtualMachineGuestOSIdentifierCentOS7
	case "centos7_64Guest":
		*t = VirtualMachineGuestOSIdentifierCentOS7x64
	case "centos8_64Guest":
		*t = VirtualMachineGuestOSIdentifierCentOS8x64
	case "centos9_64Guest":
		*t = VirtualMachineGuestOSIdentifierCentOS9x64
	}
}

func fromVimTypeLinuxOracle(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "oracleLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux
	case "oracleLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux64
	case "oracleLinux6Guest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux6
	case "oracleLinux6_64Guest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux6x64
	case "oracleLinux7Guest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux7
	case "oracleLinux7_64Guest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux7x64
	case "oracleLinux8_64Guest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux8x64
	case "oracleLinux9_64Guest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux9x64
	case "oracleLinux10_64Guest":
		*t = VirtualMachineGuestOSIdentifierOracleLinux10x64
	}
}

// fromVimTypeLinuxSUSE handles suseGuest, sles*, and opensuse*.
func fromVimTypeLinuxSUSE(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "suseGuest":
		*t = VirtualMachineGuestOSIdentifierSUSE
	case "suse64Guest":
		*t = VirtualMachineGuestOSIdentifierSUSE64
	case "slesGuest":
		*t = VirtualMachineGuestOSIdentifierSLES
	case "sles64Guest":
		*t = VirtualMachineGuestOSIdentifierSLES64
	case "sles10Guest":
		*t = VirtualMachineGuestOSIdentifierSLES10
	case "sles10_64Guest":
		*t = VirtualMachineGuestOSIdentifierSLES10x64
	case "sles11Guest":
		*t = VirtualMachineGuestOSIdentifierSLES11
	case "sles11_64Guest":
		*t = VirtualMachineGuestOSIdentifierSLES11x64
	case "sles12Guest":
		*t = VirtualMachineGuestOSIdentifierSLES12
	case "sles12_64Guest":
		*t = VirtualMachineGuestOSIdentifierSLES12x64
	case "sles15_64Guest":
		*t = VirtualMachineGuestOSIdentifierSLES15x64
	case "sles16_64Guest":
		*t = VirtualMachineGuestOSIdentifierSLES16x64
	case "opensuseGuest":
		*t = VirtualMachineGuestOSIdentifierOpenSUSE
	case "opensuse64Guest":
		*t = VirtualMachineGuestOSIdentifierOpenSUSE64
	}
}

func fromVimTypeLinuxDebian(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "debian4Guest":
		*t = VirtualMachineGuestOSIdentifierDebian4
	case "debian4_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian4x64
	case "debian5Guest":
		*t = VirtualMachineGuestOSIdentifierDebian5
	case "debian5_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian5x64
	case "debian6Guest":
		*t = VirtualMachineGuestOSIdentifierDebian6
	case "debian6_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian6x64
	case "debian7Guest":
		*t = VirtualMachineGuestOSIdentifierDebian7
	case "debian7_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian7x64
	case "debian8Guest":
		*t = VirtualMachineGuestOSIdentifierDebian8
	case "debian8_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian8x64
	case "debian9Guest":
		*t = VirtualMachineGuestOSIdentifierDebian9
	case "debian9_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian9x64
	case "debian10Guest":
		*t = VirtualMachineGuestOSIdentifierDebian10
	case "debian10_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian10x64
	case "debian11Guest":
		*t = VirtualMachineGuestOSIdentifierDebian11
	case "debian11_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian11x64
	case "debian12Guest":
		*t = VirtualMachineGuestOSIdentifierDebian12
	case "debian12_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian12x64
	case "debian13Guest":
		*t = VirtualMachineGuestOSIdentifierDebian13
	case "debian13_64Guest":
		*t = VirtualMachineGuestOSIdentifierDebian13x64
	}
}

// fromVimTypeLinuxAsianux handles asianux*, miraclelinux*, and pardus*.
func fromVimTypeLinuxAsianux(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "asianux3Guest":
		*t = VirtualMachineGuestOSIdentifierAsianux3
	case "asianux3_64Guest":
		*t = VirtualMachineGuestOSIdentifierAsianux3x64
	case "asianux4Guest":
		*t = VirtualMachineGuestOSIdentifierAsianux4
	case "asianux4_64Guest":
		*t = VirtualMachineGuestOSIdentifierAsianux4x64
	case "asianux5_64Guest":
		*t = VirtualMachineGuestOSIdentifierAsianux5x64
	case "asianux7_64Guest":
		*t = VirtualMachineGuestOSIdentifierAsianux7x64
	case "asianux8_64Guest":
		*t = VirtualMachineGuestOSIdentifierAsianux8x64
	case "asianux9_64Guest":
		*t = VirtualMachineGuestOSIdentifierAsianux9x64
	case "miraclelinux_64Guest":
		*t = VirtualMachineGuestOSIdentifierMiracleLinux64
	case "pardus_64Guest":
		*t = VirtualMachineGuestOSIdentifierPardus64
	}
}

// fromVimTypeLinuxOtherKernel handles generic/other Linux kernel variants.
func fromVimTypeLinuxOtherKernel(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "other24xLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOther24xLinux
	case "other26xLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOther26xLinux
	case "otherLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOtherLinux
	case "other3xLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOther3xLinux
	case "other4xLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOther4xLinux
	case "other5xLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOther5xLinux
	case "other6xLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOther6xLinux
	case "other7xLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierOther7xLinux
	case "genericLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierGenericLinux
	case "other24xLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOther24xLinux64
	case "other26xLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOther26xLinux64
	case "other3xLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOther3xLinux64
	case "other4xLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOther4xLinux64
	case "other5xLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOther5xLinux64
	case "other6xLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOther6xLinux64
	case "other7xLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOther7xLinux64
	case "otherLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierOtherLinux64
	}
}

// fromVimTypeLinuxMisc handles the remaining Linux distributions.
func fromVimTypeLinuxMisc(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	// Novell
	case "nld9Guest":
		*t = VirtualMachineGuestOSIdentifierNLD9
	case "oesGuest":
		*t = VirtualMachineGuestOSIdentifierOES
	case "sjdsGuest":
		*t = VirtualMachineGuestOSIdentifierSJDS
	// Mandriva / Mandrake
	case "mandrakeGuest":
		*t = VirtualMachineGuestOSIdentifierMandrake
	case "mandrivaGuest":
		*t = VirtualMachineGuestOSIdentifierMandriva
	case "mandriva64Guest":
		*t = VirtualMachineGuestOSIdentifierMandriva64
	// TurboLinux
	case "turboLinuxGuest":
		*t = VirtualMachineGuestOSIdentifierTurboLinux
	case "turboLinux64Guest":
		*t = VirtualMachineGuestOSIdentifierTurboLinux64
	// Ubuntu
	case "ubuntuGuest":
		*t = VirtualMachineGuestOSIdentifierUbuntu
	case "ubuntu64Guest":
		*t = VirtualMachineGuestOSIdentifierUbuntu64
	// Fedora
	case "fedoraGuest":
		*t = VirtualMachineGuestOSIdentifierFedora
	case "fedora64Guest":
		*t = VirtualMachineGuestOSIdentifierFedora64
	// CoreOS / Photon
	case "coreos64Guest":
		*t = VirtualMachineGuestOSIdentifierCoreOS64
	case "vmwarePhoton64Guest":
		*t = VirtualMachineGuestOSIdentifierVMwarePhoton64
	// Chinese Linux distributions
	case "fusionos_64Guest":
		*t = VirtualMachineGuestOSIdentifierFusionOS64
	case "prolinux_64Guest":
		*t = VirtualMachineGuestOSIdentifierProLinux64
	case "kylinlinux_64Guest":
		*t = VirtualMachineGuestOSIdentifierKylinLinux64
	// Amazon Linux
	case "amazonlinux2_64Guest":
		*t = VirtualMachineGuestOSIdentifierAmazonLinux2x64
	case "amazonlinux3_64Guest":
		*t = VirtualMachineGuestOSIdentifierAmazonLinux3x64
	// Rocky Linux / AlmaLinux
	case "rockylinux_64Guest":
		*t = VirtualMachineGuestOSIdentifierRockyLinux64
	case "almalinux_64Guest":
		*t = VirtualMachineGuestOSIdentifierAlmaLinux64
	}
}

func fromVimTypeSolaris(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "solaris6Guest":
		*t = VirtualMachineGuestOSIdentifierSolaris6
	case "solaris7Guest":
		*t = VirtualMachineGuestOSIdentifierSolaris7
	case "solaris8Guest":
		*t = VirtualMachineGuestOSIdentifierSolaris8
	case "solaris9Guest":
		*t = VirtualMachineGuestOSIdentifierSolaris9
	case "solaris10Guest":
		*t = VirtualMachineGuestOSIdentifierSolaris10
	case "solaris10_64Guest":
		*t = VirtualMachineGuestOSIdentifierSolaris10x64
	case "solaris11_64Guest":
		*t = VirtualMachineGuestOSIdentifierSolaris11x64
	}
}

func fromVimTypeDarwin(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "darwinGuest":
		*t = VirtualMachineGuestOSIdentifierDarwin
	case "darwin64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin64
	case "darwin10Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin10
	case "darwin10_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin10x64
	case "darwin11Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin11
	case "darwin11_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin11x64
	case "darwin12_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin12x64
	case "darwin13_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin13x64
	case "darwin14_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin14x64
	case "darwin15_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin15x64
	case "darwin16_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin16x64
	case "darwin17_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin17x64
	case "darwin18_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin18x64
	case "darwin19_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin19x64
	case "darwin20_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin20x64
	case "darwin21_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin21x64
	case "darwin22_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin22x64
	case "darwin23_64Guest":
		*t = VirtualMachineGuestOSIdentifierDarwin23x64
	}
}

func fromVimTypeVMkernel(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "vmkernelGuest":
		*t = VirtualMachineGuestOSIdentifierVMkernel
	case "vmkernel5Guest":
		*t = VirtualMachineGuestOSIdentifierVMkernel5
	case "vmkernel6Guest":
		*t = VirtualMachineGuestOSIdentifierVMkernel6
	case "vmkernel65Guest":
		*t = VirtualMachineGuestOSIdentifierVMkernel65
	case "vmkernel7Guest":
		*t = VirtualMachineGuestOSIdentifierVMkernel7
	case "vmkernel8Guest":
		*t = VirtualMachineGuestOSIdentifierVMkernel8
	case "vmkernel9Guest":
		*t = VirtualMachineGuestOSIdentifierVMkernel9
	}
}

func fromVimTypeNetWare(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "netware4Guest":
		*t = VirtualMachineGuestOSIdentifierNetWare4
	case "netware5Guest":
		*t = VirtualMachineGuestOSIdentifierNetWare5
	case "netware6Guest":
		*t = VirtualMachineGuestOSIdentifierNetWare6
	}
}

func fromVimTypeSCO(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "openServer5Guest":
		*t = VirtualMachineGuestOSIdentifierOpenServer5
	case "openServer6Guest":
		*t = VirtualMachineGuestOSIdentifierOpenServer6
	case "unixWare7Guest":
		*t = VirtualMachineGuestOSIdentifierUnixWare7
	}
}

func fromVimTypeOS2(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "os2Guest":
		*t = VirtualMachineGuestOSIdentifierOS2
	case "eComStationGuest":
		*t = VirtualMachineGuestOSIdentifierEComStation
	case "eComStation2Guest":
		*t = VirtualMachineGuestOSIdentifierEComStation2
	}
}

func fromVimTypeCRX(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "crxPod1Guest":
		*t = VirtualMachineGuestOSIdentifierCRXPod1
	case "crxSys1Guest":
		*t = VirtualMachineGuestOSIdentifierCRXSys1
	}
}

func fromVimTypeOtherOS(t *VirtualMachineGuestOSIdentifier, s string) {
	switch s {
	case "otherGuest":
		*t = VirtualMachineGuestOSIdentifierOther
	case "otherGuest64":
		*t = VirtualMachineGuestOSIdentifierOther64
	}
}
