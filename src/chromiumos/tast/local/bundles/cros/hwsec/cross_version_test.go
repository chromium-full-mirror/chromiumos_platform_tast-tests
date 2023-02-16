// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"fmt"
	"strings"
	"testing"

	"chromiumos/tast/common/genparams"
)

type crossVersionParam struct {
	Name, Fixture                string
	ExtraAttr, ExtraSoftwareDeps []string
}

type milestoneConfig struct {
	milestone int
	critical  bool
}

var milestoneConfigs = []milestoneConfig{
	// "interesting" versions that are tested in CQ:
	// * the oldest snapshotted version - R88,
	// * the first version that has non-empty password KeyData - R91,
	// * the first version that has type set in password KeyData - R93,
	// * Long-term Support (LTS) version - R96.
	// * Long-term Support (LTS) version - R102.
	// * Long-term Support (LTS) version - R108.
	// * the first version with USS enabled - R110.
	{milestone: 88, critical: true},
	{milestone: 91, critical: true},
	{milestone: 93, critical: true},
	{milestone: 96, critical: true},
	{milestone: 102, critical: true},
	{milestone: 108, critical: true},
	{milestone: 110, critical: true},
	// Other versions that are not tested in CQ
	{milestone: 89, critical: false},
	{milestone: 90, critical: false},
	{milestone: 92, critical: false},
	{milestone: 94, critical: false},
	{milestone: 97, critical: false},
	{milestone: 98, critical: false},
	{milestone: 99, critical: false},
	{milestone: 100, critical: false},
	{milestone: 101, critical: false},
	{milestone: 103, critical: false},
	{milestone: 104, critical: false},
	{milestone: 105, critical: false},
	{milestone: 106, critical: false},
	{milestone: 107, critical: false},
	{milestone: 109, critical: false},
}

type tpmVersion struct {
	name         string
	softwareDeps []string
}

var tpmVersions = []tpmVersion{
	{name: "tpm2", softwareDeps: []string{"no_tpm_dynamic"}},
	{name: "tpm_dynamic", softwareDeps: []string{"tpm_dynamic"}},
}

// The first milestone that tpm dynamic are supported in VM.
var firstTpmDynamicMilestone = 96

func toCamelCase(s string) string {
	var ret []string
	for _, token := range strings.Split(s, "_") {
		ret = append(ret, strings.ToUpper(token[0:1])+strings.ToLower(token[1:]))
	}
	return strings.Join(ret, "")
}

func TestCrossVersionParams(t *testing.T) {
	var params []crossVersionParam
	params = append(params, crossVersionParam{
		Name:      "current",
		Fixture:   "crossVersionCurrent",
		ExtraAttr: []string{"group:mainline", "informational"},
	})
	for _, tpmVer := range tpmVersions {
		for _, config := range milestoneConfigs {
			if tpmVer.name == "tpm_dynamic" && config.milestone < firstTpmDynamicMilestone {
				continue
			}

			var attr []string
			if config.critical {
				attr = []string{"group:mainline"}
			} else {
				attr = []string{"group:hwsec", "hwsec_nightly"}
			}

			name := fmt.Sprintf("%s_r%d", tpmVer.name, config.milestone)
			fixture := "crossVersion" + toCamelCase(name)

			param := crossVersionParam{
				Name:              name,
				Fixture:           fixture,
				ExtraAttr:         attr,
				ExtraSoftwareDeps: tpmVer.softwareDeps,
			}
			params = append(params, param)
		}
	}
	code := genparams.Template(t, ` {{ range .}} {
    Name: {{ .Name | fmt }},
    Fixture: {{ .Fixture | fmt }},
    {{ if .ExtraAttr }}
    ExtraAttr: {{ .ExtraAttr | fmt }},
    {{ end }}
		{{ if .ExtraSoftwareDeps }}
    ExtraSoftwareDeps: {{ .ExtraSoftwareDeps | fmt }},
    {{ end }}
  }, {{ end }}`, params)
	genparams.Ensure(t, "cross_version_login.go", code)
}
