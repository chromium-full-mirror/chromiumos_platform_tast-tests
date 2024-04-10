// Copyright 2023 The ChromiumOS Authors
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
	hostRedirectPy    = "mitmproxy_redirect_requests.py"
	httpErrorInjectPy = "mitmproxy_inject_500_requests.py"
	dumpHTTPFlowPy    = "dump_http_flow.py"
	httpFlowFilterPy  = "http_flow_filter.py"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkManipulateMitmproxy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that manipulating network traffic with mitmproxy",
		Contacts: []string{
			"cros-ufo-testing@google.com",
			"yanghenry@google.com",
		},
		BugComponent: "b:1359643", // ChromeOS > EngProd > Software > Trust & Safety
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.ChromeLoggedIn,
		Params: []testing.Param{{
			Name:      "hostredirect",
			Val:       "hostredirect",
			ExtraData: []string{hostRedirectPy},
		}, {
			Name: "urlredirect",
			Val:  "urlredirect",
		}, {
			Name: "dump",
			Val:  "dump",
		}, {
			Name:      "error",
			Val:       "error",
			ExtraData: []string{httpErrorInjectPy},
		}, {
			Name:      "dumphttpflow",
			Val:       "dumphttpflow",
			ExtraData: []string{dumpHTTPFlowPy},
		}, {
			Name:      "httpflowfilter",
			Val:       "httpflowfilter",
			ExtraData: []string{httpFlowFilterPy},
		}},
	})
}

func NetworkManipulateMitmproxy(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	mp, err := proxy.NewMitmProxy(ctx, proxyOpts(s)...)
	if err != nil {
		s.Fatal("Failed to start proxy: ", err)
	}
	defer mp.Close(cleanupCtx)
	reset, err := proxy.ConfigureChrome(ctx, mp, cr)
	if err != nil {
		s.Fatal("Failed to configure chrome for proxy: ", err)
	}
	defer reset(cleanupCtx, cr)

	if err := verify(ctx, s, cr, mp); err != nil {
		s.Fatal("Failed to verify page: ", err)
	}
}

func verify(ctx context.Context, s *testing.State, cr *chrome.Chrome, mp proxy.Proxy) error {

	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, "https://www.example.com")
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	defer closeBrowser(ctx)
	defer conn.Close()
	br.ReloadActiveTab(ctx)

	switch s.Param().(string) {
	case "hostredirect":
		return verifyPageContent(ctx, conn, "google")
	case "urlredirect":
		return verifyPageContent(ctx, conn, "google")
	case "error":
		return verifyPageContent(ctx, conn, "error injected by proxy")
	case "dumphttpflow":
		traffic, err := mp.DumpHTTPFlow(ctx, false, true)
		if err != nil {
			return err
		}

		// TODO(b/319732303): Update to Verifier when it's ready.
		foundURL := false
		for _, element := range traffic.URLs {
			if element == "https://www.example.com/" {
				foundURL = true
				break
			}
		}

		if !foundURL {
			return errors.New("fail to find URl or Hostname")
		}
		return nil
	case "httpflowfilter":
		err := conn.Navigate(ctx, "https://www.google.com")
		if err != nil {
			return errors.Wrap(err, "failed to open new tab")
		}
		return verifyPageContent(ctx, conn, "503")
	default:
		return nil
	}
}

func verifyPageContent(ctx context.Context, conn *chrome.Conn, expected string) error {
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
		return err
	}

	return nil
}

func proxyOpts(s *testing.State) []proxy.Option {
	testCase := s.Param().(string)
	var opts []proxy.Option
	switch testCase {
	case "hostredirect":
		opts = append(opts, proxy.ScriptPath(s.DataPath(hostRedirectPy)))
	case "urlredirect":
		urlMap := map[string]string{
			`//www.example.com/`: `//www.google.com/`, // replace the hostname in the URL
		}
		opts = append(opts, proxy.URLRedirect(urlMap))
	case "error":
		opts = append(opts, proxy.ScriptPath(s.DataPath(httpErrorInjectPy)))
	case "httpflowfilter":
		opts = append(opts,
			proxy.Allowlist([]string{"www.example.com", "www.example.org"}, s.DataPath(httpFlowFilterPy)),
			proxy.HealthCheck(false), // Disable health check as it uses the local domain that won't work with the allowlist set for this test.
		)
	case "dumphttpflow":
		opts = append(opts,
			proxy.DumpHTTPFlow(true, s.DataPath(dumpHTTPFlowPy)),
		)
	}
	return opts
}
