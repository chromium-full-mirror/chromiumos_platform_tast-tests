// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/trunk/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/

// See src/go.chromium.org/tast-tests/cros/local/crostini/params.go for more documentation

import (
	"sort"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/imetestutil"
	"go.chromium.org/tast-tests/cros/local/chrome/devicemode"
	"go.chromium.org/tast-tests/cros/local/crostini"
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
	"audio_playback_configurations.go": {timeout: 10 * time.Minute},
	"basic.go":                         {},
	"command_cd.go":                    {},
	"command_ps.go":                    {},
	"command_vim.go":                   {},
	"copy_files_to_linux_files.go":     {},
	"crash_reporter.go":                {},
	"drag_drop.go":                     {},
	"files_app_watch.go":               {},
	"home_directory_create_file.go":    {},
	"home_directory_delete_file.go":    {},
	"home_directory_rename_file.go":    {},
	"icon_and_username.go":             {},
	"launch_terminal.go":               {},
	"nested_vm.go":                     {},
	"notify.go":                        {},
	"no_access_to_downloads.go":        {},
	"no_shared_folder.go":              {},
	"open_with_terminal.go":            {},
	"package_info.go":                  {},
	"package_install_uninstall.go":     {},
	"pulse_audio_basic.go":             {},
	"remove_cancel.go":                 {},
	"remove_ok.go":                     {},
	"resize_cancel.go":                 {},
	"resize_ok.go":                     {},
	"resize_restart.go":                {},
	"resize_space_constrained.go":      {},
	"restart.go":                       {},
	"restart_icon.go":                  {},
	"run_with_arc.go":                  {},
	"shared_font_files.go":             {},
	"share_downloads_add_files.go":     {},
	"share_downloads.go":               {},
	"share_files_cancel.go":            {},
	"share_files_manage.go":            {},
	"share_files_ok.go":                {},
	"share_files_restart.go":           {},
	"share_files_toast.go":             {},
	"share_folders.go":                 {},
	"share_folder_zip_file.go":         {},
	"share_invalid_paths.go":           {},
	"sshfs_mount.go":                   {},
	"sync_time.go":                     {},
	"task_manager.go":                  {},
	"uninstall_invalid_app.go":         {},
	"verify_app_x11.go":                {},
	"vmc_extra_disk.go":                {},
	"vmc_start.go":                     {},
	"webserver.go":                     {},
	"xattrs.go":                        {},
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

var perfTestsExtraData = map[string][]string{
	"vim_compile.go": {"vim.tar.gz"},
}

var mainlineExpensiveTests = map[string]time.Duration{
	"oom_event.go":                   10 * time.Minute,
	"app_gedit_install_uninstall.go": 12 * time.Minute,
}

func TestExpensiveParams(t *testing.T) {
	for filename, duration := range perfTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:       duration,
			IsNotMainline: true,
			UseFixture:    true,
			ExtraData:     perfTestsExtraData[filename],
		}})
		genparams.Ensure(t, filename, params)
	}

	for filename, duration := range mainlineExpensiveTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:    duration,
			UseFixture: true,
		}})
		genparams.Ensure(t, filename, params)
	}
}

var restartTests = map[string]time.Duration{
	"backup_restore.go":        10 * time.Minute,
	"fs_corruption.go":         10 * time.Minute,
	"resize_backup_restore.go": 15 * time.Minute,
	"snapshot.go":              6 * time.Minute,
}

func TestRestartParams(t *testing.T) {
	for filename, duration := range restartTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:    duration,
			UseFixture: true,
		}})
		genparams.Ensure(t, filename, params)
	}
}

// These tests do not include the container version in the full test name for
// buster. TODO(b/234390590): move these to appTests.
// Do not add new tests here.
var oldAppTests = []string{
	"app_android_studio.go",
	"app_eclipse.go",
	"app_emacs.go",
	"app_gedit.go",
	"app_gedit_filesharing.go",
	"app_gedit_unshare_folder.go",
	"app_vscode_from_file_manager.go",
	"app_vscode.go",
	"restart_app.go",
}

func TestOldAppTestParams(t *testing.T) {
	for _, filename := range oldAppTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				UseFixture:          true,
				DeviceMode:          devicemode.TabletMode,
				NoBusterInTestName:  true,
			},
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				UseFixture:          true,
				DeviceMode:          devicemode.ClamshellMode,
				NoBusterInTestName:  true,
			}})
		genparams.Ensure(t, filename, params)
	}
}

var appTests = []string{
	"app_audacity.go",
	"app_firefox.go",
	"app_libre_office.go",
}

func TestAppTestParams(t *testing.T) {
	for _, filename := range appTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				UseFixture:          true,
				DeviceMode:          devicemode.TabletMode,
			},
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				UseFixture:          true,
				DeviceMode:          devicemode.ClamshellMode,
			}})
		genparams.Ensure(t, filename, params)
	}
}

var appClamshellOnlyTests = []string{
	"app_maximize_restore_minimize_close.go",
}

func TestAppClamshellOnlyTestParams(t *testing.T) {
	for _, filename := range appClamshellOnlyTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				UseFixture:          true,
				DeviceMode:          devicemode.ClamshellMode,
			}})
		genparams.Ensure(t, filename, params)
	}
}

var appWithSnapshotTests = []string{
	"app_vscode_uninstall.go",
}

func TestAppWithSnapshotTestParams(t *testing.T) {
	for _, filename := range appWithSnapshotTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				UseFixture:          true,
				DeviceMode:          devicemode.TabletMode,
			},
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				UseFixture:          true,
				DeviceMode:          devicemode.ClamshellMode,
			},
		})
		genparams.Ensure(t, filename, params)
	}
}

var appIMELanguageTests = []string{
	"app_gedit_ime.go",
	"app_firefox_ime.go",
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
			DeviceMode:          devicemode.ClamshellMode,
			TestIME:             true,
			IMEName:             imeName,
			Val:                 "\"" + imeName + "\"",
		})
	}
	for _, filename := range appIMELanguageTests {
		params := crostini.MakeTestParamsFromList(t, imeParams)
		genparams.Ensure(t, filename, params)
	}
}

// TODO(b/272366776) move to clamshell only when the flag is enabled by default.
var appIMEFlagTests = []string{
	"app_firefox_emoji.go",
	"app_gedit_emoji.go",
	"app_vscode_emoji.go",
	"app_firefox_nonalphanumeric_input.go",
	"app_gedit_nonalphanumeric_input.go",
	"app_vscode_nonalphanumeric_input.go",
	"app_gedit_switch_ime.go",
}

func TestAppIMEFlagTestParams(t *testing.T) {
	for _, filename := range appIMEFlagTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				UseFixture:          true,
				DeviceMode:          devicemode.ClamshellMode,
				TestIME:             true,
				IMEName:             "",
			}})
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
			Timeout:           15 * time.Minute,
			ExtraData:         []string{"hello-world-amd64.tar", "hello-world-arm64.tar"},
			ExtraSoftwareDeps: []string{"vm_host"},
			UseLargeContainer: true,
			UseFixture:        true,
			NoBusterTest:      true,
		}})
		genparams.Ensure(t, filename, params)
	}
}
