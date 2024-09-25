// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/printing/document"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Rastertoescpos,
		Desc:     "Tests that the rastertoescpos CUPS filter produces expected output",
		Contacts: []string{"project-bolton@google.com", "nmuggli@google.com"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"informational",
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
		},
		SoftwareDeps: []string{"cups"},
		Data:         []string{"rastertoescpos_input.pwg", "rastertoescpos_golden.bin", "rastertoescpos.ppd"},
	})
}

func Rastertoescpos(ctx context.Context, s *testing.State) {
	const (
		escposFilter = "rastertoescpos"
		input        = "rastertoescpos_input.pwg"
		golden       = "rastertoescpos_golden.bin"
		ppd          = "rastertoescpos.ppd"
	)

	inputContents, err := ioutil.ReadFile(s.DataPath(input))
	if err != nil {
		s.Fatal("Failed to read input file: ", err)
	}

	escposFilterPath := "/usr/libexec/cups/filter/" + escposFilter

	// args: jobID, user, title, copies, options
	escposCmd := testexec.CommandContext(ctx, escposFilterPath, "1", "chronos", "Untitled", "1", "")

	// Add the PPD environment variable
	ppdEnvVar := "PPD=" + s.DataPath(ppd)
	if ppdEnvVar != "" {
		escposCmd.Env = os.Environ()
		escposCmd.Env = append(escposCmd.Env, ppdEnvVar)
	}

	// Capture a pipe to the stdin of the rastertoescpos filter.
	escposStdin, err := escposCmd.StdinPipe()
	if err != nil {
		s.Fatal("Failed to open stdin pipe: ", err)
	}
	// Pass the contents of the given input file into the rastertoescpos filter using
	// the stdin pipe.
	go func() {
		defer escposStdin.Close()
		if _, err := escposStdin.Write(inputContents); err != nil {
			s.Error("Failed to write to stdin pipe: ", err)
		}
	}()
	rastertoescposOutput, _ := escposCmd.Output(testexec.DumpLogOnError)

	goldenBytes, err := ioutil.ReadFile(s.DataPath(golden))
	if err != nil {
		s.Fatalf("Failed to read file %s: %v", golden, err)
	}

	if document.CleanContents(string(goldenBytes)) != document.CleanContents(string(rastertoescposOutput)) {
		escposCmd.DumpLog(ctx)
		outFile := filepath.Base(s.DataPath(golden))
		outPath := filepath.Join(s.OutDir(), outFile)
		if err := ioutil.WriteFile(outPath, rastertoescposOutput, 0644); err != nil {
			s.Error("Failed to dump output: ", err)
		}
		s.Errorf("Output differs from expected: output saved to %q", outFile)
	}
}
