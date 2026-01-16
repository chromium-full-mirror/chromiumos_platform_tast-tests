// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ippprint implements printing with IPP options.
package ippprint

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/lpprint"
	"go.chromium.org/tast-tests/cros/local/printing/document"
	"go.chromium.org/tast/core/testing"
)

// Params struct used by all ipp print tests for parameterized tests.
type Params struct {
	PPDFile      string   // Name of the ppd used to print the job.
	PrintFile    string   // The file to print.
	ExpectedFile string   // The file output should be compared to.
	Options      []string // Options to be passed to the filter to change output.
}

// Collate enables collation.
func Collate() string {
	return "multiple-document-handling=separate-documents-collated-copies"
}

// WithCopies properly formats a copies option.
func WithCopies(n int) string {
	return fmt.Sprintf("copies=%d", n)
}

// WithJobPassword properly formats a job-password option.
func WithJobPassword(pass string) string {
	return fmt.Sprintf("job-password=%s", pass)
}

// WithResolution properly formats a printer-resolution option.
func WithResolution(res string) string {
	return fmt.Sprintf("printer-resolution=%s", res)
}

// Run executes the main test logic with p.Options included in the lp command.
func Run(ctx context.Context, s *testing.State, p *Params) {
	run(ctx, s, p, func(ctx context.Context) ([]byte, error) {
		return lpprint.Run(ctx, s.DataPath(p.PPDFile), s.DataPath(p.PrintFile), strings.Join(p.Options, " "))
	})
}

// run runs the given print function and compares the output to the golden file.
func run(ctx context.Context, s *testing.State, p *Params, printFun func(context.Context) ([]byte, error)) {
	expect, err := os.ReadFile(s.DataPath(p.ExpectedFile))
	if err != nil {
		s.Fatal("Failed to read golden file: ", err)
	}
	request, err := printFun(ctx)
	if err != nil {
		s.Fatal("Print job failed: ", err)
	}
	if err = document.CompareFileContents(ctx, string(request), string(expect),
		s.OutDir(), "diff.txt", p.ExpectedFile); err != nil {
		s.Error("Printer output differs from expected: ", err)
	}
}
