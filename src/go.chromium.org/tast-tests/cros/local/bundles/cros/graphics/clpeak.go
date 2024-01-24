// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"encoding/xml"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	clPeakBinPath = "/usr/local/opencl/clpeak"
	xmlArg        = "--xml-file"
	xmlFileName   = "clpeak.xml"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Clpeak,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A series of microbenchmarks for the gpu using clvk",
		SoftwareDeps: []string{"vulkan"},
		Attr:         []string{"group:graphics", "graphics_opencl", "graphics_perbuild"},
		BugComponent: "b:1171198", // ChromeOS > Platform > Graphics > GPU > OpenCL
		Contacts: []string{
			"chromeos-gfx@google.com",
			"syedfaaiz@google.com",
			"rjodin@chromium.org",
		},
		Fixture: "graphicsNoChrome",
		Timeout: 10 * time.Minute,
	})
}

func processElement(decoder *xml.Decoder, pv *perf.Values) error {
	// Continue reading tokens until we reach the end of the input stream.
	currentCategory := ""
	currentUnit := ""
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}

		if err != nil {
			return errors.Wrap(err, "failed to decode token")
		}

		switch startElement := token.(type) {
		case xml.StartElement:
			currName := startElement.Name.Local
			// if this is not a new category append the name and move on
			if currentCategory != "" && currentUnit != "" {
				currentCategory += "-" + currName
			} else {
				// this is a new category with potentially new units
				currentCategory = currName

				// extract Unit
				for _, attr := range startElement.Attr {
					if attr.Name.Local == "unit" {
						currentUnit = attr.Value
						break
					}
				}
			}
		case xml.CharData:
			// this may be a metric or just a string of whitespaces
			valueStr := strings.TrimSpace(string(startElement))
			if len(valueStr) == 0 || strings.Contains(currentCategory, "8") || strings.Contains(currentCategory, "16") {
				continue
			}
			direction := perf.BiggerIsBetter
			// smaller is better for kernel-latency
			if strings.Contains(currentCategory, "latency") {
				direction = perf.SmallerIsBetter
			}
			value, err := strconv.ParseFloat(valueStr, 64)
			if err != nil {
				return errors.Wrap(err, "failed to convert string to float")
			}
			pv.Set(perf.Metric{
				Name:      currentCategory,
				Unit:      currentUnit,
				Direction: direction,
			}, float64(value))

		case xml.EndElement:
			parts := strings.Split(currentCategory, "-")
			if len(parts) > 1 {
				currentCategory = strings.Join(parts[:len(parts)-1], "-")
			} else {
				// Reset unit and category when exiting a category
				currentUnit = ""
				currentCategory = ""
			}
		}
	}
	return nil
}

func Clpeak(ctx context.Context, s *testing.State) {
	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()

	// Allow to see clvk error and warn messages directly in test logFile.
	os.Setenv("CLVK_LOG", "2")
	// Force MAX_MEM_ALLOC_SIZE to the minimum required by OpenCL. It will avoid timeout on some devices.
	os.Setenv("CLVK_MAX_MEM_ALLOC_SIZE_MB", "1024")
	xmlFileAbsolutePath := s.OutDir() + xmlFileName

	// Run the whole suite once.
	stdout, stderr, err := testexec.CommandContext(ctx, clPeakBinPath, xmlArg, xmlFileAbsolutePath).SeparatedOutput(testexec.DumpLogOnError)
	s.Log(string(stdout), string(stderr))
	if err != nil {
		s.Errorf("Failed to run %v: %v", clPeakBinPath, err)
	}

	// Read XML file and extract the data
	xmlFile, err := os.Open(xmlFileAbsolutePath)
	if err != nil {
		s.Fatal("Failed to open file: ", err)
	}
	defer xmlFile.Close()
	decoder := xml.NewDecoder(xmlFile)
	if err = processElement(decoder, pv); err != nil {
		s.Fatal("Error while gathering metrics: ", err)
	}
}
