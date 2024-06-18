// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/pvs/pvsutils"
	"go.chromium.org/tast/core/testing"
)

var wantHomeDirContents = []string{".gitcookies", ".ssh", ".pvs"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShopUnpack,
		Desc:         "Verifies shop unpack started the container sucessfully",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"bbrotherton@google.com",
		},
		Attr:    []string{"group:pvs", "pvs_shop_daily"},
		Timeout: 20 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// ShopUnpack validates container is started up after shop unpack runs
func ShopUnpack(ctx context.Context, s *testing.State) {
	pvsRunner := pvsutils.PVSRunner{
		Dut:         s.DUT().Conn(),
		ContainerID: s.FixtValue().(string),
	}
	_, _, err := pvsRunner.RunCommandInContainer(ctx, "pvs --help")
	if err != nil {
		s.Fatal("Error occurred when trying to run 'pvs --help' in container: ", err)
	}
}
