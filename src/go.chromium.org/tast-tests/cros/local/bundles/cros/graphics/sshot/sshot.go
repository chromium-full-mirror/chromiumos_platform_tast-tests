// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sshot supports taking screenshots on devices during testing.
package sshot

import (
	"context"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/colorcmp"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SShot opens a new tab in Chrome which will retrieve a page from a local
// webserver we instantiate which renders just a solid orange background. We then
// use the passed in function to take a screenshot which we then check for having
// a majority of the pixels match our target color.
func SShot(ctx context.Context, s *testing.State, cr *chrome.Chrome, capture func(ctx context.Context, path string) error) error {
	// Show a page with orange background.
	const html = "<style>body { background-color: #c84; }</style>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		io.WriteString(w, html)
	}))
	defer server.Close()

	conn, err := cr.NewConn(ctx, server.URL)
	if err != nil {
		s.Fatal("Creating renderer failed: ", err)
	}
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	if err := tconn.Call(ctx, nil, `async() => {
		  const window = await tast.promisify(chrome.windows.getLastFocused)({});
		  await tast.promisify(chrome.windows.update)(window.id, {state: 'maximized'});
		}`); err != nil {
		s.Fatal("Maximizing the window failed: ", err)
	}

	const screenshotName = "screenshot.png"
	path := filepath.Join(s.OutDir(), screenshotName)

	// The largest differing color known to date and is #ba8b4a on sumo in
	// comparison to a target of #cc8844, so this value should be no less than
	// 0x12.
	const maxKnownColorDiff = 0x13

	iteration := 1
	expectedColor := colorcmp.RGB(0xcc, 0x88, 0x44)
	// Allow up to 2 minutes for the target screen to render.
	return testing.Poll(ctx, func(ctx context.Context) error {
		s.Logf("Starting iteration %d", iteration)
		iteration++
		if err := capture(ctx, path); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			s.Fatal("Failed opening the screenshot image: ", err)
		}
		defer f.Close()

		im, err := png.Decode(f)
		if err != nil {
			s.Fatal("Failed decoding the screenshot image: ", err)
		}

		color, ratio := colorcmp.DominantColor(im)
		if ratio >= 0.5 && colorcmp.ColorsMatch(color, expectedColor, maxKnownColorDiff) {
			s.Logf("Got close-enough color %v at ratio %0.2f (expected %v)",
				colorcmp.ColorStr(color), ratio, colorcmp.ColorStr(expectedColor))
			return nil
		}
		s.Logf("Color %v at ratio %0.2f are too far apart (expected %v)",
			colorcmp.ColorStr(color), ratio, colorcmp.ColorStr(expectedColor))
		return errors.Errorf("screenshot did not have matching dominant color; expected %v but got %v at ratio %0.2f",
			colorcmp.ColorStr(expectedColor), colorcmp.ColorStr(color), ratio)
	}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 1 * time.Second})
}
