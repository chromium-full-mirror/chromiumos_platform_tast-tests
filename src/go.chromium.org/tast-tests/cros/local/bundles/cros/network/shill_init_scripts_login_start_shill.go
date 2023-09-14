// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/shillscript"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillInitScriptsLoginStartShill,
		Desc:         "Test that shill init scripts perform as expected",
		Contacts:     []string{"cros-networking@google.com", "hugobenichi@google.com"},
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "group:network", "network_cq"},
	})
}

func ShillInitScriptsLoginStartShill(ctx context.Context, s *testing.State) {
	if err := shillscript.RunTest(ctx, testStartShill, false); err != nil {
		s.Fatal("Failed running testStartShill: ", err)
	}
}

// testStartShill tests all created path names during shill startup.
func testStartShill(ctx context.Context, env *shillscript.TestEnv) error {
	if err := upstart.StartJob(ctx, shill.JobName); err != nil {
		return errors.Wrap(err, "failed starting shill")
	}

	if err := shillscript.AssureIsDir("/run/shill"); err != nil {
		return err
	}

	if err := shillscript.AssureIsDir("/var/lib/dhcpcd"); err != nil {
		return err
	}

	if err := shillscript.AssurePathOwner("/var/lib/dhcpcd", "dhcp"); err != nil {
		return err
	}

	if err := shillscript.AssurePathGroup("/var/lib/dhcpcd", "dhcp"); err != nil {
		return err
	}

	if err := shillscript.AssureIsDir("/run/dhcpcd"); err != nil {
		return err
	}

	if err := shillscript.AssurePathOwner("/run/dhcpcd", "dhcp"); err != nil {
		return err
	}

	if err := shillscript.AssurePathGroup("/run/dhcpcd", "dhcp"); err != nil {
		return err
	}

	return nil
}
