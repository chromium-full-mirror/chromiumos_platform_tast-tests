// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalFiles,
		Desc:         "Helper test that uses data and output files",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Data: []string{
			"local_files_internal.txt",
			"local_files_external.txt",
		},
		Fixture: "metaLocalDataFilesFixture",
		Attr:    []string{"group:hw_agnostic"},
		// This test is executed by remote tests in the meta package.
	})
}

func LocalFiles(ctx context.Context, s *testing.State) {
	for _, fn := range []string{
		"local_files_internal.txt",
		"local_files_external.txt",
	} {
		s.Log("Copying ", fn)
		if err := fsutil.CopyFile(s.DataPath(fn), filepath.Join(s.OutDir(), fn)); err != nil {
			s.Errorf("Failed copying %s: %s", fn, err)
		}
	}
}
