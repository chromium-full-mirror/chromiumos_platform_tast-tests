// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// overlay defines the parameters for a HW overlay (a.k.a. DRM plane).
type overlay struct {
	format string
	size   string
}

// overlaysTestParam defines the overlays structure for a test case.
type overlaysTestParam struct {
	primaryFormats []string
	overlay        overlay
}

// Only Intel SoCs with GPU Gen 11 (JSL), Gen 12 (TGL, RLK) or later support
// P010 overlays - this list is SoCs with previous GPUs.
var preIntelGen11GPUSoCs = []string{"pinetrail", "broadwell", "apollolake", "skylake", "kabylake", "cometlake", "geminilake", "whiskeylake"}

// Intel SoCs with GPU Gen 9 (SKL) or earlier don't support scanning out 30bpp framebuffers.
var intelGen9AndEarlierGPUSoCs = []string{"pinetrail", "broadwell", "apollolake", "skylake"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlatformOverlays,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that certain configurations of primary and overlay planes are indeed supported",
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		Attr:         []string{"group:graphics", "graphics_perbuild"},
		SoftwareDeps: []string{"video_overlays", "no_qemu"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      time.Minute,
		Params: []testing.Param{{
			Name: "24bpp",
			Val: overlaysTestParam{
				primaryFormats: []string{"XR24", "XB24", "AR24", "AB24"},
			},
		}, {
			Name: "30bpp",
			Val: overlaysTestParam{
				primaryFormats: []string{"AR30", "AB30", "XR30", "XB30"},
			},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipCPUSocFamily("mediatek", "qualcomm"),
				hwdep.SkipGPUFamily(intelGen9AndEarlierGPUSoCs...),
				hwdep.SkipGPUFamily("stoney")),
		}, {
			Name: "24bpp_nv12_overlay",
			Val: overlaysTestParam{
				primaryFormats: []string{"XR24", "XB24", "AR24", "AB24"},
				overlay:        overlay{"NV12", "640x360"},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays(),
				// TODO(b/226417611): Reenable on AMD.
				hwdep.SkipGPUVendor("amd")),
		}, {
			Name: "30bpp_nv12_overlay",
			Val: overlaysTestParam{
				primaryFormats: []string{"AR30", "AB30", "XR30", "XB30"},
				overlay:        overlay{"NV12", "640x360"},
			},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SupportsNV12Overlays(),
				// 30 bpp scanout conditions.
				hwdep.SkipCPUSocFamily("mediatek", "qualcomm"),
				hwdep.SkipGPUFamily(intelGen9AndEarlierGPUSoCs...),
				hwdep.SkipGPUFamily("stoney"),
				// TODO(b/226417611): Reenable on AMD.
				hwdep.SkipGPUVendor("amd")),
		}, {
			Name: "24bpp_p010_overlay",
			Val: overlaysTestParam{
				primaryFormats: []string{"XR24", "XB24", "AR24", "AB24"},
				overlay:        overlay{"P010", "640x360"},
			},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipCPUSocFamily("mediatek", "qualcomm", "amd"),
				hwdep.SkipGPUFamily(preIntelGen11GPUSoCs...)),
		}, {
			Name: "30bpp_p010_overlay",
			Val: overlaysTestParam{
				primaryFormats: []string{"AR30", "AB30", "XR30", "XB30"},
				overlay:        overlay{"P010", "640x360"},
			},
			ExtraHardwareDeps: hwdep.D(
				// 30 bpp scanout conditions.
				hwdep.SkipCPUSocFamily("mediatek", "qualcomm"),
				hwdep.SkipGPUFamily(intelGen9AndEarlierGPUSoCs...),
				hwdep.SkipGPUFamily("stoney"),
				// P010 overlay scanout conditions.
				hwdep.SkipCPUSocFamily("mediatek", "qualcomm", "amd"),
				hwdep.SkipGPUFamily(preIntelGen11GPUSoCs...)),
		}},
		Fixture: "gpuWatchHangs",
	})
}

// PlatformOverlays runs plane_test binary test for a given format.
func PlatformOverlays(ctx context.Context, s *testing.State) {
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui job: ", err)
	}
	defer upstart.EnsureJobRunning(ctx, "ui")

	const testCommand string = "plane_test"

	f, err := os.Create(filepath.Join(s.OutDir(), filepath.Base(testCommand)+".txt"))
	if err != nil {
		s.Fatal("Failed to create a log file: ", err)
	}
	defer f.Close()

	const formatFlag string = "--format"
	primaryFormats := s.Param().(overlaysTestParam).primaryFormats
	overlayFormat := s.Param().(overlaysTestParam).overlay.format
	overlaySize := s.Param().(overlaysTestParam).overlay.size
	invocationError := make(map[string]error)

	for _, primaryFormat := range primaryFormats {
		params := []string{formatFlag, primaryFormat}
		if overlayFormat != "" {
			params = append(params, "--plane", formatFlag, overlayFormat, "--size", overlaySize)
		}

		invocationCommand := shutil.EscapeSlice(append([]string{testCommand}, params...))
		s.Log("Running ", invocationCommand)

		cmd := testexec.CommandContext(ctx, testCommand, params...)
		cmd.Stdout = f
		cmd.Stderr = f
		if err := cmd.Run(); err != nil {
			invocationError[invocationCommand] = err
		} else {
			// TODO(b/217970618): Parse the DRM response or debugfs to verify that the
			// actual plane combination is what was intended.
			s.Logf("Run succeeded for %s primary format", primaryFormat)

			// This test verifies if any of the primaryFormats is supported. This
			// emulates Chrome's behaviour, where we cannot know in advance if a
			// given format would work ahead of time (due to e.g. internal/external
			// display, format, dimensions etc).
			return
		}
	}

	s.Errorf("%s failed for all formats (Chrome must support at least one)", testCommand)
	for command, err := range invocationError {
		exitCode, ok := testexec.ExitCode(err)
		if !ok {
			s.Errorf("Failed to run %s: %v", command, err)
		} else {
			s.Errorf("Command %s exited with status %v", command, exitCode)
		}
	}
}
