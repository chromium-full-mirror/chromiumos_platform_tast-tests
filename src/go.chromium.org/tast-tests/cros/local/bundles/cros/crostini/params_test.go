// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

// To update test parameters after modifying this file, run inside cros_sdk:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/

// For more documentation, see:
// https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/crostini/params.go

import (
	"sort"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/imetestutil"
	"go.chromium.org/tast-tests/cros/local/chrome/devicemode"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/vm"
)

// Struct used to specify extra test options for standard tests.  If the timeout
// is not changed, we set it to the default value.
type testOptions struct {
	timeout time.Duration
}

const DefaultStandardTimeout = 7 * time.Minute

// Map crostini tests by file and their extra test options (if any).
var standardTests = map[string]testOptions{
	"audio_basic.go": {},
	// Audio playback configurations took about 6 minutes on model with echo reference
	"audio_playback_configurations.go":  {timeout: 10 * time.Minute},
	"backup_restore.go":                 {timeout: 10 * time.Minute},
	"basic.go":                          {},
	"close_terminal_tabs_and_window.go": {},
	"command_cd.go":                     {},
	"command_ps.go":                     {},
	"command_vim.go":                    {},
	"copy_files_to_linux_files.go":      {},
	"crash_reporter.go":                 {},
	"drag_drop.go":                      {},
	"files_app_watch.go":                {},
	"fs_corruption.go":                  {timeout: 10 * time.Minute},
	"home_directory_share.go":           {},
	"icon_and_username.go":              {},
	"launch_terminal.go":                {},
	"nested_vm.go":                      {},
	"no_access_to_downloads.go":         {},
	"no_shared_folder.go":               {},
	"notify.go":                         {},
	"open_with_terminal.go":             {},
	"package_info.go":                   {},
	"package_install_uninstall.go":      {},
	"pulse_audio_basic.go":              {},
	"remove_cancel.go":                  {},
	"remove_ok.go":                      {},
	"resize_backup_restore.go":          {timeout: 15 * time.Minute},
	"resize_cancel.go":                  {},
	"resize_ok.go":                      {},
	"resize_restart.go":                 {},
	"resize_space_constrained.go":       {},
	"restart.go":                        {},
	"restart_icon.go":                   {},
	"run_with_arc.go":                   {},
	"shared_font_files.go":              {},
	"share_downloads_add_files.go":      {},
	"share_downloads.go":                {},
	"share_files_cancel.go":             {},
	"share_files_manage.go":             {},
	"share_files_ok.go":                 {},
	"share_files_restart.go":            {},
	"share_files_toast.go":              {},
	"share_folders.go":                  {},
	"share_folder_zip_file.go":          {},
	"share_invalid_paths.go":            {},
	"snapshot.go":                       {timeout: 6 * time.Minute},
	"sshfs_mount.go":                    {},
	"sync_time.go":                      {},
	"task_manager.go":                   {},
	"uninstall_invalid_app.go":          {},
	"verify_app_x11.go":                 {},
	"vmc_extra_disk.go":                 {},
	"vmc_start.go":                      {},
	"webserver.go":                      {},
	"xattrs.go":                         {},
}

func TestFixTestParams(t *testing.T) {
	for filename, options := range standardTests {
		customTimeout := options.timeout
		// Use the default timeout if we didn't specify a custom timeout
		if customTimeout == 0 {
			customTimeout = DefaultStandardTimeout
		}
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:    customTimeout,
			UseFixture: true,
		}})
		genparams.Ensure(t, filename, params)
	}
}

var lacrosTests = []string{
	"launch_browser.go",
	"verify_app_wayland.go",
}

func TestLacrosTestParams(t *testing.T) {
	for _, filename := range lacrosTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:    3 * time.Minute,
			UseFixture: true,
			TestLacros: true,
			Val:        "browser.TypeAsh",
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

var mainlineExpensiveTests = map[string]time.Duration{
	"oom_event.go":                   10 * time.Minute,
	"app_gedit_install_uninstall.go": 12 * time.Minute,
}

func TestExpensiveParams(t *testing.T) {
	for filename, duration := range perfTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:                 duration,
			IsNotMainline:           true,
			UseFixture:              true,
			MinimumContainerVersion: vm.DebianBullseye,
		}})
		genparams.Ensure(t, filename, params)
	}

	for filename, duration := range mainlineExpensiveTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:                 duration,
			UseFixture:              true,
			MinimumContainerVersion: vm.DebianBullseye,
		}})
		genparams.Ensure(t, filename, params)
	}
}

var appTests = []string{
	"app_android_studio.go",
	"app_audacity.go",
	"app_audacity_terminal.go",
	"app_eclipse.go",
	"app_emacs.go",
	"app_firefox.go",
	"app_firefox_terminal.go",
	"app_gedit_filesharing.go",
	"app_gedit.go",
	"app_gedit_unshare_folder.go",
	"app_libre_office.go",
	"app_vlc.go",
	"app_vscode_from_file_manager.go",
	"app_vscode.go",
	"app_vscode_uninstall.go",
	"restart_app.go",
}

func TestAppTestParams(t *testing.T) {
	for _, filename := range appTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Timeout:                 15 * time.Minute,
				StableHardwareDep:       "crostini.CrostiniAppStable",
				UnstableHardwareDep:     "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:       []string{"crostini_app"},
				UseLargeContainer:       true,
				UseFixture:              true,
				DeviceMode:              devicemode.TabletMode,
				MinimumContainerVersion: vm.DebianBullseye,
			},
			{
				Timeout:                 15 * time.Minute,
				StableHardwareDep:       "crostini.CrostiniAppStable",
				UnstableHardwareDep:     "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:       []string{"crostini_app"},
				UseLargeContainer:       true,
				UseFixture:              true,
				DeviceMode:              devicemode.ClamshellMode,
				MinimumContainerVersion: vm.DebianBullseye,
			}})
		genparams.Ensure(t, filename, params)
	}
}

var appClamshellOnlyTests = []string{
	"app_audacity_window_operations.go",
	"app_emacs_window_operations.go",
	"app_firefox_window_operations.go",
	"app_vscode_window_operations.go",
	// TODO(b/304160903,b/305579517): Removed from generation to promote bookworm tests.
	// "app_firefox_emoji.go",
	"app_gedit_emoji.go",
	"app_vscode_emoji.go",
	"app_firefox_nonalphanumeric_input.go",
	"app_gedit_nonalphanumeric_input.go",
	"app_vscode_nonalphanumeric_input.go",
	"app_gedit_switch_ime.go",
}

func TestAppClamshellOnlyTestParams(t *testing.T) {
	for _, filename := range appClamshellOnlyTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Timeout:                 15 * time.Minute,
				StableHardwareDep:       "crostini.CrostiniAppStable",
				UnstableHardwareDep:     "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:       []string{"crostini_app"},
				UseLargeContainer:       true,
				UseFixture:              true,
				DeviceMode:              devicemode.ClamshellMode,
				MinimumContainerVersion: vm.DebianBullseye,
			}})
		genparams.Ensure(t, filename, params)
	}
}

var appIMELanguageTests = []string{
	"app_gedit_ime.go",
	// TODO(b/304160903,b/305579517): Removed from generation to promote bookworm tests.
	// "app_firefox_ime.go",
	"app_vscode_ime.go",
}

func TestAppIMELanguageTestParams(t *testing.T) {
	var imeParams []crostini.Param

	imeTestCases := make([]string, 0)
	for imeTestCase := range imetestutil.IMETestCases {
		imeTestCases = append(imeTestCases, imeTestCase)
	}
	sort.Strings(imeTestCases)

	for _, imeName := range imeTestCases {
		imeParams = append(imeParams, crostini.Param{
			Timeout:             15 * time.Minute,
			StableHardwareDep:   "crostini.CrostiniAppStable",
			UnstableHardwareDep: "crostini.CrostiniAppUnstable",
			ExtraSoftwareDeps:   []string{"crostini_app"},
			UseLargeContainer:   true,
			UseFixture:          true,
			IMEName:             imeName,
			Val:                 "\"" + imeName + "\"",
		})
	}
	for _, filename := range appIMELanguageTests {
		params := crostini.MakeTestParamsFromList(t, imeParams)
		genparams.Ensure(t, filename, params)
	}
}

var gaiaTests = []string{
	"no_access_to_drive.go",
	"share_drive.go",
	"share_movies.go",
}

func TestGaiaTestParams(t *testing.T) {
	for _, filename := range gaiaTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:      7 * time.Minute,
			UseGaiaLogin: true,
			UseFixture:   true,
		}})
		genparams.Ensure(t, filename, params)
	}
}

var containerTests = []string{
	"docker.go",
	"podman_root.go",
	"podman_user.go",
}

// The container managers are quite heavy in terms of install size, so they are
// installed in the "large" or "app test" container, even though they have low
// runtime performance requirements.
func TestContainerTestParams(t *testing.T) {
	for _, filename := range containerTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:                 15 * time.Minute,
			UseLargeContainer:       true,
			UseFixture:              true,
			MinimumContainerVersion: vm.DebianBullseye,
		}})
		genparams.Ensure(t, filename, params)
	}
}
