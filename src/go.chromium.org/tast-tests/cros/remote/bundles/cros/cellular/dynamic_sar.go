// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/manager"
	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/power"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	sarSampleCount      = 500
	sarDefaultThreshold = 2.5
)

type dSARTestCase struct {
	testPower       float64
	startingOptions *manager.ConfigureCallboxRequestBody
	dataPostfix     string
	bandEntry       string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DynamicSAR,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that Tx power received at the callbox is within acceptable limits for a given SAR level/band combination",
		Contacts:     []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_cmw_callbox"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		// restrict tests to models that we have SAR tables for, TODO: revisit with (b/257515425)
		HardwareDeps: hwdep.D(hwdep.Model("lazor", "pujjo")),
		Fixture:      "callboxManagedFixture",
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "lte",
				ExtraData: []string{"sar_lazor_lte.csv", "sar_pujjo_lte.csv"},
				Val: dSARTestCase{
					// perform test at max LTE power
					testPower:   24.5,
					dataPostfix: "lte",
					bandEntry:   "7",
					startingOptions: &manager.ConfigureCallboxRequestBody{
						Hardware:     manager.CallboxHardwareCMW,
						CellularType: manager.CellularTechnologyLTE,
						Parameters: []manager.CellConfiguration{
							manager.NewLteCellConfiguration(
								manager.BandOption(7),
							),
						},
					},
				},
			},
		},
	})
}

func DynamicSAR(ctx context.Context, s *testing.State) {
	tc := s.Param().(dSARTestCase)
	tf := s.FixtValue().(*manager.TestFixture)
	dutConn := s.DUT().Conn()

	if err := tf.ConnectToCallbox(ctx, dutConn, tc.startingOptions); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	session := power.NewTxMeasurementSession(tf.CallboxManagerClient, tf.RemoteCellularClient)
	defer session.Close(cleanupCtx)

	board, err := reporters.New(s.DUT()).Model(ctx)
	if err != nil {
		s.Fatal("Failed to get model name: ", err)
	}

	sarTable, err := readSARTable(ctx, s.DataPath(fmt.Sprintf("sar_%s_%s.csv", board, tc.dataPostfix)))
	if err != nil {
		s.Fatal("Failed to get supported SAR power levels: ", err)
	}

	configs, ok := sarTable[tc.bandEntry]
	if !ok {
		s.Fatalf("No band %q found in SAR table", tc.bandEntry)
	}

	// start a subtest for each supported SAR configuration
	for _, config := range configs {
		subTest := func(ctx context.Context, s *testing.State) {
			measurementConfig := &power.TxMeasurementConfiguration{
				TestPower: tc.testPower,
				// calibrate just below the target power as we may be clipped if we're close to the device's max
				CalibrationPower: config.expectedPower - 2*config.threshold,
				SARLevel:         config.level,
				SampleCount:      sarSampleCount,
			}

			result, err := session.Run(ctx, measurementConfig)
			if err != nil {
				s.Fatal("Failed to run dynamic SAR session: ", err)
			}

			if result.Average < config.expectedPower-config.threshold || result.Average > config.expectedPower+config.threshold {
				s.Fatalf("Tx power outside of limits, want %f +/- %f dBm got: %f", config.expectedPower, config.threshold, result.Average)
			}

			s.Logf("Completed SAR measurement, Min: %f, Max: %f, Average: %f +/- %f dBm", result.Min, result.Max, result.Average, result.StandardDeviation)
		}

		s.Run(ctx, config.name, subTest)
	}
}

type sarConfig struct {
	name          string
	level         int32
	expectedPower float64
	threshold     float64
}

type sarTable map[string][]sarConfig

// readSARTable fetches the supported SAR levels and expected Tx power for the requested band.
func readSARTable(ctx context.Context, filePath string) (sarTable, error) {
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read SAR data file")
	}
	reader := csv.NewReader(strings.NewReader(string(b)))

	// column headers contain requested power levels
	header, err := reader.Read()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read header line")
	}

	levels := make([]int32, 0, len(header))
	for _, level := range header[1:] {
		levelValue, err := strconv.Atoi(level)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse power level: %q", level)
		}
		levels = append(levels, int32(levelValue))
	}

	configurations := make(sarTable)
	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.Wrap(err, "failed to read line")
		}

		// don't allow asymmetric data or missing values
		if len(header) != len(line) {
			return nil, errors.Errorf("failed to parse line, expected %d columns, got: %d", len(header), len(line))
		}

		// leave band as a string since some bands may be denoted with their class e.g. 20C
		band := line[0]
		configurations[band] = make([]sarConfig, 0, len(levels))
		for i, power := range line[1:] {
			powerValue, err := strconv.ParseFloat(power, 64)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parase power: %q", power)
			}

			configurations[band] = append(
				configurations[band],
				sarConfig{
					name:          fmt.Sprintf("b%s_%d", band, levels[i]),
					level:         levels[i],
					expectedPower: powerValue,
					// just use default threshold for all, but leave in case there's a need for power-specific thresholds.
					threshold: sarDefaultThreshold,
				})
		}
	}

	return configurations, nil
}
