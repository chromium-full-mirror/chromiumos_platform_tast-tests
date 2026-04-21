// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"time"

	"github.com/mafredri/cdp/protocol/target"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     IntentForward,
		Desc:     "Checks Android intents are forwarded to Chrome",
		Contacts: []string{"arc-core@google.com", "djacobo@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: arc.ArcAppHwDeps,
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
		Attr:         []string{"group:mainline", "group:arc-functional"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "arcBooted",
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "arcBooted",
		}},
	})
}

func IntentForward(ctx context.Context, s *testing.State) {
	const (
		viewAction          = "android.intent.action.VIEW"
		viewDownloadsAction = "android.intent.action.VIEW_DOWNLOADS"
		setWallpaperAction  = "android.intent.action.SET_WALLPAPER"

		filesAppURL        = "chrome://file-manager/"
		wallpaperPickerURL = "chrome://personalization/wallpaper"
	)

	d := s.FixtValue().(*arc.PreData)
	a := d.ARC
	cr := d.Chrome

	if err := a.WaitIntentHelper(ctx); err != nil {
		s.Fatal("ArcIntentHelper did not come up: ", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "It worked!")
	}))
	defer server.Close()
	localWebURL := server.URL + "/" // Must end with a slash

	checkIntent := func(action, data, url string) {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		testing.ContextLogf(ctx, "Testing: %s(%s) -> %s", action, data, url)

		if err := a.SendIntentCommand(ctx, action, data).Run(testexec.DumpLogOnError); err != nil {
			s.Errorf("Failed to send an intent %q: %v", action, err)
			return
		}

		urlMatcher := func(t *target.Info) bool {
			matched, _ := regexp.MatchString(url, t.URL)
			return matched
		}

		conn, err := cr.NewConnForTarget(ctx, urlMatcher)
		if err != nil {
			s.Errorf("%s(%s) -> %s: %v", action, data, url, err)
			return
		}
		defer conn.Close()
	}

	checkIntent(viewAction, localWebURL, localWebURL)
	checkIntent(setWallpaperAction, "", wallpaperPickerURL)
	checkIntent(viewDownloadsAction, "", filesAppURL)
}
