// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	gotesting "testing"
)

func TestParseTempsECOS(t *gotesting.T) {
	expected := []temp{
		temp{Name: "Battery", TempKelvin: 301},
		temp{Name: "Ambient", TempKelvin: 310},
		temp{Name: "Charger", TempKelvin: 305},
	}
	ecTempsParsed, err := parseTempsOutput(context.Background(), "temps\r\n  Battery             : 301 K = 28 C\r\n  Ambient             : 310 K = 37 C\r\n  Charger             : 305 K = 32 C\r\n>")
	if err != nil {
		t.Fatal("Unexpected error: ", err)
	}
	if len(ecTempsParsed) != len(expected) {
		t.Fatalf("Incorrect number of results got %v, want %v", ecTempsParsed, expected)
	}
	for i := range expected {
		if ecTempsParsed[i].Name != expected[i].Name || ecTempsParsed[i].TempKelvin != expected[i].TempKelvin {
			t.Fatalf("Incorrect results got %+v, want %+v", ecTempsParsed[i], expected[i])
		}
	}
}

func TestParseTempsZephyr(t *gotesting.T) {
	expected := []temp{
		temp{Name: "charger-thermistor", TempKelvin: 313},
		temp{Name: "memory-thermistor", TempKelvin: 313},
		temp{Name: "cpu@4c", TempKelvin: 324},
		temp{Name: "soc-pct2075@48", TempKelvin: 312},
		temp{Name: "amb-pct2075@4f", TempKelvin: 310},
	}
	ecTempsParsed, err := parseTempsOutput(context.Background(),
		"temps\r\n   charger-thermistor    313 K (= 40 C)\r\n   memory-thermistor     313 K (= 40 C)\r\n   cpu@4c                324 K (= 51 C)\r\n"+
			"   soc-pct2075@48        312 K (= 39 C)\r\n   amb-pct2075@4f        310 K (= 37 C)           38% (305 K and 318 K)\r\n ec:~$")
	if err != nil {
		t.Fatal("Unexpected error: ", err)
	}
	if len(ecTempsParsed) != len(expected) {
		t.Fatalf("Incorrect number of results got %v, want %v", ecTempsParsed, expected)
	}
	for i := range expected {
		if ecTempsParsed[i].Name != expected[i].Name || ecTempsParsed[i].TempKelvin != expected[i].TempKelvin {
			t.Fatalf("Incorrect results got %+v, want %+v", ecTempsParsed[i], expected[i])
		}
	}
}
