// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/libvda"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LibvdaDecode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that video decoding using libvda's Mojo connection to GAVDA is working",
		Contacts:     []string{"arcvm-platform-video@google.com"},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Video
		BugComponent: "b:632502",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"android_vm", "chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{{
			Name:              "h264",
			Val:               "h264",
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraData:         []string{"test-25fps.h264"},
		}, {
			Name:              "vp8",
			Val:               "vp8",
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			ExtraData:         []string{"test-25fps.vp8"},
		}, {
			Name:              "vp9",
			Val:               "vp9",
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraData:         []string{"test-25fps.vp9"},
		}},
	})
}

func LibvdaDecode(ctx context.Context, s *testing.State) {
	logFileName := "output_libvda_" + s.Param().(string) + ".txt"
	videoFile := "test-25fps." + s.Param().(string)
	libvda.RunGPUFileDecodeTest(ctx, s, logFileName, videoFile)
}
