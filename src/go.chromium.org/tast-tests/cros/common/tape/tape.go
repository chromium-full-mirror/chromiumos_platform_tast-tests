// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tape enables access to the TAPE service which offers access to owned test accounts and
// configuration of policies on DPanel for those accounts.
// The TAPE project is documented here: TODO(alexanderhartl): add link once its finished
package tape

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const tapeURL = "https://tape-307412.ey.r.appspot.com/"
const tapeAudience = "770216225211-ihjn20dlehf94m9l4l5h0b0iilvd1vhc.apps.googleusercontent.com"
const callTimeout = 30 * time.Second
const requestAccountTimeout = 5 * time.Minute
const setPolicyTimeout = 5 * time.Minute
const deprovisionTimeout = 1 * time.Minute

// client is created with NewClient and holds a *http.Client struct with an oauth token
// for authentication against the TAPE GCP.
type client struct {
	httpClient *http.Client
	creds      []byte
}

// createTokenSource an oauth2.TokenSource from service account key credentials.
func createTokenSource(ctx context.Context, credsJSON []byte) (oauth2.TokenSource, error) {
	config, err := google.JWTConfigFromJSON(credsJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate JWT config")
	}
	customClaims := make(map[string]interface{})
	customClaims["target_audience"] = tapeAudience

	config.PrivateClaims = customClaims
	config.UseIDToken = true

	return config.TokenSource(ctx), nil
}

// NewClient creates a http client which provides the necessary oauth token to authenticate with the TAPE
// GCP from the service account credentials in credsJSON.
func NewClient(ctx context.Context, credsJSON []byte) (*client, error) {
	// Log the time and hash of the credsJSON for debugging.
	hasher := sha1.New()
	hasher.Write(credsJSON)
	hash := base64.URLEncoding.EncodeToString(hasher.Sum(nil))
	testing.ContextLogf(ctx, "CredsJSON hash: %s", hash)

	// Check if token content was written to the DUT and should be used.
	if _, err := os.Stat(dutTokenFilePath); err == nil {
		tokenBytes, err := os.ReadFile(dutTokenFilePath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read token content")
		}

		var tokenContent oauth2.Token
		if err := json.Unmarshal(tokenBytes, &tokenContent); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal token content")
		}
		return &client{
			httpClient: oauth2.NewClient(ctx, oauth2.StaticTokenSource(&tokenContent)),
		}, nil
	}

	// Return the Oauth client using the supplied credentials.
	ts, err := createTokenSource(ctx, credsJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create token from json")
	}

	return &client{
		httpClient: oauth2.NewClient(ctx, ts),
		creds:      credsJSON,
	}, nil
}

// refreshClient creates a new http client for the tape client.
func (c *client) refreshClient(ctx context.Context) error {
	// Create new Oauth client using the stored credentials.
	ts, err := createTokenSource(ctx, c.creds)
	if err != nil {
		return errors.Wrap(err, "failed to create token from json")
	}

	c.httpClient = oauth2.NewClient(ctx, ts)
	return nil
}

// sendRequestWithTimeout makes a call to the specified REST endpoint of TAPE with the given http method and payload.
func (c *client) sendRequestWithTimeout(ctx context.Context, method, endpoint string, timeout time.Duration, retries int, payloadBytes []byte) (*http.Response, error) {
	// Set the timeout of the http client and return to the original after.
	originalTimeout := c.httpClient.Timeout
	c.httpClient.Timeout = timeout
	defer func() {
		c.httpClient.Timeout = originalTimeout
	}()

	var err error
	var req *http.Request
	var response *http.Response

	for i := 0; i < retries+1; i++ {
		// Create a request.
		payload := bytes.NewReader(payloadBytes)
		req, err = http.NewRequestWithContext(ctx, method, tapeURL+endpoint, payload)
		if err != nil {
			testing.ContextLog(ctx, "Failed to create request: ", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		// Send the request.
		response, err = c.httpClient.Do(req)
		if err != nil {
			testing.ContextLog(ctx, "Failed to send request: ", err)
			// Try to refresh the client.
			if err = c.refreshClient(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to refresh the client: ", err)
			}
			continue
		}
		// Do not retry when the server is overloaded.
		if response.StatusCode == 503 {
			break
		}
		// Check if the call was successful.
		if response.StatusCode != 200 {
			testing.ContextLogf(ctx, "%s at %s returned %s", method, endpoint, response.Status)
			testing.Sleep(ctx, 15*time.Second) // GoBigSleepLint: wait a few seconds before the retry as we just spam the server otherwise.
			continue
		}
		break
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get a response from TAPE")
	}
	// Check if the call was successful.
	if response.StatusCode != 200 {
		responseBytes, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, errors.Wrapf(err, "%s at %s/%s returned %s failed to read response body", method, tapeURL, endpoint, response.Status)
		}
		return nil, errors.Errorf("%s at %s/%s returned %s %s", method, tapeURL, endpoint, response.Status, string(responseBytes))
	}
	return response, nil
}

func validateAccountRequest(timeoutInSeconds, maxTimeoutInSeconds int64, poolID *string) error {
	// Validate the provided parameters.
	if timeoutInSeconds > maxTimeoutInSeconds {
		return errors.Errorf("Timeout may not be larger than %v seconds, got %v seconds", maxTimeoutInSeconds, timeoutInSeconds)
	}

	if poolID != nil && len(*poolID) <= 0 {
		return errors.New("PoolID must not be empty when set")
	}

	return nil
}

func (c *client) requestAccount(ctx context.Context, endpoint string, params interface{}) ([]byte, error) {
	payloadBytes, err := json.Marshal(params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal data")
	}

	response, err := c.sendRequestWithTimeout(ctx, "POST", endpoint, requestAccountTimeout, 4, payloadBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request")
	}
	defer response.Body.Close()

	// Read the response.
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}

	return respBody, nil
}

func (c *client) releaseAccount(ctx context.Context, account interface{}, endpoint string) error {
	payloadBytes, err := json.Marshal(account)
	if err != nil {
		return errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", endpoint, callTimeout, 2, payloadBytes)
	if err != nil {
		return errors.Wrap(err, "failed to make request")
	}
	defer response.Body.Close()

	// Make sure the request was successful.
	if response.StatusCode != 200 {
		return errors.Errorf("failed to release account, status code: %d", response.StatusCode)
	}

	return nil
}

// DefaultAccountTimeoutInSeconds is the default timeout for an account request (5 mintues).
const DefaultAccountTimeoutInSeconds = 60 * 5

type requestAccountOption struct {
	TimeoutInSeconds int32
	PoolID           *string
}

// RequestAccountOption provides options for requesting an account.
type RequestAccountOption func(*requestAccountOption)

// WithTimeout provides the option to set a timeout for the account lease.
func WithTimeout(timeoutInSeconds int32) RequestAccountOption {
	return func(opt *requestAccountOption) {
		opt.TimeoutInSeconds = timeoutInSeconds
	}
}

// WithPoolID provides the option to set a poolID from which the account should be taken.
func WithPoolID(poolID string) RequestAccountOption {
	return func(opt *requestAccountOption) {
		opt.PoolID = &poolID
	}
}

// GenericAccount holds the data of a generic account that can be used in tests.
type GenericAccount struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	Password    string  `json:"password"`
	PoolID      string  `json:"pool_id"`
	ReleaseTime float64 `json:"release_time"`
	RequestID   string  `json:"request_id"`
}

// MaxGenericAccountTimeoutInSeconds is the maximum timeout which is allowed
// when requesting a generic account (2 hours).
const MaxGenericAccountTimeoutInSeconds = 60 * 60 * 2

// requestOwnedTestAccountParams is a struct containing the necessary data to request a generic
// account from TAPE.
type requestGenericAccountParams struct {
	TimeoutInSeconds int32   `json:"timeout"`
	PoolID           *string `json:"pool_id"`
}

// NewRequestGenericAccountParams creates a new requestGenericAccountParams struct filled with the given parameters.
func NewRequestGenericAccountParams(timeoutInSeconds int32, poolID string) *requestGenericAccountParams {
	return &requestGenericAccountParams{
		TimeoutInSeconds: timeoutInSeconds,
		PoolID:           &poolID,
	}
}

// RequestGenericAccount sends a request for leasing a generic account and returns the account in a GenericAccount struct.
func (c *client) RequestGenericAccount(ctx context.Context, opts ...RequestAccountOption) (*GenericAccount, error) {
	// Copy over all options.
	options := requestAccountOption{
		TimeoutInSeconds: DefaultAccountTimeoutInSeconds,
	}
	for _, opt := range opts {
		opt(&options)
	}
	params := requestGenericAccountParams(options)

	validateAccountRequest(int64(params.TimeoutInSeconds), int64(MaxGenericAccountTimeoutInSeconds), params.PoolID)

	respBody, err := c.requestAccount(ctx, "GenericAccount/request", params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request account")
	}

	var account GenericAccount
	err = json.Unmarshal(respBody, &account)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response")
	}
	return &account, nil
}

// ReleaseGenericAccount sends a request for releasing a leased generic account.
func (c *client) ReleaseGenericAccount(ctx context.Context, account *GenericAccount) error {
	return c.releaseAccount(ctx, account, "GenericAccount/release")
}

// MaxOwnedTestAccountTimeout is the maximum timeout which is allowed
// when requesting an owned test account (6 hours).
const MaxOwnedTestAccountTimeout = 60 * 60 * 6

// OwnedTestAccount holds all data of an owned test account which can be used in tests.
type OwnedTestAccount struct {
	GenericAccount
	GaiaID      string `json:"gaia_id"`
	CustomerID  string `json:"customer_id"`
	OrgunitID   string `json:"orgunit_id"`
	OrgUnitPath string `json:"orgunit_path"`
}

// requestOwnedTestAccountParams is a struct containing the necessary data to request an owned
// test account from TAPE.
type requestOwnedTestAccountParams struct {
	requestGenericAccountParams
	Lock bool `json:"lock"`
}

// NewRequestOwnedTestAccountParams creates a new requestOwnedTestAccountParams struct filled with the given parameters.
func NewRequestOwnedTestAccountParams(timeoutInSeconds int32, poolID string, lock bool) *requestOwnedTestAccountParams {
	params := NewRequestGenericAccountParams(timeoutInSeconds, poolID)
	return &requestOwnedTestAccountParams{
		requestGenericAccountParams: *params,
		Lock:                        lock,
	}
}

// RequestOwnedTestAccount sends a request for leasing a generic account and returns the account in an OwnedTestAccount struct.
func (c *client) RequestOwnedTestAccount(ctx context.Context, lock bool, opts ...RequestAccountOption) (*OwnedTestAccount, error) {
	// Copy over all options.
	options := requestAccountOption{
		TimeoutInSeconds: DefaultAccountTimeoutInSeconds,
	}
	for _, opt := range opts {
		opt(&options)
	}

	params := requestOwnedTestAccountParams{
		requestGenericAccountParams: requestGenericAccountParams(options),
		Lock:                        lock,
	}

	validateAccountRequest(int64(params.TimeoutInSeconds), int64(MaxOwnedTestAccountTimeout), params.PoolID)

	respBody, err := c.requestAccount(ctx, "OTA/request", params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request account")
	}

	var account OwnedTestAccount
	err = json.Unmarshal([]byte(respBody), &account)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response")
	}
	return &account, nil
}

// ReleaseAccount calls TAPE to release the Account so it becomes available again.
func (c *client) ReleaseOwnedTestAccount(ctx context.Context, account *OwnedTestAccount) error {
	return c.releaseAccount(ctx, account, "OTA/release")
}

// setPolicyRequest is a struct containing the necessary data to set a policy schema in DPanel.
type setPolicyRequest struct {
	PolicySchema string `json:"policy_schema"`
	RequestID    string `json:"request_id"`
}

// SetPolicy calls TAPE to set a policySchema in DPanel. The updateMask
// indicates which fields of the policySchema are actually going to be used to
// update the policy. This prevents the parameters of a policy that were no
// explicitly set in the policySchema to be overwritten by default values. When
// an empty slice is passed all fields will be used.
// The strings for the updateMask are equal to the field names of the
// policySchema struct starting with a lowercase letter. The
// additionalTargetKeys are key value pairs to identify other target resources
// for the policy, e.g. for modifying networks the network_id of the network
// that will be modified has to be provided in the additionalTargetKeys in the
// form of {network_id: "myNetworkID"}.
// If the policy contains acknowledgement fields that are named
// "AckNoticeFor<field_name>SetTo<value>" then setting <field_name> to <value>
// requires an acknowledgement which is given by setting the corresponding to
// acknowledgement field to true. The acknowledgement field has also to be
// added in the updateMask.
func (c *client) SetPolicy(ctx context.Context, policySchema PolicySchema, updateMask []string, additionalTargetKeys interface{}, requestID string) error {
	schemaJSONString, err := policySchema.Schema2JSON(updateMask, additionalTargetKeys)
	if err != nil {
		return errors.Wrap(err, "failed to marshal policy schema")
	}

	request := &setPolicyRequest{
		PolicySchema: string(schemaJSONString),
		RequestID:    requestID,
	}

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Policies/setPolicy", setPolicyTimeout, 0, payloadBytes)
	if err != nil {
		return errors.Wrap(err, "failed to make REST call")
	}
	defer response.Body.Close()
	return nil
}

// CleanUpAccount cleans up an owned test account identified by a requestID.
func (c *client) CleanUpAccount(ctx context.Context, requestID string) error {
	payloadBytes, err := json.Marshal(requestID)
	if err != nil {
		return errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "OTA/cleanup", callTimeout, 0, payloadBytes)
	if err != nil {
		return errors.Wrap(err, "failed to make request")
	}
	defer response.Body.Close()

	// Make sure the request was successful.
	if response.StatusCode != 200 {
		return errors.Errorf("failed to cleanup account, status code: %d", response.StatusCode)
	}

	return nil
}

// storeDeprovisioningIDsRequest is a struct containing the necessary data to store the deprovisioning IDs in TAPE.
type storeDeprovisioningIDsRequest struct {
	DeviceID           string `json:"deviceid"`
	CustomerID         string `json:"customerid"`
	StableDeviceSecret string `json:"stabledevicesecret"`
}

// StoreDeprovisioningIDs stores the deviceID and customerID with the stableDeviceSecret as key in
// the database of TAPE.
func (c *client) StoreDeprovisioningIDs(ctx context.Context, deviceID, customerID, stableDeviceSecret string) error {

	request := &storeDeprovisioningIDsRequest{
		DeviceID:           deviceID,
		CustomerID:         customerID,
		StableDeviceSecret: stableDeviceSecret,
	}

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Devices/storeDeprovisioningIDs", callTimeout, 2, payloadBytes)
	if err != nil {
		return errors.Wrap(err, "failed to make REST call")
	}
	defer response.Body.Close()
	return nil
}

type deprovisionOption struct {
	DeviceID           string
	CustomerID         string
	StableDeviceSecret string
}

// DeprovisionOption provides options for deprovisioning a device.
type DeprovisionOption func(*deprovisionOption)

// WithDeviceAndCustomerID provides the option to deprovision by customerID.
func WithDeviceAndCustomerID(deviceID, customerID string) DeprovisionOption {
	return func(opt *deprovisionOption) {
		opt.DeviceID = deviceID
		opt.CustomerID = customerID
	}
}

// WithStableDeviceSecret provides the option to deprovision by StableDeviceSecret.
func WithStableDeviceSecret(stableDeviceSecret string) DeprovisionOption {
	return func(opt *deprovisionOption) {
		opt.StableDeviceSecret = stableDeviceSecret
	}
}

// deprovisionRequest is a struct containing the necessary data to deprovision a device.
// If StableDeviceSecret is provided TAPE will get the CustomerID and DeviceID from its
// database. Otherwise the CustomerID and the DeviceID have to be provided or the call
// will fail.
type deprovisionRequest struct {
	DeviceID           string `json:"deviceid"`
	CustomerID         string `json:"customerid"`
	StableDeviceSecret string `json:"stabledevicesecret"`
}

// Deprovision calls TAPE to deprovision a device in DPanel.
func (c *client) Deprovision(ctx context.Context, opt DeprovisionOption) error {
	options := deprovisionOption{}
	opt(&options)

	request := deprovisionRequest(options)

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Devices/deprovision", deprovisionTimeout, 2, payloadBytes)
	if err != nil {
		return errors.Wrap(err, "failed to make REST call")
	}
	defer response.Body.Close()
	return nil
}

// Deprovisioned calls TAPE to check if a device with a specific deviceID is
// provisioned.
func (c *client) Deprovisioned(ctx context.Context, opt DeprovisionOption) (bool, error) {
	status, err := c.Provisioned(ctx, opt)
	if err != nil {
		return false, errors.Wrap(err, "failed to get provision status")
	}
	return !status, nil
}

// Provisioned calls TAPE to check if a device with a specific deviceID is
// provisioned.
func (c *client) Provisioned(ctx context.Context, opt DeprovisionOption) (bool, error) {
	status, err := c.GetProvisionStatus(ctx, opt)
	if err != nil {
		return false, errors.Wrap(err, "failed to get provision status")
	}
	return strings.Contains(status, "Not deprovisioned"), nil
}

// GetProvisionStatus calls TAPE to check if a device with a specific deviceID is
// provisioned.
func (c *client) GetProvisionStatus(ctx context.Context, opt DeprovisionOption) (string, error) {
	options := deprovisionOption{}
	opt(&options)

	request := deprovisionRequest(options)

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Devices/isDeprovisioned", callTimeout, 2, payloadBytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to make REST call")
	}

	// Read the response.
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response")
	}
	return string(respBody), nil
}

// getDeviceInfoRequest is a struct containing the necessary data to list devices
// in an organizational unit.
type getDeviceInfoRequest struct {
	DeviceID   string `json:"deviceid"`
	CustomerID string `json:"customerid"`
}

// GetDeviceInfo calls TAPE to retrieve device information from DPanel. This information comes in
// the form of JSON fromatted string and contains information like "cpuInfo","deviceId","deviceLicenseType",
// "ethernetMacAddress","firmwareVersion","firstEnrollmentTime","lastDeprovisionTimestamp","lastEnrollmentTime",
// "lastKnownNetwork","lastSync","orgUnitId","orgUnitPath","osVersion","platformVersion","recentUsers",
// "serialNumber","status","tpmVersionInfo". For more information see:
// https://developers.google.com/admin-sdk/directory/v1/guides/manage-chrome-devices#get_chrome_device
func (c *client) GetDeviceInfo(ctx context.Context, deviceID, customerID string) (string, error) {
	request := &getDeviceInfoRequest{
		DeviceID:   deviceID,
		CustomerID: customerID,
	}

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Devices/getDevice", callTimeout, 2, payloadBytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to make REST call")
	}

	// Read the response.
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response")
	}
	return string(respBody), nil
}

// listDevicesRequest is a struct containing the necessary data to list devices
// in an organizational unit.
type listDevicesRequest struct {
	OrgUnitPath string `json:"orgunitpath"`
	CustomerID  string `json:"customerid"`
}

// ListDevices calls TAPE to retrieve a list of devices in the provided organizational unit
// corresponding to orgUnitPath.
func (c *client) ListDevices(ctx context.Context, orgUnitPath, customerID string) (string, error) {
	request := &listDevicesRequest{
		OrgUnitPath: orgUnitPath,
		CustomerID:  customerID,
	}

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Devices/listDevices", callTimeout, 2, payloadBytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to make REST call")
	}

	// Read the response.
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response")
	}
	return string(respBody), nil
}

// MoveDevicesToOURequest is a struct containing the necessary data to move a
// device to an an organizational unit.
type MoveDevicesToOURequest struct {
	DeviceIDs   []string `json:"deviceids"`
	CustomerID  string   `json:"customerid"`
	OrgUnitPath string   `json:"orgunitpath"`
}

// MoveDevicesToOU calls TAPE to move devices, identified by their deviceIDs to an
// organizational unit with the path orgUnitPath (e.g. "myOU/mySubOU").
func (c *client) MoveDevicesToOU(ctx context.Context, deviceIDs []string, orgUnitPath, customerID string) (string, error) {
	request := &MoveDevicesToOURequest{
		DeviceIDs:   deviceIDs,
		CustomerID:  customerID,
		OrgUnitPath: orgUnitPath,
	}

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Devices/moveDevicesToOU", callTimeout, 0, payloadBytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to make REST call")
	}

	// Read the response.
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response")
	}
	return string(respBody), nil
}

// MoveUserToOURequest is a struct containing the necessary data to move a
// user to an an organizational unit.
type MoveUserToOURequest struct {
	RequestID   string `json:"request_id"`
	OrgUnitPath string `json:"orgunitpath"`
}

// MoveUserToOU calls TAPE to move a user, identified by their requestID to an
// organizational unit with the path orgUnitPath (e.g. "/myOU/mySubOU"). Moved users will
// be moved back to their original OU when the account is released.
func (c *client) MoveUserToOU(ctx context.Context, requestID, orgUnitPath string) (string, error) {
	request := &MoveUserToOURequest{
		RequestID:   requestID,
		OrgUnitPath: orgUnitPath,
	}

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Users/moveUserToOU", callTimeout, 0, payloadBytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to make REST call")
	}

	// Read the response.
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response")
	}
	return string(respBody), nil
}

// CommandTypeEnum is an enum for different types of remote commands.
type CommandTypeEnum int

const (
	// CommandTypeReboot is the enum to reboot the device.
	CommandTypeReboot = 0
	// CommandTypeTakeAScreenshot is the enum to take a screenshot.
	CommandTypeTakeAScreenshot = 1
	// CommandTypeSetVolume is the enum to set the volume.
	CommandTypeSetVolume = 2
	// CommandTypeWipeUsers is the enum to wipe the users from a device.
	CommandTypeWipeUsers = 5
	// CommandTypeDeviceStartCRDSession is the enum to start a CRD session on the device.
	CommandTypeDeviceStartCRDSession = 6
	// CommandTypeRemotePowerwash is the enum to powerwash the device.
	CommandTypeRemotePowerwash = 7
	// CommandTypeCaptureLogs is the enum to capture the logs from the device.
	CommandTypeCaptureLogs = 9
	// CommandTypeFetchCrdAvailabilityInfo is the enum to fetch CRD availability from the device.
	CommandTypeFetchCrdAvailabilityInfo = 10
	// CommandTypeFetchSupportPacket is the enum to trigger remote log upload on the device.
	CommandTypeFetchSupportPacket = 11
)

// RemoteCommand is a struct describing a remote command used in IssueCommand.
type RemoteCommand struct {
	CommandType CommandTypeEnum `json:"commandType"`
	Payload     string          `json:"payload"`
}

// IssueCommandRequest is a struct containing the necessary data to issue a
// remote command.
type IssueCommandRequest struct {
	DeviceID      string        `json:"deviceid"`
	CustomerID    string        `json:"customerid"`
	RemoteCommand RemoteCommand `json:"command"`
}

// IssueCommandResponse is a struct containing the response from an IssueCommand call.
type IssueCommandResponse struct {
	CommandID string `json:"commandId"`
}

// IssueCommand calls TAPE to issue a remote command to a device, identified by their deviceID.
// For available remote commands see https://developers.google.com/admin-sdk/directory/reference/rest/v1/customer.devices.chromeos.commands#CommandType
func (c *client) IssueCommand(ctx context.Context, deviceID, customerID string, remoteCommand RemoteCommand) (*IssueCommandResponse, error) {
	request := &IssueCommandRequest{
		DeviceID:      deviceID,
		CustomerID:    customerID,
		RemoteCommand: remoteCommand,
	}

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal data")
	}

	response, err := c.sendRequestWithTimeout(ctx, "POST", "Devices/issueCommand", callTimeout, 0, payloadBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make REST call")
	}

	// Read the response.
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}

	var issueCommandResponse IssueCommandResponse
	err = json.Unmarshal([]byte(respBody), &issueCommandResponse)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal json response: %v", response)
	}

	return &issueCommandResponse, nil
}

// GetCommandRequest is a struct containing the necessary data to retrieve the
// status of a remote command.
type GetCommandRequest struct {
	DeviceID   string `json:"deviceid"`
	CustomerID string `json:"customerid"`
	CommandID  string `json:"commandid"`
}

// RemoteCommandResult is a struct containing the result of the execution of a
// remote command on a device.
type RemoteCommandResult struct {
	Result               string    `json:"result"`
	ExecuteTime          time.Time `json:"executeTime"`
	ErrorMessage         string    `json:"errorMessage"`
	CommandResultPayload string    `json:"commandResultPayload"`
}

// GetCommandResponse is a struct containing the response from a GetCommand call.
type GetCommandResponse struct {
	CommandID         string              `json:"commandId"`
	Type              string              `json:"type"`
	IssueTime         time.Time           `json:"issueTime"`
	State             string              `json:"state"`
	CommandExpireTime string              `json:"commandExpireTime"`
	CommandResult     RemoteCommandResult `json:"commandResult"`
	Payload           string              `json:"payload"`
}

// GetCommand calls TAPE to retrieve the status of a remote command identified by a commandID.
// The status will be returned in a GetCommandResponse struct.
func (c *client) GetCommand(ctx context.Context, deviceID, customerID, commandID string) (*GetCommandResponse, error) {
	request := &GetCommandRequest{
		DeviceID:   deviceID,
		CustomerID: customerID,
		CommandID:  commandID,
	}

	payloadBytes, err := json.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal data")
	}
	response, err := c.sendRequestWithTimeout(ctx, "POST", "Devices/getCommand", callTimeout, 0, payloadBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make REST call")
	}

	// Read the response.
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}

	var getCommandResponse GetCommandResponse
	err = json.Unmarshal([]byte(respBody), &getCommandResponse)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal json response: %v", response)
	}

	return &getCommandResponse, nil
}
