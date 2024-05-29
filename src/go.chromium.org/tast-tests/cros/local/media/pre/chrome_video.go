// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast/core/testing"
)

const (
	chromeVideo featureType = featureType(uint32(1) << iota)

	// FakeMediaStreamUI avoids the need to grant camera/microphone permissions.
	FakeMediaStreamUI

	// NaCl enables support for Native Client apps.
	NaCl

	// SWDecoding disables HW accelerated video decoding.
	SWDecoding

	// GuestLogin ensures the test runs while logged in as the guest user.
	GuestLogin

	// AshComposited disables HW overlays in ash-chrome entirely in order to force video to be composited by ash-chrome.
	AshComposited

	// LacrosComposited disables HW overlays in lacros-chrome entirely in order to force video to be composited by lacros-chrome.
	LacrosComposited

	// DistinctiveIdentifier allows for a distinctive identifier with DRM playback.
	DistinctiveIdentifier

	// VCDInUtilityProcess makes the video capture service run in a utility process.
	VCDInUtilityProcess

	// GTFO avoids using the Chrome GPU process as a proxy in between renderers and video decoder utility processes (go/oopvd-gtfo-dd).
	GTFO

	// This must be defined last.
	numChromeVideoFeatures = iota
)

func initChromeVideoFixtures() {
	initChromeVideoBaseFixtures()
	initChromeVideoLacrosFixtures()
}

func initChromeVideoBaseFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideo",
		Desc:     "Logged into a user session with logging enabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoINPVD",
		Desc:     "Logged into a user session with logging and out-of-process video decoding disabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoNaCl",
		Desc:     "Logged into a user session with logging, NaCl and the MojoVideoDecoder-for-Pepper enabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs("--enable-nacl"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoNaClWithSWDecoding",
		Desc:     "Similar to chromeVideoNaClWithMojoVideoDecoder but making sure Chrome does not use any potential hardware accelerated decoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs("--enable-nacl"),
				chrome.EnableFeatures("UseMojoVideoDecoderForPepper"),
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/248528896): Remove once out-of-process video encoding is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithOOPVE",
		Desc:     "Similar to chromeVideo fixture but enabling out-of-process video encoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoEncoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGuestLogin",
		Desc:     "Similar to chromeVideo fixture but forcing login as a guest",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.GuestLogin(),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoINPVDWithGuestLogin",
		Desc:     "Like chromeVideoWithGuestLogin but with out-of-process video decoding disabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
				chrome.GuestLogin(),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	// TODO(crbug.com/958166): Use simply ChromeVideo() when HDR is launched.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithHDRScreen",
		Desc:     "Similar to chromeVideo fixture but enabling the HDR screen if present",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.EnableFeatures("UseHDRTransferFunction"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeCompositedVideo",
		Desc:     "Similar to chromeVideo fixture but disabling hardware overlays entirely to force video to be composited",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithSWDecoding",
		Desc:     "Similar to chromeVideo fixture but making sure Chrome does not use any potential hardware accelerated decoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(crbug.com/958166): Use simply ChromeVideoWithSWDecoding() when HDR is launched.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithSWDecodingAndHDRScreen",
		Desc:     "Similar to chromeVideoWithSWDecoding but also enalbing the HDR screen if present",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
				chrome.EnableFeatures("UseHDRTransferFunction"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithDistinctiveIdentifier",
		Desc:     "Similar to chromeVideo fixture but also allows a distinctive identifier which is needed for HWDRM",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoINPVDWithDistinctiveIdentifier",
		Desc:     "Like chromeVideoWithDistinctiveIdentifier but with out-of-process video decoding disabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithIntelMediaCompression",
		Desc:     "Similar to chromeVideo fixture but enabling media compression by Intel",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.EnableFeatures("EnableIntelMediaCompression"),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithV4L2FlatDecoder",
		Desc:     "Similar to chromeVideo fixture but enabling V4L2 Flat stateful decoder",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.EnableFeatures("V4L2FlatStatefulVideoDecoder"),
				chrome.EnableFeatures("V4L2FlatVideoDecoder"),
				chrome.EnableFeatures("UseChromeOSDirectVideoDecoder"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithBatchDecodingInRenderer",
		Desc:     "Similar to chromeVideo fixture but enabling batch decoding for non-MF renderer path",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("VideoDecodeBatching"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithVCDInUtilityProcess",
		Desc:     "Similar to chromeVideo fixture but running VCD in the utility process",
		Contacts: []string{"chromeos-gfx-video@google.com", "seannli@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.DisableFeatures("RunVideoCaptureServiceInBrowserProcess"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithReducedHardwareVideoDecoderBuffers",
		Desc:     "Similar to chromeVideo fixture but reduce the number of required renderer pipeline buffers to fill video frame pool",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				browser.TypeAsh,
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("ReduceHardwareVideoDecoderBuffers"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

var chromeVideoLacrosFixtureGenerator = fixtureGenerator{
	name: "chromeVideoLacros",
}

func initChromeVideoLacrosFixtures() {
	combos := []featureComboType{
		comb(chromeVideo, FakeMediaStreamUI),
		comb(chromeVideo, FakeMediaStreamUI, GTFO),
		comb(chromeVideo, FakeMediaStreamUI, NaCl),
		comb(chromeVideo, FakeMediaStreamUI, NaCl, SWDecoding),
		comb(chromeVideo, GuestLogin),
		comb(chromeVideo, GuestLogin, GTFO),
		comb(chromeVideo, AshComposited),
		comb(chromeVideo, AshComposited, GTFO),
		comb(chromeVideo, LacrosComposited),
		comb(chromeVideo, LacrosComposited, GTFO),
		comb(chromeVideo, FakeMediaStreamUI, DistinctiveIdentifier),
		comb(chromeVideo, FakeMediaStreamUI, VCDInUtilityProcess),
	}

	featureMap := map[featureType]featureInfo{
		chromeVideo: {
			"_",
			getChromeVideoOptions(browser.TypeLacros),
		},
		FakeMediaStreamUI: {
			"FakeMediaStreamUI",
			[]chrome.Option{
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...),
			},
		},
		NaCl: {
			"NaCl",
			[]chrome.Option{
				chrome.ExtraArgs("--enable-nacl"),
				chrome.LacrosExtraArgs("--enable-nacl"),
			},
		},
		SWDecoding: {
			"SWDecoding",
			[]chrome.Option{
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
				chrome.LacrosExtraArgs("--disable-accelerated-video-decode"),
			},
		},
		GuestLogin: {
			"Guest",
			[]chrome.Option{
				chrome.GuestLogin(),
			},
		},
		AshComposited: {
			"AshComposited",
			[]chrome.Option{
				chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
			},
		},
		LacrosComposited: {
			"LacrosComposited",
			[]chrome.Option{
				chrome.LacrosExtraArgs("--enable-hardware-overlays=\"\""),
			},
		},
		DistinctiveIdentifier: {
			"DistinctiveIdentifier",
			[]chrome.Option{
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
				chrome.LacrosExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
			},
		},
		VCDInUtilityProcess: {
			"VCDInUtilityProcess",
			[]chrome.Option{
				chrome.DisableFeatures("RunVideoCaptureServiceInBrowserProcess"),
			},
		},
		GTFO: {
			"GTFO",
			[]chrome.Option{
				chrome.LacrosDisableFeatures("UseOutOfProcessVideoDecoding"),
				chrome.LacrosEnableFeatures("UseGTFOOutOfProcessVideoDecoding"),
			},
		},
	}
	if len(featureMap) != numChromeVideoFeatures {
		panic("Missing feature declaration in featureMap")
	}

	chromeVideoLacrosFixtureGenerator.initialize(combos, featureMap)
	testing.AddFixture(&testing.Fixture{
		Name:            chromeVideoLacrosFixtureGenerator.name,
		Desc:            "Logged into a LaCrOS session",
		Contacts:        []string{"chromeos-gfx-video@google.com"},
		BugComponent:    "b:168352", // ChromeOS > Platform > Graphics > Video.
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(s.Param().([]chrome.Option)...)).Opts()
		}),
		Params: chromeVideoLacrosFixtureGenerator.genParams(),
	})
}

// ChromeVideoLacrosFixture returns the name of the LaCrOS video fixture corresponding to features.
func ChromeVideoLacrosFixture(features ...featureType) string {
	return chromeVideoLacrosFixtureGenerator.getFixture(add(comb(features...), chromeVideo))
}
