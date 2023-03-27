// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pci

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/errors"
)

var lspciRes = map[string]string{
	"-n": `Slot:   00:00.0                                           
Class:  0123                                       
Vendor: 12ab                       
Device: 12ab                                       
SVendor:        12ab               
SDevice:        12ab                               
                                                          
Slot:   00:00.2                                           
Class:  1a2b                                
Vendor: 12ab                
Device: 34cd                                       
SVendor:        12ab        
SDevice:        12ab                               
ProgIf: 02
Rev:    1a
Driver: iwlwifi
Module: iwlwifi

Slot:   00:00.3
Class:  1a2b
Vendor: 12eh
Device: 35cd
ProgIf: 02
Rev:    1a
Driver: iwlwifi
Module: iwlwifi

Slot:   00:00.4
Class:  1a2b
Vendor: 13eh
Device: 35cd
SVendor:        12ab        
SDevice:        0000 
ProgIf: 02
Rev:    1a
Driver: iwlwifi
Module: iwlwifi

`,
	"-d12ab:12ab": `Slot:   00:00.0                                           
Class:  Host bridge                                       
Vendor: Alice Bob Carol, Inc. [ABC]                       
Device: Device 12ab                                       
SVendor:        Alice Bob Carol, Inc. [ABC]               
SDevice:        Device 12ab                               
                                                          
                                                          
`,
	"-d12ab:34cd": `Slot:   00:00.2                                           
Class:  Network controller                                
Vendor: Alice Bob Carol, Inc. [ABC]                       
Device: Device 34cd                                       
SVendor:        Alice Bob Carol, Inc. [ABC]               
SDevice:        Device 12ab                               
ProgIf: 02
Rev:    1a
Driver: iwlwifi
Module: iwlwifi

`,
	"-d12eh:35cd": `Slot:   00:00.3                                           
Class:  Network controller                                
Vendor: Alice Bob Carol, Inc. [ABC]                       
Device: Device 34cd                                                                    
ProgIf: 02
Rev:    1a
Driver: iwlwifi
Module: iwlwifi

`,
	"-d13eh:35cd": `Slot:   00:00.4                                           
Class:  Network controller                                
Vendor: Alice Bob Carol, Inc. [ABC]                       
Device: Device 34cd                                                                    
SVendor:        Alice Bob Carol, Inc. [ABC]               
ProgIf: 02
Rev:    1a
Driver: iwlwifi
Module: iwlwifi

`,
}

func TestExpectedDevices(t *testing.T) {
	lspciCmd = func(ctx context.Context, arg string) ([]byte, error) {
		s, ok := lspciRes[arg]
		if !ok {
			return nil, errors.Errorf("unexpected argument: %v", arg)
		}
		return []byte(s), nil
	}

	g, err := ExpectedDevices(context.Background())
	if err != nil {
		t.Fatal("Failed to run ExpectedDevices: ", err)
	}
	dr := "iwlwifi"
	subVendorID := "12ab"
	subDeviceID := "12ab"
	e := []Device{
		Device{
			VendorID:    "12ab",
			DeviceID:    "12ab",
			SubVendorID: &subVendorID,
			SubDeviceID: &subDeviceID,
			Vendor:      "Alice Bob Carol, Inc. [ABC]",
			Device:      "Device 12ab",
			Class:       "0123",
			ProgIf:      "00",
			Driver:      nil,
		},
		Device{
			VendorID:    "12ab",
			DeviceID:    "34cd",
			SubVendorID: &subVendorID,
			SubDeviceID: &subDeviceID,
			Vendor:      "Alice Bob Carol, Inc. [ABC]",
			Device:      "Device 34cd",
			Class:       "1a2b",
			ProgIf:      "02",
			Driver:      &dr,
		},
		Device{
			VendorID:    "12eh",
			DeviceID:    "35cd",
			SubVendorID: nil,
			SubDeviceID: nil,
			Vendor:      "Alice Bob Carol, Inc. [ABC]",
			Device:      "Device 34cd",
			Class:       "1a2b",
			ProgIf:      "02",
			Driver:      &dr,
		},
		Device{
			VendorID:    "13eh",
			DeviceID:    "35cd",
			SubVendorID: &subVendorID,
			SubDeviceID: nil,
			Vendor:      "Alice Bob Carol, Inc. [ABC]",
			Device:      "Device 34cd",
			Class:       "1a2b",
			ProgIf:      "02",
			Driver:      &dr,
		},
	}
	if d := cmp.Diff(e, g); d != "" {
		t.Fatal("Pci test failed (-expected + got): ", d)
	}
}
