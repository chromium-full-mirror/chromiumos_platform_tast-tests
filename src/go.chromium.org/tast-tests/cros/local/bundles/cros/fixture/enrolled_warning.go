// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnrolledWarning,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Showing errors of failed attempts in enrolled fixture",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"vsavu@google.com", // Test owner
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

func EnrolledWarning(ctx context.Context, s *testing.State) {
	// TODO(b/243629567): Remove this test when the workarounds from the enrolled fixture
	// are removed.
	var fixtData policy.EnrolledFixtureData

	if err := s.FixtFillValue(&fixtData); err != nil {
		s.Fatal("Failed to deserialize fixture data with FixtFillValue: ", err)
	}

	for _, err := range fixtData.Errors {
		s.Error("Failed enrollment attempt in the Enrolled fixture: ", err)
	}
}
