// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"

	"chromiumos/tast/local/bundles/cros/network/shillscript"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/shill"
	"chromiumos/tast/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillInitScriptsStartLoggedin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that shill init scripts perform as expected",
		Contacts:     []string{"cros-networking@google.com", "hugobenichi@google.com"},
		BugComponent: "b:156085",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:network", "network_platform_unstable"},
	})
}

func ShillInitScriptsStartLoggedin(ctx context.Context, s *testing.State) {
	if err := shillscript.RunTest(ctx, testStartLoggedIn, false); err != nil {
		s.Fatal("Failed running testStartLoggedIn: ", err)
	}
}

// testStartLoggedIn tests starting up shill while user is already logged in.
func testStartLoggedIn(ctx context.Context, env *shillscript.TestEnv) error {
	cr, err := chrome.New(ctx)
	if err != nil {
		return errors.Wrap(err, "Chrome failed to log in")
	}
	defer cr.Close(ctx)

	if err := upstart.StartJob(ctx, shill.JobName); err != nil {
		return errors.Wrap(err, "failed starting shill")
	}

	return nil
}
