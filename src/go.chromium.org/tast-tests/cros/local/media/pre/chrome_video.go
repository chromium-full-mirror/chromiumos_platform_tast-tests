// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs("--enable-nacl"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs("--enable-nacl"),
				chrome.EnableFeatures("UseMojoVideoDecoderForPepper"),
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoEncoding"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.GuestLogin(),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
				chrome.GuestLogin(),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.EnableFeatures("UseHDRTransferFunction"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
				chrome.EnableFeatures("UseHDRTransferFunction"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.EnableFeatures("EnableIntelMediaCompression"),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.EnableFeatures("V4L2FlatStatefulVideoDecoder"),
				chrome.EnableFeatures("V4L2FlatVideoDecoder"),
				chrome.EnableFeatures("UseChromeOSDirectVideoDecoder"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("VideoDecodeBatching"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.DisableFeatures("RunVideoCaptureServiceInBrowserProcess"),
			}, nil
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
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("ReduceHardwareVideoDecoderBuffers"),
			}, nil
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
		comb(chromeVideo, FakeMediaStreamUI, NaCl),
		comb(chromeVideo, FakeMediaStreamUI, NaCl, SWDecoding),
		comb(chromeVideo, GuestLogin),
		comb(chromeVideo, AshComposited),
		comb(chromeVideo, LacrosComposited),
		comb(chromeVideo, FakeMediaStreamUI, DistinctiveIdentifier),
		comb(chromeVideo, FakeMediaStreamUI, VCDInUtilityProcess),
	}

	// TODO(b/337315335): ashAndLacrosVideoArgs is like chromeVideoArgs but
	// doesn't contain --disable-features or --enable-features. Once we remove
	// --disable-features/--enable-features from chromeVideoArgs, we can remove
	// ashAndLacrosVideoArgs in favor of chromeVideoArgs.
	ashAndLacrosVideoArgs := []string{
		// Enable verbose log messages for video components.
		"--vmodule=" + strings.Join([]string{
			"*/media/gpu/chromeos/*=2",
			"*/media/gpu/vaapi/*=2",
			"*/media/gpu/v4l2/*=2"}, ","),
		// Allow media autoplay. <video> tag won't automatically play upon loading the source unless this flag is set.
		"--autoplay-policy=no-user-gesture-required",
		// Do not show message center notifications.
		"--suppress-message-center-popups",
		// Make sure ARC++ is not running.
		"--arc-availability=none",
		// Ignore the list of blocked per-GPU functionality (e.g. VP8 accelerated
		// decoding on Intel Jasper Lake).
		"--disable-gpu-driver-bug-workarounds",
	}

	ashAndLacrosEnabledFeatures := []string{
		// Enable hardware encoders frame drop in WebRTC.
		// TODO(b/324998907): Remove this once the feature is enabled by default.
		"WebRTCHardwareVideoEncoderFrameDrop",
	}

	ashAndLacrosDisabledFeatures := []string{
		// The Renderer video stack might have a policy of not using hardware
		// accelerated decoding for certain small resolutions (see crbug.com/684792).
		// Disable that for testing.
		"ResolutionBasedDecoderPriority",
		// VA-API HW decoder and encoder might reject small resolutions for
		// performance (see crbug.com/1008491 and b/171041334).
		// Disable that for testing.
		"VaapiEnforceVideoMinMaxResolution",
		"VaapiVideoMinResolutionForPerformance",
		// Disable firmware update to stop chrome from executing fwupd that restarts powerd.
		"FirmwareUpdaterApp",
	}

	featureMap := map[featureType]featureInfo{
		chromeVideo: {
			"_",
			[]chrome.Option{
				chrome.ExtraArgs(ashAndLacrosVideoArgs...),
				chrome.LacrosExtraArgs(ashAndLacrosVideoArgs...),
				chrome.EnableFeatures(ashAndLacrosEnabledFeatures...),
				chrome.LacrosEnableFeatures(ashAndLacrosEnabledFeatures...),
				chrome.DisableFeatures(ashAndLacrosDisabledFeatures...),
				chrome.LacrosDisableFeatures(ashAndLacrosDisabledFeatures...),
			},
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
