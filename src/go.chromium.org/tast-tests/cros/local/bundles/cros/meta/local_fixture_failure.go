// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalFixtureFailure,
		Desc:         "Test to check whether local fixture can access DUT features",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Fixture:      "metaLocalSetupFailureFixture",
		Attr:         []string{"group:hw_agnostic"},
	})
}

func LocalFixtureFailure(ctx context.Context, s *testing.State) {
	s.Log("Intentional local fixture failure")
}
