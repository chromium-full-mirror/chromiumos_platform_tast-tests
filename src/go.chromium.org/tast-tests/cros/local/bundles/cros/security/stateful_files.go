// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	const (
		errorFile = "stateful_errors.txt"
		maxErrors = 5 // max to print
	)

	problems, numPaths, err := chk.CheckStatefulFiles(ctx)
	if err != nil {
		s.Error("Error checking stateful partition: ", err)
	}
	testing.ContextLogf(ctx, "Scanned %d path(s)", numPaths)

	f, err := os.Create(filepath.Join(s.OutDir(), errorFile))
	if err != nil {
		s.Fatal("Failed to create error file: ", err)
	}
	defer f.Close()
	for _, i := range problems {
		if _, err := fmt.Fprintf(f, "%v\n", i); err != nil {
			s.Fatal("Failed to write error file: ", err)
		}
	}

	var count = 0
	for _, i := range problems {
		if count > maxErrors {
			testing.ContextLogf(ctx, "Too many errors; aborting (see %v)", errorFile)
			break
		}
		s.Error(i)
	}
}
