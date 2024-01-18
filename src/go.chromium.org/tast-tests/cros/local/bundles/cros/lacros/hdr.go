// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lacros tests lacros-chrome running on ChromeOS.
package lacros

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	searchTimeout  = 20 * time.Second
	searchInterval = 1 * time.Second
)

var pixelFormatPattern = regexp.MustCompile(`(?:format=)\w+`)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HDR,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that hdr videos play on lacros and use a 30-bit buffer",
		Contacts:     []string{"lacros-team@google.com", "mrfemi@google.com"},
		BugComponent: "b:1456869",
		Attr:         []string{"group:mainline", "informational", "group:graphics", "graphics_perbuild"},
		HardwareDeps: hwdep.D(hwdep.Model("kohaku")),
		SoftwareDeps: []string{"chrome", "lacros"},
		Fixture:      "lacrosHDR",
		Timeout:      7 * time.Minute,
		Data:         []string{"hdr_video_1.html", "hdr_scientist_4k_yuv420p10le_20230412.webm"},
	})
}

func HDR(ctx context.Context, s *testing.State) {
	// Setup server to serve video file.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	hdrURL := server.URL + "/hdr_video_1.html"
	// Ensure display is on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch lacros-chrome: ", err)
	}

	c, err := l.NewConn(ctx, hdrURL)
	if err != nil {
		s.Fatalf("Failed to open Lacros with URL %s: %v", hdrURL, err)
	}
	defer c.Close()

	// Try for up to 20 seconds to detect a 30-bit buffer, sleeping for 1s
	// after each attempt.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		statePath, err := graphics.GetValidKernelDriverDebugFile(ctx, []string{"state"})
		if err != nil {
			return errors.New("no dri debug file exists")
		}

		data, err := os.ReadFile(statePath)
		if err != nil {
			return errors.New("could not read dri debug file")
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
				testing.ContextLogf(ctx, "30-bit buffer detected: %s", match)
			} else {
				testing.ContextLogf(ctx, "Other buffer detected: %s", match)
			}
		}
		if found30bpp {
			return nil
		}
		return errors.New("did not find 30-bit buffer")
	}, &testing.PollOptions{Timeout: searchTimeout, Interval: searchInterval}); err != nil {
		s.Error("Did not find 30-bit buffer")
	}
}
