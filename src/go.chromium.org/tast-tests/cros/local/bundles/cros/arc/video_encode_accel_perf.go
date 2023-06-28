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
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/media/videotype"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Enable to cache the extracted raw video to speed up the test.
const veapCacheExtractedVideo = false

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoEncodeAccelPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures ARC++ and ARCVM hardware video encode performance by running the arcvideoencoder_test binary",
		Contacts:     []string{"chromeos-video-eng@google.com", "arc-performance@google.com", "mwojtas@google.com"},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Video
		BugComponent: "b:632502",
		Data:         []string{c2e2etest.X86ApkName, c2e2etest.ArmApkName},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted", // TODO(akahuang): Implement new precondition to boot ARC and enable verbose at chromium.
		Timeout:      20 * time.Minute,
		Params: []testing.Param{{
			Name: "h264_1080p_i420",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Crowd1080P,
			},
			ExtraAttr:         []string{"group:arc-video", "group:crosbolt", "crosbolt_perbuild"},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_container", caps.HWEncodeH264},
		}, {
			Name: "h264_1080p_i420_sw",
			Val: video.EncodeTestOptions{
				Profile:     videotype.H264MainProf,
				Params:      video.Crowd1080P,
				EncoderType: video.SoftwareEncoder,
			},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_container", caps.HWEncodeH264},
		}, {
			Name: "h264_1080p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.H264MainProf,
				Params:  video.Crowd1080P,
			},
			ExtraAttr:         []string{"group:arc-video", "group:crosbolt", "crosbolt_perbuild"},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeH264},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnPlatform(video.EncoderBlocklistVM...)),
		}, {
			Name: "h264_1080p_i420_sw_vm",
			Val: video.EncodeTestOptions{
				Profile:     videotype.H264MainProf,
				Params:      video.Crowd1080P,
				EncoderType: video.SoftwareEncoder,
			},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name: "vp8_1080p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP8Prof,
				Params:  video.Crowd1080P,
			},
			ExtraAttr:         []string{"group:arc-video", "group:crosbolt", "crosbolt_perbuild"},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP8},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp8_1080p_i420_sw_vm",
			Val: video.EncodeTestOptions{
				Profile:     videotype.VP8Prof,
				Params:      video.Crowd1080P,
				EncoderType: video.SoftwareEncoder,
			},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name: "vp9_1080p_i420_vm",
			Val: video.EncodeTestOptions{
				Profile: videotype.VP9Prof,
				Params:  video.Crowd1080P,
			},
			ExtraAttr:         []string{"group:arc-video", "group:crosbolt", "crosbolt_perbuild"},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP9},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}, {
			Name: "vp9_1080p_i420_sw_vm",
			Val: video.EncodeTestOptions{
				Profile:     videotype.VP9Prof,
				Params:      video.Crowd1080P,
				EncoderType: video.SoftwareEncoder,
			},
			ExtraData:         []string{video.Crowd1080P.Name},
			ExtraSoftwareDeps: []string{"android_vm", caps.HWEncodeVP9},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipOnPlatform(video.EncoderBlocklistVM...),
				hwdep.Platform(video.EncoderAllowlistVPxVM...)),
		}},
	})
}

// preBenchmarkIdle returns the idle config that ensures CPU is idle before benchmarking.
// Note that this is more stringent than the idle.DefaultIdleConfig as we really want
// the system to be nice and quiet at the cost of waiting longer.
func preBenchmarkIdle() cpu.IdleConfig {
	idleConfig := cpu.DefaultIdleConfig()
	idleConfig.Timeout = 15 * time.Minute
	idleConfig.Steps = 15
	return idleConfig
}

func VideoEncodeAccelPerf(ctx context.Context, s *testing.State) {
	video.RunARCPerfVideoTest(ctx, s, s.FixtValue().(*arc.PreData).ARC,
		s.Param().(video.EncodeTestOptions), veapCacheExtractedVideo, preBenchmarkIdle())
}
