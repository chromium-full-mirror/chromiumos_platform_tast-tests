// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SecretVars,
		// Document: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#secret-variables
		Desc:         "Secret variables",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		// example.SecretVars.password is defined in tast-tests-private/vars/example.SecretVars.yaml
		// example.commonVar is defined in tast-tests-private/vars/example.yaml
		VarDeps: []string{"example.SecretVars.password", "example.commonVar"},
	})
}

func SecretVars(ctx context.Context, s *testing.State) {
	if x := s.RequiredVar("example.SecretVars.password"); x != "passw0rd" {
		// Note: Don't log secrets in real tests.
		s.Errorf(`Got %q, want "passw0rd"`, x)
	}
	if s.RequiredVar("example.commonVar") == "" {
		s.Error("example.commonVar is unexpectedly empty")
	}
}
