// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/bundles/cros/arc/storage"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/drivefs"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Drivefs,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Android app can read files on Drive FS (Google Drive) via FilesApp",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "chrome_internal", "drivefs"},
		Timeout:      4 * time.Minute,
		VarDeps:      []string{"arc.Drivefs.user1", "arc.Drivefs.password1"},
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_p"},
			}, {
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
	})
}

func Drivefs(ctx context.Context, s *testing.State) {
	const filename = "storage_drivefs.txt"

	cr, err := chrome.New(
		ctx,
		chrome.ARCEnabled(),
		chrome.UnRestrictARCCPU(),
		chrome.GAIALogin(chrome.Creds{
			User: s.RequiredVar("arc.Drivefs.user1"),
			Pass: s.RequiredVar("arc.Drivefs.password1"),
		}),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	a, err := arc.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(ctx)

	vmEnabled, err := arc.VMEnabled()
	if err != nil {
		s.Fatal("Failed to check if VM is enabled: ", err)
	}

	mountPath, err := drivefs.WaitForDriveFs(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed waiting for DriveFS to start: ", err)
	}
	drivefsRoot := path.Join(mountPath, "root")

	// Since the test account can be shared with other concurrently running
	// tests, create the test file only when it's missing, and do not remove it
	// after the test (b/179876719).
	testFilePath := filepath.Join(drivefsRoot, filename)
	if _, err := os.Stat(testFilePath); errors.Is(err, os.ErrNotExist) {
		if err := ioutil.WriteFile(testFilePath, []byte(storage.ExpectedFileContent), 0666); err != nil {
			s.Fatalf("Failed to create test file %s: %v", testFilePath, err)
		}
	}

	config := storage.TestConfig{DirName: "Google Drive", FileName: filename,
		DirTitle: "Files - My Drive", CheckFileType: true}

	var verifyContentURI (func(string) bool)
	if vmEnabled {
		subPath := filepath.Join("MyDrive", "root", config.FileName)
		verifyContentURI = arc.VerifyContentURIForArcVolumeProviderPath(subPath)
	} else {
		const mountPathPrefix = "/media/fuse/"
		absPath := filepath.Join(drivefsRoot, config.FileName)
		if !strings.HasPrefix(absPath, mountPathPrefix) {
			s.Fatalf("%v does not start with %v", absPath, mountPathPrefix)
		}
		relPath := strings.TrimPrefix(absPath, mountPathPrefix)
		expected := arc.ChromeContentProviderURIPrefix + "externalfile%3A" + url.PathEscape(relPath)
		verifyContentURI = func(actual string) bool {
			return actual == expected
		}
	}

	expectations := []storage.Expectation{
		{LabelID: storage.ActionID, Value: storage.ExpectedAction},
		{LabelID: storage.URIID, Predicate: verifyContentURI},
		{LabelID: storage.FileContentID, Value: storage.ExpectedFileContent}}

	if err := storage.TestOpenWithAndroidApp(ctx, a, cr, d, config, expectations); err != nil {
		s.Fatal("Failed to open file with Android app: ", err)
	}
}
