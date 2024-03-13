// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/common/hwsec"
)

type crossVersionParam struct {
	Name, Fixture                string
	ExtraAttr, ExtraSoftwareDeps []string
}

type milestoneConfig struct {
	critical bool
	ignore   bool
}

var ignoreMilestone = map[int]bool{
	// There is no R95 for ChromeOS
	95: true,
}

var cryptohomeCriticalMilestone = map[int]bool{
	// * the first version that has non-empty password KeyData - R91,
	// * the first version that has type set in password KeyData - R93,
	// * the first version with USS enabled - R110.
	// * the first version with USS migration - R112.
	91:  true,
	93:  true,
	110: true,
	112: true,
}

type tpmVersion struct {
	name           string
	softwareDeps   []string
	milestoneBegin int
	milestoneEnd   int
	dataPrefix     map[int]string
}

var tpmVersions = []tpmVersion{
	{
		name:           "ti50",
		softwareDeps:   []string{"no_tpm_dynamic", "gsc"},
		milestoneBegin: hwsec.FirstTi50DataMilestone,
		milestoneEnd:   hwsec.LatestTi50DataMilestone,
	}, {
		name:           "tpm2",
		softwareDeps:   []string{"no_tpm_dynamic", "no_gsc"},
		milestoneBegin: hwsec.FirstTpm2DataMilestone,
		milestoneEnd:   hwsec.LatestTpm2DataMilestone,
	}, {
		name:           "tpm_dynamic",
		softwareDeps:   []string{"tpm_dynamic", "no_gsc"},
		milestoneBegin: hwsec.FirstTpmDynamicDataMilestone,
		milestoneEnd:   hwsec.LatestTpmDynamicDataMilestone,
	},
}

// We didn't prepare WebAuthn data until M112.
const webauthnMinMilestone = 112
const defaultMinMilestone = 88

// We didn't prepare InstallAttrs data until M118.
const installAttrsMinMilestone = 118

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

func isLts(milestone int) bool {
	return milestone >= 96 && milestone%6 == 0
}

func makeTestParamsCode(t *testing.T, testMilestoneBegin int, isStable bool) string {
	params := []crossVersionParam{{
		Name:      "current",
		Fixture:   "crossVersion.current",
		ExtraAttr: []string{"group:mainline", "informational"},
	}}
	for _, tpmVer := range tpmVersions {
		milestoneBegin := max(tpmVer.milestoneBegin, testMilestoneBegin)
		for milestone := milestoneBegin; milestone <= tpmVer.milestoneEnd; milestone++ {
			if ignoreMilestone[milestone] {
				continue
			}
			critical := isLts(milestone) ||
				cryptohomeCriticalMilestone[milestone] ||
				milestone == tpmVer.milestoneBegin ||
				milestone == tpmVer.milestoneEnd

			var attr []string
			if critical && isStable {
				attr = []string{"group:mainline"}
			} else {
				// TODO(b/228279919): change this to custom test suite
				attr = []string{"group:mainline", "informational"}
			}

			name := fmt.Sprintf("%s_r%d", tpmVer.name, milestone)
			fixture := "crossVersion." + name

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
	installAttrsParamsCode := makeTestParamsCode(t, installAttrsMinMilestone, false)
	paramsCode := makeTestParamsCode(t, defaultMinMilestone, true)
	webauthnParamsCode := makeTestParamsCode(t, webauthnMinMilestone, false)
	genparams.Ensure(t, "cross_version_auth_factor.go", paramsCode)
	genparams.Ensure(t, "cross_version_chrome_login.go", paramsCode)
	genparams.Ensure(t, "cross_version_install_attributes.go", installAttrsParamsCode)
	genparams.Ensure(t, "cross_version_webauthn_login.go", webauthnParamsCode)
}
