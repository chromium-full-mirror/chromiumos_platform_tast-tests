// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

type crossVersionParam struct {
	Name, Fixture                string
	ExtraAttr, ExtraSoftwareDeps []string
}

type milestoneConfig struct {
	critical bool
	ignore   bool
}

var milestoneConfigs = map[int]milestoneConfig{
	// "interesting" versions that are tested in CQ:
	// * the oldest snapshotted version - R88,
	// * the first version that has non-empty password KeyData - R91,
	// * the first version that has type set in password KeyData - R93,
	// * Long-term Support (LTS) version - R96.
	// * Long-term Support (LTS) version - R102.
	// * Long-term Support (LTS) version - R108.
	// * the first version with USS enabled - R110.
	// * the first version with USS migration - R112.
	// * Long-term Support (LTS) version - R114.
	// * the latest version - R118.
	88:  {critical: true},
	91:  {critical: true},
	93:  {critical: true},
	96:  {critical: true},
	102: {critical: true},
	108: {critical: true},
	110: {critical: true},
	112: {critical: true},
	114: {critical: true},
	118: {critical: true},
	// Other versions that are not tested in CQ
	89:  {critical: false},
	90:  {critical: false},
	92:  {critical: false},
	94:  {critical: false},
	97:  {critical: false},
	98:  {critical: false},
	99:  {critical: false},
	100: {critical: false},
	101: {critical: false},
	103: {critical: false},
	104: {critical: false},
	105: {critical: false},
	106: {critical: false},
	107: {critical: false},
	109: {critical: false},
	111: {critical: false},
	113: {critical: false},
	115: {critical: false},
	116: {critical: false},
	117: {critical: false},
	// There is no R95 for ChromeOS
	95: {ignore: true},
}

type tpmVersion struct {
	name           string
	softwareDeps   []string
	milestoneBegin int
	milestoneEnd   int
}

var tpmVersions = []tpmVersion{
	{
		name:           "ti50",
		softwareDeps:   []string{"no_tpm_dynamic", "gsc"},
		milestoneBegin: 112,
		milestoneEnd:   118,
	}, {
		name:           "tpm2",
		softwareDeps:   []string{"no_tpm_dynamic", "no_gsc"},
		milestoneBegin: 88,
		milestoneEnd:   118,
	}, {
		name:           "tpm_dynamic",
		softwareDeps:   []string{"tpm_dynamic", "no_gsc"},
		milestoneBegin: 96,
		milestoneEnd:   118,
	},
}

// We didn't prepare WebAuthn data until M112.
const webauthnMinMilestone = 112
const defaultMinMilestone = 88

func toCamelCase(s string) string {
	var ret []string
	for _, token := range strings.Split(s, "_") {
		ret = append(ret, strings.ToUpper(token[0:1])+strings.ToLower(token[1:]))
	}
	return strings.Join(ret, "")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func makeTestParamsCode(t *testing.T, testMilestoneBegin int, isStable bool) string {
	params := []crossVersionParam{{
		Name:      "current",
		Fixture:   "crossVersionCurrent",
		ExtraAttr: []string{"group:mainline", "informational"},
	}}
	for _, tpmVer := range tpmVersions {
		milestoneBegin := max(tpmVer.milestoneBegin, testMilestoneBegin)
		for milestone := milestoneBegin; milestone <= tpmVer.milestoneEnd; milestone++ {
			config := milestoneConfigs[milestone]

			if config.ignore {
				continue
			}

			var attr []string
			if config.critical && isStable {
				attr = []string{"group:mainline"}
			} else {
				// TODO(b/228279919): change this to custom test suite
				attr = []string{"group:mainline", "informational"}
			}

			name := fmt.Sprintf("%s_r%d", tpmVer.name, milestone)
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
	tmpl := ` {{ range .}} {
    Name: {{ .Name | fmt }},
    Fixture: {{ .Fixture | fmt }},
    {{ if .ExtraAttr }}
    ExtraAttr: {{ .ExtraAttr | fmt }},
    {{ end }}
		{{ if .ExtraSoftwareDeps }}
    ExtraSoftwareDeps: {{ .ExtraSoftwareDeps | fmt }},
    {{ end }}
  }, {{ end }}`
	code := genparams.Template(t, tmpl, params)
	return code
}

func TestCrossVersionParams(t *testing.T) {
	paramsCode := makeTestParamsCode(t, defaultMinMilestone, true)
	webauthnParamsCode := makeTestParamsCode(t, webauthnMinMilestone, false)
	genparams.Ensure(t, "cross_version_auth_factor.go", paramsCode)
	genparams.Ensure(t, "cross_version_chrome_login.go", paramsCode)
	genparams.Ensure(t, "cross_version_webauthn_login.go", webauthnParamsCode)
}
