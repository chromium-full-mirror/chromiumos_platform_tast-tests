// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         QueryOffline,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests Assistant query without network",
		Contacts: []string{
			"cros-assistive@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "chrome_internal", "gaia"},
		Fixture:      "assistantWithGaia",
	})
}

func QueryOffline(ctx context.Context, s *testing.State) {
	fixtData := s.FixtValue().(*assistant.FixtData)
	cr := fixtData.Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	queryOffline := func(ctx context.Context) error {
		// We should get an error when query offline.
		const query = "test"
		const expectedErrorResponse = "Interaction closed with resolution kError"
		s.Log("Sending text query to the Assistant")
		_, errorResponse := assistant.SendTextQuery(ctx, tconn, query)
		if errorResponse == nil {
			return errors.New("Should fail to get Assistant text query response")
		}

		// The err string should contain the expected response.
		if strings.Contains(errorResponse.Error(), expectedErrorResponse) {
			return nil
		}
		return errors.Wrap(errorResponse, "Unable to find an expected error message")
	}

	// Run query offline.
	if err := network.ExecFuncOnChromeOffline(ctx, queryOffline); err != nil {
		s.Fatal("Failed to test query offline: ", err)
	}
}
