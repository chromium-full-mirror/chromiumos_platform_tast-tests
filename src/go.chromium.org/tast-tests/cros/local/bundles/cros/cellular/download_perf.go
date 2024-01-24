// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type downloadPerfParams struct {
	modbOverrideProto string
	isIPv6            bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           DownloadPerf,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that large files can be downloaded over the network and records the average speed",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
			"ejcaruso@google.com",
		},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Params: []testing.Param{{
			Name:      "ipv4",
			Val:       downloadPerfParams{"callbox_null_attach_ipv4.pbf", false},
			ExtraData: []string{"callbox_null_attach_ipv4.pbf"},
		}},
		Fixture: "cellularResetShillProfileOnPostTest",
		Timeout: 20 * time.Minute,
	})
}

func DownloadPerf(ctx context.Context, s *testing.State) {
	params := s.Param().(downloadPerfParams)
	modbOverrideProto := params.modbOverrideProto
	helper := s.FixtValue().(*cellular.FixtData).Helper

	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Failed to disable cellular: ", err)
	}

	cleanup, err := cellular.SetServiceProvidersExclusiveOverride(ctx, s.DataPath(modbOverrideProto))
	if err != nil {
		s.Fatal("Failed to set service providers override: ", err)
	}
	defer cleanup()
	if errs := helper.ResetShill(ctx); errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	if err := helper.WaitForModemRegisteredAfterReset(ctx, 20*time.Second); err != nil {
		s.Fatal("Modem not registered: ", err)
	}

	if _, err := helper.ConnectWithTimeout(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	// Ensure we can reach the network.
	verifyHostIPConnectivity := func(ctx context.Context) error {
		if err := cellular.VerifyIPConnectivityUsingCurl(ctx, testexec.CommandContext, !params.isIPv6, params.isIPv6, "/usr/bin"); err != nil {
			return errors.Wrap(err, "failed connectivity test")
		}
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPConnectivity); err != nil {
		s.Fatal("Failed to get connectivity on cellular interface: ", err)
	}

	type subtest struct {
		name string
		size int
	}

	downloadFile := func(ctx context.Context, st subtest) error {
		url := fmt.Sprintf("http://server-callbox.cros:9986/%s.bin", st.name)

		// Get data and confirm that the correct number of bytes are received.
		resp, err := http.Get(url)
		if err != nil {
			return errors.Wrapf(err, "error fetching data from URL %q", url)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return errors.Errorf("HTTP GET returned bad status code: got %d, want 200", resp.StatusCode)
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return errors.Wrap(err, "error reading data")
		}
		bytesRead := len(body)
		if bytesRead != st.size {
			return errors.Errorf("read wrong number of bytes: got %d, want %d", bytesRead, st.size)
		}
		return nil
	}

	perfValues := perf.NewValues()
	defer func() {
		if err := perfValues.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()

	for _, st := range []subtest{
		{"5MB", 5242880},
		{"20MB", 20971520},
		{"100MB", 104857600},
	} {
		s.Run(ctx, st.name, func(ctx context.Context, s *testing.State) {
			startTime := time.Now()
			if err := downloadFile(ctx, st); err != nil {
				s.Error("Failed to download file: ", err)
			}
			downloadTime := time.Now().Sub(startTime)
			perfValues.Set(perf.Metric{
				Name:      "download_time_" + st.name,
				Unit:      "seconds",
				Direction: perf.SmallerIsBetter,
				Multiple:  false,
			}, downloadTime.Seconds())
			perfValues.Set(perf.Metric{
				Name:      "download_speed_" + st.name,
				Unit:      "bps",
				Direction: perf.BiggerIsBetter,
				Multiple:  false,
			}, float64(8*st.size)/downloadTime.Seconds())
		})
	}
}
