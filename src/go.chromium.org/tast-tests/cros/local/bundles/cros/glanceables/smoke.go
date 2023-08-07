// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package glanceables

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Smoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the basic ui elements for glanceables",
		Contacts: []string{
			"cros-system-ui-eng@google.com",
			"anasalazar@google.com",
			"tbarzic@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > System UI Surfaces > Glanceables
		BugComponent: "b:1362950",
		Attr:         []string{"group:mainline", "informational"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.GlanceablesEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Vars: []string{
			"glanceables.Smoke.studentUsername",
			"glanceables.Smoke.studentPassword",
			"glanceables.Smoke.teacherUsername",
			"glanceables.Smoke.teacherPassword",
			"glanceables.Smoke.regularUsername",
			"glanceables.Smoke.regularPassword",
		},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.ManagedUserLoginTimeout + 5*time.Minute,
	})
}

// Smoke tests the basic ui pieces of glanceables.
func Smoke(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	teacherView := nodewith.ClassName("ClassroomBubbleTeacherView")
	studentView := nodewith.ClassName("ClassroomBubbleStudentView")
	tasksView := nodewith.ClassName("TasksBubbleView")
	policies := []policy.Policy{&policy.GlanceablesEnabled{Val: true}}

	for _, param := range []struct {
		// name is the subtest name.
		name string
		// user is the username to log in.
		user string
		// pass is the password used to log in.
		pass              string
		enabledFeatures   []string
		showStudentBubble bool
		showTeacherBubble bool
		showTaskBubble    bool
	}{
		{
			name:              "student",
			user:              s.RequiredVar("glanceables.Smoke.studentUsername"),
			pass:              s.RequiredVar("glanceables.Smoke.studentPassword"),
			enabledFeatures:   []string{"GlanceablesV2"},
			showStudentBubble: true,
			showTeacherBubble: false,
			showTaskBubble:    true,
		},
		{
			name:              "teacher",
			user:              s.RequiredVar("glanceables.Smoke.teacherUsername"),
			pass:              s.RequiredVar("glanceables.Smoke.teacherPassword"),
			enabledFeatures:   []string{"GlanceablesV2", "GlanceablesV2ClassroomTeacherView"},
			showStudentBubble: false,
			showTeacherBubble: true,
			showTaskBubble:    true,
		},
		{
			name:              "managed",
			user:              s.RequiredVar("glanceables.Smoke.regularUsername"),
			pass:              s.RequiredVar("glanceables.Smoke.regularPassword"),
			enabledFeatures:   []string{"GlanceablesV2"},
			showStudentBubble: false,
			showTeacherBubble: false,
			showTaskBubble:    true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), param.user, policies)
			if err != nil {
				s.Fatal("Failed to setup fake policy server: ", err)
			}
			defer fdms.Stop(cleanupCtx)

			opts := []chrome.Option{
				chrome.EnableFeatures(param.enabledFeatures...),
				chrome.GAIALogin(chrome.Creds{User: param.user, Pass: param.pass}),
				chrome.DMSPolicy(fdms.URL),
			}

			cr, err := chrome.New(ctx, opts...)
			if err != nil {
				s.Fatal("Chrome login failed: ", err)
			}
			defer cr.Close(ctx)

			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to create Test API connection: ", err)
			}

			// Ensure chrome://policy shows correct GlanceablesEnabled value.
			if err := policyutil.Verify(ctx, tconn, policies); err != nil {
				s.Fatal("Failed to verify the value of GlanceablesEnabled policy: ", err)
			}

			ui := uiauto.New(tconn)

			if err := openGlanceablesBubble(ctx, ui); err != nil {
				s.Fatal("Failed to open the glanceables bubble: ", err)
			}
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			isUIElementVisible, err := isGlanceablesBubbleVisible(ctx, ui, studentView)
			if err != nil {
				s.Fatal("Failed to check visibility of glanceables student bubble: ", err)
			}
			if isUIElementVisible != param.showStudentBubble {
				s.Fatalf("Unexpected glanceables student bubble visibility state: got %t expected %t", isUIElementVisible, param.showStudentBubble)
			}

			isUIElementVisible, err = isGlanceablesBubbleVisible(ctx, ui, teacherView)
			if err != nil {
				s.Fatal("Failed to check visibility of glanceables teacher bubble: ", err)
			}
			if isUIElementVisible != param.showTeacherBubble {
				s.Fatalf("Unexpected glanceables teacher bubble visibility state: got %t expected %t", isUIElementVisible, param.showTeacherBubble)
			}

			isUIElementVisible, err = isGlanceablesBubbleVisible(ctx, ui, tasksView)
			if err != nil {
				s.Fatal("Failed to check visibility of glanceables tasks bubble: ", err)
			}
			if isUIElementVisible != param.showTaskBubble {
				s.Fatalf("Unexpected glanceables tasks bubble visibility state: got %t expected %t", isUIElementVisible, param.showStudentBubble)
			}

			// Close the date tray.
			dateTray := nodewith.HasClass("DateTray")
			if err := ui.DoDefault(dateTray)(ctx); err != nil {
				s.Fatal("Failed to close the date tray: ", err)
			}

			// Serve disabled policy and refresh policies to trigger the UI update.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{&policy.GlanceablesEnabled{Val: false}}); err != nil {
				s.Fatal("Failed to reset policies in Chrome: ", err)
			}

			if err := openGlanceablesBubble(ctx, ui); err != nil {
				s.Fatal("Failed to open the glanceables bubble: ", err)
			}

			isUIElementVisible, err = isGlanceablesBubbleVisible(ctx, ui, studentView)
			if err != nil {
				s.Fatal("Failed to check visibility of glanceables student bubble: ", err)
			}
			if isUIElementVisible != false {
				s.Fatalf("Unexpected glanceables student bubble visibility state: got %t expected %t", isUIElementVisible, false)
			}

			isUIElementVisible, err = isGlanceablesBubbleVisible(ctx, ui, teacherView)
			if err != nil {
				s.Fatal("Failed to check visibility of glanceables teacher bubble: ", err)
			}
			if isUIElementVisible != false {
				s.Fatalf("Unexpected glanceables teacher bubble visibility state: got %t expected %t", isUIElementVisible, false)
			}

			isUIElementVisible, err = isGlanceablesBubbleVisible(ctx, ui, tasksView)
			if err != nil {
				s.Fatal("Failed to check visibility of glanceables tasks bubble: ", err)
			}
			if isUIElementVisible != false {
				s.Fatalf("Unexpected glanceables tasks bubble visibility state: got %t expected %t", isUIElementVisible, false)
			}
		})
	}
}

// openGlanceablesBubble Opens the glanceables bubble by clicking on the shelf date tray and waits until the calendar view shows.
func openGlanceablesBubble(ctx context.Context, ui *uiauto.Context) error {
	dateTray := nodewith.HasClass("DateTray")
	if err := ui.DoDefault(dateTray)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the date tray")
	}

	calendarView := nodewith.ClassName("CalendarView")
	mainHeaderTriView := nodewith.ClassName("TriView").Ancestor(calendarView).Nth(0)
	mainHeaderContainer := nodewith.ClassName("View").Ancestor(mainHeaderTriView).Nth(1)
	mainHeader := nodewith.Name("Calendar").ClassName("Label").Ancestor(mainHeaderContainer)

	if err := ui.WaitUntilExists(mainHeader)(ctx); err != nil {
		return errors.Wrap(err, "failed to find calendar main label after opening calendar view")
	}
	return nil
}

// isGlanceablesBubbleVisible Checks the visibility of the glanceables bubble.
func isGlanceablesBubbleVisible(ctx context.Context, ui *uiauto.Context, bubble *nodewith.Finder) (bool, error) {
	if err := ui.WaitUntilExists(bubble)(ctx); err != nil {
		if err := ui.EnsureGoneFor(bubble, 3*time.Second)(ctx); err != nil {
			return false, errors.Wrap(err, "failed to verify the element visibility")
		}
		return false, nil
	}

	return true, nil
}
