// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

// To update test parameters after modifying this file, run inside cros_sdk:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/

// For more documentation, see:
// https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/crostini/params.go

import (
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/chrome/devicemode"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/vm"
)

// Struct used to specify extra test options for standard tests.  If the timeout
// is not changed, we set it to the default value.
type testOptions struct {
	extraAttr       []string
	timeout         time.Duration
	requiresARC     bool
	criticalStaging bool
	needsAloop      bool
	// Foundational tests that should run on all devices. These tests should be lightweight enough
	// to be stable enough to run on low performance devices and older versions of debian.
	foundation   bool
	testBaguette bool
}

const DefaultStandardTimeout = 7 * time.Minute
const DefaultAppTimeout = 15 * time.Minute

// Map crostini tests by file and their extra test options (if any).
var standardTests = map[string]testOptions{
	"app_gedit_install_uninstall.go": {foundation: true, timeout: 12 * time.Minute, testBaguette: true},
	"audio_basic.go":                 {foundation: true, testBaguette: true, extraAttr: []string{"informational"}},
	// Audio playback configurations took about 6 minutes on model with echo reference
	// TODO(b/377716353): audio streams never settle on baguette, low priority to debug
	"audio_playback_configurations.go": {timeout: 10 * time.Minute, testBaguette: false},
	// TODO(b/377716353): unable to find terminal after restore
	"backup_restore.go":                 {timeout: 10 * time.Minute},
	"basic.go":                          {foundation: true, testBaguette: true},
	"close_terminal_tabs_and_window.go": {foundation: true, testBaguette: true},
	"command_cd.go":                     {foundation: true},
	"command_ps.go":                     {foundation: true, testBaguette: true},
	"command_vim.go":                    {foundation: true, testBaguette: true},
	"copy_files_to_linux_files.go":      {},
	// TODO(b/377716353): crash_reporter PoC exists for baguette, but requires more filtering.
	"crash_reporter.go": {testBaguette: false},
	// TODO(b/377716353): drag_drop seems to fail on just the last test in baguette, skip for now and debug more later
	"drag_drop.go":       {timeout: 14 * time.Minute, foundation: true, testBaguette: false, extraAttr: []string{"informational"}},
	"files_app_watch.go": {testBaguette: true},
	// TODO(b/377716353): missing dbus signal, requires investigation
	"fs_corruption.go":        {timeout: 10 * time.Minute},
	"home_directory_share.go": {testBaguette: true},
	"icon_and_username.go":    {testBaguette: true},
	"launch_terminal.go":      {testBaguette: true},
	// TODO(b/377716353): failed to build program
	"nested_vm.go":              {},
	"no_access_to_downloads.go": {testBaguette: true},
	// TODO(b/377716353): needs shared fonts
	"no_shared_folder.go": {},
	"notify.go":           {testBaguette: false},
	// OomEvent test was for crostini data-collection, no need in baguette.
	"oom_event.go":             {timeout: 10 * time.Minute, testBaguette: false},
	"open_with_terminal.go":    {testBaguette: true},
	"pulse_audio_basic.go":     {testBaguette: true},
	"remove_cancel.go":         {foundation: true, testBaguette: true},
	"remove_ok.go":             {foundation: true, testBaguette: true},
	"resize_backup_restore.go": {timeout: 15 * time.Minute, testBaguette: true},
	"resize_cancel.go":         {testBaguette: true},
	// TODO(b/377716353): baguette resizing may not be working, or reporting incorrectly, more to debug.
	"resize_ok.go":                {},
	"resize_restart.go":           {},
	"resize_space_constrained.go": {},
	"restart.go":                  {testBaguette: true},
	"restart_icon.go":             {testBaguette: true},
	// TODO(b/377716353): ARC fails to boot
	"run_with_arc.go": {requiresARC: true},
	// TODO(b/377716353): needs shared fonts
	"shared_font_files.go":         {foundation: true},
	"share_downloads_add_files.go": {},
	"share_downloads.go":           {},
	"share_files_cancel.go":        {},
	"share_files_manage.go":        {},
	"share_files_ok.go":            {foundation: true},
	"share_files_restart.go":       {},
	"share_files_toast.go":         {},
	"share_folders.go":             {},
	"share_folder_zip_file.go":     {},
	"share_invalid_paths.go":       {},
	// Baguette has no lxc, and therefore no snapshots.
	"snapshot.go":               {timeout: 6 * time.Minute, testBaguette: false},
	"sshfs_mount.go":            {},
	"sync_time.go":              {},
	"task_manager.go":           {},
	"uninstall_invalid_app.go":  {},
	"usb_share_mass_storage.go": {},
	"verify_app_x11.go":         {},
	// Baguette will not support vmc extra disk mounting automatically.
	"vmc_extra_disk.go": {testBaguette: false},
	"vmc_start.go":      {},
	"webserver.go":      {foundation: true},
	"xattrs.go":         {},
}

func TestFixTestParams(t *testing.T) {
	for filename, options := range standardTests {
		customTimeout := options.timeout
		// Use the default timeout if we didn't specify a custom timeout
		if customTimeout == 0 {
			customTimeout = DefaultStandardTimeout
		}
		minimumContainerVersion := vm.DebianBookworm
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:                 customTimeout,
			ExtraAttr:               options.extraAttr,
			UseFixture:              true,
			OnlyStableBoards:        !options.foundation,
			LowPerfEligible:         options.foundation,
			MinimumContainerVersion: minimumContainerVersion,
			RequiresARC:             options.requiresARC,
			TestBaguette:            options.testBaguette,
		}})
		genparams.Ensure(t, filename, params)
	}
}

var perfTests = map[string]time.Duration{
	"cpu_perf.go":      15 * time.Minute,
	"disk_io_perf.go":  60 * time.Minute,
	"input_latency.go": 10 * time.Minute,
	"mouse_perf.go":    7 * time.Minute,
	"network_perf.go":  10 * time.Minute,
	"sshfs_perf.go":    10 * time.Minute,
	"startup_perf.go":  1 * time.Minute,
	"vim_compile.go":   20 * time.Minute,
}

func TestExpensiveParams(t *testing.T) {
	for filename, duration := range perfTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:                 duration,
			IsNotMainline:           true,
			UseFixture:              true,
			MinimumContainerVersion: vm.DebianBookworm,
		}})
		genparams.Ensure(t, filename, params)
	}
}

var appTests = map[string]testOptions{
	"app_android_studio.go":                {},
	"app_audacity.go":                      {},
	"app_audacity_terminal.go":             {},
	"app_eclipse.go":                       {},
	"app_emacs.go":                         {},
	"app_firefox_emoji.go":                 {},
	"app_firefox.go":                       {},
	"app_firefox_nonalphanumeric_input.go": {},
	"app_firefox_terminal.go":              {},
	"app_gedit_emoji.go":                   {},
	"app_gedit_filesharing.go":             {},
	"app_gedit.go":                         {foundation: true},
	"app_gedit_nonalphanumeric_input.go":   {},
	"app_gedit_unshare_folder.go":          {},
	"app_libre_office.go":                  {},
	"app_vlc.go":                           {needsAloop: true},
	"app_vscode_emoji.go":                  {},
	"app_vscode_from_file_manager.go":      {},
	"app_vscode.go":                        {foundation: true},
	"app_vscode_nonalphanumeric_input.go":  {},
	"app_vscode_uninstall.go":              {},
	"restart_app.go":                       {},
}

func TestAppTestParams(t *testing.T) {
	for filename, options := range appTests {
		timeout := options.timeout
		// Use the default timeout if we didn't specify a custom timeout
		if timeout == 0 {
			timeout = DefaultAppTimeout
		}
		minimumContainerVersion := vm.DebianBookworm
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Timeout:                 timeout,
				ExtraSoftwareDeps:       []string{"crostini_app"},
				UseLargeContainer:       true,
				UseFixture:              true,
				OnlyStableBoards:        true,
				DeviceMode:              devicemode.ClamshellMode,
				MinimumContainerVersion: minimumContainerVersion,
				CriticalStaging:         options.criticalStaging,
				NeedsAloop:              options.needsAloop,
			}})
		genparams.Ensure(t, filename, params)
	}
}

var appWindowOperationsTests = map[string]testOptions{
	"app_firefox_window_operations.go":  {},
	"app_audacity_window_operations.go": {},
	"app_emacs_window_operations.go":    {},
	"app_vscode_window_operations.go":   {},
}

func TestAppWindowOperationsTestParams(t *testing.T) {
	for filename, options := range appWindowOperationsTests {
		timeout := options.timeout
		// Use the default timeout if we didn't specify a custom timeout
		if timeout == 0 {
			timeout = DefaultAppTimeout
		}
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Name:                    "maximize",
				Val:                     "apps.WindowOperationMaximize",
				Timeout:                 timeout,
				ExtraSoftwareDeps:       []string{"crostini_app"},
				UseLargeContainer:       true,
				UseFixture:              true,
				OnlyStableBoards:        true,
				DeviceMode:              devicemode.ClamshellMode,
				MinimumContainerVersion: vm.DebianBookworm,
				CriticalStaging:         options.criticalStaging,
			},
			{
				Name:                    "minimize",
				Val:                     "apps.WindowOperationMinimize",
				Timeout:                 timeout,
				ExtraSoftwareDeps:       []string{"crostini_app"},
				UseLargeContainer:       true,
				UseFixture:              true,
				OnlyStableBoards:        true,
				DeviceMode:              devicemode.ClamshellMode,
				MinimumContainerVersion: vm.DebianBookworm,
				CriticalStaging:         options.criticalStaging,
			},
			{
				Name:                    "switch_tablet",
				Val:                     "apps.WindowOperationSwitchTablet",
				Timeout:                 timeout,
				ExtraSoftwareDeps:       []string{"crostini_app"},
				UseLargeContainer:       true,
				UseFixture:              true,
				OnlyStableBoards:        true,
				DeviceMode:              devicemode.ClamshellMode,
				MinimumContainerVersion: vm.DebianBookworm,
				CriticalStaging:         options.criticalStaging,
			},
		})
		genparams.Ensure(t, filename, params)
	}
}

var gaiaTests = map[string]testOptions{
	"no_access_to_drive.go": {},
	"share_drive.go":        {},
	"share_movies.go":       {requiresARC: true},
}

func TestGaiaTestParams(t *testing.T) {
	for filename, options := range gaiaTests {
		customTimeout := options.timeout
		// Use the default timeout if we didn't specify a custom timeout
		if customTimeout == 0 {
			customTimeout = DefaultStandardTimeout
		}
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:          customTimeout,
			UseGaiaLogin:     true,
			UseFixture:       true,
			OnlyStableBoards: true,
			RequiresARC:      options.requiresARC,
		}})
		genparams.Ensure(t, filename, params)
	}
}
