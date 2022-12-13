// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/trunk/src/platform/tast/tools/go.sh test -count=1 chromiumos/tast/local/bundles/cros/crostini/

// See src/chromiumos/tast/local/crostini/params.go for more documentation

import (
	"testing"
	"time"

	"chromiumos/tast/common/genparams"
	"chromiumos/tast/local/chrome/devicemode"
	"chromiumos/tast/local/crostini"
)

// Struct used to specify extra test options for standard tests with ManaTEE variants.
// By default the ManaTEE variants are not disabled but are **not** set as critical.
// If the timeout is not changed, we set it to the default value.
type testOptions struct {
	disabledOnManatee bool
	criticalOnManatee bool
	timeout           time.Duration
}

const DefaultStandardTimeout = 7 * time.Minute

// Map crostini tests by file and their extra test options (if any).
// For tests that are broken on manatee runs but not on non-manatee buster_stable builds
// please file bugs directly in the manatee-specific component: http://b/issues?q=componentid:988046
// For context on certain tests being disabled on manatee, see: http://b/221317548#comment9
var standardTests = map[string]testOptions{
	"audio_basic.go": testOptions{},
	// Audio playback configurations took about 6 minutes on model with echo reference
	"audio_playback_configurations.go": testOptions{timeout: 10 * time.Minute},
	"basic.go":                         testOptions{criticalOnManatee: true},
	"command_cd.go":                    testOptions{},
	"command_ps.go":                    testOptions{},
	"command_vim.go":                   testOptions{},
	"copy_files_to_linux_files.go":     testOptions{},
	"crash_reporter.go":                testOptions{},
	"drag_drop.go":                     testOptions{},
	"files_app_watch.go":               testOptions{},
	"home_directory_create_file.go":    testOptions{},
	"home_directory_delete_file.go":    testOptions{},
	"home_directory_rename_file.go":    testOptions{},
	"icon_and_username.go":             testOptions{},
	"launch_terminal.go":               testOptions{},
	"nested_vm.go":                     testOptions{disabledOnManatee: true},
	"notify.go":                        testOptions{},
	"no_access_to_downloads.go":        testOptions{},
	"no_shared_folder.go":              testOptions{},
	"open_with_terminal.go":            testOptions{},
	"package_info.go":                  testOptions{},
	"package_install_uninstall.go":     testOptions{},
	"pulse_audio_basic.go":             testOptions{},
	"remove_cancel.go":                 testOptions{},
	"remove_ok.go":                     testOptions{},
	"resize_cancel.go":                 testOptions{},
	"resize_ok.go":                     testOptions{disabledOnManatee: true},
	"resize_restart.go":                testOptions{disabledOnManatee: true},
	"resize_space_constrained.go":      testOptions{disabledOnManatee: true},
	"restart.go":                       testOptions{},
	"restart_icon.go":                  testOptions{},
	"run_with_arc.go":                  testOptions{disabledOnManatee: true},
	"shared_font_files.go":             testOptions{disabledOnManatee: true},
	"share_downloads_add_files.go":     testOptions{},
	"share_downloads.go":               testOptions{},
	"share_files_cancel.go":            testOptions{},
	"share_files_manage.go":            testOptions{},
	"share_files_ok.go":                testOptions{},
	"share_files_restart.go":           testOptions{},
	"share_files_toast.go":             testOptions{},
	"share_folders.go":                 testOptions{},
	"share_folder_zip_file.go":         testOptions{},
	"share_invalid_paths.go":           testOptions{},
	"sshfs_mount.go":                   testOptions{},
	"sync_time.go":                     testOptions{},
	"task_manager.go":                  testOptions{disabledOnManatee: true},
	"uninstall_invalid_app.go":         testOptions{},
	"verify_app_x11.go":                testOptions{},
	"vmc_extra_disk.go":                testOptions{disabledOnManatee: true},
	"vmc_start.go":                     testOptions{disabledOnManatee: true},
	"webserver.go":                     testOptions{},
	"xattrs.go":                        testOptions{},
}

func TestFixTestParams(t *testing.T) {
	for filename, options := range standardTests {
		customTimeout := options.timeout
		// Use the default timeout if we didn't specify a custom timeout
		if customTimeout == 0 {
			customTimeout = DefaultStandardTimeout
		}
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:           customTimeout,
			UseFixture:        true,
			TestManatee:       !options.disabledOnManatee,
			IsManateeCritical: options.criticalOnManatee,
			ExtraSoftwareDeps: []string{"vm_host"},
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
	"cpu_perf.go":      12 * time.Minute,
	"disk_io_perf.go":  60 * time.Minute,
	"input_latency.go": 10 * time.Minute,
	"mouse_perf.go":    7 * time.Minute,
	"network_perf.go":  10 * time.Minute,
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
	"snapshot.go":              3 * time.Minute,
}

func TestRestartParams(t *testing.T) {
	for filename, duration := range restartTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:    duration,
			Restart:    true,
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
				TakeSnapshot:        true,
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
				TakeSnapshot:        true,
				UseFixture:          true,
				DeviceMode:          devicemode.TabletMode,
			},
			{
				Timeout:             15 * time.Minute,
				StableHardwareDep:   "crostini.CrostiniAppStable",
				UnstableHardwareDep: "crostini.CrostiniAppUnstable",
				ExtraSoftwareDeps:   []string{"crostini_app"},
				UseLargeContainer:   true,
				TakeSnapshot:        true,
				UseFixture:          true,
				DeviceMode:          devicemode.ClamshellMode,
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
