// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/dma"
	uiCommon "go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/swap"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/pvsched"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PreTestTimeout is the timeout duration before each test.
const PreTestTimeout = 120 * time.Second

// PostTestTimeout is the timeout duration to save logs after each test.
// It's intentionally set longer than ResetTimeout because dumping 'dumpsys' takes around 20 seconds.
const PostTestTimeout = ResetTimeout + 20*time.Second

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "arcBooted",
		Desc: "ARC is booted",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:883059", // ChromeOS > Software > ARC++ > ARCVM
		Impl:            NewArcBootedFixture(DefaultBootedFixtureConfig()),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
		Params: []testing.FixtureParam{
			{
				Val: arcBootedFixtureArgs{chrome.FieldTrialConfigDefault},
			},
			{
				Name: "fieldtrial_testing_config_off",
				Val:  arcBootedFixtureArgs{chrome.FieldTrialConfigDisable},
			},
			{
				Name: "fieldtrial_testing_config_on",
				Val:  arcBootedFixtureArgs{chrome.FieldTrialConfigEnable},
			},
			{
				// The fixture using Ethernet-hide (platform2/ethernet-hide). This will
				// boot ARC without Internet connection.
				Name:          "ehide",
				Parent:        "ehide",
				ExtraContacts: []string{"chenzikai@google.com", "cros-networking@google.com"},
			},
		},
	})

	// arcBootedWithAllowAdbRoot is a fixture similar to arcBooted, except that
	// ARC is booted with a param to allow adb root.
	fixtureConfig := DefaultBootedFixtureConfig()
	fixtureConfig.ArcvmConfig = func(context.Context) (string, error) {
		return "--params=androidboot.arc.allow_adb_root=1", nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithAllowAdbRoot",
		Desc: "ARC is booted with a param to allow adb root",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:883059", // ChromeOS > Software > ARC++ > ARCVM
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithoutUIAutomator is a fixture similar to arcBooted, except
	// that UI Automator is not enabled.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.EnableUIAutomator = false
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithoutUIAutomator",
		Desc: "ARC is booted without UI Automator",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:883059", // ChromeOS > Software > ARC++ > ARCVM
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithDisableSyncFlags is a fixture similar to arcBooted, but
	// additionally disables ARC content sync and Chrome firmware updates to
	// avoid noise during power/performance measurements.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithDisableSyncFlags",
		Desc: "ARC is booted with disabling sync flags and Chrome firmware updates",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:883059", // ChromeOS > Software > ARC++ > ARCVM
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithDisableExternalStorage is a fixture similar to
	// arcBootedWithDisableSyncFlags, but additionally disables ARC external
	// storage access to avoid noise during power/performance tests that do not
	// require external storage access.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithDisableExternalStorage",
		Desc: "ARC is booted with disabling sync flags, firmware updates and external storage access",
		Contacts: []string{
			"arc-performance@google.com",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent:    "b:168382",
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// TODO(b/325918094): Remove when paravirt sched is fully enabled on chromeos
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name:         "arcBootedWithPvSchedEnabled",
		Desc:         "ARC is booted, with paravirt sched enabled",
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Contacts: []string{
			"vineethrp@google.com",
			"cros-sw-perf@google.com",
		},
		Impl:            NewArcBootedFixtureWithPvSchedEnabled(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// TODO(b/325918094): Remove when paravirt sched is fully enabled on chromeos
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name:         "arcBootedWithDisableExternalStoragePvSchedEnabled",
		Desc:         "ARC is booted, with external storage disabled and paravirt sched enabled",
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Contacts: []string{
			"vineethrp@google.com",
			"cros-sw-perf@google.com",
		},
		Impl:            NewArcBootedFixtureWithPvSchedEnabled(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})
	// arcBootedRestricted is a fixture similar to arcBootedWithDisableExternalStorage,
	// but limits the CPU time of ARC using CGroups.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedRestricted",
		Desc: "ARC is booted in idle state",
		Contacts: []string{
			"arc-performance@google.com",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent:    "b:168382",
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithPlayStore is a fixture similar to arcBooted along with
	// GAIA login and Play Store Optin.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.UnRestrictARCCPU(),
			chrome.GAIALoginPool(dma.CredsFromPool(uiCommon.GaiaPoolDefaultVarName)),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPlayStore",
		Desc: "ARC is booted with disabling sync flags",
		Vars: []string{uiCommon.GaiaPoolDefaultVarName},
		Contacts: []string{
			"jinrongwu@google.com",
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:1487630", // ChromeOS > Software > ARC++ > Commercial > Tast Tests
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithBluetoothFloss is a fixture similar to
	// arcBooted along with Bluetooth-Floss enabled.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.EnableFeatures("Floss"),
			chrome.DisableFeatures("FlossIsAvailabilityCheckNeeded"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithBluetoothFloss",
		Desc: "ARC is booted and Bluetooth-Floss is enabled",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-device-enablement@google.com",
			"vic.lee@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:    "b:1131776", // ChromeOS > Software > Fundamentals > Connectivity > Bluetooth
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedInTabletMode is a fixture similar to arcBooted except that
	// Chrome is launched in tablet mode.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs("--force-tablet-mode=touch_view", "--enable-virtual-keyboard", "--disable-sync"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedInTabletMode",
		Desc: "ARC is booted in tablet mode",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:883059", // ChromeOS > Software > ARC++ > ARCVM
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithVideoLogging is a fixture similar to arcBootedWithDisableExternalStorage,
	// but with additional Chrome video logging enabled.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
			chrome.ExtraArgs(
				"--vmodule=" + strings.Join([]string{
					"*/media/gpu/chromeos/*=2",
					"*/media/gpu/vaapi/*=2",
					"*/media/gpu/v4l2/*=2",
					"*/components/arc/video_accelerator/*=2"}, ","))}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithVideoLogging",
		Desc: "ARC is booted with additional Chrome video logging",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:883059", // ChromeOS > Software > ARC++ > ARCVM
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithOutOfProcessVideoDecoding is similar to arcBootedWithDisableExternalStorage,
	// but Chrome is launched with out-of-process video decoding.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
			chrome.ExtraArgs("--enable-features=OutOfProcessVideoDecoding"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithOutOfProcessVideoDecoding",
		Desc: "ARC is booted with out-of-process video decoding",
		Contacts: []string{
			"andrescj@chromium.org",
			"chromeos-gfx-video@google.com",
		},
		BugComponent:    "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithVideoLoggingAndOutOfProcessVideoDecoding is a fixture similar
	// to arcBootedWithVideoLogging, but Chrome is launched with out-of-process
	// video decoding.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
			chrome.ExtraArgs(
				"--enable-features=OutOfProcessVideoDecoding",
				"--vmodule="+strings.Join([]string{
					"*/media/gpu/chromeos/*=2",
					"*/media/gpu/vaapi/*=2",
					"*/media/gpu/v4l2/*=2",
					"*/components/arc/video_accelerator/*=2"}, ","))}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithVideoLoggingAndOutOfProcessVideoDecoding",
		Desc: "ARC is booted with out-of-process video decoding and additional Chrome video logging",
		Contacts: []string{
			"andrescj@chromium.org",
			"chromeos-gfx-video@google.com",
		},
		BugComponent:    "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithVideoLoggingVD is a fixture similar to
	// arcBootedWithVideoLogging, but with additional Chrome video logging
	// enabled and the mojo::VideoDecoder stack enabled.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
			chrome.ExtraArgs(
				"--vmodule=" + strings.Join([]string{
					"*/media/gpu/chromeos/*=2",
					"*/media/gpu/vaapi/*=2",
					"*/media/gpu/v4l2/*=2",
					"*/components/arc/video_accelerator/*=2"}, ","))}, nil
	}
	fixtureConfig.ArcvmConfig = func(context.Context) (string, error) {
		return "!--video-decoder\n--video-decoder=libvda-vd\n", nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithVideoLoggingVD",
		Desc: "ARC is booted with VD and additional Chrome video logging",
		Contacts: []string{
			"arcvm-platform-video@google.com",
		},
		BugComponent:    "b:632502", // ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Video
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
		Parent:          "gpuWatchDog",
	})

	// arcBootedWithInputOverlayAlphaV2 is a fixture similar to arcBooted but
	// with the input overlay flag enabled.
	// TODO(b/335668261): In M127, remove this fixture after all arc.InputOverlay* tast
	// tests have been migrated or deprecated.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs("--disable-features=GameDashboard"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithInputOverlayAlphaV2",
		Desc: "ARC is booted with the input overlay flag enabled",
		Contacts: []string{
			"arc-app-dev@google.com",
			"pjlee@google.com",
			"cuicuiruan@google.com",
		},
		BugComponent:    "b:1373988", // ChromeOS > Software > ARC++ > Gaming
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithGameDashboard is a fixture similar to arcBooted, but
	// additionally disables ARC content sync, ARC external storage access, and
	// Chrome firmware updates to avoid noise during power/performance
	// measurements.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithGameDashboard",
		Desc: "ARC is booted with the game dashboard flag enabled",
		Contacts: []string{
			"arc-app-dev@google.com",
			"pjlee@google.com",
			"cuicuiruan@google.com",
		},
		BugComponent:    "b:1373988", // ChromeOS > Software > ARC++ > Gaming
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.BootTimeout = BootTimeout + swap.UnrestrictedTimeout
	fixtureConfig.ArcvmConfig = func(ctx context.Context) (string, error) {
		return "SKIP_SWAP_POLICY=true\n", nil
	}
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.EnableFeatures("ArcVmmSwapPolicy:arc_silence_interval_sec/1"),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedBoostedVmmSwap",
		Desc: "ARC is booted and Chrome will use a very short timeout for vmm swap",
		Contacts: []string{
			"cros-vm-technology@google.com",
			"kawasin@google.com",
			"hikalium@chromium.org",
		},
		// ChromeOS > Platform > baseOS > Virtualization
		BugComponent:    "b:882513",
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + swap.UnrestrictedTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	// s2idle state is visible via "dumpsys suspend_control_internal", which requires root
	fixtureConfig.ArcvmConfig = func(context.Context) (string, error) {
		return "--params=androidboot.arc.allow_adb_root=1", nil
	}
	// Initialization of the UiAutomator injects a KEYCODE_POWER event that races
	// with ArcIdleManager putting the device into doze mode.
	fixtureConfig.EnableUIAutomator = false
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.EnableFeatures("ArcIdleManager:ignore_battery_for_test/true/delay_ms/1000,ArcS2Idle"),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedS2Idle",
		Desc: "ARC is booted with S2Idle enabled and Chrome enabling doze aggressively",
		Contacts: []string{
			"cros-vm-technology@google.com",
			"stevensd@google.com",
		},
		// ChromeOS > Platform > baseOS > Virtualization
		BugComponent:    "b:882513",
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithFieldTrialConfigOff is a fixture similar to arcBooted with field trial config disabled.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.FieldTrialConfig(chrome.FieldTrialConfigDisable),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithFieldTrialConfigOff",
		Desc: "ARC is booted with field trial disabled",
		Contacts: []string{
			"pteerapong@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:883059", // ChromeOS > Software > ARC++ > ARCVM
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithFieldTrialConfigOn is a fixture similar to arcBooted with field trial config enabled.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.FieldTrialConfig(chrome.FieldTrialConfigEnable),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithFieldTrialConfigOn",
		Desc: "ARC is booted with field trial enabled",
		Contacts: []string{
			"pteerapong@chromium.org",
			"arcvm-eng-team@google.com",
		},
		BugComponent:    "b:883059", // ChromeOS > Software > ARC++ > ARCVM
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})
}

type arcBootedFixtureArgs struct {
	fieldTrialConfig int // Value for FieldTrialConfig Chrome parameter
}

type bootedFixture struct {
	parentStateProvider func(s *testing.FixtState) interface{}

	cr        *chrome.Chrome
	arc       *ARC
	d         *ui.Device
	init      *Snapshot
	logMarker *logsaver.Marker // Marker for per-test log.

	playStoreOptin    bool                                  // Opt into PlayStore.
	enableUIAutomator bool                                  // Enable UI Automator
	enableLightTheme  bool                                  // Enable LightTheme
	arcvmConfig       func(context.Context) (string, error) // Append config to arcvm_dev.conf
	bootTimeout       time.Duration

	fOpt chrome.OptionsCallback // Function to return chrome options.
}

// ParentState for chrome.HasParentState.
func (f *PreData) ParentState() interface{} {
	return f.parentState
}

// ParentStateProviderFn is a function signature that provides parent state in ArcBootedFixtureConfig
type ParentStateProviderFn = func(s *testing.FixtState) interface{}

// BootedFixtureConfig configures the fixture in NewArcBootedFixture
type BootedFixtureConfig struct {
	// OptionsCallback function to provide functions to chrome.
	FOpts chrome.OptionsCallback
	// specified config appended to arcvm_dev.conf.
	ArcvmConfig func(context.Context) (string, error)
	// Timeout to wait for ARC boot to complete.
	BootTimeout time.Duration
	// A factory provider that returns the parent fixture state (via a closure, generally).
	ParentStateProvider ParentStateProviderFn
	// Whether or not to enable UI automator.
	EnableUIAutomator bool
	// Whether or not to opt into the play store.
	PlayStoreOptin bool
	// Whether or not to enable light theme.
	EnableLightTheme bool
}

// DefaultBootedFixtureConfig provides sane defaults for most tests.
func DefaultBootedFixtureConfig() BootedFixtureConfig {
	return BootedFixtureConfig{
		FOpts: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.UnRestrictARCCPU()}, nil
		},
		// specified config appended to arcvm_dev.conf.
		ArcvmConfig:         nil,
		BootTimeout:         BootTimeout,
		ParentStateProvider: nil,
		EnableUIAutomator:   true,
		PlayStoreOptin:      false,
		EnableLightTheme:    false,
	}
}

// NewArcBootedFixture returns a FixtureImpl for ARC
// ARCSupported() will be appended to the fixture if playStoreOptin is specified,
// since it is necessary to use this option to enable the Play Store. Otherwise,
// ARCEnabled() will be appended, which will enable ARC without the Play Store
// and works for fake logins.
// Fixtures using this implementation should have minimum arc.ResetTimeout, arc.PreTestTimeout,
// arc.PostTestTimeout, and arc.ResetTimeout.
// Also SetUpTimeout should have at least chrome.LoginTimeout + arc.BootTimeout + ui.StartTimeout.
func NewArcBootedFixture(arcBootedFixtureConfig BootedFixtureConfig) testing.FixtureImpl {
	return &bootedFixture{
		parentStateProvider: arcBootedFixtureConfig.ParentStateProvider,
		enableUIAutomator:   arcBootedFixtureConfig.EnableUIAutomator,
		arcvmConfig:         arcBootedFixtureConfig.ArcvmConfig,
		playStoreOptin:      arcBootedFixtureConfig.PlayStoreOptin,
		enableLightTheme:    arcBootedFixtureConfig.EnableLightTheme,
		bootTimeout:         arcBootedFixtureConfig.BootTimeout,
		fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts, err := arcBootedFixtureConfig.FOpts(ctx, s)
			if err != nil {
				return nil, err
			}
			if arcBootedFixtureConfig.PlayStoreOptin {
				opts = append(opts, chrome.ARCSupported())
			} else {
				opts = append(opts, chrome.ARCEnabled())
			}
			return append(opts, chrome.DisableFeatures("ArcResizeLock")), nil
		},
	}
}

// NewArcBootedFixtureWithPvSchedEnabled returns a fixtureImple for ARC(bootedFixtureWithPvSchedEnabled).
// Similar to NewArcBootedFixture and could be refactored, but duplicating
// code without touching NewArcBootedFixture so that revert becomes easier.
// TODO(b/325918094): Remove when paravirt sched is fully enabled on chromeos
func NewArcBootedFixtureWithPvSchedEnabled(arcBootedFixtureConfig BootedFixtureConfig) testing.FixtureImpl {

	bf := bootedFixture{
		parentStateProvider: arcBootedFixtureConfig.ParentStateProvider,
		enableUIAutomator:   arcBootedFixtureConfig.EnableUIAutomator,
		arcvmConfig:         arcBootedFixtureConfig.ArcvmConfig,
		playStoreOptin:      arcBootedFixtureConfig.PlayStoreOptin,
		enableLightTheme:    arcBootedFixtureConfig.EnableLightTheme,
		bootTimeout:         arcBootedFixtureConfig.BootTimeout,
		fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts, err := arcBootedFixtureConfig.FOpts(ctx, s)
			if err != nil {
				return nil, err
			}
			if arcBootedFixtureConfig.PlayStoreOptin {
				opts = append(opts, chrome.ARCSupported())
			} else {
				opts = append(opts, chrome.ARCEnabled())
			}
			return append(opts, chrome.ExtraArgs("--disable-features=ArcResizeLock")), nil
		},
	}
	return &bootedFixtureWithPvSchedEnabled{
		bootedFixture: bf,
	}
}

// NewMtbfArcBootedFixture returns a FixtureImpl with a OptionsCallback function provided for MTBF ARC++ tests.
func NewMtbfArcBootedFixture(fOpts chrome.OptionsCallback) testing.FixtureImpl {
	return &bootedFixture{
		enableUIAutomator: false,
		playStoreOptin:    true,
		fOpt:              fOpts,
	}
}

func (f *bootedFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	success := false

	// Append additional config to the ARCVM config file, needs to be done before launching Chrome.
	if f.arcvmConfig != nil {
		if cfg, err := f.arcvmConfig(ctx); err != nil {
			s.Fatal("Failed to generate arcvm_dev.conf modification: ", err)
		} else if err := AppendToArcvmDevConf(ctx, cfg); err != nil {
			s.Fatal("Failed to write arcvm_dev.conf: ", err)
		}
	}

	defer func() {
		if !success && f.arcvmConfig != nil {
			if err := RestoreArcvmDevConf(ctx); err != nil {
				s.Fatal("Failed to restore arcvm_dev.conf: ", err)
			}
		}
	}()

	var parentState interface{}
	if f.parentStateProvider != nil {
		parentState = f.parentStateProvider(s)
	}

	opts, err := f.fOpt(ctx, s)
	if err != nil {
		s.Fatal("Failed to obtain fixture options: ", err)
	}

	if s.Param() != nil {
		args := s.Param().(arcBootedFixtureArgs)
		opts = append(opts, chrome.FieldTrialConfig(args.fieldTrialConfig))
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer func() {
		if !success {
			cr.Close(ctx)
		}
	}()

	if f.playStoreOptin {
		s.Log("Performing Play Store Optin")
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}
		st, err := GetState(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get ARC state: ", err)
		}
		if st.Provisioned {
			s.Log("ARC is already provisioned. Skipping the Play Store setup")
		} else {
			// Opt into Play Store and close the Play Store window.
			if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
				s.Fatal("Failed to opt into Play Store: ", err)
			}
		}
	}

	if f.enableLightTheme {
		s.Log("Turning on light theme")
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}
		if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.forceAutoThemeMode)`, false); err != nil {
			s.Fatal("Failed to enable light theme: ", err)
		}
	}

	bootTimeout := f.bootTimeout
	if bootTimeout == 0 {
		bootTimeout = BootTimeout
	}
	arc, err := NewWithTimeout(ctx, s.OutDir(), bootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer func() {
		if !success {
			arc.Close(ctx)
		}
	}()

	var d *ui.Device
	if f.enableUIAutomator {
		if d, err = arc.NewUIDevice(s.FixtContext()); err != nil {
			s.Fatal("Failed to initialize UI Automator: ", err)
		}
		defer func() {
			if !success {
				d.Close(ctx)
			}
		}()
	}

	init, err := NewSnapshot(ctx, arc)
	if err != nil {
		s.Fatal("Failed to take ARC state snapshot: ", err)
	}

	// Prevent the arc and chrome package's New and Close functions from
	// being called while this bootedFixture is active.
	Lock()
	chrome.Lock()

	f.cr = cr
	f.arc = arc
	f.d = d
	f.init = init
	success = true
	return &PreData{
		parentState: parentState,
		Chrome:      cr,
		ARC:         arc,
		UIDevice:    d,
	}
}

func (f *bootedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.arcvmConfig != nil {
		if err := RestoreArcvmDevConf(ctx); err != nil {
			s.Fatal("Failed to restore arcvm_dev.conf: ", err)
		}
	}

	if f.d != nil {
		if err := f.d.Close(ctx); err != nil {
			s.Log("Failed to close UI Automator: ", err)
		}
		f.d = nil
	}

	Unlock()
	if err := f.arc.Close(ctx); err != nil {
		s.Log("Failed to close ARC: ", err)
	}
	f.arc = nil

	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome: ", err)
	}
	f.cr = nil
}

func (f *bootedFixture) Reset(ctx context.Context) error {
	if f.d != nil && !f.d.Alive(ctx) {
		return errors.New("UI Automator is dead")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed to reset chrome")
	}
	return f.init.Restore(ctx, f.arc)
}

func (f *bootedFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.arc.ResetOutDir(ctx, s.OutDir()); err != nil {
		s.Error("Failed to to reset outDir field of ARC object: ", err)
	}

	if err := f.arc.CloseSystemDialogs(ctx); err != nil {
		s.Error("Failed to close ARC system dialogs: ", err)
	}

	if f.logMarker != nil {
		s.Log("A log marker is already created but not cleaned up")
	}
	logMarker, err := logsaver.NewMarker(f.cr.LogFilename())
	if err == nil {
		f.logMarker = logMarker
	} else {
		s.Log("Failed to start the log saver: ", err)
	}
}

func (f *bootedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.arc.SaveLogFiles(ctx); err != nil {
		s.Error("Failed to to save ARC-related log files: ", err)
	}

	if f.logMarker != nil {
		if err := f.logMarker.Save(filepath.Join(s.OutDir(), "chrome.log")); err != nil {
			s.Log("Failed to store per-test log data: ", err)
		}
		f.logMarker = nil
	}

	if s.HasError() {
		faillogDir := filepath.Join(s.OutDir(), "faillog")
		if err := os.MkdirAll(faillogDir, 0755); err != nil {
			s.Error("Failed to make faillog/ directory: ", err)
			return
		}
		if err := saveProcessList(ctx, f.arc, faillogDir); err != nil {
			s.Error("Failed to save the process list in ARCVM: ", err)
		}
		if err := saveDumpsys(ctx, f.arc, faillogDir); err != nil {
			s.Error("Failed to save dumpsys output in ARCVM: ", err)
		}
	}
}

func saveProcessList(ctx context.Context, a *ARC, outDir string) error {
	path := filepath.Join(outDir, "ps-arcvm.txt")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd := a.Command(ctx, "ps", "-AfZ")
	cmd.Stdout = file
	return cmd.Run()
}

func saveDumpsys(ctx context.Context, a *ARC, outDir string) error {
	path := filepath.Join(outDir, "dumpsys.txt")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd := a.Command(ctx, "dumpsys")
	cmd.Stdout = file
	return cmd.Run()
}

// Similar to bootedFixture but enables paravirt sched feature.
// TODO(b/325918094): Remove when paravirt sched is fully enabled on chromeos
type bootedFixtureWithPvSchedEnabled struct {
	pvSchedEnabled bool // Current state of PvSched feature
	bootedFixture
}

func (f *bootedFixtureWithPvSchedEnabled) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var err error

	f.pvSchedEnabled, err = pvsched.Enabled()
	if err != nil {
		s.Fatal("Failed to get Paravirt Sched state: ", err)
	}

	if !f.pvSchedEnabled {
		pvsched.Enable()
	}

	return f.bootedFixture.SetUp(ctx, s)
}

func (f *bootedFixtureWithPvSchedEnabled) TearDown(ctx context.Context, s *testing.FixtState) {

	if !f.pvSchedEnabled {
		pvsched.Disable()
	}

	f.bootedFixture.TearDown(ctx, s)
}

func (f *bootedFixtureWithPvSchedEnabled) Reset(ctx context.Context) error {
	return f.bootedFixture.Reset(ctx)
}

func (f *bootedFixtureWithPvSchedEnabled) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.bootedFixture.PreTest(ctx, s)
}

func (f *bootedFixtureWithPvSchedEnabled) PostTest(ctx context.Context, s *testing.FixtTestState) {
	f.bootedFixture.PostTest(ctx, s)
}
