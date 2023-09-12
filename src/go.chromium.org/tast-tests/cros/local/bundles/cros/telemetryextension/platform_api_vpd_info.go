// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/fixture"
	"go.chromium.org/tast-tests/cros/local/crosconfig"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlatformAPIVPDInfo,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests chrome.os.telemetry.getVpdInfo Chrome Extension API function exposed to Telemetry Extension",
		Contacts:     []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:mainline", "informational",
			"group:criticalstaging",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: fixture.TelemetryExtensionSkipOEMNameCheck,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.TelemetryExtensionSkipOEMNameCheckLacros,
			},
		},
	})
}

// PlatformAPIVPDInfo tests chrome.os.telemetry.getVpdInfo Chrome Extension API functionality.
func PlatformAPIVPDInfo(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	want, err := fetchVPDInfo(ctx)
	if err != nil {
		s.Fatal("Failed to get VPD info: ", err)
	}

	var resp vpdInfoResponse
	if err := v.ExtConn.Call(ctx, &resp,
		"tast.promisify(chrome.os.telemetry.getVpdInfo)",
	); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	if !reflect.DeepEqual(resp, want) {
		s.Errorf("Unexpected VPD info: got %q; want %q", resp, want)
	}
}

type vpdInfoResponse struct {
	ActivateDate string `json:"activateDate"`
	ModelName    string `json:"modelName"`
	SerialNumber string `json:"serialNumber"`
	SkuNumber    string `json:"skuNumber"`
}

func fetchVPDInfo(ctx context.Context) (vpdInfoResponse, error) {
	activateDate, err := fetchOptionalVpdField("/sys/firmware/vpd/rw/ActivateDate")
	if err != nil {
		return vpdInfoResponse{}, errors.Wrap(err, "failed to fetch ActivateDate VPD field")
	}

	modelName, err := fetchOptionalVpdField("/sys/firmware/vpd/ro/model_name")
	if err != nil {
		return vpdInfoResponse{}, errors.Wrap(err, "failed to fetch model_name VPD field")
	}
	// cros_healthd trims model name.
	modelName = strings.TrimSpace(modelName)

	serialNumber, err := fetchOptionalVpdField("/sys/firmware/vpd/ro/serial_number")
	if err != nil {
		return vpdInfoResponse{}, errors.Wrap(err, "failed to fetch serial_number VPD field")
	}

	skuNumber, err := skuNumber(ctx)
	if err != nil {
		return vpdInfoResponse{}, errors.Wrap(err, "failed to fetch sku_number VPD field")
	}

	return vpdInfoResponse{
		ActivateDate: activateDate,
		ModelName:    modelName,
		SerialNumber: serialNumber,
		SkuNumber:    skuNumber,
	}, nil
}

// skuNumber returns expected sku_number that chrome.os.telemetry.getVpdInfo API
// should return.
//
// cros_healthd does not read the sku_number VPD field if cros-config does not
// have proper configuration even if the VPD field is actually present on the
// device. For reference b/210439856.
func skuNumber(ctx context.Context) (string, error) {
	got, err := crosconfig.Get(ctx, "/cros-healthd/cached-vpd", "has-sku-number")
	if err != nil && !crosconfig.IsNotFound(err) {
		return "", errors.Wrap(err, "failed to get has-sku-number value from cros config")
	}

	// has-sku-number is missing in cros-config, so API should return empty
	// `sku_number`.
	if got == "" || got == "false" {
		return "", nil
	}

	if got != "true" {
		return "", errors.Errorf(`unexpected has-sku-number value = got %q, want "", "false" or "true"`, got)
	}

	return fetchOptionalVpdField("/sys/firmware/vpd/ro/sku_number")
}

// fetchOptionalVpdField returns the value of an optional VPD field or the empty
// string if this VPD field does not exist. However it returns an error if the
// existing VPD field is empty.
func fetchOptionalVpdField(path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", errors.Wrapf(err, "failed to check whether %q exists", path)
	}

	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %q", path)
	}
	if len(bytes) == 0 {
		return "", errors.Errorf("%s is empty", path)
	}
	return string(bytes), nil
}
