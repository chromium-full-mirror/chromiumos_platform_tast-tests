// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/c2e2etest"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/video"
	"go.chromium.org/tast-tests/cros/local/media/videotype"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// Enable to cache the extracted raw video to speed up the test.
	veaCacheExtractedVideo = false
	// Enable to download the video file encoded by the test.
	veaPullEncodedVideo = false
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoEncodeAccel,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies ARC++ and ARCVM hardware encode acceleration by running the arcvideoencoder_test binary",
		Contacts:     []string{"chromeos-video-eng@google.com", "mwojtas@google.com"},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Video
		BugComponent: "b:632502",
		Attr:         []string{"group:arc-video"},
		Data:         []string{c2e2etest.X86ApkName, c2e2etest.ArmApkName},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithVideoLogging",
		Timeout:      12 * time.Minute,
		Params: []testing.Param{{
			Name: "h264_192p_i420",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Bear192P,
			},
			ExtraData:         []string{video.Bear192P.Name},
			ExtraSoftwareDeps: []string{"android_p", caps.HWEncodeH264},
		}, {
			Name: "h264_192p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Bear192P,
			},
			ExtraData:         []string{video.Bear192P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeH264},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(video.EncoderBlocklistVM...)),
		}, {
			Name: "h264_360p_i420",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Tulip360P,
			},
			ExtraData:         []string{video.Tulip360P.Name},
			ExtraSoftwareDeps: []string{"android_p", caps.HWEncodeH264},
		}, {
			Name: "h264_360p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Tulip360P,
			},
			ExtraData:         []string{video.Tulip360P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeH264},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(video.EncoderBlocklistVM...)),
		}, {
			Name: "h264_720p_i420",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Tulip720P,
			},
			ExtraData:         []string{video.Tulip720P.Name},
			ExtraSoftwareDeps: []string{"android_p", caps.HWEncodeH264},
		}, {
			Name: "h264_720p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Tulip720P,
			},
			ExtraData:         []string{video.Tulip720P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeH264},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(video.EncoderBlocklistVM...)),
		}, {
			Name: "h264_1080p_i420",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Crowd1080P,
			},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_p", caps.HWEncodeH264},
		}, {
			Name: "h264_1080p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Crowd1080P,
			},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeH264},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(video.EncoderBlocklistVM...)),
		}, {
			Name: "vp8_192p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP8Prof,
				Params:  video.Bear192P,
			},
			ExtraData:         []string{video.Bear192P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP8},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp8_360p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP8Prof,
				Params:  video.Tulip360P,
			},
			ExtraData:         []string{video.Tulip360P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP8},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp8_720p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP8Prof,
				Params:  video.Tulip720P,
			},
			ExtraData:         []string{video.Tulip720P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP8},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp8_1080p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP8Prof,
				Params:  video.Crowd1080P,
			},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP8},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp9_192p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP9Prof,
				Params:  video.Bear192P,
			},
			ExtraData:         []string{video.Bear192P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP9},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp9_360p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP9Prof,
				Params:  video.Tulip360P,
			},
			ExtraData:         []string{video.Tulip360P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP9},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp9_720p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP9Prof,
				Params:  video.Tulip720P,
			},
			ExtraData:         []string{video.Tulip720P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP9},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp9_1080p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP9Prof,
				Params:  video.Crowd1080P,
			},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP9},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}},
	})
}

func VideoEncodeAccel(ctx context.Context, s *testing.State) {
	video.RunARCVideoTest(ctx, s, s.FixtValue().(*arc.PreData).ARC,
		s.Param().(video.EncodeTestOptions), veaPullEncodedVideo, veaCacheExtractedVideo)
}
