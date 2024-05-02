// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/security/selinux"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SELinuxRootFS,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that SELinux file labels are set correctly for rootfs files",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		SoftwareDeps: []string{"selinux"},
		Attr:         []string{"group:mainline"},
	})
}

func SELinuxRootFS(ctx context.Context, s *testing.State) {
	testArgs := []selinux.FileTestCase{
		{Path: "/bin", Context: "cros_coreutils_exec", Recursive: true, Filter: selinux.InvertFilterSkipFile(selinux.SkipCoreutilsFile)},
		{Path: "/bin/bash", Context: "sh_exec"},
		{Path: "/bin/dash", Context: "sh_exec"},
		{Path: "/bin/sh", Context: "sh_exec"},
		{Path: "/etc", Context: "cros_conf_file", Recursive: true, Filter: selinux.IgnorePaths([]string{
			"/etc/hosts.d", "/etc/localtime", "/etc/passwd", "/etc/group", "/etc/shadow", "/etc/selinux", "/etc/featured",
		})},
		{Path: "/etc/featured", Context: "cros_featured_file", Recursive: true},
		{Path: "/etc/group", Context: "cros_passwd_file"},
		{Path: "/etc/hosts.d", Recursive: true, Context: "cros_run_crosdns", IgnoreErrors: true},
		{Path: "/etc/localtime", Context: "cros_tz_data_file"},
		{Path: "/etc/passwd", Context: "cros_passwd_file"},
		{Path: "/etc/selinux", Context: "cros_selinux_config_file", Recursive: true},
		{Path: "/sbin/featured", Context: "cros_featured_exec"},
		{Path: "/usr/bin", Context: "cros_coreutils_exec", Recursive: true, Filter: selinux.InvertFilterSkipFile(selinux.SkipCoreutilsFile)},
		{Path: "/usr/bin/anomaly_detector", Context: "cros_anomaly_detector_exec"},
		{Path: "/usr/bin/dbus-uuidgen", Context: "cros_dbus_uuidgen_exec"},
		{Path: "/usr/bin/ionice", Context: "cros_ionice_exec"},
		{Path: "/usr/bin/kmod", Context: "cros_modprobe_exec"},
		{Path: "/usr/bin/logger", Context: "cros_logger_exec"},
		{Path: "/usr/bin/metrics_client", Context: "cros_metrics_client_exec"},
		{Path: "/usr/bin/metrics_daemon", Context: "cros_metrics_daemon_exec"},
		{Path: "/usr/bin/periodic_scheduler", Context: "cros_periodic_scheduler_exec"},
		{Path: "/usr/sbin/bootstat", Context: "cros_bootstat_exec"},
		{Path: "/usr/sbin/chapsd", Context: "cros_chapsd_exec"},
		{Path: "/usr/sbin/cryptohomed", Context: "cros_cryptohomed_exec"},
		{Path: "/usr/sbin/hpsd", Context: "cros_hpsd_exec", IgnoreErrors: true},
		{Path: "/usr/sbin/rsyslogd", Context: "cros_rsyslogd_exec"},
		{Path: "/usr/sbin/tcsd", Context: "cros_tcsd_exec", IgnoreErrors: true},
		{Path: "/usr/sbin/update_engine", Context: "cros_update_engine_exec"},
		{Path: "/usr/sbin/usbguard-daemon", Context: "cros_usbguard_exec", IgnoreErrors: true},
		{Path: "/usr/sbin/vtpmd", Context: "cros_vtpmd_exec", IgnoreErrors: true},
		{Path: "/usr/share/cros/init", Context: "cros_init_shell_scripts", Recursive: true, Filter: selinux.IgnorePathsButNotContents([]string{
			"/usr/share/cros/init/activate_date.sh",
			"/usr/share/cros/init/crx-import.sh",
			"/usr/share/cros/init/powerd-pre-start.sh",
			"/usr/share/cros/init/shill.sh",
			"/usr/share/cros/init/shill-pre-start.sh",
			"/usr/share/cros/init/temp_logger.sh",
			"/usr/share/cros/init/ui-pre-start",
			"/usr/share/cros/init/ui-respawn",
		})},
		{Path: "/usr/share/cros/init/activate_date.sh", Context: "cros_init_activate_date_script", IgnoreErrors: true},
		{Path: "/usr/share/cros/init/crx-import.sh", Context: "cros_init_crx_import_script"},
		{Path: "/usr/share/cros/init/powerd-pre-start.sh", Context: "cros_init_powerd_pre_start_script"},
		{Path: "/usr/share/cros/init/shill.sh", Context: "cros_init_shill_shell_script"},
		{Path: "/usr/share/cros/init/shill-pre-start.sh", Context: "cros_init_shill_shell_script"},
		{Path: "/usr/share/cros/init/temp_logger.sh", Context: "cros_init_temp_logger_shell_script"},
		{Path: "/usr/share/cros/init/ui-pre-start", Context: "cros_init_ui_pre_start_shell_script"},
		{Path: "/usr/share/cros/init/ui-respawn", Context: "cros_init_ui_respawn_shell_script"},
		{Path: "/usr/share/policy", Context: "cros_seccomp_policy_file", Recursive: true},
	}

	selinux.FilesTestInternal(ctx, s, testArgs)
}
