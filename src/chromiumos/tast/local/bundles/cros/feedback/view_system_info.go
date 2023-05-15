// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package feedback

import (
	"context"
	"encoding/base64"
	"regexp"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/feedbackapp"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type viewSystemInfoParam struct {
	uiautoTimeout    time.Duration
	validatePerfData bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ViewSystemInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify user can click and view system info",
		Contacts: []string{
			"cros-feedback-app@google.com",
			"xiangdongkong@google.com",
		},
		// ChromeOS > Data > Engineering > Feedback
		BugComponent: "b:1033360",
		Fixture:      "chromeLoggedInWithOsFeedback",
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-3f028d06-0100-4b5b-b1f3-99ceeaf3d62b",
			},
		},
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{{
			Val: viewSystemInfoParam{
				uiautoTimeout:    20 * time.Second,
				validatePerfData: false,
			},
		}, {
			Name: "validate_perf_data",
			Val: viewSystemInfoParam{
				uiautoTimeout:    100 * time.Second,
				validatePerfData: true},
		}},
	})
}

// validatePerfData validates that the perf-data content can be base64-decoded.
func validatePerfData(ctx context.Context, ui *uiauto.Context) error {
	// perf-data is multiline and needs to be expanded. Wait until it exists and click on it.
	perfDataExpandButton := nodewith.NameStartingWith("Expand").NameContaining("perf-data").Role(role.Button)
	if err := uiauto.Combine("Expand perf-data",
		ui.WaitUntilExists(perfDataExpandButton),
		ui.DoDefault(perfDataExpandButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to expand the perf-data item")
	}

	// Wait until the content of perf-data exists. The first line contains the description.
	perfDataText := nodewith.NameStartingWith("perf-data contains performance profiling information").Role(role.StaticText)
	if err := ui.WaitUntilExists(perfDataText)(ctx); err != nil {
		return errors.Wrap(err, "failed to find perf-data")
	}

	perfDataInfo, err := ui.NodesInfo(ctx, perfDataText)
	if err != nil || len(perfDataInfo) != 1 {
		return errors.Wrap(err, "failed to extract node info")
	}
	text := perfDataInfo[0].Name

	// Extract the base64 blob.
	base64BlobRe := regexp.MustCompile(`<base64>: ([0-9a-zA-Z\/\+]+=*)$`)
	groups := base64BlobRe.FindStringSubmatch(text)
	if len(groups) != 2 {
		return errors.Errorf("unexpected perf-data content: %s", text)
	}
	perfDataBlob := groups[1]

	if _, err = base64.StdEncoding.DecodeString(perfDataBlob); err != nil {
		return errors.Wrap(err, "failed to base64-decode perf data")
	}

	return nil
}

// ViewSystemInfo verifies user can click and view system info.
func ViewSystemInfo(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	params := s.Param().(viewSystemInfoParam)
	ui := uiauto.New(tconn).WithTimeout(params.uiautoTimeout)

	// Launch feedback app and go to share data page.
	feedbackRootNode, err := feedbackapp.LaunchAndGoToShareDataPage(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch feedback app and go to share data page: ", err)
	}

	// Click system and app info link.
	systemAndAppInfo := nodewith.Name("system & app info").Role(
		role.Link).Ancestor(feedbackRootNode)
	if err := ui.DoDefault(systemAndAppInfo)(ctx); err != nil {
		s.Fatal("Failed to find and click system and app info link: ", err)
	}

	// Verify user can view system info details.
	systemInfoDetails := nodewith.Name("System Information Preview").First()
	if err := ui.WaitUntilExists(systemInfoDetails)(ctx); err != nil {
		s.Error("Failed to view system and app info: ", err)
	}

	if params.validatePerfData {
		if err := validatePerfData(ctx, ui); err != nil {
			s.Fatal("Failed to validate perf-data: ", err)
		}
	}
}
