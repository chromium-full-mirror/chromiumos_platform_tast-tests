// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"strings"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
)

func init() {
	initChromeVideoFixtures()
	initChromeVideoPowerFixtures()
	initChromeVideoStressFixtures()
	initChromeFakeWebCamFixtures()
	initChromeCameraPerfFixtures()
	initChromeCaptureFixtures()
	initChromeRTCFixtures()
}

// getChromeVideoOptions returns the base chrome.Options that Chrome is started with
// in most video-related tests (whether that happens or not depends on the specific
// fixture in use) plus extraOpts.
func getChromeVideoOptions(bt browser.Type, extraOpts ...chrome.Option) []chrome.Option {
	// IMPORTANT: do not add --enable-features or --disable-features to chromeVideoBaseArgs
	// as doing so may be problematic for lacros-chrome (see b/337315335). Instead, use
	// chromeVideoBaseEnabledFeatures and chromeVideoBaseDisabledFeatures.
	chromeVideoBaseArgs := []string{
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
	chromeVideoBaseEnabledFeatures := []string{
		// Enable hardware encoders frame drop in WebRTC.
		// TODO(b/324998907): Remove this once the feature is enabled by default.
		"WebRTCHardwareVideoEncoderFrameDrop",
	}
	chromeVideoBaseDisabledFeatures := []string{
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
	for _, a := range chromeVideoBaseArgs {
		if strings.HasPrefix(a, "--enable-features=") || strings.HasPrefix(a, "--disable-features=") {
			panic("Avoid --enable-features/--disable-features in chromeVideoBaseArgs; " +
				"use chromeVideoBaseEnabledFeatures/chromeVideoBaseDisabledFeatures instead (b/337315335)")
		}
	}
	options := []chrome.Option{
		chrome.ExtraArgs(chromeVideoBaseArgs...),
		chrome.EnableFeatures(chromeVideoBaseEnabledFeatures...),
		chrome.DisableFeatures(chromeVideoBaseDisabledFeatures...),
	}
	if bt == browser.TypeLacros {
		options = append(options,
			chrome.LacrosExtraArgs(chromeVideoBaseArgs...),
			chrome.LacrosEnableFeatures(chromeVideoBaseEnabledFeatures...),
			chrome.LacrosDisableFeatures(chromeVideoBaseDisabledFeatures...),
		)
	}
	return append(options, extraOpts...)
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
	"--enable-blink-features=RTCEncodedFrameSetMetadata,RTCEncodedVideoFrameAdditionalMetadata",
	"--enable-features=AllowRTCEncodedVideoFrameSetMetadataAllFields",
}
