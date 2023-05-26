// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Enrolled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Indicator test for the enrolled fixture",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"gabormagda@google.com", // Test author
		},
		BugComponent: "b:1170223", // ChromeOS > Software > Commercial (Enterprise) > EngProd
		Params: []testing.Param{{
			Name:      "golden",
			ExtraAttr: []string{"group:golden_tier"},
		}, {
			Name:      "medium",
			ExtraAttr: []string{"group:medium_low_tier"},
		}, {
			Name:      "hardware",
			ExtraAttr: []string{"group:hardware"},
		}, {
			Name:      "complementary",
			ExtraAttr: []string{"group:complementary"},
		}},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.Enrolled,
	})
}

func Enrolled(ctx context.Context, s *testing.State) {
	// The test should always pass.
	// When this test fails, it indicates a fixture failure.
}
