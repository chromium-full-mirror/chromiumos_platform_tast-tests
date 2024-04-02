// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featured

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
			"chromeos-data-eng@google.com",
			"iby@chromium.org",
		},
		BugComponent: "b:1096648",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.MinLoginTimeout + 20*time.Second,
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
	// A sentinel error value used when we're not yet sure if the test has passed in the "experiment disabled, file exists" case.
	experimentDisabledSentinel = "experiment_disabled_sentinel"
	// A sentinel error value used when we're not yet sure if the test has passed in the "file does not exist" cases.
	fileNotExistSentinel = "file_not_exist_sentinel"
)

type keepWaiting struct {
	desc string
}

func (e keepWaiting) Error() string {
	return fmt.Sprintf("Keep waiting: %s", e.desc)
}

func (e keepWaiting) Is(target error) bool {
	switch target.(type) {
	case keepWaiting:
		return true
	default:
		return false
	}
}

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
	s.Log("Restarting featured")
	if err := upstart.RestartJob(ctx, "featured"); err != nil {
		s.Fatal("Failed to restart featured: ", err)
	}

	// TODO(b/274490519): This *might* race -- if chrome finishes logging in before featured
	// starts and registers its OnSessionStateChanged signal handler, featured might miss the
	// signal and hence fail to enable the feature when appropriate.
	// If we run into flakiness, add functionality to featured to write a file once it's registered
	// the signal handler.

	s.Log("Restarting chrome")
	arg := chrome.EnableFeatures(enabledFeature)
	if !params.ExperimentEnabled {
		arg = chrome.DisableFeatures(enabledFeature)
	}
	cr, err := chrome.New(ctx, arg)
	if err != nil {
		s.Fatal("Failed to create chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	if err := testing.Poll(ctx, func(c context.Context) error {
		if params.FileExist {
			// In all events, it should exist -- featured should not have removed it.
			if _, err := os.Stat(filePath); err != nil {
				return testing.PollBreak(errors.Wrapf(err, "failed to stat %s", filePath))
			}
			contents, err := os.ReadFile(filePath)
			if err != nil {
				return testing.PollBreak(errors.Wrapf(err, "failed to read %s", filePath))
			}
			if params.ExperimentEnabled {
				if string(contents) != expectedContents {
					return errors.Errorf("unexpected contents: got %q, want %q", string(contents), expectedContents)
				}
			} else {
				if len(contents) != 0 {
					return errors.Errorf("unexpected contents: got %q, wanted empty", string(contents))
				}
				// Return a special sentinel error so we do not immediately assume featured won't overwrite this
				// file, but instead keep waiting.
				// Later, we'll treat failures with this sentinel as successes.
				return keepWaiting{experimentDisabledSentinel}
			}
		} else {
			// In all events, it should NOT exist -- featured should not have created it.
			if _, err := os.Stat(filePath); err == nil {
				return testing.PollBreak(errors.Errorf("file %s existed but it should not have", filePath))
			}
			// Return a special sentinel error so we do not immediately assume featured won't create this
			// file, but instead keep waiting.
			// Later, we'll treat failures with this sentinel as successes.
			return keepWaiting{fileNotExistSentinel}
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		if errors.Is(err, keepWaiting{}) && !(params.FileExist && params.ExperimentEnabled) {
			// We expected to have a keepWaiting, and got one.
			s.Logf("Suceeding because err was %s", err.Error())
			return
		}
		s.Error("Failed to wait for featured: ", err)
	}
}
