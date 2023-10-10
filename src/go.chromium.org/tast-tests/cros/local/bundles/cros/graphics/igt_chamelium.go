// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IgtChamelium,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies IGT Chamelium test binaries run successfully",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		// ChromeOS > Platform > Graphics > Display
		BugComponent: "b:188154",
		SoftwareDeps: []string{"drm_atomic", "igt", "no_qemu"},
		VarDeps:      []string{"graphics.chameleon_ip"},
		Attr:         []string{"group:graphics", "graphics_chameleon_igt"},
		Fixture:      "chromeGraphicsIgt",
		Params: []testing.Param{
			{
				Name: "kms_chamelium_audio_dp_audio_edid",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_audio",
					Subtests: []string{"dp-audio-edid"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_audio_hdmi_audio",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_audio",
					Subtests: []string{"hdmi-audio"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_audio_hdmi_audio_edid",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_audio",
					Subtests: []string{"hdmi-audio-edid"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_degamma",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"degamma"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_gamma",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"gamma"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_red_to_blue",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-red-to-blue"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_green_to_red",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-green-to-red"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_blue_to_red",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-blue-to-red"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_max",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-max"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_negative",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-negative"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_0_25",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-0-25"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_0_50",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-0-50"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_0_75",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-0-75"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_color_ctm_limited_range",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_color",
					Subtests: []string{"ctm-limited-range"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_dp_edid_read",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"dp-edid-read"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_dp_edid_stress_resolution_4k",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"dp-edid-stress-resolution-4k"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_dp_edid_stress_resolution_non_4k",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"dp-edid-stress-resolution-non-4k"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_dp_edid_resolution_list",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"dp-edid-resolution-list"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_dp_edid_change_during_suspend",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"dp-edid-change-during-suspend"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_dp_edid_change_during_hibernate",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"dp-edid-change-during-hibernate"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_dp_mode_timings",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"dp-mode-timings"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_hdmi_edid_read",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"hdmi-edid-read"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_hdmi_edid_stress_resolution_4k",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"hdmi-edid-stress-resolution-4k"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_hdmi_edid_stress_resolution_non_4k",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"hdmi-edid-stress-resolution-non-4k"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_hdmi_edid_change_during_suspend",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"hdmi-edid-change-during-suspend"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_hdmi_edid_change_during_hibernate",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"hdmi-edid-change-during-hibernate"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_edid_hdmi_mode_timings",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_edid",
					Subtests: []string{"hdmi-mode-timings"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_dp_crc_single",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"dp-crc-single"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_dp_crc_fast",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"dp-crc-fast"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_dp_crc_multiple",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"dp-crc-multiple"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_dp_frame_dump",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"dp-frame-dump"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_crc_single",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-crc-single"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_crc_fast",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-crc-fast"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_crc_multiple",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-crc-multiple"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_crc_nonplanar_formats",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-crc-nonplanar-formats"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_crc_planes_random",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-crc-planes-random"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_cmp_planar_formats",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-cmp-planar-formats"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_cmp_planes_random",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-cmp-planes-random"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_frame_dump",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-frame-dump"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_frames_hdmi_aspect_ratio",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_frames",
					Subtests: []string{"hdmi-aspect-ratio"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd_fast",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd-fast"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd_enable_disable_mode",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd-enable-disable-mode"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd_with_enabled_mode",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd-with-enabled-mode"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd_for_each_pipe",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd-for-each-pipe"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd_after_suspend",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd-after-suspend"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd_after_hibernate",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd-after-hibernate"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd_storm",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd-storm"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_dp_hpd_storm_disable",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"dp-hpd-storm-disable"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd_fast",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd-fast"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd_enable_disable_mode",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd-enable-disable-mode"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd_with_enabled_mode",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd-with-enabled-mode"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd_for_each_pipe",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd-for-each-pipe"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd_after_suspend",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd-after-suspend"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd_after_hibernate",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd-after-hibernate"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd_storm",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd-storm"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_hdmi_hpd_storm_disable",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"hdmi-hpd-storm-disable"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_common_hpd_after_suspend",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"common-hpd-after-suspend"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_chamelium_hpd_common_hpd_after_hibernate",
				Val: graphics.IgtTest{
					Exe:      "kms_chamelium_hpd",
					Subtests: []string{"common-hpd-after-hibernate"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
		},
	})
}

func setIgtrcFile(s *testing.State) {
	igtFilePath := "/tmp/.igtrc"
	igtFile, err := os.Create(igtFilePath)
	if err != nil {
		s.Fatal("Failed to create .igtrc: ", err)
	}
	defer igtFile.Close()

	// Get Chameleon IP
	// This is used for local dev env.
	url, err := graphics.ChameleonGetURL()
	if err != nil {
		s.Fatal("Failed to get the Chameleon URL: ", err)
	}

	content := `
[Common]
# The path to dump frames that fail comparison checks
FrameDumpPath=/tmp

[DUT]
SuspendResumeDelay=15

[Chamelium]
URL=` + url + `

`

	// Write config content to .igtrc
	_, err = igtFile.WriteString(content)
	if err != nil {
		s.Fatal("Failed to write to igtrc: ", err)
		return
	}

	// Set the file path as env variable for IGT to find it.
	os.Setenv("IGT_CONFIG_PATH", igtFilePath)

	s.Log("$IGT_CONFIG_PATH = ", igtFilePath)
	s.Log("Chameleon Device URL = ", url)
}

func IgtChamelium(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.IgtTest)
	f, err := os.Create(filepath.Join(s.OutDir(), filepath.Base(testOpt.Exe)+".txt"))
	if err != nil {
		s.Fatal("Failed to create a log file: ", err)
	}
	defer f.Close()

	setIgtrcFile(s)

	isExitErr, exitErr, err := graphics.IgtExecuteTests(ctx, testOpt, f)

	isError, outputLog := graphics.IgtProcessResults(testOpt, f, isExitErr, exitErr, err)

	if isError {
		s.Error(outputLog)
	} else {
		s.Log(outputLog)
	}
}
