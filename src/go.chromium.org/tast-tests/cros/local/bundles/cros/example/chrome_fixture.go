// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeFixture,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Demonstrates Chrome fixture",
		Contacts:     []string{"tast-core@google.com", "nya@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func ChromeFixture(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	const content = "Hooray, it worked!"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, content)
	}))
	defer srv.Close()

	conn, err := cr.NewConn(ctx, srv.URL)
	if err != nil {
		s.Fatal("Creating tab failed: ", err)
	}
	defer conn.Close()

	var actual string
	if err := conn.Eval(ctx, "document.documentElement.innerText", &actual); err != nil {
		s.Fatal("Getting page content failed: ", err)
	}
	if actual != content {
		s.Fatalf("Unexpected page content: got %q; want %q", actual, content)
	}
}
