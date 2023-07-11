// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"

	chk "go.chromium.org/tast-tests/cros/local/security/filecheck"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StatefulFiles,
		Desc: "Checks ownership and permissions of files on the stateful partition",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		BugComponent: "b:1040049",
		Attr:         []string{"group:enterprise-reporting", "group:mainline"},
	})
}

func StatefulFiles(ctx context.Context, s *testing.State) {
	if errors := chk.CheckStatefulFiles(ctx, s.OutDir()); len(errors) != 0 {
		for _, err := range errors {
			s.Error("Error checking stateful partition: ", err)
		}
	}
}
