// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"io"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeGaiaAPI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks Gaia calls credentials passing API properly",
		Contacts: []string{
			"cros-3pidp@google.com",
			"lmasopust@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1253671", // ChromeOS > Software > Commercial (Enterprise) > Identity > 3P IdP
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:mainline",
			"informational",
		},
		VarDeps: []string{
			ui.GaiaPoolDefaultVarName,
		},
		Timeout: chrome.GAIALoginTimeout + time.Minute,
		Params: []testing.Param{{
			Val: false,
		}, {
			Name:    "enrolled",
			Val:     false,
			Fixture: fixture.FakeDMSEnrolled,
		}, {
			Name: "sandbox",
			Val:  true,
		}},
	})
}

func ChromeGaiaAPI(ctx context.Context, s *testing.State) {
	useSandboxGaia := s.Param().(bool)
	fdms, ok := s.FixtValue().(*fakedms.FakeDMS)
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	options := []chrome.Option{chrome.GAIALoginPool(s.RequiredVar(ui.GaiaPoolDefaultVarName)), chrome.DeferLogin()}
	if useSandboxGaia {
		options = append(options, chrome.UseSandboxGaia())
	}
	if ok {
		options = append(options, chrome.KeepEnrollment())
		options = append(options, chrome.DMSPolicy(fdms.URL))
	}

	cr, err := chrome.New(
		ctx,
		options...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	defer cr.Close(cleanupCtx)
	logReader, err := syslog.NewChromeReader(ctx, syslog.ChromeLogFile)
	if err != nil {
		s.Fatal("Could not get Chrome log reader: ", err)
	}
	defer logReader.Close()

	if err = cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	// Verify that chrome log does not contain the error log. The log is protected from deletion by http://crrev.com/c/3528679.
	for {
		entry, err := logReader.Read()
		if err == io.EOF {
			break
		}
		if strings.Contains(entry.Content, "SamlHandler.onAPICall_") && entry.Severity == "WARNING" {
			s.Fatal("Found error in the Chrome log: ", entry.Content)
		}
	}
}
