// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type mtuTestParams struct {
	modbOverrideProto string
	mtu               int32
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           MTU,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that MTU information from the network/MODB is correctly honored",
		Contacts:       []string{"chromeos-cellular-team@google.com", "ejcaruso@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Params: []testing.Param{{
			Name:      "high_mtu",
			Val:       mtuTestParams{"callbox_high_mtu.pbf", 1800},
			ExtraData: []string{"callbox_high_mtu.pbf"},
		}, {
			Name:      "low_mtu",
			Val:       mtuTestParams{"callbox_low_mtu.pbf", 1400},
			ExtraData: []string{"callbox_low_mtu.pbf"},
		}, {
			Name:      "high_mtu_modb_override",
			Val:       mtuTestParams{"callbox_high_mtu_override.pbf", 1550},
			ExtraData: []string{"callbox_high_mtu_override.pbf"},
		}, {
			Name:      "low_mtu_modb_no_override",
			Val:       mtuTestParams{"callbox_low_mtu_override.pbf", 1400},
			ExtraData: []string{"callbox_low_mtu_override.pbf"},
		}},
		Fixture: "cellularResetShillProfileOnPostTest",
		Timeout: 2 * time.Minute,
	})
}

func MTU(ctx context.Context, s *testing.State) {
	params := s.Param().(mtuTestParams)
	modbOverrideProto := params.modbOverrideProto
	mtuExpected := params.mtu

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
	service, err := helper.ConnectWithTimeout(ctx, 10*time.Second)
	if err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	ipConfig, err := service.GetCurrentIPConfig(ctx)
	if err != nil {
		s.Fatal("Unable to find IP configuration for service: ", err)
	}

	ipProps, err := ipConfig.GetIPProperties(ctx)
	if err != nil {
		s.Fatal("Unable to fetch IP configuration properties: ", err)
	}

	if ipProps.MTU != mtuExpected {
		err := cellular.TagKnownBugOnModem(ctx, nil, "b/292770737", cellular.ModemFwFilterL850MR7AndLower)
		s.Fatalf("Unexpected MTU value for service: got %v, want %v. %s", ipProps.MTU, mtuExpected, cellular.ErrorToCleanString(err))
	}

	configuredMTU, err := getConfiguredMTU(ctx, service)
	if err != nil {
		s.Fatal("Unable to get MTU configured on net interface: ", err)
	}

	if configuredMTU > ipProps.MTU {
		s.Fatalf("Unexpected MTU value on net interface: got %v, want at most %v", configuredMTU, ipProps.MTU)
	}
}

func getConfiguredMTU(ctx context.Context, service *shill.Service) (int32, error) {
	device, err := service.GetDevice(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "could not fetch device for service")
	}

	p, err := device.GetProperties(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "could not fetch device properties")
	}

	iface, err := p.GetString(shillconst.DevicePropertyCellularPrimaryMultiplexedInterface)
	if err != nil {
		return 0, errors.Wrap(err, "could not fetch device interface name")
	}

	ifaceMTUFile := filepath.Join("/sys/class/net", iface, "mtu")
	bytes, err := ioutil.ReadFile(ifaceMTUFile)
	if err != nil {
		return 0, errors.Wrap(err, "could not read sysfs file for MTU")
	}

	mtu, err := strconv.Atoi(strings.TrimSpace(string(bytes)))
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse MTU value from sysfs file")
	}

	return int32(mtu), nil
}
