// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/media/logging"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
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

	// Primarily used in stress testing to not reset Chrome state between each test runs.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoStress",
		Desc:     "Logged into a user session with logging enabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: &chromeVideoStressImpl{
			browserType: browser.TypeAsh,
			fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return []chrome.Option{
					chrome.ExtraArgs(chromeVideoArgs...),
					chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				}, nil
			},
		},
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoOOPVD",
		Desc:     "Logged into a user session with logging and out-of-process video decoding enabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoDecoding"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoOOPVDAndDedicatedDecoderThread",
		Desc:     "Logged into a user session with logging and out-of-process video decoding enabled and a dedicated decoder thread for hardware video decoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoDecoding"),
				chrome.EnableFeatures("UseDedicatedDecoderThreadInVideoDecoderProcess"),
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

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoLacros",
		Desc:     "Logged into a user session with logging enabled (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// Same as chromeVideoLacros but used for stress testing.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoStressLacros",
		Desc:     "Logged into a user session with logging enabled (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: &chromeVideoStressImpl{
			browserType: browser.TypeLacros,
			fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
					chrome.ExtraArgs(chromeVideoArgs...),
					chrome.LacrosExtraArgs(chromeVideoArgs...),
					chrome.ExtraArgs(chromeBypassPermissionsArgs...),
					chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...))).Opts()
			},
		},
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
		Name:     "chromeVideoLacrosNaCl",
		Desc:     "Logged into a user session with logging, NaCl and the MojoVideoDecoder-for-Pepper enabled (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs("--enable-nacl"),
				chrome.LacrosExtraArgs("--enable-nacl"))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoLacrosNaClWithSWDecoding",
		Desc:     "Similar to chromeVideoNaClWithMojoVideoDecoder but making sure Chrome does not use any potential hardware accelerated decoding (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs("--enable-nacl"),
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
				chrome.LacrosExtraArgs("--enable-nacl"),
				chrome.LacrosExtraArgs("--disable-accelerated-video-decode"))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeCameraPerfLacros",
		Desc:     "Logged into a user session on Lacros without verbose logging that can affect the performance",
		Contacts: []string{"chromeos-camera-eng@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeSuppressNotificationsArgs...),
				chrome.LacrosExtraArgs(chromeSuppressNotificationsArgs...))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
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
		Name:     "chromeVideoOOPVDWithGuestLogin",
		Desc:     "Like chromeVideoWithGuestLogin but with out-of-process video decoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoDecoding"),
				chrome.GuestLogin(),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGuestLoginLacros",
		Desc:     "Similar to chromeVideo fixture but forcing login as a guest (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.GuestLogin())).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
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
		Name:     "chromeVideoWithGlobalVaapiLockDisabled",
		Desc:     "Similar to chromeVideo fixture but the global VA-API lock is disabled if applicable",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
				chrome.ExtraArgs("--disable-features=LimitConcurrentDecoderInstances"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGlobalVaapiLockDisabledAndMediaServiceSequence",
		Desc:     "Similar to chromeVideo fixture but the global VA-API lock is disabled if applicable and use SequencedTaskRunner for MediaService",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
				chrome.ExtraArgs("--disable-features=LimitConcurrentDecoderInstances"),
				chrome.ExtraArgs("--enable-features=UseSequencedTaskRunnerForMediaService"),
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
		Name:     "chromeAshCompositedVideoLacros",
		Desc:     "Similar to chromeVideoLacros fixture but disabling hardware overlays in ash-chrome entirely to force video to be composited",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--enable-hardware-overlays=\"\""))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLacrosCompositedVideoLacros",
		Desc:     "Similar to chromeVideoLacros fixture but disabling hardware overlays in lacros-chrome entirely to force video to be composited",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs("--enable-hardware-overlays=\"\""))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcam",
		Desc:     "Similar to chromeVideo fixture but supplementing it with the use of a fake video/audio capture device (a.k.a. 'fake webcam'), see https://webrtc.org/testing/",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoOOPVDWithFakeWebcam",
		Desc:     "Like chromeVideoWithFakeWebcam but with out-of-process video decoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoDecoding"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoLacrosWithFakeWebcam",
		Desc:     "Similar to chromeVideo fixture but supplementing it with the use of a fake video/audio capture device (a.k.a. 'fake webcam'), see https://webrtc.org/testing/ (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndV4L2FlatStatefulDecoder",
		Desc:     "Similar to chromeVideoWithFakeWebcam fixture but using the V4L2 Flat stateful VD",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.EnableFeatures("V4L2FlatStatefulVideoDecoder"),
				chrome.EnableFeatures("UseChromeOSDirectVideoDecoder"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/248528896): Remove once out-of-process video encoding is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndOOPVE",
		Desc:     "Similar to chromeVideoWithFakeWebcam fixture but using the out-of-process video encoder",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoEncoding"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/248528896): Remove once both out-of-process video decoding and encoding are enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndOOPVDAndOOPVE",
		Desc:     "Similar to chromeVideoWithFakeWebcam fixture but using the out-of-process video decoder and encoder",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoDecoding"),
				chrome.EnableFeatures("UseOutOfProcessVideoEncoding"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/236546408): Remove once hardware variable bitrate encoding is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndHWVBREncoding",
		Desc:     "Similar to chromeVideoWebCam but enabling hardware VBR encoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs("--enable-features=ChromeOSHWVBREncoding"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndNoHwAcceleration",
		Desc:     "Similar to chromeVideoWithFakeWebcam fixture but with both hardware decoding and encoding disabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
				chrome.ExtraArgs("--disable-accelerated-video-encode"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndSWEncoding",
		Desc:     "Similar to chromeVideoWithFakeWebcam fixture but hardware encoding disabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs("--disable-accelerated-video-encode"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndGlobalVaapiLockDisabled",
		Desc:     "Similar to chromeVideoWithFakeWebcam fixture but the global VA-API lock is disabled if applicable",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndZeroLatencyRtc",
		Desc:     "Similar to chromeVideo fixture but supplementing it with the use of a fake video/audio capture device (a.k.a. 'fake webcam'), see https://webrtc.org/testing/, and the webrtc rendering smoothness algorithm disabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcam60fpsArgs...),
				chrome.ExtraArgs("--disable-rtc-smoothness-algorithm"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoLacrosWithFakeWebcamAndZeroLatencyRtc",
		Desc:     "Similar to chromeVideo fixture but supplementing it with the use of a fake video/audio capture device (a.k.a. 'fake webcam'), see https://webrtc.org/testing/, and the webrtc rendering smoothness algorithm disabled (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeFakeWebcam60fpsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs("--disable-rtc-smoothness-algorithm"))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeScreenCapture",
		Desc:     "Logged into a user session with flag so that Chrome always picks the entire screen for getDisplayMedia(), bypassing the picker UI",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(`--auto-select-desktop-capture-source=display`),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeWindowCapture",
		Desc:     "Logged into a user session with flag so that Chrome always picks the Chromium window for getDisplayMedia(), bypassing the picker UI",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(`--auto-select-window-capture-source-by-title=test`),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeTabCapture",
		Desc:     "Logged into a user session with flag so that Chrome always picks the current tab for getDisplayMedia(), bypassing the picker UI",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				// Chrome automatically selects a tab page whose title contains "test".
				chrome.ExtraArgs("--auto-select-tab-capture-source-by-title=test"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeTabCaptureWithOOPVDAndSWEncoding",
		Desc:     "Like chromeTabCapture but with out-of-process video decoding (OOP-VD) and software encoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				// Chrome automatically selects a tab page whose title contains "test".
				chrome.ExtraArgs("--auto-select-tab-capture-source-by-title=test"),
				chrome.ExtraArgs("--disable-accelerated-video-encode"),
				chrome.EnableFeatures("UseOutOfProcessVideoDecoding"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeTabCaptureWithLacrosAndSWEncoding",
		Desc:     "Like chromeTabCapture but with LaCrOS and software encoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				// Chrome automatically selects a tab page whose title contains "test".
				chrome.LacrosExtraArgs("--auto-select-tab-capture-source-by-title=test"),
				chrome.LacrosExtraArgs("--disable-accelerated-video-encode"))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeZeroCopyScreenCapture",
		Desc:     "Logged into a user session with flag so that Chrome always picks the entire screen for getDisplayMedia(), bypassing the picker UI",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(`--auto-select-desktop-capture-source=display`),
				chrome.ExtraArgs("--enable-features=ZeroCopyTabCapture"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeZeroCopyWindowCapture",
		Desc:     "Logged into a user session with flag so that Chrome always picks the Chromium window for getDisplayMedia(), bypassing the picker UI",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(`--auto-select-window-capture-source-by-title=test`),
				chrome.ExtraArgs("--enable-features=ZeroCopyTabCapture"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeZeroCopyTabCapture",
		Desc:     "Logged into a user session with flag so that Chrome always picks the current tab for getDisplayMedia(), bypassing the picker UI",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				// Chrome automatically selects a tab page whose title contains "test".
				chrome.ExtraArgs("--auto-select-tab-capture-source-by-title=test"),
				chrome.ExtraArgs("--enable-features=ZeroCopyTabCapture"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeScreenCaptureLacros",
		Desc:     "Logged into a user session with flag so that Chrome always picks the entire screen for getDisplayMedia(), bypassing the picker UI (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs("--auto-select-desktop-capture-source=Entire screen"))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeWindowCaptureLacros",
		Desc:     "Logged into a user session with flag so that Chrome always picks the Chromium window for getDisplayMedia(), bypassing the picker UI (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs("--auto-select-window-capture-source-by-title=test"))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeTabCaptureLacros",
		Desc:     "Logged into a user session with flag so that Chrome always picks the current tab for getDisplayMedia(), bypassing the picker UI (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs("--auto-select-tab-capture-source-by-title=test"))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
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

	// TODO(b/236546408): Remove once hardware variable bitrate encoding is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithHWVBREncoding",
		Desc:     "Similar to chromeVideo but also enables hardware VBR encoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--enable-features=ChromeOSHWVBREncoding"),
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
		Name:     "chromeCameraPerf",
		Desc:     "Logged into a user session with camera tests-specific setting and without verbose logging that can affect the performance. This fixture should be used only for performance tests",
		Contacts: []string{"chromeos-camera-eng@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeSuppressNotificationsArgs...),
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
		Name:     "chromeVideoOOPVDWithDistinctiveIdentifier",
		Desc:     "Like chromeVideoWithDistinctiveIdentifier but with out-of-process video decoding",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoDecoding"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoLacrosWithDistinctiveIdentifier",
		Desc:     "Like chromeVideoWithDistinctiveIdentifier, but with lacros",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.LacrosExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
				chrome.LacrosExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...))).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGlobalVaapiLockDisabledAndOneDedicatedThreadSharedByAllDecoders",
		Desc:     "Similar to chromeVideoWithGlobalVaapiLockDisabled but use a single decoder specific thread for all hardware decoders",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
				chrome.ExtraArgs("--disable-features=LimitConcurrentDecoderInstances"),
				chrome.ExtraArgs("--chromeos-video-decoder-task-runner=OneDedicatedThreadSharedByAllDecoders"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGlobalVaapiLockDisabledAndOneDedicatedThreadSharedByAllDecodersAndMediaServiceSequence",
		Desc:     "Similar to chromeVideoWithGlobalVaapiLockDisabled but use a single decoder specific thread for all hardware decoders and use SequencedTaskRunner for MediaService",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
				chrome.ExtraArgs("--disable-features=LimitConcurrentDecoderInstances"),
				chrome.ExtraArgs("--chromeos-video-decoder-task-runner=OneDedicatedThreadSharedByAllDecoders"),
				chrome.ExtraArgs("--enable-features=UseSequencedTaskRunnerForMediaService"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGlobalVaapiLockDisabledAndOneThreadPoolThreadSharedByAllDecoders",
		Desc:     "Similar to chromeVideoWithGlobalVaapiLockDisabled but use a SingleThreadTaskRunner obtained from a ThreadPool for all hardware decoders",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
				chrome.ExtraArgs("--disable-features=LimitConcurrentDecoderInstances"),
				chrome.ExtraArgs("--chromeos-video-decoder-task-runner=OneThreadPoolThreadSharedByAllDecoders"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGlobalVaapiLockDisabledAndOneThreadPoolThreadSharedByAllDecodersAndMediaServiceSequence",
		Desc:     "Similar to chromeVideoWithGlobalVaapiLockDisabled but use a SingleThreadTaskRunner obtained from a ThreadPool for all hardware decoders and use SequencedTaskRunner for MediaService",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
				chrome.ExtraArgs("--disable-features=LimitConcurrentDecoderInstances"),
				chrome.ExtraArgs("--chromeos-video-decoder-task-runner=OneThreadPoolThreadSharedByAllDecoders"),
				chrome.ExtraArgs("--enable-features=UseSequencedTaskRunnerForMediaService"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGlobalVaapiLockDisabledAndOneThreadPoolSequenceSharedByAllDecoders",
		Desc:     "Similar to chromeVideoWithGlobalVaapiLockDisabled but use a SequencedTaskRunner obtained from a ThreadPool for all hardware decoders",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
				chrome.ExtraArgs("--disable-features=LimitConcurrentDecoderInstances"),
				chrome.ExtraArgs("--chromeos-video-decoder-task-runner=OneThreadPoolSequenceSharedByAllDecoders"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithGlobalVaapiLockDisabledAndOneThreadPoolSequenceSharedByAllDecodersAndMediaServiceSequence",
		Desc:     "Similar to chromeVideoWithGlobalVaapiLockDisabled but use a SequencedTaskRunner obtained from a ThreadPool for all hardware decoders and use SequencedTaskRunner for MediaService",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
				chrome.ExtraArgs("--disable-features=LimitConcurrentDecoderInstances"),
				chrome.ExtraArgs("--chromeos-video-decoder-task-runner=OneThreadPoolSequenceSharedByAllDecoders"),
				chrome.ExtraArgs("--enable-features=UseSequencedTaskRunnerForMediaService"),
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
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithV4L2FlatStatefulDecoder",
		Desc:     "Similar to chromeVideo fixture but enabling V4L2 Flat stateful decoder",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeVideoArgs...),
				chrome.EnableFeatures("V4L2FlatStatefulVideoDecoder"),
				chrome.EnableFeatures("UseChromeOSDirectVideoDecoder"),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeRTCPerf",
		Desc:     "Logged into a user session with rtc performance settings",
		Contacts: []string{"chromeos-rtc@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs([]string{
					// Do not show message center notifications.
					"--suppress-message-center-popups",
					// Disable ARC++.
					"--arc-availability=none",
					// Disable firmware update to stop chrome from executing fwupd that restarts powerd.
					"--disable-features=FirmwareUpdaterApp",
					// Avoid the need to grant camera/microphone permissions.
					"--auto-accept-camera-and-microphone-capture",
					// Chrome automatically selects a tab page whose title contains "test".
					"--auto-select-tab-capture-source-by-title=test",
					// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
					// so that each test run does not remember info from last test run.
					"--disable-sync",
					// Allow 2 windows side by side.
					"--force-tablet-mode=clamshell",
				}...),
				chrome.ExtraArgs(chromeWebRTCEncodedFrameArgs...),
				chrome.EnableFeatures(
					// Prefer using constant frame rate for camera streaming.
					"PreferConstantFrameRate",
				),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

var chromeVideoArgs = []string{
	// Enable verbose log messages for video components.
	"--vmodule=" + strings.Join([]string{
		"*/media/gpu/chromeos/*=2",
		"*/media/gpu/vaapi/*=2",
		"*/media/gpu/v4l2/*=2"}, ","),
	// The Renderer video stack might have a policy of not using hardware
	// accelerated decoding for certain small resolutions (see crbug.com/684792).
	// Disable that for testing.
	"--disable-features=ResolutionBasedDecoderPriority",
	// VA-API HW decoder and encoder might reject small resolutions for
	// performance (see crbug.com/1008491 and b/171041334).
	// Disable that for testing.
	"--disable-features=VaapiEnforceVideoMinMaxResolution",
	"--disable-features=VaapiVideoMinResolutionForPerformance",
	// Allow media autoplay. <video> tag won't automatically play upon loading the source unless this flag is set.
	"--autoplay-policy=no-user-gesture-required",
	// Do not show message center notifications.
	"--suppress-message-center-popups",
	// Make sure ARC++ is not running.
	"--arc-availability=none",
	// Disable firmware update to stop chrome from executing fwupd that restarts powerd.
	"--disable-features=FirmwareUpdaterApp",
	// Ignore the list of blocked per-GPU functionality (e.g. VP8 accelerated
	// decoding on Intel Jasper Lake).
	"--disable-gpu-driver-bug-workarounds",
}

var chromeBypassPermissionsArgs = []string{
	// Avoid the need to grant camera/microphone permissions.
	"--use-fake-ui-for-media-stream",
}

var chromeSuppressNotificationsArgs = []string{
	// Do not show message center notifications.
	"--suppress-message-center-popups"}

var chromeFakeWebcamArgs = []string{
	// Use a fake media capture device instead of live webcam(s)/microphone(s).
	"--use-fake-device-for-media-stream",
	// Avoid the need to grant camera/microphone permissions.
	"--use-fake-ui-for-media-stream"}

var chromeFakeWebcam60fpsArgs = []string{
	// Use a fake media capture device with 60fps instead of live webcam(s)/microphone(s).
	"--use-fake-device-for-media-stream=fps=60",
	// Avoid the need to grant camera/microphone permissions.
	"--use-fake-ui-for-media-stream"}

var chromeAllowDistinctiveIdentifierArgs = []string{
	// Allows distinctive identifier with DRM playback when in dev mode. We don't
	// actually use RA for this, but it correlates to the same flag.
	"--allow-ra-in-dev-mode",
	// Prevents showing permission prompt and automatically grants permission to
	// allow a distinctive identifier for localhost which is where we server the
	// DRM content from in the test.
	"--unsafely-allow-protected-media-identifier-for-domain=127.0.0.1"}

var chromeWebRTCEncodedFrameArgs = []string{
	"--enable-blink-features=RTCEncodedFrameSetMetadata,RTCEncodedVideoFrameAdditionalMetadata,RTCEncodedVideoFrameClone",
	"--enable-features=AllowRTCEncodedVideoFrameSetMetadataAllFields",
}

type chromeVideoStressImpl struct {
	browserType             browser.Type
	fOpt                    chrome.OptionsCallback
	resetChromeBetweenTests bool

	logMarker         *logsaver.Marker // Marker for per-test log.
	origShelfBehavior ash.ShelfBehavior
	vl                *logging.VideoLogger
	cr                *chrome.Chrome
}

func (f *chromeVideoStressImpl) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if f.resetChromeBetweenTests {
		if err := f.cr.ResetState(ctx); err != nil {
			return errors.Wrap(err, "failed resetting existing Chrome session")
		}
	}
	return nil
}

func (f *chromeVideoStressImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
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

func (f *chromeVideoStressImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.logMarker != nil {
		if err := f.logMarker.Save(filepath.Join(s.OutDir(), "chrome.log")); err != nil {
			s.Log("Failed to store per-test log data: ", err)
		}
		f.logMarker = nil
	}
}

func (f *chromeVideoStressImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	crOpts, err := f.fOpt(ctx, s)
	if err != nil {
		s.Fatal("Failed to obtain Chrome options: ", err)
	}

	cr, err := chrome.New(ctx, crOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.cr = cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Set shelf to auto-hide.
	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get primary display info: ", err)
	}
	origShelfBehavior, err := ash.GetShelfBehavior(ctx, tconn, dispInfo.ID)
	if err != nil {
		s.Fatal("Failed to get shelf behavior: ", err)
	}
	f.origShelfBehavior = origShelfBehavior
	if err := ash.SetShelfBehavior(ctx, tconn, dispInfo.ID, ash.ShelfBehaviorAlwaysAutoHide); err != nil {
		s.Fatal("Failed to set shelf behavior to Never Auto Hide: ", err)
	}

	// video Logger and mute the devices.
	vl, err := logging.NewVideoLogger()
	if err != nil {
		s.Fatal("Failed to set values for verbose logging")
	}
	f.vl = vl
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute device: ", err)
	}
	chrome.Lock()
	return cr
}

func (f *chromeVideoStressImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()

	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	// Set shelf to auto-hide.
	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get primary display info: ", err)
	}
	if err := ash.SetShelfBehavior(ctx, tconn, dispInfo.ID, f.origShelfBehavior); err != nil {
		s.Fatal("Failed to set shelf behavior to Never Auto Hide: ", err)
	}

	if f.vl != nil {
		f.vl.Close()
	}
	crastestclient.Unmute(ctx)

	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}
