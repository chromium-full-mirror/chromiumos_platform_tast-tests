// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/clipboard"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/files"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/session"
	pb "go.chromium.org/tast-tests/cros/services/cros/dlp"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterDataLeakPreventionServiceServer(srv, &DataLeakPreventionService{s: s})
		},
	})
}

type DataLeakPreventionService struct {
	s      *testing.ServiceState
	chrome *chrome.Chrome
}

// EnrollAndLogin enrolls the device and logs in with the provided account credentials.
func (service *DataLeakPreventionService) EnrollAndLogin(ctx context.Context, req *pb.EnrollAndLoginRequest) (_ *empty.Empty, retErr error) {

	opts := []chrome.Option{
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.GAIALogin(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.EnableFeatures(req.EnabledFeatures),
		chrome.DMSPolicy(req.DmserverUrl),
		chrome.EncryptedReportingAddr(fmt.Sprintf("%v/record", req.ReportingServerUrl)),
		chrome.CustomLoginTimeout(chrome.EnrollmentAndLoginTimeout),
	}

	bt := browser.TypeAsh
	var lcfg *lacrosfixt.Config
	if req.EnableLacros {
		bt = browser.TypeLacros
		lcfg = lacrosfixt.NewConfig(lacrosfixt.Selection(lacros.NotSelected))
	}

	cr, err := browserfixt.NewChrome(ctx, bt, lcfg, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	service.chrome = cr
	return &empty.Empty{}, nil
}

// StopChrome disconnects from Chrome.
func (service *DataLeakPreventionService) StopChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	var firstErr error

	if service.chrome == nil {
		return nil, errors.New("no active Chrome instance")
	}

	if err := service.chrome.Close(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close Chrome: ", err)
		firstErr = errors.Wrap(err, "failed to close Chrome")
	}
	service.chrome = nil

	return &empty.Empty{}, firstErr
}

func (service *DataLeakPreventionService) ClientID(ctx context.Context, req *empty.Empty) (*pb.ClientIdResponse, error) {
	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create session_manager binding")
	}

	p, err := session.RetrievePolicyData(ctx, sm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve settings")
	}

	return &pb.ClientIdResponse{ClientId: *p.DeviceId}, nil
}

// CreateTempDir creates a temporary directory in tmp and returns its path.
func (service *DataLeakPreventionService) CreateTempDir(ctx context.Context, req *empty.Empty) (*pb.CreateTempDirResponse, error) {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, err
	}
	return &pb.CreateTempDirResponse{Path: dir}, nil
}

// RemoveTempDir removes the temporary directory with its contents.
func (service *DataLeakPreventionService) RemoveTempDir(ctx context.Context, req *pb.RemoveTempDirRequest) (*empty.Empty, error) {
	err := os.RemoveAll(req.Path)
	if err != nil {
		return &empty.Empty{}, err
	}
	return &empty.Empty{}, nil
}

// createHTMLTextPage creates an HTML page with some text.
func createHTMLTextPage(path string) error {
	textContent := []byte("<!DOCTYPE html><html lang='en'><head><meta charset='utf-8'><title>Random Text</title></head><body>Sample text about random things.</body></html>")
	if err := os.WriteFile(path, textContent, 0644); err != nil {
		return errors.Wrap(err, "failed to write a file")
	}
	return nil
}

// createHTMLTextAreaPage creates an HTML page with a text area element.
func createHTMLTextAreaPage(path string) (*nodewith.Finder, error) {
	inputContent := []byte("<!DOCTYPE html><html lang='en'><head><meta charset='utf-8'><title>Editable Text Box</title></head><body><textarea aria-label='textarea' rows='1' cols='100'></textarea></body></html>")
	if err := os.WriteFile(path, inputContent, 0644); err != nil {
		return nil, errors.Wrap(err, "failed to write a file")
	}
	textAreaNode := nodewith.Name("textarea").Role(role.TextField).State(state.Editable, true).First()
	return textAreaNode, nil
}

// setupBrowser returns a Browser instance for the given browser type and a closure to close the browser instance.
func setupBrowser(ctx context.Context, chrome *chrome.Chrome, browserType pb.BrowserType) (*browser.Browser, func(ctx context.Context) error, error) {
	bt := browser.TypeAsh
	if browserType == pb.BrowserType_LACROS {
		bt = browser.TypeLacros
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, chrome, bt)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to open the browser")
	}

	return br, closeBrowser, nil
}

// copyToDriveAndVerifyWarning tries to copy the file to Google Drive.
// If waitForWarning is true waits for DLP warning to appear, otherwise ensures it doesn't appear.
func copyToDriveAndVerifyWarning(ctx context.Context, ui *uiauto.Context, f *filesapp.FilesApp, kb *input.KeyboardEventWriter, filename string, waitForWarning bool) error {
	if err := uiauto.Combine("copy the file to Google Drive",
		f.OpenDownloads(),
		f.CopyFileToClipboard(filename),
		f.OpenDrive(),
		f.PasteFileFromClipboard(kb),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to copy the file to Google Drive")
	}

	if err := files.VerifyWarning(ctx, ui, waitForWarning); err != nil {
		return errors.Wrap(err, "failed to verify warning")
	}

	return nil
}

// ClipboardCopyPaste performs a copy and paste action.
func (service *DataLeakPreventionService) ClipboardCopyPaste(ctx context.Context, req *pb.ActionRequest) (_ *empty.Empty, retErr error) {

	const baseDir = "/tmp"

	textFilename := "text.html"
	if err := createHTMLTextPage(filepath.Join(baseDir, textFilename)); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while creating the HTML text page")
	}

	inputFilename := "input.html"
	textAreaNode, err := createHTMLTextAreaPage(filepath.Join(baseDir, inputFilename))
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while creating the HTML input page")
	}

	br, closeBrowser, err := setupBrowser(ctx, service.chrome, req.BrowserType)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while setting up the browser")
	}
	defer closeBrowser(ctx)

	sourceServer := httptest.NewServer(http.FileServer(http.Dir(baseDir)))
	defer sourceServer.Close()

	sourceURL := sourceServer.URL + "/" + textFilename
	sourceConn, err := br.NewConn(ctx, sourceURL)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to open the source page")
	}
	defer sourceConn.Close()

	if err := webutil.WaitForQuiescence(ctx, sourceConn, 10*time.Second); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to wait to achieve quiescence")
	}

	// Connect to Test API.
	tconn, err := service.chrome.TestAPIConn(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to connect to test API")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	if err := tconn.WaitForExpr(ctx, "chrome.clipboard"); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to wait for chrome.clipboard API to become available")
	}

	if err := uiauto.Combine("copy all text from source website",
		keyboard.AccelAction("Ctrl+A"),
		keyboard.AccelAction("Ctrl+C"))(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to copy text from source browser")
	}

	copiedString, err := clipboard.GetClipboardContent(ctx, tconn)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to get clipboard content")
	}

	destServer := httptest.NewServer(http.FileServer(http.Dir(baseDir)))
	defer destServer.Close()

	destURL := destServer.URL + "/" + inputFilename
	destConn, err := br.NewConn(ctx, destURL)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to open the destination page")
	}
	defer destConn.Close()

	if err := webutil.WaitForQuiescence(ctx, destConn, 10*time.Second); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to wait to achieve quiescence")
	}

	ui := uiauto.New(tconn)

	if err := uiauto.Combine("Pasting into text box",
		ui.WaitUntilExists(textAreaNode.Visible()),
		ui.LeftClick(textAreaNode),
		ui.WaitUntilExists(textAreaNode.Focused()),
		keyboard.AccelAction("Ctrl+V"),
	)(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to paste into text box")
	}

	// We are only testing report mode, since we expect the text to be copied.
	if err := clipboard.CheckPastedContent(ctx, ui, copiedString); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to verify pasted content")
	}

	return &empty.Empty{}, nil
}

// Print performs a print action.
func (service *DataLeakPreventionService) Print(ctx context.Context, req *pb.ActionRequest) (_ *empty.Empty, retErr error) {

	const baseDir = "/tmp"

	textFilename := "text.html"
	if err := createHTMLTextPage(filepath.Join(baseDir, textFilename)); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while creating the HTML text page")
	}

	br, closeBrowser, err := setupBrowser(ctx, service.chrome, req.BrowserType)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while setting up the browser")
	}
	defer closeBrowser(ctx)

	server := httptest.NewServer(http.FileServer(http.Dir(baseDir)))
	defer server.Close()

	conn, err := br.NewConn(ctx, server.URL+"/"+textFilename)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to open page")
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to wait to achieve quiescence")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	// Test printing using hotkey (Ctrl + P).
	if err := keyboard.Accel(ctx, "Ctrl+P"); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to type printing hotkey")
	}

	return &empty.Empty{}, nil

}

// Screenshot takes a screenshot.
func (service *DataLeakPreventionService) Screenshot(ctx context.Context, req *pb.ActionRequest) (_ *empty.Empty, retErr error) {

	const baseDir = "/tmp"

	textFilename := "text.html"
	if err := createHTMLTextPage(filepath.Join(baseDir, textFilename)); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while creating the HTML text page")
	}

	br, closeBrowser, err := setupBrowser(ctx, service.chrome, req.BrowserType)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while setting up the browser")
	}
	defer closeBrowser(ctx)

	server := httptest.NewServer(http.FileServer(http.Dir(baseDir)))
	defer server.Close()

	conn, err := br.NewConn(ctx, server.URL+"/"+textFilename)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to open page")
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to wait to achieve quiescence")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	layout, err := input.KeyboardTopRowLayout(ctx, keyboard)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to retrieve keyboard top row layout")
	}

	// Take a screenshot using hotkey (Ctrl+F5)
	if err := keyboard.Accel(ctx, "Ctrl+"+layout.SelectTask); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to type screenshot hotkey")
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, service.chrome.NormalizedUser())
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to retrieve user's Download path")
	}
	// Clean up previous screenshots.
	if err := screenshot.RemoveScreenshots(downloadsPath); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to remove screenshots")
	}

	return &empty.Empty{}, nil

}

// Screenshare shares the screen.
func (service *DataLeakPreventionService) Screenshare(ctx context.Context, req *pb.ActionRequest) (_ *empty.Empty, retErr error) {

	const baseDir = "/tmp"

	textFilename := "text.html"
	if err := createHTMLTextPage(filepath.Join(baseDir, textFilename)); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while creating the HTML text page")
	}

	br, closeBrowser, err := setupBrowser(ctx, service.chrome, req.BrowserType)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while setting up the browser")
	}
	defer closeBrowser(ctx)

	server := httptest.NewServer(http.FileServer(http.Dir(baseDir)))
	defer server.Close()

	conn, err := br.NewConn(ctx, server.URL+"/"+textFilename)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to open page")
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to wait to achieve quiescence")
	}

	// Connect to Test API.
	tconn, err := service.chrome.TestAPIConn(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to connect to test API")
	}

	screenRecorder, err := uiauto.NewScreenRecorder(ctx, tconn)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to create screen recorder")
	}
	if screenRecorder == nil {
		return &empty.Empty{}, errors.Wrap(err, "screen recorder not found")
	}
	if err := screenRecorder.Start(ctx, tconn); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to start screen recorder")
	}

	return &empty.Empty{}, nil

}

// FilesDriveCopyPaste downloads a restricted file, and then copy-pastes it to Google Drive.
func (service *DataLeakPreventionService) FilesDriveCopyPaste(ctx context.Context, req *pb.ActionRequest) (_ *empty.Empty, retErr error) {
	_, err := drivefs.NewDriveFs(ctx, service.chrome.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to start DriveFS")
	}

	br, closeBrowser, err := setupBrowser(ctx, service.chrome, req.BrowserType)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "error while setting up the browser")
	}
	defer closeBrowser(ctx)

	tconn, err := service.chrome.TestAPIConn(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to connect to test API")
	}

	// Download the file.
	if err := files.DownloadFile(ctx, tconn, br, http.Dir(req.DataPath)); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to download file")
	}

	// Open the Files app.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to launch the Files App")
	}
	defer filesApp.Close(ctx)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	if err := uiauto.Combine("copy pasting managed file",
		filesApp.OpenDownloads(),
		filesApp.WaitForFile(files.DlFileName),
		filesApp.CopyFileToClipboard(files.DlFileName),
		filesApp.OpenDrive(),
		filesApp.PasteFileFromClipboard(keyboard),
	)(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to copy and paste managed file")
	}

	return &empty.Empty{}, nil
}

// TestCopyFileToDrive tests copying a DLP restricted file to Google Drive.
func (service *DataLeakPreventionService) TestCopyFileToDrive(ctx context.Context, req *pb.TestCopyFileToDriveRequest) (_ *empty.Empty, retErr error) {
	_, err := drivefs.NewDriveFs(ctx, service.chrome.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to start DriveFS")
	}

	tconn, err := service.chrome.TestAPIConn(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to connect to test API")
	}

	// Download the file.
	if err := files.DownloadFile(ctx, tconn, service.chrome.Browser(), http.Dir(req.DataPath)); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to download file")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	ui := uiauto.New(tconn)

	// Open the Files app.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to launch the Files App")
	}
	defer filesApp.Close(ctx)

	if err := filesApp.OpenDownloads()(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to open Downloads")
	}

	// Rename the file to a unique name and check that it's still managed.
	// This also ensures that different tests don't interact with each other, after copying the file to Drive.
	uuid, err := uuid.NewRandom()
	if err != nil {
		return &empty.Empty{}, err
	}
	dlFileName := fmt.Sprintf("data-%s.txt", uuid.String())
	if err := filesApp.RenameFile(keyboard, files.DlFileName, dlFileName)(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to rename the file")
	}

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, dlFileName, true); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "file isn't managed after renaming")
	}

	// Copy to Drive and cancel the warning.
	if err := copyToDriveAndVerifyWarning(ctx, ui, filesApp, keyboard, dlFileName, true); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to copy the file")
	}

	if err := files.CancelWarningAndVerify(ctx, ui, tconn, keyboard, dlFileName); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to cancel the paste")
	}

	// Copy again and accept the warning. Copies shouldn't be managed.
	if err := copyToDriveAndVerifyWarning(ctx, ui, filesApp, keyboard, dlFileName, true); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to copy the file")
	}

	if err := files.AcceptWarningAndVerify(ctx, ui, tconn, keyboard, dlFileName); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to proceed the paste")
	}

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, dlFileName, false); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "file is managed after copying")
	}

	// GoBigSleepLint - Add some delay to allow for Drive sync to finish.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to sleep")
	}

	// Delete the file and copy again.
	if err := filesApp.DeleteFileOrFolder(keyboard, dlFileName)(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to delete the file")
	}

	// Warning should be bypassed silently.
	if err := copyToDriveAndVerifyWarning(ctx, ui, filesApp, keyboard, dlFileName, false); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to copy the file")
	}

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, dlFileName, false); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "file is managed after copying")
	}

	// Delete the file so we don't unnecessarily take space in Drive.
	if err := filesApp.DeleteFileOrFolder(keyboard, dlFileName)(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to delete the file")
	}

	// GoBigSleepLint - Add some delay to allow for Drive sync to finish.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to sleep")
	}

	return &empty.Empty{}, nil
}
