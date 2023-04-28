// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	//	"github.com/diskfs/go-diskfs"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/local/cpu"
	"chromiumos/tast/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type statisticsType int64

const (
	briefStatistics statisticsType = iota
	fullStatistics
)

// iterationInterval specifies an interval over which
// the number of iterations completed will be extracted.
//
// Specified in minutes from test start.
type iterationInterval struct {
	firstMinute uint // The first minute in the interval (0 = very start)
	lastMinute  uint // The last minute in the interval
}

type testOptions struct {
	testDuration      time.Duration       // The test duration for this variant
	secondsPerReport  time.Duration       // Number of seconds per report
	statsType         statisticsType      // The type of stats to report at the end of the test
	recordedIntervals []iterationInterval // The iteration intervals to record after test completion
	imageFileName     string              // The image file name to be used for this variant
}

type iterationData struct {
	totalIterations []uint64            // The total number of iterations completed per reporting period
	coresIteration  map[string][]uint64 // Iterations completed by each core at each reporting period, indexed by core id
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VMCPUQuantifiedWork,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs QuantifiedWork UEFI stress test app on a DUT and logs the result",
		// ChromeOS > Platform > Graphics > Gaming > Steam > Core Gfx
		BugComponent: "b:961455",
		// CROSVM.fd contains the OVMF fork for use with crosvm
		// The latest build of this can be obtained from Kokoro on
		// prod:glinux/ovmf/release
		Data:         []string{"CROSVM.fd"},
		Attr:         []string{"group:graphics"},
		SoftwareDeps: []string{"vm_host"},
		HardwareDeps: hwdep.D(hwdep.X86()),
		// This test does not do GPU work, but add this fixture anyway.
		Fixture: "gpuWatchHangs",

		// Each image can be obtained by building the VmPerfEval package here:
		// src/platform/graphics/src/vm_perf_eval
		//
		// And the image files were built by running the following commands:
		//
		// For QuantifiedWorkEfi2min.img:
		// ./VmPerfPackager.py --image_name QuantifiedWork2min --efi_binary <PathToVmPerfEvalQuantifiedWork.efi>\
		// --image_size_in_mbs 16 --option_filename QuantifiedWork --option "TestDuration=120"
		//
		// For QuantifiedWorkEfi10min.img
		// ./VmPerfPackager.py --image_name QuantifiedWork10min --efi_binary <PathToVmPerfEvalQuantifiedWork.efi>\
		// --image_size_in_mbs 16 --option_filename QuantifiedWork --option "TestDuration=600"
		Params: []testing.Param{
			{
				Name: "02_minutes",
				Val: testOptions{
					120,
					60,
					briefStatistics,
					[]iterationInterval{
						{0, 1},
						{1, 2},
					},
					"QuantifiedWorkEfi2min.img"},
				// Setting up/shutting down the VM can take a while.
				// In addition, waiting for the device to cool down to an appropriate temperature
				// for testing can add to this time (especially in a lab setting). For these reasons,
				// we have opted to use generous timeout values for all variations of this test.
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"graphics_nightly"},
				ExtraData: []string{"QuantifiedWorkEfi2min.img"},
			},
			{
				Name: "10_minutes",
				Val: testOptions{
					600,
					60,
					briefStatistics,
					[]iterationInterval{
						{0, 1},
						{1, 2},
						{5, 6},
						{9, 10},
					},
					"QuantifiedWorkEfi10min.img"},
				Timeout:   20 * time.Minute,
				ExtraAttr: []string{"graphics_weekly"},
				ExtraData: []string{"QuantifiedWorkEfi10min.img"},
			},
		},
		Contacts: []string{
			"chromeos-gaming-core@google.com",
			"mrisaacb@google.com",
		}})
}

// extractIterations takes crosvm's stdout
// and determines how many iterations were completed
// For each "key", an array is included which includes all
// reports successfully extracted from the log file.
func extractIterations(outdirPath string) (*iterationData, error) {
	var cpuIterations iterationData

	cpuIterations.coresIteration = make(map[string][]uint64)

	re := regexp.MustCompile("[0-9]+")

	// Open the file up
	fileReader, err := os.OpenFile(filepath.Join(outdirPath, "crosvm.log"), os.O_RDWR, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open crosvm output log! Did the test run?")
	}

	scanner := bufio.NewScanner(fileReader)

	totalIterationsRe := regexp.MustCompile(`Total Completed Iterations:\s*(\d+)`)

	// This captures all iterations of the individual core results and
	// the total results and leaves it to the remaining parts of the test
	// to extract the data points of interest
	for scanner.Scan() {
		if totalIterationsRe.MatchString(scanner.Text()) {
			matches := totalIterationsRe.FindStringSubmatch(scanner.Text())
			value, err := strconv.ParseUint(matches[1], 10, 64)
			if err != nil {
				return nil, err
			}
			cpuIterations.totalIterations = append(cpuIterations.totalIterations, value)
		} else if strings.Contains(scanner.Text(), "Core") {
			matches := re.FindAllString(scanner.Text(), -1)
			if len(matches) == 4 {
				// Index 0 = CPU ID
				// Index 1 = APIC ID
				// Index 2 = Total Iterations (so far)
				// Index 3 = Iterations completed in this cycle (delta)
				value, err := strconv.ParseUint(matches[2], 10, 64)
				if err != nil {
					return nil, err
				}
				cpuIterations.coresIteration[matches[0]] = append(cpuIterations.coresIteration[matches[0]], value)
			}
		}
	}

	return &cpuIterations, nil
}

func convertToMeasuredUnits(value uint64) float64 {
	return float64(value) / 1000000.0
}

func insertIntervalsIntoPerf(pv *perf.Values, data *iterationData, intervals []iterationInterval) {
	var startIteration uint64
	var endIteration uint64

	for _, interval := range intervals {

		// Get the start
		if interval.firstMinute == 0 {
			startIteration = 0
		} else {
			startIteration = data.totalIterations[interval.firstMinute-1]
		}

		// Get the end
		if interval.lastMinute > 0 {
			endIteration = data.totalIterations[interval.lastMinute-1]
		} else {
			endIteration = 0
		}

		// Insert the value into perf infra
		pv.Set(
			perf.Metric{
				Name:      fmt.Sprintf("total_iterations_%d_%d_min", interval.firstMinute, interval.lastMinute),
				Unit:      "1e6_iterations",
				Direction: perf.BiggerIsBetter,
			}, convertToMeasuredUnits(endIteration-startIteration))
	}
}

// VMCPUQuantifiedWork will execute the test and then
// report the completed iterations into the testing
// framework
func VMCPUQuantifiedWork(ctx context.Context, s *testing.State) {
	params := s.Param().(testOptions)

	td, err := os.MkdirTemp(s.OutDir(), "")
	if err != nil {
		s.Fatal("Failed to create temporary directory: ", err)
	}
	defer os.RemoveAll(td)

	numCPU, numCPUErr := cpu.GetNumberOfCPU(ctx)
	if numCPUErr != nil {
		s.Fatal("Could not get number of CPU(s) in DUT: ", numCPUErr)
	}

	// The socket path has to be less than 108 chars.
	// This is why the file name is kept intentionally short.
	vmSocketPath := filepath.Join(td, "vms")
	if len(vmSocketPath) >= 108 {
		s.Fatal("VM socket path length is above 108 characters")
	}

	s.Logf("Using %d CPU(s) in VM", numCPU)
	ps := vm.NewCrosvmParamsBIOS(
		s.DataPath("CROSVM.fd"),
		vm.RWDisks(s.DataPath(params.imageFileName)),
		vm.NumCpus(uint(numCPU)),
		vm.SerialOutput(filepath.Join(s.OutDir(), "crosvm.log")),
		vm.Socket(vmSocketPath))

	if _, err := cpu.WaitUntilCoolDown(ctx, cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)); err != nil {
		testing.ContextLog(ctx, "Unable get cool machine using setting: ", err)
		if _, err := cpu.WaitUntilCoolDown(ctx,
			cpu.CoolDownConfig{PollTimeout: 2 * time.Minute,
				PollInterval:         2 * time.Second,
				TemperatureThreshold: 65000,
				CoolDownMode:         cpu.CoolDownPreserveUI}); err != nil {
			// Log cooldown failure
			s.Error("Unable to get cool machine to reach 60C: ", err)
		}
	}

	// We use the shortened deadline context to create the CrosVM
	// object. This is so that if the shortened deadline is hit by
	// a VM launch/run failure we will still have time to close CrosVM
	// before the context times out.
	shortenCtx, shortenCancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer shortenCancel()

	// Fire up the VM here, with our timeout for executing the VM code
	cvm, err := vm.NewCrosvm(shortenCtx, ps)
	if err != nil {
		s.Fatal("Failed to launch crosvm: ", err)
	}
	defer func() {
		// Only close when the socket is available
		// If it completed successfully, we shouldn't
		// make this attempt
		if _, err := os.Stat(vmSocketPath); err == nil {
			cvm.Close(ctx)
		}
	}()

	// Sit here and wait for CrosVM to exit
	// It is expected that the test will terminate the VM once it has
	// completed. If this does not happen then the overall test timeout
	// should catch this and cancel the test.
	if err = cvm.WaitForCompletion(); err != nil {
		s.Fatal("crosvm did not complete successfully: ", err)
	}

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Fatal("Failed to save perf data: ", err)
		}
	}()

	// Extract the completed iterations from the log file
	// and dump them into the performance metrics
	// framework
	extractedValues, errConvert := extractIterations(s.OutDir())
	if errConvert != nil {
		s.Fatal("Failed to extract iteration data", errConvert)
	}

	if params.statsType == fullStatistics {
		// Full report type will spit out
		// the total number of iterations completed
		// for each CPU individually and in aggregate.
		for id, value := range extractedValues.coresIteration {
			// Get the last value
			valueLength := len(value)
			coreIterations := convertToMeasuredUnits(value[valueLength-1])
			pv.Set(
				perf.Metric{
					Name:      fmt.Sprintf("cpu%s_iterations_at_end", id),
					Unit:      "1e6_iterations",
					Direction: perf.BiggerIsBetter,
				}, coreIterations)

			// Dump the total to the log output
			s.Logf("%f Million Iterations completed for CPU %s", coreIterations, id)
		}
	}

	// Dump the total output (this is so that we have the total as the last data point)
	totalArrayLength := len(extractedValues.totalIterations)
	totalIterations := convertToMeasuredUnits(extractedValues.totalIterations[totalArrayLength-1])

	pv.Set(
		perf.Metric{
			Name:      "total_iterations_at_end",
			Unit:      "1e6_iterations",
			Direction: perf.BiggerIsBetter,
		}, totalIterations)

	s.Logf("%f Million Iterations completed in aggregate", totalIterations)

	// If there are any (aggregate) time intervals of interest for this particular test,
	// report them.
	if len(params.recordedIntervals) > 0 {
		insertIntervalsIntoPerf(pv, extractedValues, params.recordedIntervals)
	}

	// Do another cooldown attempt to get a temperature reading post test execution
	// 15 second timeout, 1 second poll interval
	if _, err := cpu.WaitUntilCoolDown(ctx,
		cpu.CoolDownConfig{PollTimeout: 15 * time.Second,
			PollInterval:         1 * time.Second,
			TemperatureThreshold: 65000,
			CoolDownMode:         cpu.CoolDownPreserveUI}); err != nil {
		// Log cooldown failure
		s.Log("CPU could not cool down in time alloted: ", err)
	}

}
