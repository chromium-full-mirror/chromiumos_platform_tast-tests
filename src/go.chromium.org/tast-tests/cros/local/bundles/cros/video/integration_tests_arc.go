// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/media/arc/c2e2etest"
	arcvideo "go.chromium.org/tast-tests/cros/local/media/arc/video"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IntegrationTestsARC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies ARCVM/ARC++ hardware decode acceleration using a media::VideoDecoder by running the c2_e2e_test APK",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"pmolinalopez@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Data:         []string{c2e2etest.X86ApkName, c2e2etest.ArmApkName},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithVideoLoggingVD",
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{{
			Name:              "h264_vm",
			Val:               arcvideo.DecodeTestOptions{TestVideo: "test-25fps.h264"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "android_vm"},
			ExtraData:         []string{"test-25fps.h264", "test-25fps.h264.json"},
		}, {
			Name:              "vp8_vm",
			Val:               arcvideo.DecodeTestOptions{TestVideo: "test-25fps.vp8"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "android_vm"},
			ExtraData:         []string{"test-25fps.vp8", "test-25fps.vp8.json"},
		}, {
			Name:              "vp9_vm",
			Val:               arcvideo.DecodeTestOptions{TestVideo: "test-25fps.vp9"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "android_vm"},
			ExtraData:         []string{"test-25fps.vp9", "test-25fps.vp9.json"},
		}, {
			Name:              "h264_container_r",
			Val:               arcvideo.DecodeTestOptions{TestVideo: "test-25fps.h264"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "android_container_r"},
			ExtraData:         []string{"test-25fps.h264", "test-25fps.h264.json"},
		}},
	})
}

func IntegrationTestsARC(ctx context.Context, s *testing.State) {
	arcvideo.RunAllARCVideoTests(ctx, s, s.Param().(arcvideo.DecodeTestOptions))
}
