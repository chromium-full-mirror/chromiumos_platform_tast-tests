// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"context"
	"strings"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LabelMap is the type label map.
type LabelMap map[string][]string

// DUTInfo contains additional info about a cellular DUT.
type DUTInfo struct {
	Labels              []string
	ModemInfo           *labapi.ModemInfo
	SimInfo             []*labapi.SIMInfo
	CarrierName         string
	StarfishSlotMapping string
	DevicePools         []string
}

// NewDUTInfoFromStringArgs creates a new DUT config from command line args.
func NewDUTInfoFromStringArgs(ctx context.Context, cmd func(name string) (val string, ok bool), labelName string) (*DUTInfo, error) {
	labels, err := GetLabelsAsStringArray(ctx, cmd, labelName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read labels")
	}
	return &DUTInfo{
		Labels:              labels,
		ModemInfo:           GetModemInfoFromHostInfoLabels(ctx, labels),
		SimInfo:             GetSIMInfoFromHostInfoLabels(ctx, labels),
		CarrierName:         GetCellularCarrierFromHostInfoLabels(ctx, labels),
		StarfishSlotMapping: GetStarfishMappingFromHostInfoLabels(ctx, labels),
		DevicePools:         GetDevicePoolFromHostInfoLabels(ctx, labels),
	}, nil
}

// NewDUTInfoFromConfig creates a new DUT config from a lab DUT config.
func NewDUTInfoFromConfig(config *labapi.Dut) *DUTInfo {
	chrome := config.GetChromeos()
	di := &DUTInfo{
		ModemInfo:   chrome.GetModemInfo(),
		SimInfo:     chrome.GetSimInfos(),
		CarrierName: chrome.GetCellular().GetCarrier(),
	}
	return di
}

// LogInfo prints the DUT info.
func (d *DUTInfo) LogInfo(ctx context.Context) {
	d.PrintLabels(ctx)
	d.PrintModemInfo(ctx)
	d.PrintSIMInfo(ctx)
}

// PrintLabels prints the DUT labels.
func (d *DUTInfo) PrintLabels(ctx context.Context) {
	for _, label := range d.Labels {
		testing.ContextLog(ctx, "Labels: ", label)
	}
}

// PrintModemInfo prints modem details
func (d *DUTInfo) PrintModemInfo(ctx context.Context) {
	if d.ModemInfo == nil {
		testing.ContextLog(ctx, "Unable to print modem info, modem info is not populated")
		return
	}
	testing.ContextLog(ctx, "Modem Type            : ", d.ModemInfo.Type)
	testing.ContextLog(ctx, "Modem IMEI            : ", d.ModemInfo.Imei)
	testing.ContextLog(ctx, "Modem Supported Bands : ", d.ModemInfo.SupportedBands)
	testing.ContextLog(ctx, "Modem SIM Count       : ", d.ModemInfo.SimCount)
}

// PrintSIMInfo prints SIM details
func (d *DUTInfo) PrintSIMInfo(ctx context.Context) {
	for _, s := range d.SimInfo {
		testing.ContextLog(ctx, "SIM Slot ID              : ", s.SlotId)
		testing.ContextLog(ctx, "SIM Type                 : ", s.Type)
		testing.ContextLog(ctx, "SIM Test eSIM            : ", s.TestEsim)
		testing.ContextLog(ctx, "SIM EID                  : ", s.Eid)
		for _, p := range s.ProfileInfo {
			testing.ContextLog(ctx, "SIM Profile ICCID        : ", p.Iccid)
			testing.ContextLog(ctx, "SIM Profile PIN          : ", p.SimPin)
			testing.ContextLog(ctx, "SIM Profile PUK          : ", p.SimPuk)
			testing.ContextLog(ctx, "SIM Profile Carrier Name : ", p.CarrierName)
			testing.ContextLog(ctx, "SIM Profile Own Number   : ", p.OwnNumber)
		}
	}
}

// GetSlotForStarfishCarrier searches all SIM profiles to find which slot
// contains the requested carrier.
func (d *DUTInfo) GetSlotForStarfishCarrier(carrier string) (int32, error) {
	// The carrier starfish is looking for will just be: att, verizon, tmobile, etc. So we should
	// preappend network_ to it so it will match the SIM network string.
	find := "network_" + carrier
	for _, s := range d.SimInfo {
		for _, p := range s.ProfileInfo {
			if strings.EqualFold(p.CarrierName.String(), find) {
				// Starfish indices are 0 indexed.
				return s.SlotId - 1, nil
			}
		}
	}
	return 0, errors.Errorf("failed to find SIM with carrier name %q", find)
}
