// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package glanceables

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testCase struct {
	// user is the username to log in.
	user      string
	isManaged bool
	// pass is the password used to log in - only used if isManaged is set.
	pass            string
	enabledFeatures []string
	// whether glanceables are hidden if the policy value is not set
	showBubblesWithoutPolicy bool
	showStudentBubble        bool
	showTeacherBubble        bool
	showTaskBubble           bool
}

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
			pci.SearchFlag(&policy.GlanceablesEnabled{}, pci.VerifiedFunctionalityOS), {
				Key:   "feature_id",
				Value: "screenplay-ace3b729-5402-40cd-b2bf-d488bc95b7e2",
			},
		},
		Vars: []string{
			"glanceables.Smoke.studentUsername",
			"glanceables.Smoke.studentPassword",
			"glanceables.Smoke.teacherUsername",
			"glanceables.Smoke.teacherPassword",
			"glanceables.Smoke.regularUsername",
			"glanceables.Smoke.regularPassword",
		},
		VarDeps: []string{"ui.gaiaPoolDefault"},
		Params: []testing.Param{{
			Name: "student",
			Val: testCase{
				isManaged:         true,
				user:              "glanceables.Smoke.studentUsername",
				pass:              "glanceables.Smoke.studentPassword",
				enabledFeatures:   []string{"GlanceablesV2"},
				showStudentBubble: true,
				showTeacherBubble: false,
				showTaskBubble:    true,
			},
		}, {
			Name: "student_trusted_tester",
			Val: testCase{
				isManaged:         true,
				user:              "glanceables.Smoke.studentUsername",
				pass:              "glanceables.Smoke.studentPassword",
				enabledFeatures:   []string{"GlanceablesV2TrustedTesters"},
				showStudentBubble: true,
				showTeacherBubble: false,
				showTaskBubble:    true,
			},
		}, {
			Name: "teacher",
			Val: testCase{
				isManaged:         true,
				user:              "glanceables.Smoke.teacherUsername",
				pass:              "glanceables.Smoke.teacherPassword",
				enabledFeatures:   []string{"GlanceablesV2,GlanceablesV2ClassroomTeacherView"},
				showStudentBubble: false,
				showTeacherBubble: true,
				showTaskBubble:    true,
			},
		}, {
			Name: "teacher_trusted_tester",
			Val: testCase{
				isManaged:         true,
				user:              "glanceables.Smoke.teacherUsername",
				pass:              "glanceables.Smoke.teacherPassword",
				enabledFeatures:   []string{"GlanceablesV2TrustedTesters"},
				showStudentBubble: false,
				showTeacherBubble: false,
				showTaskBubble:    true,
			},
		}, {
			Name: "teacher_trusted_tester_with_teacher_view",
			Val: testCase{
				isManaged:         true,
				user:              "glanceables.Smoke.teacherUsername",
				pass:              "glanceables.Smoke.teacherPassword",
				enabledFeatures:   []string{"GlanceablesV2TrustedTesters,GlanceablesV2ClassroomTeacherView"},
				showStudentBubble: false,
				showTeacherBubble: true,
				showTaskBubble:    true,
			},
		}, {
			Name: "managed",
			Val: testCase{
				isManaged:         true,
				user:              "glanceables.Smoke.regularUsername",
				pass:              "glanceables.Smoke.regularPassword",
				enabledFeatures:   []string{"GlanceablesV2"},
				showStudentBubble: false,
				showTeacherBubble: false,
				showTaskBubble:    true,
			},
		}, {
			Name: "managed_trusted_tester",
			Val: testCase{
				isManaged:         true,
				user:              "glanceables.Smoke.regularUsername",
				pass:              "glanceables.Smoke.regularPassword",
				enabledFeatures:   []string{"GlanceablesV2TrustedTesters"},
				showStudentBubble: false,
				showTeacherBubble: false,
				showTaskBubble:    true,
			},
		}, {
			Name: "regular",
			Val: testCase{
				isManaged:         false,
				user:              "",
				pass:              "",
				enabledFeatures:   []string{"GlanceablesV2"},
				showStudentBubble: false,
				showTeacherBubble: false,
				showTaskBubble:    true,
			},
		}, {
			Name: "regular_trusted_tester_flag",
			Val: testCase{
				isManaged:         false,
				user:              "",
				pass:              "",
				enabledFeatures:   []string{"GlanceablesV2TrustedTesters"},
				showStudentBubble: false,
				showTeacherBubble: false,
				showTaskBubble:    false,
			},
		}},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.ManagedUserLoginTimeout + 5*time.Minute,
	})
}

// Smoke tests the basic ui pieces of glanceables.
func Smoke(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	param := s.Param().(testCase)

	var opts []chrome.Option
	var policies []policy.Policy
	var fdms *fakedms.FakeDMS
	if param.isManaged {
		policies = []policy.Policy{&policy.GlanceablesEnabled{Val: true}}

		fdmsLocal, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), s.RequiredVar(param.user), policies)
		if err != nil {
			s.Fatal("Failed to setup fake policy server: ", err)
		}
		fdms = fdmsLocal
		defer fdms.Stop(cleanupCtx)

		opts = []chrome.Option{
			chrome.EnableFeatures(param.enabledFeatures...),
			chrome.GAIALogin(chrome.Creds{User: s.RequiredVar(param.user), Pass: s.RequiredVar(param.pass)}),
			chrome.DMSPolicy(fdms.URL),
		}
	} else {
		opts = []chrome.Option{
			chrome.EnableFeatures(param.enabledFeatures...),
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		}
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if param.isManaged {
		// Ensure chrome://policy shows correct GlanceablesEnabled value.
		if err := policyutil.Verify(ctx, tconn, policies); err != nil {
			s.Fatal("Failed to verify the value of GlanceablesEnabled policy: ", err)
		}
	}

	ui := uiauto.New(tconn)

	if err := verifyGlanceableBubblesVisibility(ctx, ui, param.showTaskBubble, param.showStudentBubble, param.showTeacherBubble); err != nil {
		s.Fatal("Failed verifying bubbles with initial glanceables policy: ", err)
	}

	if param.isManaged {
		// Serve disabled policy and refresh policies to trigger the UI update.
		if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{&policy.GlanceablesEnabled{Val: false}}); err != nil {
			s.Fatal("Failed to reset policies in Chrome: ", err)
		}

		if err := verifyGlanceableBubblesVisibility(ctx, ui, false, false, false); err != nil {
			s.Fatal("Failed verifying bubbles with glanceables disabled by policy: ", err)
		}
	}
}

// verifyGlanceableBubblesVisibility shows time management surface by pressing date tray, and verifies visibility of different bubbles within the UI surface.
func verifyGlanceableBubblesVisibility(ctx context.Context, ui *uiauto.Context, showTaskBubble, showStudentBubble, showTeacherBubble bool) error {
	if err := openGlanceablesBubble(ctx, ui); err != nil {
		return errors.Wrap(err, "failed to open the glanceables bubble")
	}

	studentView := nodewith.ClassName("ClassroomBubbleStudentView")
	isUIElementVisible, err := isGlanceablesBubbleVisible(ctx, ui, studentView)
	if err != nil {
		return errors.Wrap(err, "failed to check visibility of glanceables student bubble")
	}
	if isUIElementVisible != showStudentBubble {
		return errors.Wrapf(err, "unexpected glanceables student bubble visibility state: got %t expected %t", isUIElementVisible, showStudentBubble)
	}

	teacherView := nodewith.ClassName("ClassroomBubbleTeacherView")
	isUIElementVisible, err = isGlanceablesBubbleVisible(ctx, ui, teacherView)
	if err != nil {
		return errors.Wrap(err, "failed to check visibility of glanceables teacher bubble")
	}
	if isUIElementVisible != showTeacherBubble {
		return errors.Wrapf(err, "unexpected glanceables teacher bubble visibility state: got %t expected %t", isUIElementVisible, showTeacherBubble)
	}

	tasksView := nodewith.ClassName("TasksBubbleView")
	isUIElementVisible, err = isGlanceablesBubbleVisible(ctx, ui, tasksView)
	if err != nil {
		return errors.Wrap(err, "failed to check visibility of glanceables tasks bubble")
	}
	if isUIElementVisible != showTaskBubble {
		return errors.Wrapf(err, "unexpected glanceables tasks bubble visibility state: got %t expected %t", isUIElementVisible, showTaskBubble)
	}

	// Close the date tray.
	dateTray := nodewith.HasClass("DateTray")
	if err := ui.DoDefault(dateTray)(ctx); err != nil {
		return errors.Wrap(err, "failed to close the date tray")
	}

	return nil
}

// openGlanceablesBubble Opens the glanceables bubble by clicking on the shelf date tray and waits until the calendar view shows.
func openGlanceablesBubble(ctx context.Context, ui *uiauto.Context) error {
	dateTray := nodewith.HasClass("DateTray")
	if err := ui.DoDefault(dateTray)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the date tray")
	}

	calendarView := nodewith.ClassName("CalendarView")
	mainHeaderTriView := nodewith.ClassName("TriView").Ancestor(calendarView).Nth(0)
	mainHeaderContainer := nodewith.ClassName("RelayoutView").Ancestor(mainHeaderTriView).Nth(1)
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
