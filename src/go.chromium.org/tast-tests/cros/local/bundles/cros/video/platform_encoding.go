// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/platform"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/videovars"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/media/encoding"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// regExpFPSVP8 is the regexp to find the FPS output from the VP8 binary log.
var regExpFPSVP8 = regexp.MustCompile(`Processed \d+ frames in \d+ ms \((\d+\.\d+) FPS\)`)

// regExpFPSVP9 is the regexp to find the FPS output from the VP9 binary log.
var regExpFPSVP9 = regexp.MustCompile(`encode \d+ frames in \d+.\d+ secondes, FPS is (\d+\.\d+)`)

// regExpFPSH264 is the regexp to find the FPS output from the H.264 binary log.
var regExpFPSH264 = regexp.MustCompile(`PERFORMANCE:\s+Frame Rate\s+: (\d+.\d+)`)

// regExpFPSAV1 is the regexp to find the FPS output from the AV1 binary log.
var regExpFPSAV1 = regExpFPSH264

// regExpFPSV4L2 is the regexp to find the FPS output from v4l2_stateful_encoder.
var regExpFPSV4L2 = regexp.MustCompile(`\((\d+\.\d+)fps\)`)

// regExpFPSVpxenc is the regexp to find the FPS output from vpxenc's log.
var regExpFPSVpxenc = regexp.MustCompile(`\((\d+\.\d+) fps\)`)

// regExpFPSAomenc is the regexp to find the FPS output from aomenc's log.
var regExpFPSAomenc = regexp.MustCompile(`\((\d+\.\d+) fps\)`)

// regExpFPSOpenh264enc is the regexp to find the FPS output from openh264enc's log.
var regExpFPSOpenh264enc = regexp.MustCompile(`(\d+\.\d+) fps`)

// testParam is used to describe the config used to run each test.
type testParam struct {
	command        string                          // The command path to be run. This should be relative to /usr/local/bin.
	filename       string                          // Input file name. This will be decoded to produce the uncompressed input to the encoder binary, so it can come in any format/container.
	size           coords.Size                     // Width x Height in pixels of the input file.
	numFrames      int                             // Number of frames of the input file.
	fps            float64                         // FPS of the input file.
	commandBuilder platform.CommandBuilderEncodeFn // Function to create the command line arguments.
	regExpFPS      *regexp.Regexp                  // Regexp to find the FPS from output.
	decoder        encoding.Decoder                // Command line decoder binary
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlatformEncoding,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Verifies platform encoding by using the libva-utils encoder binaries",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Fixture:      "graphicsNoChrome",
		Attr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
		// Guado, buddy and rikku have a companion video acceleration chip
		// (called Kepler), skip this test in these models.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("guado", "buddy", "rikku")),
		Params: []testing.Param{{
			Name: "vaapi_av1_180",
			Val: testParam{
				command:        "av1encode",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.AV1ArgsVAAPI,
				regExpFPS:      regExpFPSAV1,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData:         []string{"tulip2-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeAV1},
		}, {
			Name: "vaapi_av1_360",
			Val: testParam{
				command:        "av1encode",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.AV1ArgsVAAPI,
				regExpFPS:      regExpFPSAV1,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData:         []string{"tulip2-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeAV1},
		}, {
			Name: "vaapi_av1_720",
			Val: testParam{
				command:        "av1encode",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.AV1ArgsVAAPI,
				regExpFPS:      regExpFPSAV1,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData:         []string{"tulip2-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeAV1},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vaapi_av1_180_meet",
			Val: testParam{
				command:        "av1encode",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.AV1ArgsVAAPI,
				regExpFPS:      regExpFPSAV1,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData:         []string{"gipsrestat-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeAV1},
		}, {
			Name: "vaapi_av1_360_meet",
			Val: testParam{
				command:        "av1encode",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.AV1ArgsVAAPI,
				regExpFPS:      regExpFPSAV1,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData:         []string{"gipsrestat-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeAV1},
		}, {
			Name: "vaapi_av1_720_meet",
			Val: testParam{
				command:        "av1encode",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.AV1ArgsVAAPI,
				regExpFPS:      regExpFPSAV1,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData:         []string{"gipsrestat-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeAV1},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vaapi_vp8_180",
			Val: testParam{
				command:        "vp8enc",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.VP8ArgsVAAPI,
				regExpFPS:      regExpFPSVP8,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP8},
		}, {
			Name: "vaapi_vp8_360",
			Val: testParam{
				command:        "vp8enc",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.VP8ArgsVAAPI,
				regExpFPS:      regExpFPSVP8,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP8},
		}, {
			Name: "vaapi_vp8_720",
			Val: testParam{
				command:        "vp8enc",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.VP8ArgsVAAPI,
				regExpFPS:      regExpFPSVP8,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP8},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vaapi_vp8_180_meet",
			Val: testParam{
				command:        "vp8enc",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.VP8ArgsVAAPI,
				regExpFPS:      regExpFPSVP8,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP8},
		}, {
			Name: "vaapi_vp8_360_meet",
			Val: testParam{
				command:        "vp8enc",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.VP8ArgsVAAPI,
				regExpFPS:      regExpFPSVP8,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP8},
		}, {
			Name: "vaapi_vp8_720_meet",
			Val: testParam{
				command:        "vp8enc",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.VP8ArgsVAAPI,
				regExpFPS:      regExpFPSVP8,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP8},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vaapi_vp9_180",
			Val: testParam{
				command:        "vp9enc",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.VP9ArgsVAAPI,
				regExpFPS:      regExpFPSVP9,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP9},
		}, {
			Name: "vaapi_vp9_360",
			Val: testParam{
				command:        "vp9enc",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.VP9ArgsVAAPI,
				regExpFPS:      regExpFPSVP9,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP9},
		}, {
			Name: "vaapi_vp9_720",
			Val: testParam{
				command:        "vp9enc",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.VP9ArgsVAAPI,
				regExpFPS:      regExpFPSVP9,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP9},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vaapi_vp9_180_meet",
			Val: testParam{
				command:        "vp9enc",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.VP9ArgsVAAPI,
				regExpFPS:      regExpFPSVP9,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP9},
		}, {
			Name: "vaapi_vp9_360_meet",
			Val: testParam{
				command:        "vp9enc",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.VP9ArgsVAAPI,
				regExpFPS:      regExpFPSVP9,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP9},
		}, {
			Name: "vaapi_vp9_720_meet",
			Val: testParam{
				command:        "vp9enc",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.VP9ArgsVAAPI,
				regExpFPS:      regExpFPSVP9,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeVP9},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vaapi_h264_180",
			Val: testParam{
				command:        "h264encode",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.H264ArgsVAAPI,
				regExpFPS:      regExpFPSH264,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"tulip2-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeH264},
		}, {
			Name: "vaapi_h264_360",
			Val: testParam{
				command:        "h264encode",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.H264ArgsVAAPI,
				regExpFPS:      regExpFPSH264,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"tulip2-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeH264},
		}, {
			Name: "vaapi_h264_720",
			Val: testParam{
				command:        "h264encode",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.H264ArgsVAAPI,
				regExpFPS:      regExpFPSH264,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"tulip2-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeH264},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vaapi_h264_180_meet",
			Val: testParam{
				command:        "h264encode",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.H264ArgsVAAPI,
				regExpFPS:      regExpFPSH264,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"gipsrestat-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeH264},
		}, {
			Name: "vaapi_h264_360_meet",
			Val: testParam{
				command:        "h264encode",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.H264ArgsVAAPI,
				regExpFPS:      regExpFPSH264,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"gipsrestat-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeH264},
		}, {
			Name: "vaapi_h264_720_meet",
			Val: testParam{
				command:        "h264encode",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.H264ArgsVAAPI,
				regExpFPS:      regExpFPSH264,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"gipsrestat-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"vaapi", caps.HWEncodeH264},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vpxenc_vp8_180",
			Val: testParam{
				command:        "vpxenc",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"tulip2-320x180.vp9.webm"},
		}, {
			Name: "vpxenc_vp8_360",
			Val: testParam{
				command:        "vpxenc",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"tulip2-640x360.vp9.webm"},
		}, {
			Name: "vpxenc_vp8_720",
			Val: testParam{
				command:        "vpxenc",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"tulip2-1280x720.vp9.webm"},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vpxenc_vp8_180_meet",
			Val: testParam{
				command:        "vpxenc",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"gipsrestat-320x180.vp9.webm"},
		}, {
			Name: "vpxenc_vp8_360_meet",
			Val: testParam{
				command:        "vpxenc",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"gipsrestat-640x360.vp9.webm"},
		}, {
			Name: "vpxenc_vp8_720_meet",
			Val: testParam{
				command:        "vpxenc",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"gipsrestat-1280x720.vp9.webm"},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vpxenc_vp9_180",
			Val: testParam{
				command:        "vpxenc",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"tulip2-320x180.vp9.webm"},
		}, {
			Name: "vpxenc_vp9_360",
			Val: testParam{
				command:        "vpxenc",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"tulip2-640x360.vp9.webm"},
		}, {
			Name: "vpxenc_vp9_720",
			Val: testParam{
				command:        "vpxenc",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"tulip2-1280x720.vp9.webm"},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "vpxenc_vp9_180_meet",
			Val: testParam{
				command:        "vpxenc",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"gipsrestat-320x180.vp9.webm"},
		}, {
			Name: "vpxenc_vp9_360_meet",
			Val: testParam{
				command:        "vpxenc",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"gipsrestat-640x360.vp9.webm"},
		}, {
			Name: "vpxenc_vp9_720_meet",
			Val: testParam{
				command:        "vpxenc",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsVpxenc,
				regExpFPS:      regExpFPSVpxenc,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData: []string{"gipsrestat-1280x720.vp9.webm"},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "aomenc_av1_180",
			Val: testParam{
				command:        "aomenc",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsAomenc,
				regExpFPS:      regExpFPSAomenc,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData: []string{"tulip2-320x180.vp9.webm"},
		}, {
			Name: "aomenc_av1_360",
			Val: testParam{
				command:        "aomenc",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsAomenc,
				regExpFPS:      regExpFPSAomenc,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData: []string{"tulip2-640x360.vp9.webm"},
		}, {
			Name: "aomenc_av1_720",
			Val: testParam{
				command:        "aomenc",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsAomenc,
				regExpFPS:      regExpFPSAomenc,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData: []string{"tulip2-1280x720.vp9.webm"},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "aomenc_av1_180_meet",
			Val: testParam{
				command:        "aomenc",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsAomenc,
				regExpFPS:      regExpFPSAomenc,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData: []string{"gipsrestat-320x180.vp9.webm"},
		}, {
			Name: "aomenc_av1_360_meet",
			Val: testParam{
				command:        "aomenc",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsAomenc,
				regExpFPS:      regExpFPSAomenc,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData: []string{"gipsrestat-640x360.vp9.webm"},
		}, {
			Name: "aomenc_av1_720_meet",
			Val: testParam{
				command:        "aomenc",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsAomenc,
				regExpFPS:      regExpFPSAomenc,
				decoder:        encoding.LibaomDecoder,
			},
			ExtraData: []string{"gipsrestat-1280x720.vp9.webm"},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "openh264enc_180",
			Val: testParam{
				command:        "openh264enc",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsOpenh264enc,
				regExpFPS:      regExpFPSOpenh264enc,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData: []string{"tulip2-320x180.vp9.webm"},
		}, {
			Name: "openh264enc_360",
			Val: testParam{
				command:        "openh264enc",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsOpenh264enc,
				regExpFPS:      regExpFPSOpenh264enc,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData: []string{"tulip2-640x360.vp9.webm"},
		}, {
			Name: "openh264enc_720",
			Val: testParam{
				command:        "openh264enc",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsOpenh264enc,
				regExpFPS:      regExpFPSOpenh264enc,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData: []string{"tulip2-1280x720.vp9.webm"},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "openh264enc_180_meet",
			Val: testParam{
				command:        "openh264enc",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsOpenh264enc,
				regExpFPS:      regExpFPSOpenh264enc,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData: []string{"gipsrestat-320x180.vp9.webm"},
		}, {
			Name: "openh264enc_360_meet",
			Val: testParam{
				command:        "openh264enc",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsOpenh264enc,
				regExpFPS:      regExpFPSOpenh264enc,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData: []string{"gipsrestat-640x360.vp9.webm"},
		}, {
			Name: "openh264enc_720_meet",
			Val: testParam{
				command:        "openh264enc",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsOpenh264enc,
				regExpFPS:      regExpFPSOpenh264enc,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData: []string{"gipsrestat-1280x720.vp9.webm"},
			// Devices with small SSDs can't store the files, see b/181165183.
			ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(24)),
		}, {
			Name: "v4l2_h264_180",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"tulip2-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeH264},
		}, {
			Name: "v4l2_h264_360",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"tulip2-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeH264},
		}, {
			Name: "v4l2_h264_720",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"tulip2-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeH264},
		}, {
			Name: "v4l2_h264_180_meet",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"gipsrestat-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeH264},
		}, {
			Name: "v4l2_h264_360_meet",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"gipsrestat-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeH264},
		}, {
			Name: "v4l2_h264_720_meet",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.OpenH264Decoder,
			},
			ExtraData:         []string{"gipsrestat-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeH264},
		}, {
			Name: "v4l2_vp8_180",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "tulip2-320x180.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeVP8},
		}, {
			Name: "v4l2_vp8_360",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "tulip2-640x360.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeVP8},
		}, {
			Name: "v4l2_vp8_720",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "tulip2-1280x720.vp9.webm",
				numFrames:      500,
				fps:            30,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"tulip2-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeVP8},
		}, {
			Name: "v4l2_vp8_180_meet",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "gipsrestat-320x180.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(320, 180),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-320x180.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeVP8},
		}, {
			Name: "v4l2_vp8_360_meet",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "gipsrestat-640x360.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(640, 360),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-640x360.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeVP8},
		}, {
			Name: "v4l2_vp8_720_meet",
			Val: testParam{
				command:        "v4l2_stateful_encoder",
				filename:       "gipsrestat-1280x720.vp9.webm",
				numFrames:      846,
				fps:            50,
				size:           coords.NewSize(1280, 720),
				commandBuilder: platform.ArgsV4L2,
				regExpFPS:      regExpFPSV4L2,
				decoder:        encoding.LibvpxDecoder,
			},
			ExtraData:         []string{"gipsrestat-1280x720.vp9.webm"},
			ExtraSoftwareDeps: []string{"v4l2_codec", caps.HWEncodeVP8},
		}},
		Timeout: 30 * time.Minute,
	})
}

// PlatformEncoding verifies platform encoding by running a command line encoder
// binary and comparing its result with the original input. The encoder input is
// an uncompressed YUV file which would be too large to be stored, so it is
// produced on the fly from testParam.filename. The compressed bitstream output
// is decompressed using testParam.decoder so that it can be compared with the
// original YUV file.
func PlatformEncoding(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(testParam)

	yuvFile, err := encoding.DecodeInI420(ctx, s.DataPath(testOpt.filename))
	if err != nil {
		s.Fatal("Failed to prepare YUV file: ", err)
	} else if videovars.ShouldRemoveArtifacts(ctx) {
		defer os.Remove(yuvFile)
	}

	command, encodedFile, targetBitrate, err := testOpt.commandBuilder(ctx, s.TestName(), testOpt.command, yuvFile, testOpt.size, int(testOpt.fps))
	if err != nil {
		s.Fatal("Failed to construct the command line: ", err)
	}

	energy, raplErr := power.NewRAPLSnapshot()
	if raplErr != nil || energy == nil {
		s.Log("Energy consumption is not available for this board")
	}
	startTime := time.Now()

	s.Log("Running ", shutil.EscapeSlice(command))
	logFile, err := platform.RunEncoderBinary(ctx, s.OutDir(), command[0], command[1:]...)
	if err != nil {
		s.Fatal("Failed to run binary: ", err)
	} else if videovars.ShouldRemoveArtifacts(ctx) {
		defer os.Remove(encodedFile)
	}

	timeDelta := time.Now().Sub(startTime)
	var energyDiff *power.RAPLValues
	var energyErr error
	if raplErr == nil && energy != nil {
		if energyDiff, energyErr = energy.DiffWithCurrentRAPL(); energyErr != nil {
			s.Log("Energy consumption measurement failed: ", energyErr)
		}
	}

	fps, err := extractValue(logFile, testOpt.regExpFPS)
	if err != nil {
		s.Fatal("Failed to extract FPS: ", err)
	}

	psnr, ssim, vmaf, err := encoding.CompareFiles(ctx, testOpt.decoder, yuvFile, encodedFile, s.OutDir(), testOpt.size)
	if err != nil {
		s.Fatal("Failed to decode and compare results: ", err)
	}

	p := perf.NewValues()
	p.Set(perf.Metric{
		Name:      "fps",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
	}, fps)
	p.Set(perf.Metric{
		Name:      "SSIM",
		Unit:      "percent",
		Direction: perf.BiggerIsBetter,
	}, ssim*100)
	p.Set(perf.Metric{
		Name:      "PSNR",
		Unit:      "dB",
		Direction: perf.BiggerIsBetter,
	}, psnr)
	p.Set(perf.Metric{
		Name:      "VMAF",
		Unit:      "percent",
		Direction: perf.BiggerIsBetter,
	}, vmaf)

	if energyDiff != nil && energyErr == nil {
		energyDiff.ReportWattPerfMetrics(p, "", timeDelta)
	}

	actualBitrate, err := calculateBitrate(encodedFile, testOpt.fps, testOpt.numFrames)
	if err != nil {
		s.Fatal("Failed to calculate the resulting bitrate: ", err)
	}
	p.Set(perf.Metric{
		Name:      "bitrate_deviation",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, (100.0*actualBitrate/float64(targetBitrate))-100.0)

	keyFrames, err := countKeyFrames(ctx, s.OutDir(), encodedFile)
	if err != nil {
		s.Fatal("Failed to calculate the amount of keyframes: ", err)
	}
	p.Set(perf.Metric{
		Name:      "KeyFrames",
		Unit:      "keyframes",
		Direction: perf.SmallerIsBetter,
	}, float64(keyFrames))

	s.Log(p)
	if err := p.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf results: ", err)
	}
}

// extractValue parses logFile using r and returns a single float64 match.
func extractValue(logFile string, r *regexp.Regexp) (value float64, err error) {
	b, err := ioutil.ReadFile(logFile)
	if err != nil {
		return 0.0, errors.Wrapf(err, "failed to read file %s", logFile)
	}

	matches := r.FindAllStringSubmatch(string(b), -1)
	if len(matches) != 1 {
		return 0.0, errors.Errorf("found %d matches in %q; want 1", len(matches), b)
	}

	matchString := matches[0][1]
	if value, err = strconv.ParseFloat(matchString, 64); err != nil {
		return 0.0, errors.Wrapf(err, "failed to parse value %q", matchString)
	}
	return
}

// calculateBitrate calculates the bitrate of encodedFile.
func calculateBitrate(encodedFile string, fileFPS float64, numFrames int) (value float64, err error) {
	s, err := os.Stat(encodedFile)
	if err != nil {
		return 0.0, errors.Wrapf(err, "failed to get stats for file %s", encodedFile)
	}
	return float64(s.Size()) * 8 /* bits per byte */ * fileFPS / float64(numFrames), nil
}

// countKeyFrames counts IDR frame/keyframe in |file| using ffprobe.
func countKeyFrames(ctx context.Context, outDir, file string) (count int, err error) {
	cmd := []string{"ffprobe", "-show_frames", "-show_entries", "frame=pict_type", file}
	testing.ContextLogf(ctx, "Running: %s", shutil.EscapeSlice(cmd))
	ffprobeFile, err := platform.RunEncoderBinary(ctx, outDir, cmd[0], cmd[1:]...)
	if err != nil {
		return 0, errors.Wrap(err, "failed to run ffprobe")
	}

	// ffprobe calls "I" pictures both key-frames (VP8/9, AV1)and I(DR)-frames (H.264/5).
	cmd = []string{"grep", "-c", "pict_type=I", ffprobeFile}
	grepOut, err := testexec.CommandContext(ctx, cmd[0], cmd[1:]...).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "failed to run ffmpeg")
	}

	grepOutStr := strings.TrimSpace(string(grepOut[:]))
	cnt, err := strconv.Atoi(grepOutStr)
	if err != nil {
		return 0, errors.Wrapf(err, "failed converting to int: %s", grepOutStr)
	}
	if cnt == 0 {
		return 0, errors.Wrap(err, "At least one keyframe must exist")
	}

	return cnt, nil
}
