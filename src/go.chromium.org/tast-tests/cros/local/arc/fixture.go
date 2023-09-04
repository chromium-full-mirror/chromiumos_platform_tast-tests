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

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PreTestTimeout is the timeout duration to reset output directory before each test.
const PreTestTimeout = 15 * time.Second

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
		Impl:            NewArcBootedFixture(DefaultBootedFixtureConfig()),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig := DefaultBootedFixtureConfig()
	fixtureConfig.EnableUIAutomator = false
	// arcBootedWithoutUIAutomator is a fixture similar to arcBooted. The only difference from arcBooted is that UI Automator is not enabled.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithoutUIAutomator",
		Desc: "ARC is booted without UI Automator",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	// arcBootedWithDisableSyncFlags is a fixture similar to arcBooted. The only difference from arcBooted is that ARC content sync
	// and Chrome firmware updates are disabled to avoid noise during power/performance measurements.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithDisableSyncFlags",
		Desc: "ARC is booted with disabling sync flags and Chrome firmware updates",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	// arcBootedWithDisableExternalStorage is a fixture similar to arcBootedWithDisableSyncFlags. The only difference from
	// arcBootedWithDisableSyncFlags is that ARC external storage access is disabled to avoid noise during power/performance
	// measurements for power/performance tests that do not require external storage access.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithDisableExternalStorage",
		Desc: "ARC is booted with disabling sync flags, firmware updates and external storage access",
		Contacts: []string{
			"alanding@chromium.org",
			"arc-performance@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
		}, nil
	}
	// arcBootedRestricted is a fixture similar to arcBootedWithDisableExternalStorage. The only difference
	// from arcBootedWithDisableExternalStorage is that CGroups is used to limit the CPU time of ARC.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedRestricted",
		Desc: "ARC is booted in idle state",
		Contacts: []string{
			"alanding@chromium.org",
			"arc-performance@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.UnRestrictARCCPU(),
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		}, nil
	}
	// arcBootedWithPlayStore is a fixture similar to arcBooted along with GAIA login and Play Store Optin.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPlayStore",
		Desc: "ARC is booted with disabling sync flags",
		Vars: []string{"ui.gaiaPoolDefault"},
		Contacts: []string{
			"jinrongwu@google.com",
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.EnableFeatures("QsRevamp"),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.UnRestrictARCCPU(),
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		}, nil
	}
	// arcBootedWithPlayStoreQsRevampEnabled is similar to fixture arcBootedWithPlayStore and has quick settings revamp enabled.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPlayStoreQsRevampEnabled",
		Desc: "ARC is booted with sync flags disabled and quick settings revamp enabled",
		Vars: []string{"ui.gaiaPoolDefault"},
		Contacts: []string{
			"jamescook@google.com",
			"cros-status-area-eng@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

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
	// arcBootedWithPlayStoreAndBluetoothBlueZ is a fixture similar to arcBootedWithPlayStore along with Bluetooth-BlueZ is enabled.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPlayStoreAndBluetoothBlueZ",
		Desc: "ARC is booted with disabling sync flags and Bluetooth-BlueZ is enabled",
		Vars: []string{"ui.gaiaPoolDefault"},
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
			"kinwang.lao@cienet.com",
			"cienet-development@googlegroups.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.EnableFeatures("Floss"),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.UnRestrictARCCPU(),
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		}, nil
	}
	// arcBootedWithPlayStoreAndBluetoothFloss is a fixture similar to arcBootedWithPlayStore along with Bluetooth-Floss is enabled.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPlayStoreAndBluetoothFloss",
		Desc: "ARC is booted with disabling sync flags and Bluetooth-Floss is enabled",
		Vars: []string{"ui.gaiaPoolDefault"},
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
			"kinwang.lao@cienet.com",
			"cienet-development@googlegroups.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs("--force-tablet-mode=touch_view", "--enable-virtual-keyboard"),
		}, nil
	}
	// arcBootedInTabletMode is a fixture similar to arcBooted. The only difference from arcBooted is that Chrome is launched in tablet mode in this fixture.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedInTabletMode",
		Desc: "ARC is booted in tablet mode",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.ExtraArgs(
			"--vmodule=" + strings.Join([]string{
				"*/media/gpu/chromeos/*=2",
				"*/media/gpu/vaapi/*=2",
				"*/media/gpu/v4l2/*=2",
				"*/components/arc/video_accelerator/*=2"}, ","))}, nil
	}
	// arcBootedWithVideoLogging is a fixture similar to arcBooted, but with additional Chrome video logging enabled.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithVideoLogging",
		Desc: "ARC is booted with additional Chrome video logging",
		Contacts: []string{
			"niwa@chromium.org",
			"arcvm-eng-team@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs("--enable-features=OutOfProcessVideoDecoding"),
		}, nil
	}
	// arcBootedWithOutOfProcessVideoDecoding is a fixture similar to arcBooted. The only difference from arcBooted is that Chrome is launched with out-of-process
	// video decoding in this fixture.
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
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.ExtraArgs(
			"--enable-features=OutOfProcessVideoDecoding",
			"--vmodule="+strings.Join([]string{
				"*/media/gpu/chromeos/*=2",
				"*/media/gpu/vaapi/*=2",
				"*/media/gpu/v4l2/*=2",
				"*/components/arc/video_accelerator/*=2"}, ","))}, nil
	}
	// arcBootedWithVideoLoggingAndOutOfProcessVideoDecoding is a fixture similar to arcBootedWithVideoLogging, but Chrome is launched with out-of-process video
	// decoding.
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
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// arcBootedWithVideoLoggingVD is a fixture similar to arcBootedWithVideoLogging, but with additional Chrome
	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.ExtraArgs(
			"--vmodule=" + strings.Join([]string{
				"*/media/gpu/chromeos/*=2",
				"*/media/gpu/vaapi/*=2",
				"*/media/gpu/v4l2/*=2",
				"*/components/arc/video_accelerator/*=2"}, ","))}, nil
	}
	fixtureConfig.ArcvmConfig = "!--video-decoder\n--video-decoder=libvda-vd\n"
	// video logging enabled and the mojo::VideoDecoder stack enabled.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithVideoLoggingVD",
		Desc: "ARC is booted with VD and additional Chrome video logging",
		Contacts: []string{
			"arcvm-eng-team@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.ARCEnabled(), chrome.UnRestrictARCCPU())).Opts()
	}
	// lacrosWithArcBooted is a fixture that combines the functionality of arcBooted and lacros.
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosWithArcBooted",
		Desc: "Lacros Chrome from a pre-built image with ARC booted",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@chromium.org",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.ExtraArgs("--force-tablet-mode=touch_view", "--enable-virtual-keyboard"))).Opts()
	}
	// lacrosWithArcBootedInTabletMode is a fixture similar to lacrosWithArcBooted. The only difference is that Chrome is launched in tablet mode in this fixture.
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosWithArcBootedInTabletMode",
		Desc: "Lacros Chrome from a pre-built image with ARC booted in tablet mode",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@chromium.org",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
			chrome.ARCEnabled(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.UnRestrictARCCPU(),
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))).Opts()
	}
	// lacrosWithArcBootedAndPlayStore is a fixture that combines the functionality of arcBootedWithPlayStore and lacros.
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosWithArcBootedAndPlayStore",
		Desc: "Lacros Chrome from a pre-built image with ARC booted and the Play Store enabled",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@chromium.org",
		},
		Vars:            []string{"ui.gaiaPoolDefault"},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs(DisableSyncFlags()...),
			chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"))).Opts()
	}
	// lacrosWithArcBootedAndDisableExternalStorage is a fixture that combines the functionality of arcBootedWithDisableExternalStorage and lacros.
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosWithArcBootedAndDisableExternalStorage",
		Desc: "Lacros Chrome from a pre-built image with ARC booted and external storage disabled",
		Contacts: []string{
			"hungmn@google.com",
			"arc-performance@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.ExtraArgs("--enable-features=ArcInputOverlayAlphaV2"),
		}, nil
	}
	// arcBootedWithInputOverlay is a fixture similar to arcBooted but with the input overlay flag enabled.
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
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.EnableFeatures("PasspointARCSupport"),
		}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedWithPasspoint",
		Desc: "ARC is booted with kPasspointARCSupport feature enabled",
		Contacts: []string{
			"jasongustaman@google.com",
			"cros-networking@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.ARCEnabled(),
			chrome.UnRestrictARCCPU(),
			chrome.EnableFeatures("QsRevamp"),
		}, nil
	}
	// arcBootedQsRevampEnabled is similar to fixture arcBooted and has quick settings revamp enabled.
	testing.AddFixture(&testing.Fixture{
		Name: "arcBootedQsRevampEnabled",
		Desc: "ARC is booted with quick settings revamp enabled",
		Contacts: []string{
			"jamescook@google.com",
			"cros-status-area-eng@google.com",
		},
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	fixtureConfig = DefaultBootedFixtureConfig()
	fixtureConfig.BootTimeout = BootTimeout + swap.UnrestrictedTimeout
	fixtureConfig.ArcvmConfig = "SKIP_SWAP_POLICY=true"
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
		Impl:            NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + BootTimeout + swap.UnrestrictedTimeout + ui.StartTimeout,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})
}

type bootedFixture struct {
	cr   *chrome.Chrome
	arc  *ARC
	d    *ui.Device
	init *Snapshot

	playStoreOptin    bool   // Opt into PlayStore.
	enableUIAutomator bool   // Enable UI Automator
	arcvmConfig       string // Append config to arcvm_dev.conf
	bootTimeout       time.Duration

	fOpt chrome.OptionsCallback // Function to return chrome options.
}

// BootedFixtureConfig configures the fixture in NewArcBootedFixture
type BootedFixtureConfig struct {
	// OptionsCallback function to provide functions to chrome.
	FOpts chrome.OptionsCallback
	// specified config appended to arcvm_dev.conf.
	ArcvmConfig string
	// Timeout to wait for ARC boot to complete.
	BootTimeout time.Duration
	// Whether or not to enable UI automator.
	EnableUIAutomator bool
	// Whether or not to opt into the play store.
	PlayStoreOptin bool
}

// DefaultBootedFixtureConfig provides sane defaults for most tests.
func DefaultBootedFixtureConfig() BootedFixtureConfig {
	return BootedFixtureConfig{
		FOpts: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.ARCEnabled(), chrome.UnRestrictARCCPU()}, nil
		},
		// specified config appended to arcvm_dev.conf.
		ArcvmConfig:       "",
		BootTimeout:       BootTimeout,
		EnableUIAutomator: true,
		PlayStoreOptin:    false,
	}
}

// NewArcBootedFixture returns a FixtureImpl for ARC
// ARCEnabled() will always be added to the Chrome options returned by OptionsCallback.
func NewArcBootedFixture(arcBootedFixtureConfig BootedFixtureConfig) testing.FixtureImpl {
	return &bootedFixture{
		enableUIAutomator: arcBootedFixtureConfig.EnableUIAutomator,
		arcvmConfig:       arcBootedFixtureConfig.ArcvmConfig,
		playStoreOptin:    arcBootedFixtureConfig.PlayStoreOptin,
		bootTimeout:       arcBootedFixtureConfig.BootTimeout,
		fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts, err := arcBootedFixtureConfig.FOpts(ctx, s)
			if err != nil {
				return nil, err
			}
			return append(opts, chrome.ARCEnabled(), chrome.ExtraArgs("--disable-features=ArcResizeLock")), nil
		},
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
	if f.arcvmConfig != "" {
		if err := AppendToArcvmDevConf(ctx, f.arcvmConfig); err != nil {
			s.Fatal("Failed to write arcvm_dev.conf: ", err)
		}
	}
	defer func() {
		if !success && f.arcvmConfig != "" {
			if err := RestoreArcvmDevConf(ctx); err != nil {
				s.Fatal("Failed to restore arcvm_dev.conf: ", err)
			}
		}
	}()

	opts, err := f.fOpt(ctx, s)
	if err != nil {
		s.Fatal("Failed to obtain fixture options: ", err)
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
		Chrome:   cr,
		ARC:      arc,
		UIDevice: d,
	}
}

func (f *bootedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.arcvmConfig != "" {
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
	// TODO(crbug.com/1136382): Support per-test logcat once we get pre/post-test
	// hooks in fixtures.

	if err := f.arc.ResetOutDir(ctx, s.OutDir()); err != nil {
		s.Error("Failed to to reset outDir field of ARC object: ", err)
	}
}

func (f *bootedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// TODO(crbug.com/1136382): Support per-test logcat once we get pre/post-test
	// hooks in fixtures.

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
