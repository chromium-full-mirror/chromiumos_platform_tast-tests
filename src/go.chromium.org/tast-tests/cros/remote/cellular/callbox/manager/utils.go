// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package manager

import (
	"context"

	"go.chromium.org/tast/core/testing/cellularconst"
)

// GetMaxLTETxThroughputInMbps returns known modem max upload throughput.
func GetMaxLTETxThroughputInMbps(ctx context.Context, modemType uint32) int {
	switch cellularconst.ModemType(modemType) {
	case cellularconst.ModemTypeL850, cellularconst.ModemTypeNL668, cellularconst.ModemTypeFM101:
		return 50
	case cellularconst.ModemTypeFM350:
		return 211
	case cellularconst.ModemTypeSC7180, cellularconst.ModemTypeSC7280:
		return 150
	default:
		return 50
	}
}

// GetMaxLTERxThroughputInMbps returns known modem max download throughput.
func GetMaxLTERxThroughputInMbps(ctx context.Context, modemType uint32) int {
	switch cellularconst.ModemType(modemType) {
	case cellularconst.ModemTypeL850:
		return 450
	case cellularconst.ModemTypeNL668:
		return 150
	case cellularconst.ModemTypeFM101:
		return 300
	case cellularconst.ModemTypeFM350:
		return 1600
	case cellularconst.ModemTypeSC7180:
		return 800
	case cellularconst.ModemTypeSC7280:
		return 600
	default:
		return 100
	}
}

// Get3CAConfiguration returns best known throughput configuration to configure callbox with three CC.
func Get3CAConfiguration(ctx context.Context, hw CallboxHardware, techType CellularTechnology, bands []int, mimo MimoMode, tm TransmissionMode) *ConfigureCallboxRequestBody {
	return &ConfigureCallboxRequestBody{
		Hardware:     hw,
		CellularType: techType,
		Parameters: []CellConfiguration{
			NewLteCellConfiguration(
				BandOption(bands[0]),
				AntennaOption(mimo, tm),
			),
			NewLteCellConfiguration(
				BandOption(bands[1]),
				AntennaOption(mimo, tm),
			),
			NewLteCellConfiguration(
				BandOption(bands[2]),
				AntennaOption(mimo, tm),
			),
		},
	}
}

// Get2CAConfiguration returns best known throughput configuration to configure callbox with two Carrier Components.
func Get2CAConfiguration(ctx context.Context, hw CallboxHardware, techType CellularTechnology, bands []int, mimo MimoMode, tm TransmissionMode) *ConfigureCallboxRequestBody {
	return &ConfigureCallboxRequestBody{
		Hardware:     hw,
		CellularType: techType,
		Parameters: []CellConfiguration{
			NewLteCellConfiguration(
				BandOption(bands[0]),
				AntennaOption(mimo, tm),
			),
			NewLteCellConfiguration(
				BandOption(bands[1]),
				AntennaOption(mimo, tm),
			),
		},
	}
}

// GetConfiguration returns best known throughput configuration to configure callbox with one CC.
func GetConfiguration(ctx context.Context, hw CallboxHardware, techType CellularTechnology, band int, mimo MimoMode, tm TransmissionMode) *ConfigureCallboxRequestBody {
	return &ConfigureCallboxRequestBody{
		Hardware:     hw,
		CellularType: techType,
		Parameters: []CellConfiguration{
			NewLteCellConfiguration(
				BandOption(band),
				AntennaOption(mimo, tm),
			),
		},
	}
}

// GetCellConfiguration returns a throughput configuration based on modem type.
func GetCellConfiguration(ctx context.Context, hw CallboxHardware, modemType uint32) *ConfigureCallboxRequestBody {
	switch cellularconst.ModemType(modemType) {
	case cellularconst.ModemTypeL850:
		bands := []int{3, 3, 7}
		return Get3CAConfiguration(ctx, hw, CellularTechnologyLTE, bands, MimoMode2x2, TransmissionMode3)
	case cellularconst.ModemTypeNL668:
		return GetConfiguration(ctx, hw, CellularTechnologyLTE, 2, MimoMode1x1, TransmissionMode3)
	case cellularconst.ModemTypeFM101:
		bands := []int{3, 7}
		return Get2CAConfiguration(ctx, hw, CellularTechnologyLTE, bands, MimoMode2x2, TransmissionMode3)
	case cellularconst.ModemTypeFM350:
		bands := []int{3, 3, 7}
		return Get3CAConfiguration(ctx, hw, CellularTechnologyLTE, bands, MimoMode2x2, TransmissionMode3)
	case cellularconst.ModemTypeSC7180:
		bands := []int{3, 7}
		return Get2CAConfiguration(ctx, hw, CellularTechnologyLTE, bands, MimoMode2x2, TransmissionMode3)
	case cellularconst.ModemTypeSC7280:
		bands := []int{3, 7}
		return Get2CAConfiguration(ctx, hw, CellularTechnologyLTE, bands, MimoMode2x2, TransmissionMode3)
	default:
		bands := []int{3, 7}
		return Get2CAConfiguration(ctx, hw, CellularTechnologyLTE, bands, MimoMode2x2, TransmissionMode3)
	}
}
