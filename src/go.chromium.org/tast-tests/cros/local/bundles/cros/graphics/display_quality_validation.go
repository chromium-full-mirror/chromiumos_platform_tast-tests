// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisplayQualityValidation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Performs quality validation of the driver in its earlier stages. The list is based on intel-ci fast feedback testlist and AMD's basic validation testlist",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		// ChromeOS > Platform > Graphics > Display
		BugComponent: "b:188154",
		SoftwareDeps: []string{"drm_atomic", "igt", "no_qemu"},
		Attr:         []string{"group:graphics", "graphics_igt"},
		Fixture:      "chromeGraphicsIgt",
		Params: []testing.Param{
			{
				Name: "amdgpu",
				Val: graphics.IgtTest{
					Exe:      "amd_mode_switch",
					Subtests: []string{"mode-switch-first-last-pipe-0"},
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraSoftwareDeps: []string{"amd_cpu"},
			},

			{
				Name: "core_auth",
				Val: graphics.IgtTest{
					Exe:      "core_auth",
					Subtests: []string{"basic-auth"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_addfb_basic",
				Val: graphics.IgtTest{
					Exe: "kms_addfb_basic",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_atomic",
				Val: graphics.IgtTest{
					Exe: "kms_atomic",
					Subtests: []string{"plane-overlay-legacy",
						"plane-primary-legacy",
						"plane-cursor-legacy",
						"plane-invalid-params",
						"crtc-invalid-params",
						"atomic-invalid-params",
						"plane-invalid-params-fence",
						"crtc-invalid-params-fence",
						"test-only",
						"plane-immutable-zpos"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},

			{
				Name: "kms_atomic_interruptible",
				Val: graphics.IgtTest{
					Exe: "kms_atomic_interruptible",
					Subtests: []string{
						"legacy-setmode",
						"atomic-setmode",
						"legacy-dpms",
						"legacy-pageflip",
						"legacy-cursor",
						"universal-setplane-primary",
						"universal-setplane-cursor",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_async_flips",
				Val: graphics.IgtTest{
					Exe:      "kms_async_flips",
					Subtests: []string{"async-flip-with-page-flip-events", "alternate-sync-async-flip", "test-time-stamp", "test-cursor", "crc"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_busy",
				Val: graphics.IgtTest{
					Exe:              "kms_busy",
					Subtests:         []string{"basic"},
					DisableHangCheck: true,
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},

			{
				Name: "kms_bw",
				Val: graphics.IgtTest{
					Exe: "kms_bw",
					Subtests: []string{
						"linear-tiling-1-displays-1920x1080p",
						"linear-tiling-1-displays-2560x1440p",
						"linear-tiling-1-displays-3840x2160p",
						"linear-tiling-2-displays-1920x1080p",
						"linear-tiling-2-displays-2560x1440p",
						"linear-tiling-2-displays-3840x2160p",
						"linear-tiling-3-displays-1920x1080p",
						"linear-tiling-3-displays-2560x1440p",
						"linear-tiling-3-displays-3840x2160p",
						"linear-tiling-4-displays-1920x1080p",
						"linear-tiling-4-displays-2560x1440p",
						"linear-tiling-4-displays-3840x2160p",
						"linear-tiling-5-displays-1920x1080p",
						"linear-tiling-5-displays-2560x1440p",
						"linear-tiling-5-displays-3840x2160p",
						"linear-tiling-6-displays-1920x1080p",
						"linear-tiling-6-displays-2560x1440p",
						"linear-tiling-6-displays-3840x2160p",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_color",
				Val: graphics.IgtTest{
					Exe: "kms_color",
					Subtests: []string{
						"ctm-red-to-blue",
						"ctm-green-to-red",
						"ctm-blue-to-red",
						"ctm-max",
						"ctm-negative",
						"ctm-0-25",
						"ctm-0-50",
						"ctm-0-75",
						"gamma",
						"degamma",
						"legacy-gamma",
						"legacy-gamma-reset",
						"invalid-gamma-lut-sizes",
						"invalid-degamma-lut-sizes",
						"invalid-ctm-matrix-sizes",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_concurrent",
				Val: graphics.IgtTest{
					Exe: "kms_concurrent",
					Subtests: []string{
						"pipe-A",
						"pipe-B",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_content_protection",
				Val: graphics.IgtTest{
					Exe: "kms_concurrent",
					Subtests: []string{
						"atomic-dpms", "atomic", "legacy",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_cursor_crc",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_crc",
					Subtests: []string{
						"cursor-size-change",
						"cursor-alpha-opaque",
						"cursor-alpha-transparent",
						"cursor-dpms",
						"cursor-suspend",
						"cursor-onscreen-32x32",
						"cursor-offscreen-32x32",
						"cursor-onscreen-32x10",
						"cursor-offscreen-32x10",
						"cursor-onscreen-64x64",
						"cursor-offscreen-64x64",
						"cursor-onscreen-64x21",
						"cursor-offscreen-64x21",
						"cursor-onscreen-128x128",
						"cursor-offscreen-128x128",
						"cursor-onscreen-128x42",
						"cursor-offscreen-128x42",
						"cursor-onscreen-256x256",
						"cursor-offscreen-256x256",
						"cursor-onscreen-256x85",
						"cursor-offscreen-256x85",
						"cursor-onscreen-512x512",
						"cursor-offscreen-512x512",
						"cursor-onscreen-512x170",
						"cursor-offscreen-512x170",
						"cursor-onscreen-max-size",
						"cursor-offscreen-max-size",
						"cursor-sliding-32x32",
						"cursor-sliding-32x10",
						"cursor-sliding-64x64",
						"cursor-sliding-64x21",
						"cursor-sliding-128x128",
						"cursor-sliding-128x42",
						"cursor-sliding-256x256",
						"cursor-sliding-256x85",
						"cursor-sliding-512x512",
						"cursor-sliding-512x170",
						"cursor-sliding-max-size",
						"cursor-random-32x32",
						"cursor-random-32x10",
						"cursor-random-64x64",
						"cursor-random-64x21",
						"cursor-random-128x128",
						"cursor-random-128x42",
						"cursor-random-256x256",
						"cursor-random-256x85",
						"cursor-random-512x512",
						"cursor-random-512x170",
						"cursor-random-max-size",
						"cursor-rapid-movement-32x32",
						"cursor-rapid-movement-32x10",
						"cursor-rapid-movement-64x64",
						"cursor-rapid-movement-64x21",
						"cursor-rapid-movement-128x128",
						"cursor-rapid-movement-128x42",
						"cursor-rapid-movement-256x256",
						"cursor-rapid-movement-256x85",
						"cursor-rapid-movement-512x512",
						"cursor-rapid-movement-512x170",
						"cursor-rapid-movement-max-size",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_cursor_edge_walk",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_edge_walk",
					Subtests: []string{
						"left-edge",
						"right-edge",
						"top-edge",
						"top-bottom",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_cursor_legacy",
				Val: graphics.IgtTest{
					Exe: "kms_cursor_legacy",
					Subtests: []string{"basic-busy-flip-before-cursor-atomic",
						"basic-busy-flip-before-cursor-legacy",
						"basic-flip-after-cursor-atomic",
						"basic-flip-after-cursor-legacy",
						"basic-flip-after-cursor-varying-size",
						"basic-flip-before-cursor-atomic",
						"basic-flip-before-cursor-legacy",
						"basic-flip-before-cursor-varying-size",
						"flip-vs-cursor*",
						"cursor-vs-flip*"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_flip",
				Val: graphics.IgtTest{
					Exe: "kms_flip",
					Subtests: []string{"basic-flip-vs-dpms",
						"basic-flip-vs-modeset",
						"basic-flip-vs-wf_vblank",
						"basic-plain-flip",
						"basic-plain-flip",
						"basic-flip-vs-dpms",
						"flip-vs-dpms-off-vs-modeset"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_flip_event_leak",
				Val: graphics.IgtTest{
					Exe:      "kms_flip_event_leak",
					Subtests: []string{"basic"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_force_connector_basic",
				Val: graphics.IgtTest{
					Exe:      "kms_force_connector_basic",
					Subtests: []string{"force-connector-state", "force-edid", "force-load-detect", "prune-stale-modes"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_frontbuffer_tracking",
				Val: graphics.IgtTest{
					Exe:      "kms_frontbuffer_tracking",
					Subtests: []string{"basic"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_getfb",
				Val: graphics.IgtTest{
					Exe: "kms_getfb",
					Subtests: []string{
						"getfb-handle-valid",
						"getfb-handle-zero",
						"getfb2-handle-zero",
						"getfb-handle-closed",
						"getfb2-handle-closed",
						"getfb-handle-not-fb",
						"getfb2-handle-not-fb",
						"getfb-addfb-different-handles",
						"getfb-repeated-different-handles",
						"getfb-handle-protection",
						"getfb2-handle-protection",
						"getfb2-into-addfb2",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_panel_fitting",
				Val: graphics.IgtTest{
					Exe:      "kms_panel_fitting",
					Subtests: []string{"legacy", "atomic-fastset"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_pipe_crc_basic",
				Val: graphics.IgtTest{
					Exe: "kms_pipe_crc_basic",
					Subtests: []string{"bad-source",
						"compare-crc-sanitycheck",
						"hang-read-crc",
						"nonblocking-crc",
						"nonblocking-crc-frame-sequence",
						"nonblocking-crc-frame-sequence",
						"read-crc",
						"read-crc-frame-sequence",
						"disable-crc-after-crtc",
						"hang-read-crc",
						"compare-crc-sanitycheck",
						"suspend-read-crc"},
					DisableHangCheck: true,
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_plane",
				Val: graphics.IgtTest{
					Exe: "kms_plane",
					Subtests: []string{
						"plane-panning-top-left",
						"plane-panning-bottom-right",
						"plane-position-covered",
						"plane-position-hole",
						"plane-position-hole-dpms",
						"plane-panning-bottom-right-suspend",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_plane_alpha_blend",
				Val: graphics.IgtTest{
					Exe: "kms_plane_alpha_blend",
					Subtests: []string{
						"alpha-basic",
						"alpha-7efc",
						"coverage-7efc",
						"coverage-vs-premult-vs-constant",
						"alpha-transparent-fb",
						"alpha-opaque-fb",
						"constant-alpha-min",
						"constant-alpha-mid",
						"constant-alpha-max",
						"plane-panning-bottom-right-suspend",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_plane_cursor",
				Val: graphics.IgtTest{
					Exe: "kms_plane_cursor",
					Subtests: []string{
						"overlay*",
						"primary*",
						"viewport*",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_prop_blob",
				Val: graphics.IgtTest{
					Exe: "kms_prop_blob",
					Subtests: []string{"basic",
						"blob-prop-core",
						"blob-prop-validate",
						"blob-prop-lifetime",
						"blob-multiple",
						"invalid-get-prop-any",
						"invalid-get-prop",
						"invalid-set-prop-any",
						"invalid-set-prop"},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_properties",
				Val: graphics.IgtTest{
					Exe: "kms_properties",
					Subtests: []string{
						"plane-properties-legacy",
						"plane-properties-atomic",
						"crtc-properties-legacy",
						"crtc-properties-atomic",
						"connector-properties-legacy",
						"connector-properties-atomic",
						"invalid-properties-legacy",
						"invalid-properties-atomic",
						"get_properties-sanity-atomic",
						"get_properties-sanity-non-atomic",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_psr",
				Val: graphics.IgtTest{
					Exe: "kms_psr",
					Subtests: []string{
						"primary_page_flip",
						"cursor_plane_move",
						"sprite_plane_onoff",
						"primary_mmap_gtt",
					},
					IsSkipOk: true,
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_setmode",
				Val: graphics.IgtTest{
					Exe: "kms_setmode",
					Subtests: []string{
						"basic",
						"basic-clone-single-crtc",
						"invalid-clone-single-crtc",
						"invalid-clone-exclusive-crtc",
						"clone-exclusive-crtc",
						"invalid-clone-single-crtc-stealing",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},

			{
				Name: "kms_sysfs_edid_timing",
				Val: graphics.IgtTest{
					Exe: "kms_sysfs_edid_timing",
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_vblank",
				Val: graphics.IgtTest{
					Exe: "kms_vblank",
					Subtests: []string{
						"pipe-a-ts-continuation-suspend",
					},
				},
				Timeout:   5 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
			},
			{
				Name: "kms_vrr",
				Val: graphics.IgtTest{
					Exe: "kms_vrr",
					Subtests: []string{
						"flip-basic",
						"flip-dpms",
					},
				},
				Timeout:           5 * time.Minute,
				ExtraAttr:         []string{"graphics_nightly"},
				ExtraHardwareDeps: hwdep.D(hwdep.VRR()),
			},
		}})
}

func DisplayQualityValidation(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.IgtTest)
	if testOpt.DisableHangCheck {
		graphics.DisableHangCheck()
	}

	f, err := os.Create(filepath.Join(s.OutDir(), filepath.Base(testOpt.Exe)+".txt"))
	if err != nil {
		s.Fatal("Failed to create a log file: ", err)
	}
	defer f.Close()

	isExitErr, exitErr, err := graphics.IgtExecuteTests(ctx, testOpt, f)

	isError, outputLog := graphics.IgtProcessResults(testOpt, f, isExitErr, exitErr, err)

	if isError {
		s.Error(outputLog)
	} else {
		s.Log(outputLog)
	}
}
