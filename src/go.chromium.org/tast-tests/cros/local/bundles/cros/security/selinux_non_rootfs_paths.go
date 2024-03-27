// Copyright 2024 The ChromiumOS Authors
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
		Func:         SELinuxNonRootFSPaths,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that SELinux file labels are set correctly for stateful partition files",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		SoftwareDeps: []string{"selinux"},
		Attr:         []string{"group:mainline", "informational"},
	})
}

func SELinuxNonRootFSPaths(ctx context.Context, s *testing.State) {
	gpuDevices, err := selinux.GpuDevices()
	if err != nil {
		// Error instead of Fatal to continue testing other testcases.
		// We don't want to "hide" other failures since SELinuxFiles tests are mostly independent test cases.
		s.Error("Failed to enumerate gpu devices: ", err)
	}

	testArgs := []selinux.FileTestCase{
		{Path: "/proc/sys/kernel/sched_use_eevdf", Context: "proc_sched", IgnoreErrors: true},
		{Path: "/run/dbus", Context: "cros_run_dbus"},
		{Path: "/run/dbus/system_bus_socket", Context: "cros_system_bus_socket"},
		{Path: "/run/featured/active", Context: "cros_run_featured_active", Recursive: true},
		{Path: "/run/metrics", Context: "cros_run_metrics"},
		{Path: "/run/metrics/external", Context: "cros_run_metrics_external"},
		{Path: "/run/metrics/external/crash-reporter", Context: "cros_run_metrics_external_crash"},
		{Path: "/run/rsyslogd", Context: "cros_run_rsyslogd"},
		{Path: "/run/udev", Context: "cros_run_udev", Recursive: true, IgnoreErrors: true},
		{Path: "/sys", Context: "sysfs.*", Recursive: true, Filter: selinux.IgnorePathsRegex(append([]string{
			"/sys/bus/iio/devices", // Remove once iioservice is deployed everywhere.
			"/sys/class/drm",
			"/sys/devices/system/cpu",
			"/sys/firmware/efi/efivars",
			"/sys/fs/bpf",
			"/sys/fs/cgroup",
			"/sys/fs/pstore",
			"/sys/fs/selinux",
			"/sys/kernel/config",
			"/sys/kernel/debug",
			"/sys/kernel/security",
			// we don't have anything special of conntrack files than others. conntrack slab cache changes when connections established or closes, and may cause flakiness.
			"/sys/kernel/slab/nf_conntrack_.*",
			"/sys/kernel/tracing",
		}, gpuDevices...))},
		// efivars: This dir won't be found when running in "legacy"/bios mode, necessitating IgnoreErrors (this is what the CQ appears to test);
		// plus on boards running efi that don't set kernel CONFIG_EFIVAR_FS an empty efivars dir will exist labeled sysfs, so we ignore that path but not its contents.
		{Path: "/sys/firmware/efi/efivars", Context: "efivarfs", Recursive: true, Filter: selinux.IgnorePathButNotContents("/sys/firmware/efi/efivars"), IgnoreErrors: true},
		{Path: "/sys/fs/cgroup", Context: "cgroup", Recursive: true, Filter: selinux.IgnorePathButNotContents("/sys/fs/cgroup")},
		{Path: "/sys/fs/cgroup", Context: "tmpfs"},
		{Path: "/sys/fs/pstore", Context: "pstorefs"},
		{Path: "/sys/fs/selinux", Context: "selinuxfs", Recursive: true, Filter: selinux.IgnorePathButNotContents("/sys/fs/selinux/null")},
		{Path: "/sys/fs/selinux/null", Context: "null_device"},
		{Path: "/sys/kernel/config", Context: "configfs", IgnoreErrors: true},
		{Path: "/sys/kernel/debug", Context: "debugfs"},
		{Path: "/sys/kernel/debug/debugfs_tracing_on", Context: "debugfs_tracing", IgnoreErrors: true},
		{Path: "/sys/kernel/debug/tracing", Context: "debugfs_tracing_debug"},
		{Path: "/sys/kernel/debug/tracing/trace_marker", Context: "debugfs_trace_marker", IgnoreErrors: true},
		{Path: "/sys/kernel/debug/sync", Context: "debugfs_sync", IgnoreErrors: true},
		{Path: "/sys/kernel/debug/sync/info", Context: "debugfs_sync", IgnoreErrors: true},
		{Path: "/sys/kernel/security", Context: "securityfs"},
		{Path: "/sys/kernel/tracing", Context: "debugfs_tracing_debug"},
		{Path: "/sys/kernel/tracing/trace_marker", Context: "debugfs_trace_marker", IgnoreErrors: true},
		{Path: "/var", Context: "cros_var", Log: true},
		{Path: "/var/cache/dlc", Context: "cros_var_cache_dlcservice", Log: true},
		{Path: "/var/cache/modem-utilities", Context: "cros_var_cache_modem_utilities"},
		{Path: "/var/lib", Context: "cros_var_lib", Log: true},
		{Path: "/var/lib/dlcservice", Context: "cros_var_lib_dlcservice", Recursive: true},
		{Path: "/var/lib/metrics", Context: "cros_metrics_file", Recursive: true, Filter: selinux.IgnorePaths([]string{
			"/var/lib/metrics/uma-events",
			"/var/lib/metrics/uma-events.d",
		})},
		{Path: "/var/lib/metrics/uma-events", Context: "cros_metrics_uma_events_file"},
		{Path: "/var/lib/metrics/uma-events.d", Context: "cros_metrics_uma_events_file", Recursive: true},
		{Path: "/var/lib/power_manager", Context: "cros_var_lib_power_manager", Recursive: true},
		{Path: "/var/lib/shill", Context: "cros_var_lib_shill", Recursive: true},
		{Path: "/var/lib/update_engine", Context: "cros_var_lib_update_engine", Recursive: true},
		{Path: "/var/lib/devicesettings", Context: "cros_var_lib_devicesettings", Recursive: true},
		{Path: "/var/log", Context: "cros_var_log", Log: true},
		{Path: "/var/log/arc.log", Context: "cros_arc_log", Log: true},
		{Path: "/var/log/asan", Context: "cros_var_log_asan", Recursive: true, Log: true},
		{Path: "/var/log/boot.log", Context: "cros_boot_log", Log: true},
		{Path: "/var/log/messages", Context: "cros_syslog", Log: true},
		{Path: "/var/log/mount-encrypted.log", Context: "cros_var_log", IgnoreErrors: true, Log: true},
		{Path: "/var/log/net.log", Context: "cros_net_log", Log: true},
		{Path: "/var/log/secure", Context: "cros_secure_log", Log: true},
		{Path: "/var/log/tlsdate.log", Context: "cros_tlsdate_log", Log: true},
		{Path: "/var/spool/cron-lite", Context: "cros_periodic_scheduler_cache_t", Recursive: true, Log: true},
	}

	selinux.FilesTestInternal(ctx, s, testArgs)
}
