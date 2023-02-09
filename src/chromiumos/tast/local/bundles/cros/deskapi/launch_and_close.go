// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskapi

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/event"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/testing"
)

const dmServerURL = "https://crosman-alpha.sandbox.google.com/devicemanagement/data/api"

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchAndClose,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks using desk API to launch and remove desk",
		// Chrome OS Server Projects > Enterprise Management > Commercial Productivity
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

func LaunchAndClose(ctx context.Context, s *testing.State) {
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
	cr, err := chrome.New(ctx, chrome.KeepState(), chrome.TryReuseSession(), chrome.GAIALogin(chrome.Creds{User: structVal.Username, Pass: structVal.Password}), chrome.DMSPolicy(dmServerURL))
	if err != nil {
		s.Fatal("Failed to start chrome")
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

	ac := uiauto.New(tconn)
	const url string = "https://continuous-sincere-relation.glitch.me"

	// Create a new browser connection with target page opened.
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to create a new browser connection: ", err)
	}
	defer conn.Close()

	// Pin window to all-desks.
	if err := conn.Eval(ctx, `new Promise((resolve, reject) => {
		chrome.runtime.sendMessage(
			"kflgdebkpepnpjobkdfeeipcjdahoomc", {
				"messageType": "SetWindowProperties",
				"operands": {
					"allDesks": true
				}
		    },
		    function(response) {
				if(response.errorMessage) {
					reject(new Error(chrome.runtime.lastError.message));
				}
				resolve();
			});
		})`, nil); err != nil {
		s.Fatal("Failed to pin window to all desks: ", err)
	}
	if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		s.Fatal("Failed to wait for all desks animation to be completed: ", err)
	}
	deskCount, err := ash.GetDeskCount(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get desk count: ", err)
	}
	if deskCount != 1 {
		s.Fatalf("Unexpected desk cound: want 1, got %d", deskCount)
	}

	// Launch a new desk.
	var deskID string
	if err := conn.Eval(ctx, `new Promise((resolve, reject) => {
		chrome.runtime.sendMessage(
			"kflgdebkpepnpjobkdfeeipcjdahoomc", {
				"messageType": "LaunchDesk",
				"operands": {
					"deskName": "test" // Specify desk name.
				}
			},
			function(response) {
				if(response.errorMessage) {
					reject(new Error(chrome.runtime.lastError.message));
				}
				resolve(response.operands.deskUuid);
			});
		})`, &deskID); err != nil {
		s.Fatal("Failed to launch new desks: ", err)
	}

	if err := ash.WaitUntilDesksFinishAnimating(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for launch desk animation: ", err)
	}

	deskCount, err = ash.GetDeskCount(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get desk count: ", err)
	}
	if deskCount != 2 {
		s.Fatalf("Unexpected desk cound: want 2, got %d", deskCount)
	}

	// Remove desk and skip confirmation window.
	if err := conn.Call(ctx, nil, `async (deskId) => {
		await new Promise((resolve, reject) => {
			chrome.runtime.sendMessage(
				"kflgdebkpepnpjobkdfeeipcjdahoomc", {
					"messageType": "RemoveDesk",
					"operands": {
						"deskId": deskId,
						"skipConfirmation": true
					}
				},
				function(response) {
					if (response.errorMessage) {
						reject(new Error(chrome.runtime.lastError.message));
					}
					resolve();
				});
			});
		}`, deskID); err != nil {
		s.Fatal("Failed to remove desk: ", err)
	}

	if err := ash.WaitUntilDesksFinishAnimating(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for remove desk animation: ", err)
	}

	// Desk clean up is not synchronous. Wait before verify desk count.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		deskCount, err := ash.GetDeskCount(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get desks count"))
		}
		if deskCount == 1 {
			return nil
		}
		return errors.New("desks are not being removed")
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed to remove new desks")
	}

}
