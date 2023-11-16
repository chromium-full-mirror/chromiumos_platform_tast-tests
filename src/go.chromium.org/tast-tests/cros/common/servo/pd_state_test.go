// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"regexp"
	"testing"
)

func TestPDStateRegex(t *testing.T) {
	for idx, tc := range []struct {
		version    TCPMVersion
		input      string
		PortNumber string
		CCPolarity string
		Connection string
		PowerRole  string
		DataRole   string
		VConn      string
		TCState    string
		TCFlags    string
		PEState    string
		PEFlags    string
	}{
		{version: TCPMv1, input: "Port C0 CC1, Ena - Role: SNK-DFP State: 8(), Flags: 0x16946\n",
			PortNumber: "0", CCPolarity: "CC1", Connection: "Ena", PowerRole: "SNK", DataRole: "DFP", PEState: "PD_STATE_SNK_READY", PEFlags: "16946"},
		{version: TCPMv1, input: "Port C0 CC1, Ena - Role: SNK-DFP State: 8(A_STATE_NAME), Flags: 0x16946\n",
			PortNumber: "0", CCPolarity: "CC1", Connection: "Ena", PowerRole: "SNK", DataRole: "DFP", PEState: "A_STATE_NAME", PEFlags: "16946"},
		{version: TCPMv1, input: "Port C0 CC1, Ena - Role: SNK-DFP State: SNK_READY, Flags: 0x4946\n",
			PortNumber: "0", CCPolarity: "CC1", Connection: "Ena", PowerRole: "SNK", DataRole: "DFP", PEState: "SNK_READY", PEFlags: "4946"},
		{version: TCPMv2, input: "Port C0 CC1, Enable - Role: SNK-DFP TC State: Attached.SNK, Flags: 0x9012 PE State: PE_SNK_Ready, Flags: 0x0201 SPR\n",
			PortNumber: "0", CCPolarity: "CC1", Connection: "Enable", PowerRole: "SNK", DataRole: "DFP", TCState: "Attached.SNK", TCFlags: "9012", PEState: "PE_SNK_Ready", PEFlags: "0201"},
	} {
		testRe := pdStateCmdRegexp[tc.version] + "|" + pdStateInvalidPortRegexp
		re := regexp.MustCompile(testRe)
		m := re.FindStringSubmatch(tc.input)
		if m == nil {
			t.Errorf("[%d]Input did not match regex", idx)
			continue
		}
		tokens := pdStateTokens(m)

		if m[pdStateFieldIndex[tc.version]["PortNumber"]] != tc.PortNumber {
			t.Errorf("[%d]PortNumber incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["PortNumber"]], tc.PortNumber)
		}
		if m[pdStateFieldIndex[tc.version]["CCPolarity"]] != tc.CCPolarity {
			t.Errorf("[%d]CCPolarity incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["CCPolarity"]], tc.CCPolarity)
		}
		if m[pdStateFieldIndex[tc.version]["Connection"]] != tc.Connection {
			t.Errorf("[%d]Connection incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["Connection"]], tc.Connection)
		}
		if m[pdStateFieldIndex[tc.version]["PowerRole"]] != tc.PowerRole {
			t.Errorf("[%d]PowerRole incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["PowerRole"]], tc.PowerRole)
		}
		if m[pdStateFieldIndex[tc.version]["DataRole"]] != tc.DataRole {
			t.Errorf("[%d]DataRole incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["DataRole"]], tc.DataRole)
		}
		if m[pdStateFieldIndex[tc.version]["VConn"]] != tc.VConn {
			t.Errorf("[%d]VConn incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["VConn"]], tc.VConn)
		}
		if tc.version == TCPMv2 && m[pdStateFieldIndex[tc.version]["TCState"]] != tc.TCState {
			t.Errorf("[%d]TCState incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["TCState"]], tc.TCState)
		}
		if tc.version == TCPMv2 && m[pdStateFieldIndex[tc.version]["TCFlags"]] != tc.TCFlags {
			t.Errorf("[%d]TCFlags incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["TCFlags"]], tc.TCFlags)
		}
		if peStateName, err := tokens.peStateName(tc.version); err != nil {
			t.Errorf("[%d]peStateName failed: %v", idx, err)
		} else if peStateName != tc.PEState {
			t.Errorf("[%d]PEState incorrect, got %q, want %q", idx, peStateName, tc.PEState)
		}
		if m[pdStateFieldIndex[tc.version]["PEFlags"]] != tc.PEFlags {
			t.Errorf("[%d]PEFlags incorrect, got %q, want %q", idx, m[pdStateFieldIndex[tc.version]["PEFlags"]], tc.PEFlags)
		}
	}
}
