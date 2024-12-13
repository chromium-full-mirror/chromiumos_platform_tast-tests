// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/local/ready"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NotEnterpriseOwned,
		Desc:         "Check that the DUT is not enterprise owned",
		BugComponent: "b:1111632",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test author
			"nya@chromium.org",
		},
		Attr: []string{"group:mainline", "group:cq-medium", "group:hw_agnostic"},
	})
}

func NotEnterpriseOwned(ctx context.Context, s *testing.State) {
	contents, err := os.ReadFile(ready.EnterpriseOwnedLogLocation)
	if os.IsNotExist(err) {
		return // Test passed as error log is missing.
	}

	if err != nil {
		s.Fatalf("Failed to read error log %q: %v", ready.ClearPoliciesLogLocation, err)
	}

	for _, line := range strings.Split(string(contents), "\n") {
		s.Errorf("Found error log for ready.go: %s", line)
	}
}
