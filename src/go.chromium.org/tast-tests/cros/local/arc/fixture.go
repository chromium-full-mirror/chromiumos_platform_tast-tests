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
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/swap"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/pvsched"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PreTestTimeout is the timeout duration before each test.
const PreTestTimeout = 60 * time.Second

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
		},
	})

	// arcBootedWithNoDownloadsBindMount is a fixture similar to arcBooted, except
	// that it runs RemoveBindMount fixture before it.
	// TODO(b/328698041): Remove this after removal of bind mount is released.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithNoDownloadsBindMount",
		Desc: "ARC is booted with no downloads bind mount",
		Contacts: []string{
			"wenbojie@chromium.org",
			"chromeos-files-syd@google.com",
		},
		BugComponent:    "b:167289",
		Impl:            NewArcBootedFixture(DefaultBootedFixtureConfig()),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
		Parent:          "removeBindMountFixture",
	})

	// arcBootedWithoutUIAutomator is a fixture similar to arcBooted, except
	// that UI Automator is not enabled.
	fixtureConfig := DefaultBootedFixtureConfig()
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
			"alanding@chromium.org",
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
			"alanding@chromium.org",
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
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPlayStore",
		Desc: "ARC is booted with disabling sync flags",
		Vars: []string{"ui.gaiaPoolDefault"},
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

	// arcBootedWithPlayStoreAndBluetoothBlueZ is a fixture similar to
	// arcBootedWithPlayStore along with Bluetooth-BlueZ enabled.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.DisableFeatures("Floss"),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.UnRestrictARCCPU(),
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPlayStoreAndBluetoothBlueZ",
		Desc: "ARC is booted with disabling sync flags and Bluetooth-BlueZ is enabled",
		Vars: []string{"ui.gaiaPoolDefault"},
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
			"kinwang.lao@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:    "b:1131776", // ChromeOS > Software > Fundamentals > Connectivity > Bluetooth
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithPlayStoreAndBluetoothFloss is a fixture similar to
	// arcBootedWithPlayStore along with Bluetooth-Floss enabled.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.EnableFeatures("Floss"),
			// FlossIsAvailabilityCheckNeeded needs to be disabled when Floss is enabled (b/300999731).
			chrome.DisableFeatures("FlossIsAvailabilityCheckNeeded"),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.UnRestrictARCCPU(),
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPlayStoreAndBluetoothFloss",
		Desc: "ARC is booted with disabling sync flags and Bluetooth-Floss is enabled",
		Vars: []string{"ui.gaiaPoolDefault"},
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
			"kinwang.lao@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:    "b:1131776", // ChromeOS > Software > Fundamentals > Connectivity > Bluetooth
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
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
	})

	// lacrosWithArcBooted is a fixture that combines the functionality of
	// arcBooted and lacros.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.ARCEnabled(), chrome.UnRestrictARCCPU())).Opts()
	}
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosWithArcBooted",
		Desc: "Lacros Chrome from a pre-built image with ARC booted",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@chromium.org",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// lacrosWithArcBootedInTabletMode is a fixture similar to lacrosWithArcBooted,
	// except that Chrome is launched in tablet mode.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.ExtraArgs("--force-tablet-mode=touch_view", "--enable-virtual-keyboard"))).Opts()
	}
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosWithArcBootedInTabletMode",
		Desc: "Lacros Chrome from a pre-built image with ARC booted in tablet mode",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@chromium.org",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// lacrosWithArcBootedAndPlayStore is a fixture that combines the
	// functionality of arcBootedWithPlayStore and lacros.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
			chrome.ARCEnabled(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.UnRestrictARCCPU(),
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))).Opts()
	}
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosWithArcBootedAndPlayStore",
		Desc: "Lacros Chrome from a pre-built image with ARC booted and the Play Store enabled",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@chromium.org",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Vars:            []string{"ui.gaiaPoolDefault"},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// lacrosWithArcBootedAndDisableExternalStorage is a fixture that combines
	// the functionality of arcBootedWithDisableExternalStorage and lacros.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"))).Opts()
	}
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosWithArcBootedAndDisableExternalStorage",
		Desc: "Lacros Chrome from a pre-built image with ARC booted and external storage disabled",
		Contacts: []string{
			"hungmn@google.com",
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
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PreTestTimeout:  PreTestTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithGameDashboard is a fixture similar to arcBooted but
	// with the `GameDashboard` and `DropdownPanel` feature flags enabled.
	// TODO(b/331704817): In M126, remove the "DropdownPanel" feature flag as a Chrome arg.
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
			chrome.ExtraArgs("--enable-features=DropdownPanel"), chrome.ExtraArgs("--enable-features=GameDashboard"),
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

	cr   *chrome.Chrome
	arc  *ARC
	d    *ui.Device
	init *Snapshot

	playStoreOptin    bool                                  // Opt into PlayStore.
	enableUIAutomator bool                                  // Enable UI Automator
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
}

func (f *bootedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.arc.SaveLogFiles(ctx); err != nil {
		s.Error("Failed to to save ARC-related log files: ", err)
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
