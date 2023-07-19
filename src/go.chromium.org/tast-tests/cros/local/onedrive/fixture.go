// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package onedrive

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         "onedrive",
		Desc:         "Sets up 3 office files docx, pptx and xlsx. At tear down tries to remove them from the remote service via ODFS",
		Contacts:     []string{"lucmult@chromium.org", "chromeos-files-syd@chromum.org"},
		Impl:         &fixture{bt: browser.TypeAsh},
		SetUpTimeout: chrome.LoginTimeout,
		ResetTimeout: chrome.ResetTimeout,
		Parent:       "driveFsStartedWithOfficeEnabled", // TODO(b/291524698): Create more DriveFS accounts.
		Data:         []string{"Sample_DOCX_file_20230704.docx", "Sample_PPTX_file_20230704.pptx", "Sample_XLSX_file_20230704.xlsx"},
	})
}

// FixtureData is the struct exposed to tests.
type FixtureData struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn

	// Generated folder in MyFiles.
	TargetFolder string

	// Generated file name to be used in the test, the data file SrcDocx is copied into DocxName as part of the setup.
	SrcDocx  string
	DocxName string
	SrcPptx  string
	PptxName string
	SrcXlsx  string
	XlsxName string
}

type fixture struct {
	cr    *chrome.Chrome
	tconn *chrome.TestConn
	// chromeOptions []chrome.Option
	bt             browser.Type
	cleanUpFiles   []string
	screenRecorder *uiauto.ScreenRecorder
}

// generateTestFileName generates a unique-ish file name based on a provided
// prefix, the current time, and a random number.
func generateTestFileName(fName string) string {
	ext := filepath.Ext(fName)
	baseName := strings.TrimSuffix(fName, ext)
	return fmt.Sprintf("%s-%d-%d%s", baseName, time.Now().UnixNano(), rand.Intn(10000), ext)
}

func prepareOfficeFile(srcPath, targetFolder string) (srcFilePath, finalName string, err error) {
	finalName = generateTestFileName(filepath.Base(srcPath))
	srcFilePath = srcPath

	dst := filepath.Join(targetFolder, finalName)
	if err := fsutil.CopyFile(srcPath, dst); err != nil {
		return "", "", errors.Wrapf(err, "failed to copy the file %s  to: %s", srcPath, dst)

	}
	if err := os.Chown(dst, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		return "", "", errors.Wrapf(err, "failed to chown test file %s", dst)
	}

	return srcPath, finalName, nil
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr := s.ParentValue().(*drivefs.FixtureData).Chrome
	f.tconn = s.ParentValue().(*drivefs.FixtureData).TestAPIConn

	// Copy the docx, pptx and xlsx to MyFiles to be used in the tests.
	targetBaseName := "odfs_files"
	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's MyFiles path: ", err)
	}
	targetFolder := filepath.Join(myFilesPath, targetBaseName)
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		s.Fatal("Failed to create target dir: ", err, targetFolder)
	}
	if err := os.Chown(targetFolder, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		s.Fatal("Failed to chown the test folder: ", err, targetFolder)
	}

	var srcDocx, docx, srcPptx, pptx, srcXlsx, xlsx string
	if srcDocx, docx, err = prepareOfficeFile(s.DataPath("Sample_DOCX_file_20230704.docx"), targetFolder); err != nil {
		s.Fatal("Failed to prepare file: ", err)
	}
	if srcPptx, pptx, err = prepareOfficeFile(s.DataPath("Sample_PPTX_file_20230704.pptx"), targetFolder); err != nil {
		s.Fatal("Failed to prepare file: ", err)
	}
	if srcXlsx, xlsx, err = prepareOfficeFile(s.DataPath("Sample_XLSX_file_20230704.xlsx"), targetFolder); err != nil {
		s.Fatal("Failed to prepare file: ", err)
	}
	f.cleanUpFiles = append(f.cleanUpFiles, docx, pptx, xlsx)

	return &FixtureData{
		Chrome:       cr,
		TestAPIConn:  f.tconn,
		TargetFolder: targetBaseName,

		SrcDocx:  srcDocx,
		DocxName: docx,
		SrcPptx:  srcPptx,
		PptxName: pptx,
		SrcXlsx:  srcXlsx,
		XlsxName: xlsx,
	}
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	f.cleanUp(ctx, s)
}

func (f *fixture) Reset(ctx context.Context) error {
	return nil
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	recorder, err := uiauto.NewScreenRecorder(ctx, f.tconn)
	if err != nil {
		s.Log("Failed to create screen recorder: ", err)
	}
	if recorder != nil {
		if err := recorder.Start(ctx, f.tconn); err != nil {
			s.Log("Failed to start screen recorder: ", err)
		}
		f.screenRecorder = recorder
	}
}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.screenRecorder != nil {
		f.screenRecorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}

// cleanUp makes a best effort attempt to restore the state to where it was pretest.
func (f *fixture) cleanUp(ctx context.Context, s *testing.FixtState) {
	// NOTE: The deletion below fails if the files are open in the UI (Office 365 PWA).
	for _, name := range f.cleanUpFiles {
		files, err := filepath.Glob("/media/fuse/fusebox/fsp.*/" + name)
		if err != nil {
			s.Log("Failed cleaning up file: ", name, " ", err)
		} else {
			for _, file := range files {
				s.Log("Deleting: ", file)
				if err := os.Remove(file); err != nil {
					s.Log("Failed deleting the file: ", file, err)
				}
			}
		}
	}
	f.cleanUpFiles = []string{}
}
