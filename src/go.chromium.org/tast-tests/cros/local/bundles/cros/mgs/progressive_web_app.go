// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mgs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast/core/testing"
)

const port = 8080
const url = "http://localhost:%v/pwa_index.html"
const appID = "cpdpbfelifklonephgpieimdpcecgoen"

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProgressiveWebApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that Progressive Web Apps (PWA) are working in a managed guest session by trying to install and start a test PWA",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Data:    []string{"pwa_manifest.json", "pwa_service.js", "pwa_index.html", "pwa_icon.png"},
		Fixture: fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Auto-launch MGS.
			Value: "screenplay-c6a4d9e8-7522-448d-9fda-c8bc7db48507",
		}}})
}

func ProgressiveWebApp(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	mgs, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
	)
	if err != nil {
		s.Fatal("Failed to start MGS: ", err)
	}
	defer mgs.Close(ctx)

	mux := http.NewServeMux()
	fs := http.FileServer(s.DataFileSystem())
	mux.Handle("/", fs)

	server := &http.Server{Addr: fmt.Sprintf(":%v", port), Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			s.Fatal("Failed to create local server: ", err)
		}
	}()
	defer server.Shutdown(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	if err := apps.InstallPWAForURL(ctx, tconn, cr.Browser(), fmt.Sprintf(url, port), 15*time.Second); err != nil {
		s.Fatal("Failed to install PWA for URL: ", err)
	}

	if err := ash.WaitForChromeAppInstalled(ctx, tconn, appID, 15*time.Second); err != nil {
		s.Fatal("Failed to wait for PWA to be installed: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, appID, 15*time.Second); err != nil {
		s.Fatal("Failed to wait for PWA to open: ", err)
	}
}
