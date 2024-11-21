// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/ui/mahicuj"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	mockResponseScript = "mahi_response_mock.py"
	pdfExampleZip      = "mahi_example_pdf.zip"
	pdfExampleName     = "example.pdf"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MahiGalleryCUJ,
		Desc: "CUJ of Help Me Read feature on Gallery surface",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"alanlxl@google.com",
			"thanhdng@google.com",
		},
		BugComponent: "b:1116342",
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
		Data: []string{
			pdfExampleZip,
			mockResponseScript,
		},
		Fixture: "loggedInToCUJUserWithMahiEnabled",
		Attr:    []string{"group:cuj"},
	})
}

func MahiGalleryCUJ(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	re := regexp.MustCompile("^/home/user/.*/Downloads$")
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Error("Failed to get user's Download path: ", err)
	}
	if !re.MatchString(downloadsPath) {
		s.Errorf("Download format invalid, should match %q, get %q", re.String(), downloadsPath)
	}

	// Clean the downloads path
	defer func() error {
		files, err := os.ReadDir(downloadsPath)
		if err != nil {
			return errors.Wrap(err, "failed to read file list in download path")
		}

		for _, f := range files {
			path := filepath.Join(downloadsPath, f.Name())
			if err := os.RemoveAll(path); err != nil {
				return errors.Wrapf(err, "failed to remove file (%q)", path)
			}
		}
		return nil
	}()

	pdfZipInDownloadsPath := filepath.Join(downloadsPath, pdfExampleZip)
	if err := fsutil.CopyFile(s.DataPath(pdfExampleZip), pdfZipInDownloadsPath); err != nil {
		s.Error("Failed to copy pdf zip to Download path: ", err)
	}

	if err := testexec.CommandContext(ctx, "unzip", pdfZipInDownloadsPath, "-d", downloadsPath).Run(testexec.DumpLogOnError); err != nil {
		s.Error("Failed to unzip the pdf to Download path: ", err)
	}

	if _, err := mahicuj.GalleryCUJRun(ctx, cr, s.DataPath(mockResponseScript), pdfExampleName, s.OutDir()); err != nil {
		s.Fatal("Failed to run MahiBrowserCUJ: ", err)
	}

}
