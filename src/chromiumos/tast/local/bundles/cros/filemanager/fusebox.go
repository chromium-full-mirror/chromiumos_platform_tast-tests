// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Fusebox,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Mount fusebox daemon and verify it responds to requests",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@chromium.org",
			"nigeltao@chromium.org",
			"noel@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
	})
}

func Fusebox(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Logging into Chrome should launch Fusebox (via cros-disks).
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}

	// Poll until the "fuse_status" file shows up. The "fuse_status" and "ok\n"
	// magic strings are defined in "platform2/fusebox/built_in.cc".
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		const fuseStatusFilename = "/media/fuse/fusebox/built_in/fuse_status"
		if got, err := os.ReadFile(fuseStatusFilename); err != nil {
			return err
		} else if want := "ok\n"; string(got) != want {
			return testing.PollBreak(errors.Errorf("got %q, want %q", got, want))
		}
		return nil
	}, nil); err != nil {
		s.Fatal("ReadFile(fuse_status) failed: ", err)
	}

	// Make a temporary directory.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}
	tdd := tempDirData{}
	if err := tconn.Call(ctx, &tdd, `tast.promisify(chrome.autotestPrivate.makeFuseboxTempDir)`); err != nil {
		s.Fatal("makeFuseboxTempDir failed: ", err)
	} else if tdd.FuseboxFilePath == "" {
		s.Fatal("FuseboxFilePath is empty")
	}
	defer tconn.Call(cleanupCtx, nil, `chrome.autotestPrivate.removeFuseboxTempDir`, tdd.FuseboxFilePath)

	// That temporary directory has two names: a fusebox one at
	// "/media/fuse/fusebox/tmp.foo" and an underlying one at "/tmp/.foo".
	// Creating "wfru.txt" in the first should be visible in the second and
	// vice versa for "wurf.txt".
	checkFuseboxRoundTrip(s, tdd.FuseboxFilePath, tdd.UnderlyingFilePath,
		"wfru.txt", "write fusebox; read underlying")
	checkFuseboxRoundTrip(s, tdd.UnderlyingFilePath, tdd.FuseboxFilePath,
		"wurf.txt", "write underlying; read fusebox")
}

func checkFuseboxRoundTrip(s *testing.State, writeFilePath, readFilePath, baseName, data string) {
	writeFilename := filepath.Join(writeFilePath, baseName)
	readFilename := filepath.Join(readFilePath, baseName)
	if err := os.WriteFile(writeFilename, []byte(data), 0777); err != nil {
		s.Fatalf("WriteFile(%q) failed: %v", writeFilename, err)
	} else if got, err := os.ReadFile(readFilename); err != nil {
		s.Fatalf("ReadFile(%q) failed: %v", readFilename, err)
	} else if string(got) != data {
		s.Fatalf("ReadFile(%q): got %q, want %q", readFilename, got, data)
	}
}

type tempDirData struct {
	FuseboxFilePath    string `json:"fuseboxFilePath"`
	UnderlyingFilePath string `json:"underlyingFilePath"`
}
