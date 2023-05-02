// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featured

import (
	"context"
	"os"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type latePlatformParams struct {
	FileExist         bool
	ExperimentEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LatePlatformFeatures,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify platform-features.json enables features at login",
		Contacts: []string{
			"cros-telemetry@google.com",
			"mutexlox@google.com",
			"kendraketsui@google.com",
		},
		BugComponent: "b:1096648",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "file_exists_enabled",
			Val: latePlatformParams{
				FileExist:         true,
				ExperimentEnabled: true,
			},
		}, {
			Name: "file_exists_disabled",
			Val: latePlatformParams{
				FileExist:         true,
				ExperimentEnabled: false,
			},
		}, {
			Name: "file_not_exist_enabled",
			Val: latePlatformParams{
				FileExist:         false,
				ExperimentEnabled: true,
			},
		}, {
			Name: "file_not_exist_disabled",
			Val: latePlatformParams{
				FileExist:         false,
				ExperimentEnabled: false,
			},
		}},
	})
}

const (
	// enabledFeature is the name of the test feature in platform-features.json.
	// It should always match the name in that file exactly.
	enabledFeature = "CrOSLateBootTestFeature"
	// dirPath is the path to the directory this test uses.
	dirPath = "/run/featured_test"
	// filePath is the file whose existence gates the behavior of featured.
	// If it exists and the experiment is enabled, featured should write a string to it.
	// Otherwise, it should do nothing.
	filePath = "/run/featured_test/test_write"
	// expectedContents is the expected contents of the filePath after featured writes to it.
	expectedContents = "test_featured"
)

func LatePlatformFeatures(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	params := s.Param().(latePlatformParams)

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		s.Fatalf("Failed to create directory %s: %v", dirPath, err)
	}
	defer func() {
		if err := os.RemoveAll(dirPath); err != nil {
			s.Errorf("Failed to remove %s: %v", dirPath, err)
		}
	}()

	if params.FileExist {
		if err := os.WriteFile(filePath, []byte{}, 0664); err != nil {
			s.Fatalf("Failed to write %s: %v", filePath, err)
		}
	} else {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			s.Fatalf("Failed to remove %s: %v", filePath, err)
		}
	}

	// Restart featured.
	if err := upstart.RestartJob(ctx, "featured"); err != nil {
		s.Fatal("Failed to restart featured: ", err)
	}

	// TODO(b/274490519): This *might* race -- if chrome finishes logging in before featured
	// starts and registers its OnSessionStateChanged signal handler, featured might miss the
	// signal and hence fail to enable the feature when appropriate.
	// If we run into flakiness, add functionality to featured to write a file once it's registered
	// the signal handler.

	arg := chrome.EnableFeatures(enabledFeature)
	if !params.ExperimentEnabled {
		arg = chrome.DisableFeatures(enabledFeature)
	}
	cr, err := chrome.New(ctx, arg)
	if err != nil {
		s.Fatal("Failed to create chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	if params.FileExist {
		// In all events, it should exist -- featured should not have removed it.
		if _, err := os.Stat(filePath); err != nil {
			s.Fatalf("Failed to stat %s: %v", filePath, err)
		}
		contents, err := os.ReadFile(filePath)
		if err != nil {
			s.Fatalf("Failed to read %s: %v", filePath, err)
		}
		if params.ExperimentEnabled {
			if string(contents) != expectedContents {
				s.Fatalf("Unexpected contents: got %q, want %q", string(contents), expectedContents)
			}
		} else {
			if len(contents) != 0 {
				s.Fatalf("Unexpected contents: got %q, wanted empty", string(contents))
			}
		}
	} else {
		// In all events, it should NOT exist -- featured should not have created it.
		if _, err := os.Stat(filePath); err == nil {
			s.Fatalf("File %s existed but it should not have", filePath)
		}
	}
}
