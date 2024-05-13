// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	responseMockPy = "response_mock.py"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ResponseMockMitmproxy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that mocking traffic response with mitmproxy",
		Contacts: []string{
			"cros-ufo-testing@google.com",
			"yanghenry@google.com",
		},
		BugComponent: "b:1359643", // ChromeOS > EngProd > Software > Trust & Safety
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.ChromeLoggedIn,
		Data:         []string{responseMockPy},
	})
}

func ResponseMockMitmproxy(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Setup.
	mp, err := proxy.NewMitmProxy(ctx,
		proxy.ScriptPath(s.DataPath(responseMockPy)),
	)
	if err != nil {
		s.Fatal("Failed to start proxy: ", err)
	}
	defer mp.Close(cleanupCtx)
	reset, err := proxy.ConfigureChrome(ctx, mp, cr)
	if err != nil {
		s.Fatal("Failed to configure chrome for proxy: ", err)
	}
	defer reset(cleanupCtx, cr)

	// Execute test logic.
	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, "https://www.example.com")
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
	br.ReloadActiveTab(ctx)

	// Verify.
	expected := "Content injected by proxy"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		content, err := conn.PageContent(ctx)
		if err != nil {
			return errors.Wrap(err, "fail to get page content")
		}
		if !strings.Contains(content, expected) {
			return errors.Wrapf(err, "page doesn't contain expected content: %s, page is %s", expected, content)
		}

		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 2 * time.Second}); err != nil {
		s.Fatal("Failed to mock response: ", err)
	}
}
