// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskapi

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/deskapi/constants"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeskSwitch,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks using desk API to switch desk",
		BugComponent: "b:1020793",
		Contacts: []string{
			"cros-commercial-productivity-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"aprilzhou@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		Fixture:      fixture.DeskAPI,
	})
}

func DeskSwitch(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	//Local test unable to access data from remote fixture in normal way. Use FixtFillValue instead
	structVal := fixture.DeskFixtData{}
	if err := s.FixtFillValue(&structVal); err != nil {
		s.Fatal("Failed to deserialize remote fixture data: ", err)
	}

	// Use the same DMServer endpoint as the in the enrollment
	cr, err := chrome.New(ctx, chrome.KeepState(), chrome.TryReuseSession(), chrome.GAIALogin(chrome.Creds{User: structVal.Username, Pass: structVal.Password}), chrome.DMSPolicy(constants.DmServerURL))
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer ash.CleanUpDesks(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Close all existing windows.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}

	const url string = "https://continuous-sincere-relation.glitch.me"

	// Create a new browser connection with target page opened.
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to create a new browser connection: ", err)
	}
	defer conn.Close()

	// Should only have 1 valid desk at initialization time.
	deskCount, err := ash.GetDeskCount(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get desk count: ", err)
	}
	if deskCount != 1 {
		s.Fatalf("Unexpected desk cound: want 1, got %d", deskCount)
	}

	const getDeskFunc = `new Promise((resolve, reject) => {
		chrome.runtime.sendMessage(
			"kflgdebkpepnpjobkdfeeipcjdahoomc", {
				"messageType": "GetActiveDesk",
			},
			(response) => {
				if(response.errorMessage) {
					reject(new Error(response.errorMessage));
					return;
				}
				resolve(response.operands.deskUuid);
			});
		})`
	// Get current active desk
	var deskID string
	if err := conn.Eval(ctx, getDeskFunc, &deskID); err != nil {
		s.Fatal("Failed to get active desk: ", err)
	}

	if err := ash.CreateNewDesk(ctx, tconn); err != nil {
		s.Fatal("Failed to launch a new desk: ", err)
	}

	if err := ash.ActivateDeskAtIndex(ctx, tconn, 1); err != nil {
		s.Fatal("Failed to activate the new desk: ", err)
	}

	if err := ash.WaitUntilDesksFinishAnimating(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for launch desk animation: ", err)
	}

	var deskID1 string
	if err := conn.Eval(ctx, getDeskFunc, &deskID1); err != nil {
		s.Fatal("Failed to get active desk: ", err)
	}

	if deskID == deskID1 {
		s.Fatal("Failed to move to new desk: ", err)
	}

	const switchDeskFunc = `async (deskId) => {
		await new Promise((resolve, reject) => {
			chrome.runtime.sendMessage(
				"kflgdebkpepnpjobkdfeeipcjdahoomc", {
					"messageType": "SwitchDesk",
					"operands": {
						"deskId": deskId
					}
				},
				(response) => {
					if(response.errorMessage) {
						reject(new Error(response.errorMessage));
						return;
					}
					resolve();
				});
			});
		}`
	if err := conn.Call(ctx, nil, switchDeskFunc, deskID); err != nil {
		s.Fatal("Failed to switch desk: ", err)
	}

	if err := ash.WaitUntilDesksFinishAnimating(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for launch desk animation: ", err)
	}

	var deskID2 string
	if err := conn.Eval(ctx, getDeskFunc, &deskID2); err != nil {
		s.Fatal("Failed to get active desk: ", err)
	}

	if deskID != deskID2 {
		s.Fatalf("Failed to switch back to previous desk, want:%s, got %s", deskID, deskID2)
	}

}
