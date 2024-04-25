// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/saml"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeSAML,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Chrome can make real SAML logins",
		Contacts: []string{
			"cros-3pidp@google.com",
			"lmasopust@google.com",
		},
		BugComponent: "b:1253671", // ChromeOS > Software > Commercial (Enterprise) > Identity > 3P IdP
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		VarDeps: []string{
			"saml.testidp_username",
			"saml.testidp_password",
			"ui.signinProfileTestExtensionManifestKey",
		},
		Params: []testing.Param{{
			Name:      "",
			ExtraAttr: []string{"group:hw_agnostic"},
		}, {
			Name:    "enrolled",
			Fixture: fixture.FakeDMSEnrolled,
		}},
		Timeout: chrome.GAIALoginTimeout + time.Minute,
	})
}

func ChromeSAML(ctx context.Context, s *testing.State) {
	username := s.RequiredVar("saml.testidp_username")
	password := s.RequiredVar("saml.testidp_password")
	fdms, ok := s.FixtValue().(*fakedms.FakeDMS)
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	opts := []chrome.Option{
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	}
	if ok {
		opts = append(opts, chrome.KeepEnrollment())
		opts = append(opts, chrome.DMSPolicy(fdms.URL))
	}

	cr, err := saml.LoginWithSAMLAccount(
		ctx,
		username,
		saml.HandleTestIdPLogin(username, password),
		opts...,
	)
	if err != nil {
		s.Fatal("Chrome SAML login failed: ", err)
	}
	defer cr.Close(cleanupCtx)
}
