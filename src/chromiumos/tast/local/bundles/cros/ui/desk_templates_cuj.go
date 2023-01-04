// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/optin"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/event"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/saveddesks"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeskTemplatesCUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the performance of desks templates",
		Contacts: []string{
			"chromeos-wmp@google.com",
			"cros-commercial-productivity-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"zhumatthew@google.com",
			"yzd@google.com",
			"aprilzhou@google.com",
		},
		// Chrome OS Server Projects > Enterprise Management > Commercial Productivity
		BugComponent: "b:1020793",
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 3*time.Minute,
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		Params: []testing.Param{{
			Fixture: "savedDesksCUJEnableWithoutArc",
			Val:     []apps.App{apps.FilesSWA},
		}, {
			Name:              "arc_enabled",
			Fixture:           "savedDesksCUJEnableWithArc",
			Val:               []apps.App{apps.FilesSWA, apps.PlayStore},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func DeskTemplatesCUJ(ctx context.Context, s *testing.State) {
	// TODO(b/238645466): Remove `no_kernel_upstream` from SoftwareDeps once kernel_uprev boards are more stable.
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(*saveddesks.SavedDeskFixtData).Chrome
	// Set up the apps to launch list.
	appsList := s.Param().([]apps.App)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	// Close all existing windows.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	ac := uiauto.New(tconn)
	// Delete all existing saved desks.
	if err := saveddesks.DeleteSavedDesks(cleanupCtx, tconn, ac); err != nil {
		s.Fatal("Failed to delete saved desks: ", err)
	}
	// Set up metrics recorder for TPS calculation
	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}

	defer func(ctx context.Context) {
		if err := recorder.Close(ctx); err != nil {
			s.Error("Failed to stop recorder: ", err)
		}
	}(cleanupCtx)

	if err := recorder.AddCommonMetrics(tconn, tconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	recorder.EnableTracing(s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile))

	pv := perf.NewValues()
	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// Open PlayStore, Chrome and Files.
		browserApp, err := apps.PrimaryBrowser(ctx, tconn)
		if err != nil {
			s.Fatal("Could not find the primary browser app info: ", err)
		}
		appsList = append(appsList, browserApp)
		if err := saveddesks.OpenApps(ctx, tconn, ac, appsList); err != nil {
			return errors.Wrap(err, "failed to open apps")
		}

		// Enter overview mode.
		if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
			return errors.Wrap(err, "error in setting overview mode")
		}
		if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for overview animation to be completed")
		}
		defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

		// Save current desk as `Template 1` of type `Template`.
		if err := ash.SaveCurrentDesk(ctx, ac, ash.Template, "Template 1"); err != nil {
			return errors.Wrap(err, "failed to save current desk as 'Template 1' of type 'Template'")
		}

		// Verify saved desk.
		if err := ash.VerifySavedDesk(ctx, ac, []string{"Template 1"}); err != nil {
			return errors.Wrap(err, "failed to verify saved desk")
		}

		// Exit overview mode.
		if err = ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
			return errors.Wrap(err, "unable to exit overview mode")
		}
		if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for overview animation to be completed")
		}

		// Close Play Store.
		if err := optin.ClosePlayStore(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close Play Store")
		}

		if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for close Play store action to be completed")
		}

		// Close all existing windows.
		if err := ash.CloseAllWindows(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all windows")
		}

		// Enter overview mode, and launch the saved desk template.
		if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
			return errors.Wrap(err, "unable to set overview mode")
		}
		if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for overview animation to be completed")
		}

		// Enter library page.
		if err := ash.EnterLibraryPage(ctx, ac); err != nil {
			return errors.Wrap(err, "failed to enter library page")
		}

		// Confirm there is one desk template.
		deskTemplatesInfo, err := ash.FindSavedDesks(ctx, ac)
		if err != nil {
			return errors.Wrap(err, "unable to find desk templates")
		}
		if len(deskTemplatesInfo) != 1 {
			return errors.Errorf("got %v desk template(s), there should be one desk template", len(deskTemplatesInfo))
		}

		// Launch saved desk `Template 1` of type `Template`.
		if err := ash.LaunchSavedDesk(ctx, ac, "Template 1", 0); err != nil {
			return errors.Wrap(err, "failed to launch saved desk 'Template 1' of type 'Template'")
		}

		// Exit overview mode.
		if err = ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
			return errors.Wrap(err, "unable to exit overview mode")
		}
		if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for overview animation to be completed")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
}
