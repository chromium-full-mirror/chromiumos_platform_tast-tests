// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast/core/errors"
)

func assignLastIntValueAndDropKey(d LabelMap, to *int, key string) LabelMap {
	if v, ok := getLastIntValue(d, key); ok {
		*to = v
	}
	delete(d, key)
	return d
}

func getLastIntValue(d LabelMap, key string) (int, bool) {
	if s, ok := getLastStringValue(d, key); ok {
		if c, err := strconv.Atoi(s); err == nil {
			return c, true
		}
	}
	return -1, false
}

func assignLastBoolValueAndDropKey(d LabelMap, to *bool, key string) LabelMap {
	if v, ok := getLastBoolValue(d, key); ok {
		*to = v
	}
	delete(d, key)
	return d
}

func getLastBoolValue(d LabelMap, key string) (bool, bool) {
	if s, ok := getLastStringValue(d, key); ok {
		return strings.ToLower(s) == "true", true
	}
	return false, false
}

func assignLastStringValueAndDropKey(d LabelMap, to *string, key string) LabelMap {
	if v, ok := getLastStringValue(d, key); ok {
		*to = v
	}
	delete(d, key)
	return d
}

func getLastStringValue(d LabelMap, key string) (string, bool) {
	if vs, ok := d[key]; ok && len(vs) > 0 {
		return vs[len(vs)-1], true
	}
	return "", false
}

func getLabelMap(labels []string) LabelMap {
	dims := make(LabelMap)
	for _, label := range labels {
		val := strings.SplitN(label, ":", 2)
		switch len(val) {
		case 1:
			dims[val[0]] = append(dims[val[0]], "")
		case 2:
			dims[val[0]] = append(dims[val[0]], val[1])
		}
	}
	return dims
}

// GetCellularCarrierFromHostInfoLabels return the current carrier name from host_info_labels, else return empty string
func GetCellularCarrierFromHostInfoLabels(ctx context.Context, labels []string) string {
	if c, ok := getLastStringValue(getLabelMap(labels), "carrier"); ok {
		return c
	}
	return ""
}

// GetStarfishMappingFromHostInfoLabels return the starfish slot mapping from host_info_labels, else return empty string
func GetStarfishMappingFromHostInfoLabels(ctx context.Context, labels []string) string {
	if c, ok := getLastStringValue(getLabelMap(labels), "starfish_slot_mapping"); ok {
		return c
	}
	return ""
}

// GetDevicePoolFromHostInfoLabels return the current device pool name from host_info_labels, else return empty string
func GetDevicePoolFromHostInfoLabels(ctx context.Context, labels []string) []string {
	var pools []string
	d := getLabelMap(labels)
	for _, v := range d["pool"] {
		pools = append(pools, v)
	}
	return pools
}

// GetModemInfoFromHostInfoLabels populate Modem info from host_info_labels
func GetModemInfoFromHostInfoLabels(ctx context.Context, labels []string) *labapi.ModemInfo {
	var modemInfo labapi.ModemInfo
	d := getLabelMap(labels)
	if v, ok := getLastStringValue(d, "modem_type"); ok {
		if p, ok := labapi.ModemType_value[v]; ok {
			modemInfo.Type = labapi.ModemType(p)
		}
		delete(d, "modem_type")
	}
	if c, ok := getLastStringValue(d, "modem_imei"); ok {
		modemInfo.Imei = c
	}
	if c, ok := getLastStringValue(d, "modem_supported_bands"); ok {
		modemInfo.SupportedBands = c
	}
	if c, ok := getLastStringValue(d, "modem_sim_count"); ok {
		if j, err := strconv.ParseInt(c, 10, 32); err == nil {
			modemInfo.SimCount = int32(j)
		} else {
			modemInfo.SimCount = 0
		}
	}
	return &modemInfo
}

// GetSIMInfoFromHostInfoLabels populate SIM info from host_info_labels
func GetSIMInfoFromHostInfoLabels(ctx context.Context, labels []string) []*labapi.SIMInfo {
	d := getLabelMap(labels)
	numSim := len(d["sim_slot_id"])
	simInfo := make([]*labapi.SIMInfo, numSim)

	for i, v := range d["sim_slot_id"] {
		simID := v
		s := &labapi.SIMInfo{}
		if j, err := strconv.ParseInt(v, 10, 32); err == nil {
			s.SlotId = int32(j)
		}

		// Example: sim_1_type:SIM_PHYSICAL
		lv := "sim_" + simID + "_type"
		if v, ok := getLastStringValue(d, lv); ok {
			if p, ok := labapi.SIMType_value[v]; ok {
				s.Type = labapi.SIMType(p)
			}
			delete(d, lv)
		}

		lv = "sim_" + simID + "_eid"
		d = assignLastStringValueAndDropKey(d, &s.Eid, lv)

		lv = "sim_" + simID + "_test_esim"
		d = assignLastBoolValueAndDropKey(d, &s.TestEsim, lv)

		lv = "sim_" + simID + "_num_profiles"
		numProfiles := 0
		d = assignLastIntValueAndDropKey(d, &numProfiles, lv)

		s.ProfileInfo = make([]*labapi.SIMProfileInfo, numProfiles)
		for j := 0; j < numProfiles; j++ {
			s.ProfileInfo[j] = &labapi.SIMProfileInfo{}
			profileID := strconv.Itoa(j)
			lv = "sim_" + simID + "_" + profileID + "_iccid"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].Iccid, lv)

			lv = "sim_" + simID + "_" + profileID + "_pin"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].SimPin, lv)

			lv = "sim_" + simID + "_" + profileID + "_puk"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].SimPuk, lv)

			// Example: sim_1_0_carrier_name:NETWORK_TMOBILE
			lv = "sim_" + simID + "_" + profileID + "_carrier_name"
			if v, ok := getLastStringValue(d, lv); ok {
				if c, ok := labapi.NetworkProvider_value[v]; ok {
					s.ProfileInfo[j].CarrierName = labapi.NetworkProvider(c)
				}
				delete(d, lv)
			}

			lv = "sim_" + simID + "_" + profileID + "_own_number"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].OwnNumber, lv)
		}
		simInfo[i] = s
	}

	return simInfo
}

// GetLabelsAsStringArray returns the labels as a string array
func GetLabelsAsStringArray(ctx context.Context, cmd func(name string) (val string, ok bool), labelName string) ([]string, error) {
	labelsStr, ok := cmd(labelName)
	if !ok {
		return nil, errors.New("failed to read autotest_host_info_labels")
	}

	var labels []string
	if err := json.Unmarshal([]byte(labelsStr), &labels); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal label string")
	}

	return labels, nil
}
