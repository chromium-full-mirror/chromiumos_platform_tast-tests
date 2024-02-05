// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shimlessrma contains integration tests for Shimless RMA SWA.
package shimlessrma

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/shimlessrma/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WelcomeCancel,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Can successfully start and cancel the Shimless RMA app",
		Contacts: []string{
			"chromeos-shimless-eng@google.com",
			"chenghan@google.com",
			"jeffulin@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Shimless RMA
		BugComponent: "b:1002147",
		Attr:         []string{"group:mainline", "informational"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model(util.ShimlessRmaEnabledModels...)),
		Timeout:      5 * time.Minute,
	})
}

// WelcomeCancel verifies that the Shimless RMA app can open and immediately
// cancel.
func WelcomeCancel(ctx context.Context, s *testing.State) {
	resources, app, err := util.InitResource(ctx, s.RequiredVar("ui.signinProfileTestExtensionManifestKey"), "")
	if err != nil {
		s.Fatal("Failed to init resource: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer resources.DisposeResource(cleanupCtx)

	// Wait for Welcome page to load.
	if err := action.Combine("Welcome and cancel",
		app.WaitForPageToLoad("Chromebook repair", 20*time.Second),
		app.LeftClickButton("Exit"),
	)(ctx); err != nil {
		s.Fatal("Fail to load welcome page and click Cancel: ", err)
	}

}
