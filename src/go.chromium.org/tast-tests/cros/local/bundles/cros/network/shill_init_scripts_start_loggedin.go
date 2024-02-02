// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/shillscript"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillInitScriptsStartLoggedin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that shill init scripts perform as expected",
		Contacts:     []string{"cros-networking@google.com", "hugobenichi@google.com"},
		BugComponent: "b:1493959",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:network", "network_platform"},
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
