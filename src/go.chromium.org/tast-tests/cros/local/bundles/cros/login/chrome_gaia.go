// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeGAIA,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Chrome can make real GAIA logins",
		Contacts: []string{
			"cros-lurs@google.com",
			"antrim@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"gaia",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
		},
		VarDeps: []string{
			ui.GaiaPoolDefaultVarName,
		},
		Params: []testing.Param{{
			Name: "",
		}, {
			Name:      "enrolled",
			Fixture:   fixture.FakeDMSEnrolled,
			ExtraAttr: []string{"informational"},
		}},
		Timeout: chrome.GAIALoginTimeout + time.Minute,
	})
}

func ChromeGAIA(ctx context.Context, s *testing.State) {
	fdms, ok := s.FixtValue().(*fakedms.FakeDMS)
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()
	opts := []chrome.Option{
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
	}

	if ok {
		opts = append(opts, chrome.KeepEnrollment())
		opts = append(opts, chrome.DMSPolicy(fdms.URL))
	}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)
}
