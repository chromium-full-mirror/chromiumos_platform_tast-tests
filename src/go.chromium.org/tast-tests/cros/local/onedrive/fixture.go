// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package onedrive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesinternals"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// OdfsUnpackedLocation makes tests run the unpacked version of ODFS instead of
// the version from Web Store.
var OdfsUnpackedLocation = testing.RegisterVarString(
	"onedrive.OdfsUnpackedLocation",
	"",
	"ODFS unpacked extension location",
)

func init() {
	var opts = []chrome.Option{chrome.EnableFeatures("UploadOfficeToCloud"), chrome.ExtraArgs("--disable-sync", "--vmodule=cloud_upload*=3", "--extension-force-channel=dev")}

	testing.AddFixture(&testing.Fixture{
		Name:         "onedrive",
		Desc:         "Sets up 3 office files docx, pptx and xlsx. At tear down tries to remove them from the remote service via ODFS",
		Contacts:     []string{"lucmult@chromium.org", "chromeos-files-syd@google.com"},
		BugComponent: "b:167289",
		Impl: &onedriveFixture{
			bt:            browser.TypeAsh,
			chromeOptions: opts,
			provider:      filesconsts.OneDrive,
		},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: 30 * time.Second,
		PreTestTimeout:  60 * time.Second,
		PostTestTimeout: 30 * time.Second,
		Data:            []string{"Sample_DOCX_file_20230704.docx", "Sample_PPTX_file_20230704.pptx", "Sample_XLSX_file_20230724.xlsx"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "onedriveLacros",
		Desc:         "Lacros variant of onedrive",
		Contacts:     []string{"lucmult@chromium.org", "chromeos-files-syd@google.com"},
		BugComponent: "b:167289",
		Impl: &onedriveFixture{
			bt:            browser.TypeLacros,
			chromeOptions: opts,
			provider:      filesconsts.OneDrive,
		},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: 30 * time.Second,
		PreTestTimeout:  60 * time.Second,
		PostTestTimeout: 30 * time.Second,
		Data:            []string{"Sample_DOCX_file_20230704.docx", "Sample_PPTX_file_20230704.pptx", "Sample_XLSX_file_20230724.xlsx"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "onedriveManaged",
		Desc:         "Enterprise variant of onedrive with the corresponding policies set to 'allowed'",
		Contacts:     []string{"lmasopust@google.com", "cros-commercial-clippy-eng@google.com"},
		BugComponent: "b:1401215", // ChromeOS > Software > Commercial (Enterprise) > Identity > 3P IdP > Enterprise Clippy
		Impl: &onedriveFixture{
			bt:            browser.TypeAsh,
			chromeOptions: opts,
			provider:      filesconsts.OneDrive,
		},
		Parent:          fixture.FakeDMS,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: 30 * time.Second,
		PreTestTimeout:  60 * time.Second,
		PostTestTimeout: 30 * time.Second,
		Data:            []string{"Sample_DOCX_file_20230704.docx", "Sample_PPTX_file_20230704.pptx", "Sample_XLSX_file_20230724.xlsx"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "onedriveAndGoogleDrive",
		Desc:         "Sets up 3 office files docx, pptx and xlsx. At tear down tries to remove them from the remote service via ODFS",
		Contacts:     []string{"lucmult@chromium.org", "chromeos-files-syd@google.com"},
		BugComponent: "b:167289",
		Impl: &onedriveFixture{
			bt:       browser.TypeAsh,
			provider: filesconsts.DriveFs,
		},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: 30 * time.Second,
		PreTestTimeout:  60 * time.Second,
		PostTestTimeout: 30 * time.Second,
		Parent:          "driveFsStartedWithOfficeEnabled", // TODO(b/291524698): Create more DriveFS accounts.
		Data:            []string{"Sample_DOCX_file_20230704.docx", "Sample_PPTX_file_20230704.pptx", "Sample_XLSX_file_20230724.xlsx"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "onedriveAndGoogleDriveLacros",
		Desc:         "Lacros variant of onedriveAndGoogleDrive",
		Contacts:     []string{"lucmult@chromium.org", "chromeos-files-syd@google.com"},
		BugComponent: "b:167289",
		Impl: &onedriveFixture{
			bt:       browser.TypeLacros,
			provider: filesconsts.DriveFs,
		},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: 30 * time.Second,
		PreTestTimeout:  60 * time.Second,
		PostTestTimeout: 30 * time.Second,
		Parent:          "driveFsStartedWithOfficeEnabledLacros", // TODO(b/291524698): Create more DriveFS accounts.
		Data:            []string{"Sample_DOCX_file_20230704.docx", "Sample_PPTX_file_20230704.pptx", "Sample_XLSX_file_20230724.xlsx"},
	})
}

// TestFile generated by the fixture.
type TestFile struct {
	// File name auto-generated as part of the fixture with a random suffix.
	FileName string
	// This is the whole path for the original file from the repo, it doesn't have the random suffix.
	SrcFile string
	// File type: docx, pptx or xlsx.
	FileType string
}

// FixtureData is the struct exposed to tests.
type FixtureData struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn

	// Generated folder in MyFiles.
	TargetFolder string

	// A list of the generated files, the test is expected to iterate over those files.
	GeneratedFiles []TestFile

	// The DriveFS helper, reused by tests. This will be nil for tests without DriveFS.
	DriveFs *drivefs.DriveFs

	// The APIClient singleton used to make calls directly to Drive.
	// This will be nil for tests without DriveFS.
	DriveAPIClient *drivefs.APIClient

	// FakeDMS is the running DMS server if any.
	fakeDMS *fakedms.FakeDMS
}

// FakeDMS implements the HasFakeDMS interface.
func (f FixtureData) FakeDMS() *fakedms.FakeDMS {
	if f.fakeDMS == nil {
		panic("FakeDMS is called with nil fakeDMS instance")
	}
	return f.fakeDMS
}

type onedriveFixture struct {
	cr            *chrome.Chrome
	tconn         *chrome.TestConn
	chromeOptions []chrome.Option
	bt            browser.Type
	// Full path to the new folder created in Downloads to host the office files used by the test.
	downloadSubFolder string
	screenRecorder    *uiauto.ScreenRecorder
	provider          filesconsts.Provider
	// Holds a pointer to the FixtureData so the PreTest() can push new files to the test.
	data *FixtureData
	// Maps the keys docx, xlsx and pptx to the full local path in the  Data directory.
	// Used to share the path between SetUp() and PreTest().
	srcFiles map[string]string
	fdms     *fakedms.FakeDMS
}

// prepareOfficeFile copies the test file to a sub-folder of downloads with a unique name.
func prepareOfficeFile(srcPath, targetFolder string) (testFile TestFile, err error) {
	testFile.FileName = filemanager.GenerateTestFileName(filepath.Base(srcPath))
	testFile.SrcFile = srcPath
	testFile.FileType = strings.Replace(filepath.Ext(testFile.FileName), ".", "", 1)

	dst := filepath.Join(targetFolder, testFile.FileName)
	if err := fsutil.CopyFile(srcPath, dst); err != nil {
		return TestFile{}, errors.Wrapf(err, "failed to copy the file %s  to: %s", srcPath, dst)

	}
	if err := os.Chown(dst, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		return TestFile{}, errors.Wrapf(err, "failed to chown test file %s", dst)
	}

	return testFile, nil
}

func (f *onedriveFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var cr *chrome.Chrome
	var err error
	var driveFsClient *drivefs.DriveFs
	var driveAPIClient *drivefs.APIClient

	if f.provider == filesconsts.DriveFs {
		// Lacros is handled by the its parent fixture.
		cr = s.ParentValue().(*drivefs.FixtureData).Chrome
		f.tconn = s.ParentValue().(*drivefs.FixtureData).TestAPIConn
		driveFsClient = s.ParentValue().(*drivefs.FixtureData).DriveFs
		driveAPIClient = s.ParentValue().(*drivefs.FixtureData).APIClient
	} else if f.provider == filesconsts.OneDrive {
		opts := f.chromeOptions
		odfsDevPath := OdfsUnpackedLocation.Value()
		isOdfsDev := len(odfsDevPath) > 0

		// Checks whether the current fixture has a FakeDMS parent fixture.
		fdms, isPolicyTest := s.ParentValue().(*fakedms.FakeDMS)
		if isPolicyTest {
			s.Log("fdms found")
			if err := fdms.Ping(ctx); err != nil {
				s.Fatal("Failed to ping FakeDMS: ", err)
			}

			f.fdms = fdms

			pb := policy.NewBlob()
			pb.PolicyUser = fixtures.Username
			pb.AddPolicies([]policy.Policy{
				&policy.MicrosoftOneDriveMount{Val: "allowed"},
				&policy.MicrosoftOfficeCloudUpload{Val: "allowed"},
				&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
			})
			if err := fdms.WritePolicyBlob(pb); err != nil {
				s.Fatal("Failed to write policies to FakeDMS: ", err)
			}

			opts = append(opts, chrome.DMSPolicy(f.fdms.URL))
			opts = append(opts, chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}))
		}

		if f.bt == browser.TypeLacros {
			if isOdfsDev {
				opts = append(opts, chrome.LacrosUnpackedExtension(odfsDevPath))
			}
			var err error
			opts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
			if err != nil {
				s.Fatal("Failed to get lacros options: ", err)
			}
		} else {
			if isOdfsDev {
				opts = append(opts, chrome.UnpackedExtension(odfsDevPath))
			}
		}

		ctx, cancel := context.WithTimeout(ctx, chrome.LoginTimeout)
		defer cancel()

		cr, err = chrome.New(ctx, opts...)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed creating test API connection: ", err)
		}
		f.tconn = tconn
	}
	f.cr = cr

	// Prepare folder to receive files for tests.
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
	f.downloadSubFolder = targetFolder

	f.srcFiles = make(map[string]string)

	f.srcFiles["docx"] = s.DataPath("Sample_DOCX_file_20230704.docx")
	f.srcFiles["pptx"] = s.DataPath("Sample_PPTX_file_20230704.pptx")
	f.srcFiles["xlsx"] = s.DataPath("Sample_XLSX_file_20230724.xlsx")

	f.data = &FixtureData{
		Chrome:         cr,
		TestAPIConn:    f.tconn,
		TargetFolder:   targetFolder,
		DriveFs:        driveFsClient,
		DriveAPIClient: driveAPIClient,
		fakeDMS:        f.fdms,
	}
	return f.data
}

func (f *onedriveFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// For DriveFS the parent fixture takes care of closing it.
	if f.provider == filesconsts.OneDrive {
		if err := f.cr.Close(ctx); err != nil {
			s.Log("Failed closing chrome: ", err)
		}
		f.cr = nil
	}
}

func (f *onedriveFixture) Reset(ctx context.Context) error {
	if err := ash.CloseAllWindows(ctx, f.tconn); err != nil {
		testing.ContextLog(ctx, "Failed trying to close all windows: ", err)
	}
	return nil
}

func (f *onedriveFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
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

	// Copy the docx, pptx and xlsx to MyFiles to be used in the tests.
	var docx, pptx, xlsx TestFile
	var generatedFiles []TestFile

	if docx, err = prepareOfficeFile(f.srcFiles["docx"], f.downloadSubFolder); err != nil {
		s.Fatal("Failed to prepare file: ", err)
	}
	if pptx, err = prepareOfficeFile(f.srcFiles["pptx"], f.downloadSubFolder); err != nil {
		s.Fatal("Failed to prepare file: ", err)
	}
	if xlsx, err = prepareOfficeFile(f.srcFiles["xlsx"], f.downloadSubFolder); err != nil {
		s.Fatal("Failed to prepare file: ", err)
	}
	generatedFiles = []TestFile{
		docx,
		pptx,
		xlsx,
	}
	f.data.GeneratedFiles = generatedFiles

	// Open a new tab for Lacros to make sure Lacros process is alive during the entire test.
	// Note:
	//  * The Lacros process is required for both cleanup and the actual test,
	// both PWA installation/uninstalling and ODFS mounting/unmounting need this.
	//  * We also call this in Ash, it will open chrome://newtab in Lacros
	// but not in Ash, which is exactly what we need.
	if _, _, _, err = browserfixt.SetUpBrowserOrUseCurrent(ctx, f.cr, f.bt); err != nil {
		s.Fatal("Failed to get a browser to open new tab: ", err)
	}

	fi, err := filesinternals.Start(ctx, f.tconn, f.cr)
	if err != nil {
		s.Fatal("Failed to start chrome://files-internals: ", err)
	}

	if err := action.Combine("Clear prefs on chrome://files-internals",
		fi.ClearOfficeFileHandlers(),
		fi.ClearAlwaysMoveOneDrive(),
		fi.ClearAlwaysMoveGoogleDrive(),
	)(ctx); err != nil {
		s.Fatal("Failed to clear prefs on chrome://files-internals: ", err)
	}

	if err := fi.Close(ctx); err != nil {
		s.Fatal("Failed to close chrome://files-internals: ", err)
	}

	if err := MaybeUnmountOdfs(ctx, f.tconn, f.cr); err != nil {
		s.Fatal("Failed to unmount ODFS: ", err)
	}

	if err := ms365.MaybeUninstallPwa(ctx, f.cr, f.tconn); err != nil {
		s.Fatal("Failed to uninstall MS365 PWA: ", err)
	}

	if err := ms365.ClearBrowserCookiesForOffice(ctx, f.cr, f.bt); err != nil {
		s.Fatal("Failed to clear browser cookies for office website: ", err)
	}
}

// PostTests makes a best effort attempt to restore the state to where it was pretest.
func (f *onedriveFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Local Files.
	dirEntries, err := os.ReadDir(f.downloadSubFolder)
	if err != nil {
		s.Logf("Failed to list local directory: %q - %v", f.downloadSubFolder, err)
	} else {
		for _, e := range dirEntries {
			info, err := e.Info()
			if (err == nil) && info.Mode().IsRegular() {
				if err := os.Remove(filepath.Join(f.downloadSubFolder, info.Name())); err != nil {
					s.Logf("Failed to remove local file: %q - %v", info.Name(), err)
				}
			}
		}
	}

	// Old remote files from previous runs. NOTE: This is done at the end of
	// each test, because each test can be using a different Microsoft account.
	if len(f.data.GeneratedFiles) > 0 {
		DeleteOldRemoteFiles(ctx, f.data.GeneratedFiles[0].FileName)
	}

	// Remote files added during the fixture. NOTE: The deletion below fails if
	// the files are still open in the UI (Office 365 PWA), or are still in a
	// locked state in ODFS.
	for _, testFile := range f.data.GeneratedFiles {
		name := testFile.FileName
		files, err := filepath.Glob(filemanager.FuseboxDirPath + "/fsp.*/" + name)
		if err != nil {
			s.Logf("Failed cleaning up file: %s. %v", name, err)
		} else {
			for _, file := range files {
				s.Log("Deleting: ", file)
				if err := deleteFileRetrying(ctx, file); err != nil {
					s.Logf("Failed deleting the remote file: %s. %v", file, err)
				}
			}
		}
	}

	if f.cr != nil {
		f.cr.SaveLogsOnError(ctx, s.OutDir(), s.HasError)
	}

	if f.screenRecorder != nil {
		f.screenRecorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}

	// Close the active browser if there's any.
	browserfixt.MaybeConnectBrowserAndClose(ctx, f.cr, f.bt)
}

func deleteFileRetrying(ctx context.Context, file string) error {
	return action.RetryWithExponentialBackoff(5, func(ctx context.Context) error {
		return os.Remove(file)
	}, 500*time.Millisecond, 2)(ctx)
}
