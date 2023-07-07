// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIMetrics,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests if Chrome Camera App sends metrics correctly",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome", "metrics_consent"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
		BugComponent: "b:978428",
	})
}

// CCAUIMetrics tests if Chrome Camera App sends end_session events correctly.
func CCAUIMetrics(ctx context.Context, s *testing.State) {
	fixtureData := s.FixtValue().(cca.FixtureData)
	received := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cr := fixtureData.Chrome
	if err := crash.SetUpCrashTest(ctx, crash.WithConsent(cr)); err != nil {
		s.Fatal("Failed to enable consent option: ", err)
	}
	defer crash.TearDownCrashTest(ctx)

	app := fixtureData.App()
	tb := fixtureData.TestBridge()
	if err := app.EnableBypassCSP(ctx, tb); err != nil {
		s.Fatal("Failed to enable bypassing CSP: ", err)
	}

	if err := app.EnableGa4Metrics(ctx, server.URL); err != nil {
		s.Fatal("Failed to set measurement protocol url: ", err)
	}

	// Refresh CCA to trigger end_session event.
	if err := app.Refresh(ctx, tb); err != nil {
		s.Fatal("Failed to refresh CCA: ", err)
	}

	if err := app.WaitForVideoActive(ctx); err != nil {
		s.Fatal("Preview is not shown after refreshing: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if received {
			return nil
		}
		return errors.New("request not received")
	}, &testing.PollOptions{Timeout: time.Second * 3}); err != nil {
		s.Fatal("Failed to wait for the request: ", err)
	}
}
