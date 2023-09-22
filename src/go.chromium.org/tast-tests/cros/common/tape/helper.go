// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tape

import (
	"context"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	ts "go.chromium.org/tast-tests/cros/services/cros/tape"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// ServiceAccountVar holds the name of the variable which stores the service account credentials for TAPE.
const ServiceAccountVar = "tape.service_account_key"

type clientOption struct {
	client      *client
	credsJSON   []byte
	requestOpts []RequestAccountOption
}

// ClientOption provides options for getting a client for an account manager.
type ClientOption func(*clientOption)

// WithCredsJSON provides the option to get a client from service account credentials.
func WithCredsJSON(jsonData []byte) ClientOption {
	return func(opt *clientOption) {
		opt.credsJSON = jsonData
	}
}

// WithClient provides the option to use an existing client.
func WithClient(client *client) ClientOption {
	return func(opt *clientOption) {
		opt.client = client
	}
}

func getClient(ctx context.Context, opts ...ClientOption) (*client, error) {
	// Copy over all options.
	options := clientOption{}
	for _, opt := range opts {
		opt(&options)
	}

	var client *client
	var err error
	if options.client != nil {
		client = options.client
	} else if len(options.credsJSON) > 0 {
		client, err = NewClient(ctx, options.credsJSON)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create tape client")
		}
	} else {
		return nil, errors.New("One of tape.client or credsJSON must be set")
	}
	return client, nil
}

// GenericAccountManager holds the client and the generic accounts data.
type GenericAccountManager struct {
	client   *client
	Accounts []*GenericAccount
}

// NewGenericAccountManager leases a generic account, stores it in a GenericAccountManager struct and returns both. It requires
// a credsJSON byte array with the credentials of a service account to create a tape client for the GenericAccountManager.
func NewGenericAccountManager(ctx context.Context, credsJSON []byte, opts ...RequestAccountOption) (*GenericAccountManager, *GenericAccount, error) {
	client, err := NewClient(ctx, credsJSON)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create tape client")
	}

	return NewGenericAccountManagerFromClient(ctx, client, opts...)
}

// NewGenericAccountManagerFromClient leases a generic account, stores it in a GenericAccountManager struct and returns both. It requires
// a tape client to assign it to the GenericAccountManager.
func NewGenericAccountManagerFromClient(ctx context.Context, client *client, opts ...RequestAccountOption) (*GenericAccountManager, *GenericAccount, error) {
	manager := &GenericAccountManager{
		client: client,
	}

	account, err := manager.RequestAccount(ctx, opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to request account")
	}
	return manager, account, nil
}

// RequestAccount leases a generic account, stores it in the GenericAccountManager and returns it.
func (ah *GenericAccountManager) RequestAccount(ctx context.Context, opts ...RequestAccountOption) (*GenericAccount, error) {
	account, err := ah.client.RequestGenericAccount(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request owned test account")
	}

	ah.Accounts = append(ah.Accounts, account)

	return account, nil
}

// CleanUp releases all generic accounts that are stored in the GenericAccountManager.
func (ah *GenericAccountManager) CleanUp(ctx context.Context) error {
	var combinedErrors error
	for _, account := range ah.Accounts {
		err := ah.client.ReleaseGenericAccount(ctx, account)
		if err != nil {
			combinedErrors = errors.Wrap(combinedErrors, err.Error())
		}
	}

	if combinedErrors != nil {
		return errors.Wrap(combinedErrors, "failed to release some accounts")
	}

	return nil
}

// OwnedTestAccountManager holds the client and the Owned accounts data.
type OwnedTestAccountManager struct {
	client   *client
	Accounts []*OwnedTestAccount
}

// NewOwnedTestAccountManager leases an owned test account, stores it in an OwnedTestAccountManager struct and returns both. It requires
// a credsJSON byte array with the credentials of a service account to create a tape client for the OwnedTestAccountManager.
func NewOwnedTestAccountManager(ctx context.Context, credsJSON []byte, lock bool, opts ...RequestAccountOption) (*OwnedTestAccountManager, *OwnedTestAccount, error) {
	client, err := NewClient(ctx, credsJSON)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create tape client")
	}

	return NewOwnedTestAccountManagerFromClient(ctx, client, lock, opts...)
}

// NewOwnedTestAccountManagerFromClient leases an owned test account, stores it in an OwnedTestAccountManager struct and returns both.
// It requires a tape client to assign it to the OwnedTestAccountManager.
func NewOwnedTestAccountManagerFromClient(ctx context.Context, client *client, lock bool, opts ...RequestAccountOption) (*OwnedTestAccountManager, *OwnedTestAccount, error) {
	manager := &OwnedTestAccountManager{
		client: client,
	}

	account, err := manager.RequestAccount(ctx, lock, opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to request account")
	}
	return manager, account, nil
}

// RequestAccount leases an owned test account, stores it in the OwnedTestAccountManager and returns it.
func (ah *OwnedTestAccountManager) RequestAccount(ctx context.Context, lock bool, opts ...RequestAccountOption) (*OwnedTestAccount, error) {
	account, err := ah.client.RequestOwnedTestAccount(ctx, lock, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request owned test account")
	}

	ah.Accounts = append(ah.Accounts, account)

	return account, nil
}

// CleanUp releases all owned test accounts that are stored in the OwnedTestAccountManager.
func (ah *OwnedTestAccountManager) CleanUp(ctx context.Context) error {
	var combinedErrors error
	for _, account := range ah.Accounts {
		err := ah.client.ReleaseOwnedTestAccount(ctx, account)
		if err != nil {
			combinedErrors = errors.Wrap(combinedErrors, err.Error())
		}
	}

	if combinedErrors != nil {
		return errors.Wrap(combinedErrors, "failed to release some accounts")
	}

	return nil
}

// DeprovisionHelper is a helper function to deprovision a device in a managed domain.
func (c *client) DeprovisionHelper(ctx context.Context, rpcClient *rpc.Client, customerID, orgUnitPath string) error {
	tapeService := ts.NewServiceClient(rpcClient.Conn)
	// Get the device ID of the DUT to deprovision it at the end of the test.
	ids, err := tapeService.GetDeviceID(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get the customer and deviceID")
	}

	return c.DeprovisionAndVerify(ctx, WithDeviceAndCustomerID(ids.DeviceID, ids.CustomerID))
}

// DeprovisionAndVerify is a helper function to deprovision a device in a managed domain.
func (c *client) DeprovisionAndVerify(ctx context.Context, opt DeprovisionOption) error {

	err := c.Deprovision(ctx, opt)
	if err != nil {
		return errors.Wrap(err, "failed to deprovision device")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isDeprovisioned, err := c.Deprovisioned(ctx, opt)
		if err != nil {
			return err
		}
		if !isDeprovisioned {
			return errors.New("device was not deprovisioned yet")
		}
		return nil
	}, &testing.PollOptions{
		Interval: 10 * time.Second,
	}); err != nil {
		return err
	}
	return nil
}

// MoveDeviceToOU is a helper function to move a device to an OU.
func (c *client) MoveDeviceToOU(ctx context.Context, rpcClient *rpc.Client, orgUnitPath string) error {
	tapeService := ts.NewServiceClient(rpcClient.Conn)
	// Get the device ID of the DUT.
	ids, err := tapeService.GetDeviceID(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get the deviceID")
	}
	_, err = c.MoveDevicesToOU(ctx, []string{ids.DeviceID}, orgUnitPath, ids.CustomerID)
	if err != nil {
		return errors.Wrapf(err, "failed to move device %s to %s", ids.DeviceID, orgUnitPath)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isDeprovisioned, err := c.Deprovisioned(ctx, WithDeviceAndCustomerID(ids.DeviceID, ids.CustomerID))
		if err != nil {
			return err
		}
		if isDeprovisioned {
			return errors.New("device not found in OU")
		}
		return nil
	}, &testing.PollOptions{
		Interval: 5 * time.Second,
	}); err != nil {
		return err
	}

	return nil
}

// GetDeviceIDHelper retrieves the device id from the /var/lib/devicesettings/policy.1 file.
func GetDeviceIDHelper(ctx context.Context) (deviceID, customerID string, retErr error) {
	const deviceSettingsFileName = "/var/lib/devicesettings/policy.1"

	data, err := ioutil.ReadFile(deviceSettingsFileName)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to read %s", deviceSettingsFileName)
	}
	deviceSettings := strings.ToValidUTF8(string(data), "")

	// The deviceID is prefixed with $ and has the format xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx and is separated by a
	// SOH(x01) character and a tab from the customerID which has the format Cxxxxxxxx.
	r, err := regexp.Compile("\\$([a-z0-9]){8}-([a-z0-9]){4}-([a-z0-9]){4}-([a-z0-9]){4}-([a-z0-9]){12}\x01\tC([a-z]|[0-9]){8}")
	if err != nil {
		return "", "", errors.Wrap(err, "failed to compile regular expression")
	}

	deviceAndCustomerID := strings.Split(r.FindString(deviceSettings)[1:], "\x01\t")
	if len(deviceAndCustomerID) != 2 {
		return "", "", errors.New("failed to find device and customerID in devicesettings")
	}

	return deviceAndCustomerID[0], deviceAndCustomerID[1], nil
}

// Additional target keys for policies.

// NetworkKey is an additionalTargetKey for network related policies. It takes
// a network_id to identify the network that is being modified with the policy.
type NetworkKey struct {
	NetworkID string `json:"network_id"`
}

// AppKey is an additionalTargetKey for ArcPolicy related policies.
type AppKey struct {
	AppID string `json:"app_id"`
}
