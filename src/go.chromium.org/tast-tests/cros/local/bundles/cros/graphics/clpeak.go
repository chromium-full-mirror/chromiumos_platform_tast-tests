// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type clpeakTestCase struct {
	args  []string // command args
	dType string   // result dType string
}

const (
	clPeakBinPath = "/usr/local/opencl/clpeak"
	regexVal      = `[0-9]?[0-9]?\s*:\s*(\d+\.\d+)`
)

var (
	/* This order shows how different data types are tested in clpeak
	Sample output :
	float   : 767.48
	float2  : 810.81
	float4  : 843.06
	float8  : 726.12
	float16 : 735.98
	*/
	typeAppend = []string{"", "2", "4", "8", "16"}
	// Argument -> data type map.
	arg2Type = []clpeakTestCase{
		{args: strings.Split("--compute-hp", " "), dType: "half"},
		{args: strings.Split("--compute-sp", " "), dType: "float"},
		{args: strings.Split("--compute-integer", " "), dType: "int"},
	}
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

func savePerfClpeak(number float64, name string, pv *perf.Values) {
	direction := perf.BiggerIsBetter
	var unit = "GFLOPS"
	if strings.Contains(name, "int") {
		unit = "GIOPS"
	}
	pv.Set(perf.Metric{
		Name:      name,
		Unit:      unit,
		Direction: direction,
	}, float64(number))
}

func extractData(data, dType string) ([]float64, error) {
	re := regexp.MustCompile(dType + regexVal)
	// Iterate over the lines in the string.
	var values []float64
	for _, line := range strings.Split(data, "\n") {
		// Find all matches for the regular expression in the line.
		matches := re.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			val, err := strconv.ParseFloat(match[1], 64)
			if err != nil {
				return nil, errors.Errorf("failed to convert %v to float64", match[1])
			}
			values = append(values, val)
		}
	}
	if len(values) != len(typeAppend) {
		return nil, errors.Errorf("size of values for %s does not match the number of data types : %s", dType, data)
	}
	return values, nil
}

func Clpeak(ctx context.Context, s *testing.State) {

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()

	// Run the whole suite once.
	stdout, stderr, err := testexec.CommandContext(ctx, clPeakBinPath).SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("Failed to run %v: %v", string(stderr), err)
	}
	s.Log(string(stdout))

	// Run individual benchmarks
	for _, test := range arg2Type {
		stdout, stderr, err := testexec.CommandContext(ctx, clPeakBinPath, test.args...).SeparatedOutput(testexec.DumpLogOnError)
		data := string(stdout)
		if err != nil {
			s.Errorf("Failed to run %v: %v", string(stderr), err)
			continue
		}
		if strings.Contains(data, "Skipped") {
			s.Log("Warning: Device does not support", test.dType)
			continue
		}
		values, err := extractData(data, test.dType)
		if err != nil {
			s.Error("Error occured : ", err)
		}
		for index, val := range values {
			savePerfClpeak(val, test.dType+typeAppend[index], pv)
		}
	}
}
