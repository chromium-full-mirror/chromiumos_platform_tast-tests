// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package stork contains utilities for communicating with the Stork API, which creates test
// eSIM profiles.
package stork

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// CleanupProfileTime is the time which should be allocated for running a CleanupProfileFunc.
	CleanupProfileTime = 1 * time.Minute
)

// Constants for data sent to/from Stork.
const (
	// Used in POST request.
	gtsTestProfileListKey    = "gtsTestProfileList"
	maxDownloadAttemptsValue = 5
	profileStatusValue       = "RELEASED"
	profileClassValue        = "OPERATIONAL"
	serviceProviderNameValue = "CarrierConfirmationCode"
	generateSmdsEventValue   = true

	// Returned in Stork response.
	sessionIDKey  = "sessionId"
	matchingIDKey = "matchingId"
)

// curl command constants.
const (
	curlCommandName    = "curl"
	cacertArgName      = "--cacert"
	cacertArgValue     = "/usr/share/hermes-ca-certificates/test/gsma-ci.pem"
	hArgName           = "-H"
	hArgValue          = "Content-Type:application/json"
	xArgName           = "-X"
	xArgValue          = "POST"
	dataArgName        = "--data"
	startGtsSessionURL = "https://prod.smdp-plus.rsp.goog/gts/startGtsSession"
)

// Stork URLs.
const (
	// URL for discarding a Stork profile, which needs to be provided a sessionId parameter.
	endGtsSessionURLPrefix = "https://prod.smdp-plus.rsp.goog/gts/endGtsSession?sessionId="

	// Prefix for the
	activationCodePrefix = "1$prod.smdp-plus.rsp.goog$"
)

// ActivationCode to be used to install an eSIM profile.
type ActivationCode string

// CleanupProfileFunc alerts Stork that the profile has been used.
type CleanupProfileFunc func(ctx context.Context) error

// ProfileListData represents the JSON structure of profile list metadata sent to Stork.
type ProfileListData struct {
	Eid                         string `json:"eid"`
	ConfirmationCode            string `json:"confirmationCode"`
	MaxConfirmationCodeAttempts int    `json:"maxConfirmationCodeAttempts"`
	MaxDownloadAttempts         int    `json:"maxDownloadAttempts"`
	ProfileStatus               string `json:"profileStatus"`
	ProfileClass                string `json:"profileClass"`
	ServiceProviderName         string `json:"serviceProviderName"`
	GenerateSmdsEvent           bool   `json:"generateSmdsEvent"`
	ProfilePolicyRules          []int  `json:"profilePolicyRules"`
}

// RequestData represents the JSON structure of an eSIM profile request sent to Stork.
type RequestData struct {
	GtsTestProfileList []ProfileListData `json:"gtsTestProfileList"`
	Eid                string            `json:"eid"`
}

func generateStorkRequestData(eid string, numProfiles int, confirmationCode string, maxConfirmationCodeAttempts int) (string, error) {
	profileListData := ProfileListData{
		Eid:                         eid,
		ConfirmationCode:            confirmationCode,
		MaxConfirmationCodeAttempts: maxConfirmationCodeAttempts,
		MaxDownloadAttempts:         maxDownloadAttemptsValue,
		ProfileStatus:               profileStatusValue,
		ProfileClass:                profileClassValue,
		ServiceProviderName:         serviceProviderNameValue,
		GenerateSmdsEvent:           generateSmdsEventValue,
		ProfilePolicyRules:          []int{},
	}

	storkRequestData := RequestData{
		GtsTestProfileList: []ProfileListData{},
		Eid:                eid,
	}

	for i := 0; i < numProfiles; i++ {
		p := profileListData
		storkRequestData.GtsTestProfileList = append(storkRequestData.GtsTestProfileList, p)
	}
	jsonBytes, err := json.Marshal(&storkRequestData)
	if err != nil {
		return "", errors.Wrap(err, "JSON encoding failed")
	}

	// Stork expects JSON with single quotes.
	return strings.Replace(string(jsonBytes), "\"", "'", -1), nil
}

func getActivationCode(storkResponse map[string]json.RawMessage) (ActivationCode, error) {
	gtsTestProfileListValue, ok := storkResponse[gtsTestProfileListKey]
	if !ok {
		return ActivationCode(""), errors.Errorf("Stork response did not contain %v", gtsTestProfileListKey)
	}

	var gtsTestProfileList []json.RawMessage
	if err := json.Unmarshal(gtsTestProfileListValue, &gtsTestProfileList); err != nil {
		return ActivationCode(""), errors.Wrap(err, "invalid Stork gtsTestProfileList")
	}

	var profileInfo map[string]interface{}
	if err := json.Unmarshal(gtsTestProfileList[0], &profileInfo); err != nil {
		return ActivationCode(""), errors.Wrap(err, "Stork gtsTestProfile response was invalid")
	}

	matchingID, ok := profileInfo[matchingIDKey].(string)
	if !ok {
		return ActivationCode(""), errors.New("Stork matchingId was missing")
	}

	return ActivationCode(activationCodePrefix + matchingID), nil
}

func getSessionID(storkResponse map[string]json.RawMessage) (string, error) {
	sessionIDKeyValue, ok := storkResponse[sessionIDKey]
	if !ok {
		return "", errors.Errorf("Stork response did not contain %v", sessionIDKey)
	}

	var sessionID string
	if err := json.Unmarshal(sessionIDKeyValue, &sessionID); err != nil {
		return "", errors.Wrap(err, "invalid Stork sessionID")
	}
	return sessionID, nil
}

func performFetchStorkProfile(ctx context.Context, data string) (ActivationCode, CleanupProfileFunc, error) {
	command := testexec.CommandContext(ctx, curlCommandName,
		cacertArgName, cacertArgValue,
		hArgName, hArgValue,
		xArgName, xArgValue,
		dataArgName, data,
		startGtsSessionURL)

	output, err := tryCommand(ctx, command)
	if err != nil {
		return ActivationCode(""), nil, errors.Wrap(err, "failed sending Stork request")
	}

	var jsonOutput map[string]json.RawMessage
	if err := json.Unmarshal(output, &jsonOutput); err != nil {
		return ActivationCode(""), nil, errors.Wrap(err, "Stork response was invalid")
	}

	sessionID, err := getSessionID(jsonOutput)
	if err != nil {
		return ActivationCode(""), nil, errors.Wrap(err, "could not find session ID")
	}

	cleanupProfile := CleanupProfileFunc(func(ctx context.Context) error {
		command := testexec.CommandContext(ctx, curlCommandName,
			cacertArgName, cacertArgValue,
			endGtsSessionURLPrefix+sessionID)
		if _, err := tryCommand(ctx, command); err != nil {
			return errors.Wrap(err, "failed Stork cleanup request")
		}
		return nil
	})

	activationCode, err := getActivationCode(jsonOutput)
	if err != nil {
		return ActivationCode(""), cleanupProfile, errors.Wrap(err, "could not find an activation code")
	}

	return activationCode, cleanupProfile, nil
}

func tryCommand(ctx context.Context, command *testexec.Cmd) ([]byte, error) {
	testing.ContextLog(ctx, "STORK COMMAND: ", command.String())
	var output []byte
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var outErr error
		if output, outErr = command.Output(); outErr != nil {
			return outErr
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: 10 * time.Second,
	}); err != nil {
		return nil, err
	}
	return output, nil
}

// FetchStorkProfile fetches a test eSIM profile that does not require a confirmation code from Stork.
func FetchStorkProfile(ctx context.Context) (ActivationCode, CleanupProfileFunc, error) {
	return FetchStorkProfilesForEid(ctx, "", 1)
}

// FetchStorkProfilesForEid fetches a test eSIM profile without a confirmation code for a specific eID.
func FetchStorkProfilesForEid(ctx context.Context, eid string, numProfiles int) (ActivationCode, CleanupProfileFunc, error) {
	data, err := generateStorkRequestData(eid, numProfiles, "", 1)
	if err != nil {
		return ActivationCode(""), nil, err
	}
	return performFetchStorkProfile(ctx, data)
}

// FetchStorkProfileWithCustomConfirmationCode fetches a test eSIM profile with a custom confirmation code from Stork.
func FetchStorkProfileWithCustomConfirmationCode(ctx context.Context, confirmationCode string, maxConfirmationCodeAttempts int) (ActivationCode, CleanupProfileFunc, error) {
	data, err := generateStorkRequestData("", 1, confirmationCode, maxConfirmationCodeAttempts)
	if err != nil {
		return ActivationCode(""), nil, err
	}
	return performFetchStorkProfile(ctx, data)
}
