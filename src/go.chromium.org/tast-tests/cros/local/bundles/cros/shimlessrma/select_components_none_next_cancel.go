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
		Func:         SelectComponentsNoneNextCancel,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Can successfully start in select components state, go to next screen and cancel the Shimless RMA app",
		Contacts: []string{
			"chromeos-shimless-eng@google.com",
			"chenghan@google.com",
			"jeffulin@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Shimless RMA
		BugComponent: "b:1002147",
		Attr:         []string{"group:mainline"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model(util.ShimlessRmaEnabledModels...)),
		Timeout:      5 * time.Minute,
		// TODO(jeffulin): Add critical parameters when staging results found stable.
		Params: []testing.Param{{
			Name:      "staging",
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}},
	})
}

const (
	state = `{"state_history":[1,2]}`
)

// SelectComponentsNoneNextCancel verifies that the Shimless RMA app can
// open in select components state, move to the next screen and then cancel.
func SelectComponentsNoneNextCancel(ctx context.Context, s *testing.State) {
	resources, app, err := util.InitResource(ctx, s.RequiredVar("ui.signinProfileTestExtensionManifestKey"), state)
	if err != nil {
		s.Fatal("Failed to init resource: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer resources.DisposeResource(cleanupCtx)

	// Wait for Select Components page to load.
	if err := action.Combine("Select Component none and then next and then cancel",
		app.WaitForPageToLoad("Select which components were replaced", 30*time.Second),
		app.LeftClickButton("Next"),
		app.WaitForPageToLoad("After repair, who will be using the device?", 20*time.Second),
		app.LeftClickButton("Exit"),
	)(ctx); err != nil {
		s.Fatal("Fail to select component none and click next and then click cancel: ", err)
	}
}
