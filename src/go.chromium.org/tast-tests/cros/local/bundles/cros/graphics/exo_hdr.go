// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var pixelFormatPattern = regexp.MustCompile(`(?:format=)\w+`)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExoHDR,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that a client can send a 30-bit buffer to exo",
		BugComponent: "b:1021073", // ChromeOS > Platform > Graphics > Compositor
		Contacts: []string{
			"chromeos-gfx-compositor@google.com",
			"mrfemi@google.com",
		},
		Attr:         []string{"group:graphics", "graphics_perbuild"},
		Fixture:      "chromeGraphicsHDR",
		SoftwareDeps: []string{"chrome"},
		// TODO(b/268091281): Add more devices here.
		// Right now kohaku is the only device with a built in HDR display.
		HardwareDeps: hwdep.D(hwdep.Model("kohaku")),
		Timeout:      3 * time.Minute,
	})
}

func ExoHDR(ctx context.Context, s *testing.State) {
	// Launch a small app that will display HDR content to the screen over exo.
	hdrClientPath := filepath.Join(chrome.BinTestDir, "wayland_hdr_client")
	cmd := testexec.CommandContext(ctx, "env", "XDG_RUNTIME_DIR=/var/run/chrome", hdrClientPath, "--use-drm")
	// The buffer format needs to be queried while the app is running and
	// presenting to the screen. So start the app in the background and kill
	// it when the test is over.
	if err := cmd.Start(); err != nil {
		s.Fatal("Failed to run wayland_hdr_client: ", err)
	}
	defer cmd.Kill()

	// Try for up to 20 seconds to detect a 30-bit buffer, sleeping for 1s
	// after each attempt.
	for i := 0; i < 20; i++ {
		statePath, err := graphics.GetValidKernelDriverDebugFile(ctx, []string{
			"state",
		})
		if err != nil {
			s.Fatal("No dri debug file exists")
		}

		data, err := os.ReadFile(statePath)
		if err != nil {
			s.Fatal("Could not read dri debug file")
		}
		matches := pixelFormatPattern.FindStringSubmatch(string(data))
		found30bpp := false
		for _, match := range matches {
			if match == "format=AR30" ||
				match == "format=AB30" ||
				match == "format=XR30" ||
				match == "format=XB30" {
				// We want to log all buffers that were found so don't return just yet.
				found30bpp = true
				s.Logf("30-bit buffer detected: %s", match)
			} else {
				s.Logf("Other buffer detected: %s", match)
			}
		}
		if found30bpp {
			return
		}
		testing.Sleep(ctx, 1*time.Second)
	}
	s.Error("Did not find 30-bit buffer")
}
