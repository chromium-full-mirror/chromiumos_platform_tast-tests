// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PublicVars,
		Desc:         "Public variables",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		// example.PublicVars.foo is defined in tast-tests/vars/example.PublicVars.yaml
		VarDeps: []string{"example.PublicVars.foo"},
	})
}

func PublicVars(ctx context.Context, s *testing.State) {
	if x := s.RequiredVar("example.PublicVars.foo"); x != "bar" {
		s.Errorf(`Got %q, want "bar"`, x)
	}
}
