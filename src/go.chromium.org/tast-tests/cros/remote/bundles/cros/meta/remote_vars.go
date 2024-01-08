// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteVars,
		Desc:         "Helper test that inspects a runtime variable",
		Contacts:     []string{"tast-core@google.com", "nya@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		VarDeps:      []string{"meta.RemoteVars.var"},
		Attr:         []string{"group:hw_agnostic"},
		// This test is called by remote tests in the meta package.
	})
}

func RemoteVars(ctx context.Context, s *testing.State) {
	p := filepath.Join(s.OutDir(), "var.txt")
	if err := ioutil.WriteFile(p, []byte(s.RequiredVar("meta.RemoteVars.var")), 0644); err != nil {
		s.Error("Failed to write variable: ", err)
	}
}
