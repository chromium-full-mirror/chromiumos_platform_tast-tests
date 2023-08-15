// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/martianproxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	redirectRequestRule = "proxy_redirect_request.json"
	mockResponseRule    = "proxy_mock_response.json"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkManipulate,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that manipulating network traffic with proxy",
		Contacts: []string{
			"shengjun@google.com",
			"yanghenry@google.com",
		},
		BugComponent: "b:1359643", // ChromeOS > EngProd > Software > Trust & Safety
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Fixture:      fixture.ChromeLoggedIn,
		Params: []testing.Param{{
			Name:      "redirect",
			Val:       redirectRequestRule,
			ExtraData: []string{redirectRequestRule},
		}, {
			Name:      "mock",
			Val:       mockResponseRule,
			ExtraData: []string{mockResponseRule},
		},
			{
				Name: "dump",
				Val:  "",
			}},
	})
}

func NetworkManipulate(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	isDumpOnly := strings.HasSuffix(s.TestName(), "dump")

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	proxy := martianproxy.New()
	if isDumpOnly {
		proxy.SetHar(true).SetOutDir(s.OutDir())
	}

	if err := cr.LaunchAndApplyProxy(ctx, proxy); err != nil {
		s.Fatal("Failed to launch and apply proxy: ", err)
	}
	defer proxy.Close(cleanupCtx)

	if !isDumpOnly {
		ruleFile := s.DataPath(s.Param().(string))
		if err := proxy.ConfigureWithJSON(ctx, ruleFile); err != nil {
			s.Fatalf("Failed to configure proxy with json file %q: %v", ruleFile, err)
		}
	}

	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, "https://www.example.com")
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
}
