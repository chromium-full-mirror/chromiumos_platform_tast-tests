// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/play"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type playDrmParams struct {
	fileName    string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayDRM,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks HW protected DRM video playback in Chrome is working",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"jkardatzke@google.com",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome", "protected_content"},
		Params: []testing.Param{{
			Name: "cencv1_h264_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv1_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv1_ctr.mp4", "tulip_audio_aac_cencv1_ctr.mp4", "tulip_480p_h264_cencv1_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV1H264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv1_h264_ctr_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv1_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv1_ctr.mp4", "tulip_audio_aac_cencv1_ctr.mp4", "tulip_480p_h264_cencv1_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV1H264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv1_h264_ctr_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv1_ctr.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv1_ctr.mp4", "tulip_audio_aac_cencv1_ctr.mp4", "tulip_480p_h264_cencv1_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV1H264, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv1_h264_multislice_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_multislice_cencv1_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_multislice_cencv1_ctr.mp4", "tulip_audio_aac_cencv1_ctr.mp4", "tulip_480p_h264_multislice_cencv1_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV1H264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_h264_cbc",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_h264_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3H264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_h264_cbc_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_h264_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3H264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_h264_cbc_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv3_cbc.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_h264_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3H264, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_h264_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_h264_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3H264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_h264_ctr_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_h264_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3H264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_h264_ctr_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv3_ctr.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_h264_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3H264, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_h264_cbc_then_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv3_cbc_then_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_h264_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_h264_cencv3_cbc_then_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3H264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_h264_cbc_then_ctr_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_480p_h264_cencv3_cbc_then_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_h264_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_h264_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_h264_cencv3_cbc_then_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3H264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc_cbc",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_hevc_cencv3_cbc.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc_cbc_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_hevc_cencv3_cbc.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc_cbc_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc_cencv3_cbc.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_hevc_cencv3_cbc.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3HEVC, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_hevc_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_hevc_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc_ctr_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_hevc_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc_ctr_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc_cencv3_ctr.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_hevc_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_hevc10_cbc",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc10_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc10_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_hevc10_cencv3_cbc.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc10_cbc_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc10_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc10_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_hevc10_cencv3_cbc.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc10_cbc_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc10_cencv3_cbc.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc10_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_hevc10_cencv3_cbc.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3HEVC, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_hevc10_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc10_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc10_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_hevc10_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc10_ctr_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc10_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc10_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_hevc10_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc10_ctr_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_hevc10_cencv3_ctr.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_hevc10_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_hevc10_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_hevc10_4k_ctr",
			Val: playDrmParams{
				fileName:    "tulip_4k_hevc10_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_4k_hevc10_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_4k_hevc10_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc10_4k_ctr_inpvd",
			Val: playDrmParams{
				fileName:    "tulip_4k_hevc10_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_4k_hevc10_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_4k_hevc10_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_hevc10_4k_ctr_lacros",
			Val: playDrmParams{
				fileName:    "tulip_4k_hevc10_cencv3_ctr.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_4k_hevc10_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_4k_hevc10_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_vp9_cbc",
			Val: playDrmParams{
				fileName:    "tulip_480p_vp9_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_vp9_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_vp9_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3VP9, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_vp9_cbc_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_vp9_cencv3_cbc.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_vp9_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_vp9_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3VP9, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_vp9_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_vp9_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_vp9_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_vp9_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3VP9, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_vp92_cbc",
			Val: playDrmParams{
				fileName:    "tulip_480p_vp92_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_vp92_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_vp92_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3VP9, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_vp92_cbc_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_vp92_cencv3_cbc.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_vp92_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_vp92_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3VP9, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_vp92_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_vp92_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_vp92_cencv3_ctr.mp4", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_vp92_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3VP9, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_av1_cbc",
			Val: playDrmParams{
				fileName:    "tulip_480p_av1_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_av1_cencv3_cbc.webm", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_av1_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3AV1, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_av1_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_av1_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_av1_cencv3_ctr.webm", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_av1_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3AV1, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_av1_ctr_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_av1_cencv3_ctr.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_av1_cencv3_ctr.webm", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_av1_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3AV1, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}, {
			Name: "cencv3_av1_10bit_cbc",
			Val: playDrmParams{
				fileName:    "tulip_480p_av1_10bit_cencv3_cbc.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_av1_10bit_cencv3_cbc.mp4", "tulip_audio_aac_cencv3_cbc.mp4", "tulip_480p_av1_10bit_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3AV1, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_av1_10bit_ctr",
			Val: playDrmParams{
				fileName:    "tulip_480p_av1_10bit_cencv3_ctr.mpd",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_av1_10bit_cencv3_ctr.webm", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_av1_10bit_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3AV1, "proprietary_codecs"},
			Fixture:           "chromeVideoWithDistinctiveIdentifier",
		}, {
			Name: "cencv3_av1_10bit_ctr_lacros",
			Val: playDrmParams{
				fileName:    "tulip_480p_av1_10bit_cencv3_ctr.mpd",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.DRMDataFiles(), "tulip_480p_av1_10bit_cencv3_ctr.webm", "tulip_audio_aac_cencv3_ctr.mp4", "tulip_480p_av1_10bit_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3AV1, "proprietary_codecs"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI, pre.VideoFeatureDistinctiveIdentifier),
		}},
	})
}

// PlayDRM plays a given file in Chrome and verifies that the playback happens
// correctly and that a screenshot of the video will be solid black (which is
// another indicator of HW DRM). This will use the Shaka player to playback a
// Widevine DRM protected MPD video via MSE/EME.
func PlayDRM(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(playDrmParams)

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), testOpt.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	const unmutePlayer = false

	if err := play.TestPlay(ctx, s, cs, cr, testOpt.fileName, play.DRMVideo, play.VerifyHWDRMUsed, unmutePlayer); err != nil {
		s.Fatal("TestPlay failed: ", err)
	}
}
