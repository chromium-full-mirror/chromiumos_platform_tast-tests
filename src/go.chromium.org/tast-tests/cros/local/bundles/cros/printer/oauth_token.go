// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/printing/lp"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     OAuthToken,
		Desc:     "Tests that ensure the oauth token is passed to printer",
		Contacts: []string{"project-bolton@google.com", "nmuggli@google.com"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
		},
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{"chrome", "cups"},
		Data:         []string{"to_print.pdf"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
	})
}

func OAuthToken(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Create the temp dir to store the HTTP headers.
	httpHeaderFiles := "printer.OAuthToken.httpHeaders"
	tmpDir, err := os.MkdirTemp("", httpHeaderFiles)
	if err != nil {
		s.Fatal("Failed to create temporary directory")
	}
	defer os.RemoveAll(tmpDir)

	printer, err := usbprinter.Start(ctx,
		usbprinter.WithIPPUSBDescriptors(),
		usbprinter.WithHTTPLogDirectory(tmpDir),
		usbprinter.WithGenericIPPAttributes(),
		usbprinter.WaitUntilConfigured())
	if err != nil {
		s.Fatal("Failed to attach virtual printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop virtual printer: ", err)
		}
	}(cleanupCtx)

	// Clean out the files in the tmp dir.  These will have requests that are
	// involved with printer setup, so we don't expect those to have the oauth
	// token in the http headers - we just want to check all requests after we
	// start our print job.
	if files, err := fs.Glob(os.DirFS(tmpDir), "*"); err != nil {
		s.Fatalf("Unable to read dir %s: %s", tmpDir, err)
	} else {
		for _, file := range files {
			deleteMe := filepath.Join(tmpDir, file)
			if err := os.Remove(deleteMe); err != nil {
				s.Fatalf("Unable to remove %s: %s", deleteMe, err)
			}
		}
	}

	printerName := printer.ConfiguredName
	oauthTokenString := "qwertyasdf1234="

	job, err := lp.CupsStartPrintJob(ctx, printerName,
		s.DataPath("to_print.pdf"), "-o",
		"chromeos-access-oauth-token="+oauthTokenString)
	if err != nil {
		s.Fatal("Failed to start printer: ", err)
	}

	s.Logf("Waiting for %s to complete", job)
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		if done, err := lp.JobCompleted(ctx, printerName, job); err != nil {
			return err
		} else if !done {
			return errors.Errorf("Job %s is not done yet", job)
		}
		testing.ContextLogf(ctx, "Job %s is complete", job)
		return nil
	}, nil); err != nil {
		s.Fatal("Print job didn't complete: ", err)
	}

	// Look at all of our http header files and make sure they have the correct
	// oauth access token.
	if files, err := fs.Glob(os.DirFS(tmpDir), "http-header-*"); err != nil {
		s.Fatalf("Unable to read dir %s: %s", tmpDir, err)
	} else {
		for _, file := range files {
			httpData, err := os.ReadFile(filepath.Join(tmpDir, file))
			if err != nil {
				s.Fatalf("Unable to read HTTP header file %s: %s", file, err)
			}

			httpOauth := "Authorization: Bearer " + oauthTokenString + "\n"
			if !(bytes.Contains(httpData, []byte(httpOauth))) {
				// Get-Printer-Attributes is an exception because it
				// can be made without an OAuth header, so check for it
				ippGetAttr := "Op ID: Get-Printer-Attributes"

				ippData, err := getIppFileData(tmpDir, file)
				if err != nil {
					s.Fatalf("Unable to read IPP header file %s: %s", file, err)
				}

				if !(bytes.Contains(ippData, []byte(ippGetAttr))) {
					s.Fatal("HTTP header does not contain correct oauth token")
				}
			}
		}
	}
}

// getIppFileData finds the corresponding IPP header file for a given HTTP header file
// and returns its contents
func getIppFileData(directory, httpFile string) ([]byte, error) {
	re := regexp.MustCompile(`http-header-(\d+)\.txt`)
	matches := re.FindStringSubmatch(httpFile)
	if len(matches) != 2 {
		return []byte{}, errors.Errorf("file %s does not match expected pattern", httpFile)
	}

	ippFile := fmt.Sprintf("ipp-header-%s.txt", matches[1])

	// Read the corresponding IPP file
	ippFilePath := filepath.Join(directory, ippFile)
	ippData, err := os.ReadFile(ippFilePath)
	if err != nil {
		return nil, errors.Errorf("unable to read IPP header file %s: %s", ippFile, err)
	}

	return ippData, nil
}
