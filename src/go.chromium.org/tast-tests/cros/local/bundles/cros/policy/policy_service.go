// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	empb "go.chromium.org/chromiumos/policy/chromium/policy/enterprise_management_proto"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/media/vm"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/externaldata"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/syslog"
	ppb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			ppb.RegisterPolicyServiceServer(srv, &PolicyService{
				s:              s,
				extensionConns: make(map[string]*chrome.Conn),
				sharedObject:   common.SharedObjectsForServiceSingleton,
			})
		},
		// GuaranteeCompatibility allows tests outside ChromeOS to call this service.
		GuaranteeCompatibility: true,
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

// PolicyService implements tast.cros.policy.PolicyService.
type PolicyService struct { // NOLINT
	s *testing.ServiceState

	extensionConns map[string]*chrome.Conn
	extensionDirs  []string
	fakeDMS        *fakedms.FakeDMS
	fakeDMSDir     string
	fakeDMSRemoval bool
	chromeReader   *syslog.ChromeReader

	eds          *externaldata.Server
	sharedObject *common.SharedObjectsForService
}

func (c *PolicyService) VerifyPolicyStatus(ctx context.Context, req *ppb.VerifyPolicyStatusRequest) (*empty.Empty, error) {
	if c.sharedObject.Chrome == nil {
		return nil, errors.New("chrome is not started")
	}

	testing.ContextLog(ctx, "Verifying the policy is set to correct status")
	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create test API connection")
	}

	policies, err := policy.UnmarshalList(req.Policies)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal")
	}

	if err := policyutil.Verify(ctx, tconn, policies); err != nil {
		return nil, errors.Wrap(err, "failed to verify policies")
	}

	return &empty.Empty{}, nil
}

// StartNewChromeReader starts new syslog reader. When using this make sure to always call a function at the end
// to close the reader such as WaitForEnrollmentError.
func (c *PolicyService) StartNewChromeReader(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	chromeReader, err := syslog.NewChromeReader(ctx, syslog.ChromeLogFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start NewReader")
	}

	c.chromeReader = chromeReader
	return &empty.Empty{}, nil
}

// WaitForEnrollmentError checks for enrollment error in logs using syslog.
func (c *PolicyService) WaitForEnrollmentError(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {

	const (
		logScanTimeout     = 45 * time.Second
		enrollmentErrorLog = "Enrollment error"
	)

	testing.ContextLog(ctx, "Waiting for enrollment failure message")

	for {
		entry, err := c.chromeReader.Read()
		if err == io.EOF {
			break
		}
		testing.ContextLog(ctx, entry.Content)
		if strings.Contains(entry.Content, enrollmentErrorLog) {
			testing.ContextLog(ctx, "Error message encountered")
			defer c.chromeReader.Close()
			return &empty.Empty{}, nil
		}
	}
	defer c.chromeReader.Close()
	return &empty.Empty{}, errors.New("failed to find enrollment error message")
}

// StoreIDsForDeprovisioningAndLogErrors calls StoreIDsForDeprovisioning and logs returned errors.
func (c *PolicyService) StoreIDsForDeprovisioningAndLogErrors(ctx context.Context) {
	if err := c.StoreIDsForDeprovisioning(ctx); err != nil {
		testing.ContextLog(ctx, "StoreIDsForDeprovisioning failed: ", err)
	}
}

// StoreIDsForDeprovisioning stores the customerID and deviceID in TAPE to enable the deprovisioning of the device.
// This function should be deferred before any enrollment with real GAIA is performed. As enrollment can still fail after
// the provisioning it is important to also do this when enrollment fails.
func (c *PolicyService) StoreIDsForDeprovisioning(ctx context.Context) error {
	deviceAndCustomerIDResponse, err := c.DeviceAndCustomerID(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get device and customer id")
	}
	deviceID := deviceAndCustomerIDResponse.DeviceID
	customerID := deviceAndCustomerIDResponse.CustomerID

	// Get the stable_device_secret as the key under which we store the deviceID and customerID
	// as that won't change.
	const stableDeviceSecretFileName = "/sys/firmware/vpd/ro/stable_device_secret_DO_NOT_SHARE"
	data, err := ioutil.ReadFile(stableDeviceSecretFileName)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", stableDeviceSecretFileName)
	}
	stableDeviceSecret := string(data)

	tapeServiceAccount, ok := c.s.Var(tape.ServiceAccountVar)
	if !ok {
		return errors.New("missing tape.ServiceAccount variable")
	}

	tapeClient, err := tape.NewClient(ctx, []byte(tapeServiceAccount))
	if err != nil {
		return errors.Wrap(err, "failed to create TAPE client")
	}

	if err := tapeClient.StoreDeprovisioningIDs(ctx, deviceID, customerID, stableDeviceSecret); err != nil {
		return errors.Wrap(err, "failed to store the ids needed to deprovision in TAPE")
	}

	return nil
}

// GAIAEnrollAndLoginUsingChrome enrolls the device using dmserver. Specified user is logged in after this function completes.
func (c *PolicyService) GAIAEnrollAndLoginUsingChrome(ctx context.Context, req *ppb.GAIAEnrollAndLoginUsingChromeRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Enrolling using Chrome with username: %s, dmserver: %s", string(req.Username), string(req.DmserverURL))

	// Store the IDs we need for deprovisioning, as enrollment can fail after provisioning we need to defer this function before enrolling.
	defer c.StoreIDsForDeprovisioningAndLogErrors(ctx)

	if err := c.newChrome(
		ctx,
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.GAIALogin(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.DMSPolicy(req.DmserverURL),
	); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	return &empty.Empty{}, nil
}

// GAIAEnrollUsingChrome enrolls the device using dmserver.
func (c *PolicyService) GAIAEnrollUsingChrome(ctx context.Context, req *ppb.GAIAEnrollUsingChromeRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Enrolling using Chrome with username: %s, dmserver: %s", string(req.Username), string(req.DmserverURL))

	// Store the IDs we need for deprovisioning, as enrollment can fail after provisioning we need to defer this function before enrolling.
	defer c.StoreIDsForDeprovisioningAndLogErrors(ctx)

	if err := c.newChrome(
		ctx,
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.NoLogin(),
		chrome.DMSPolicy(req.DmserverURL),
	); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	return &empty.Empty{}, nil
}

// ZeroTouchEnrollUsingChrome enrolls the device using dmserver.
func (c *PolicyService) ZeroTouchEnrollUsingChrome(ctx context.Context, req *ppb.ZeroTouchEnrollUsingChromeRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Zero Touch Enrolling using Chrome with dmserver: %s", string(req.DmserverURL))

	// Store the IDs we need for deprovisioning, as enrollment can fail after provisioning we need to defer this function before enrolling.
	defer c.StoreIDsForDeprovisioningAndLogErrors(ctx)

	if err := c.newChrome(
		ctx,
		chrome.ZeroTouchEnroll(),
		chrome.KeepState(),
		chrome.NoLogin(),
		chrome.DMSPolicy(req.DmserverURL),
		chrome.LoadSigninProfileExtension(req.ManifestKey),
	); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	return &empty.Empty{}, nil
}

func (c *PolicyService) TokenBasedEnrollUsingChrome(ctx context.Context, req *ppb.TokenBasedEnrollUsingChromeRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Token-based enrolling using Chrome with DMServer: %s", req.DmserverURL)

	// Store the IDs we need for deprovisioning, as enrollment can fail after provisioning we need to defer this function before enrolling.
	defer c.StoreIDsForDeprovisioningAndLogErrors(ctx)

	if err := c.newChrome(
		ctx,
		chrome.TokenBasedEnroll(),
		chrome.KeepState(),
		chrome.NoLogin(),
		chrome.DMSPolicy(req.DmserverURL),
		chrome.LoadSigninProfileExtension(req.ManifestKey),
	); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	return &empty.Empty{}, nil
}

// GAIALoginForReporting logsa a managed user into an unmanaged ChromeOS device.
func (c *PolicyService) GAIALoginForReporting(ctx context.Context, req *ppb.GAIALoginForReportingRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Logging into unmanaged ChromeOS device for reporting with managed user: %s, dmserver: %s", string(req.Username), string(req.DmserverUrl))
	// Managed user login
	if err := c.newChrome(
		ctx,
		chrome.GAIALogin(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.DMSPolicy(req.DmserverUrl),
		chrome.EnableFeatures(req.EnabledFeatures),
		chrome.EncryptedReportingAddr(fmt.Sprintf("%v/record", req.ReportingServerUrl)),
		chrome.ExtraArgs(req.ExtraArgs),
		chrome.CustomLoginTimeout(chrome.EnrollmentAndLoginTimeout),
	); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	return &empty.Empty{}, nil

}

// GAIAEnrollForReporting enrolls the device. If `req.SkipLogin` is true, skips login, otherwise logs into the device as a managed user.
func (c *PolicyService) GAIAEnrollForReporting(ctx context.Context, req *ppb.GAIAEnrollForReportingRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Enrolling using Chrome for reporting with username: %s, dmserver: %s", string(req.Username), string(req.DmserverUrl))

	var loginOption chrome.Option
	if req.SkipLogin {
		loginOption = chrome.NoLogin()
	} else {
		loginOption = chrome.GAIALogin(chrome.Creds{User: req.Username, Pass: req.Password})
	}

	// Store the IDs we need for deprovisioning, as enrollment can fail after provisioning we need to defer this function before enrolling.
	defer c.StoreIDsForDeprovisioningAndLogErrors(ctx)

	if err := c.newChrome(
		ctx,
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: req.Username, Pass: req.Password}),
		loginOption,
		chrome.DMSPolicy(req.DmserverUrl),
		chrome.EnableFeatures(req.EnabledFeatures),
		chrome.EncryptedReportingAddr(fmt.Sprintf("%v/record", req.ReportingServerUrl)),
		chrome.ExtraArgs(req.ExtraArgs),
		chrome.CustomLoginTimeout(chrome.EnrollmentAndLoginTimeout),
	); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	return &empty.Empty{}, nil
}

// SAMLTestIdPEnrollUsingChrome enrolls the device using dmserver.
func (c *PolicyService) SAMLTestIdPEnrollUsingChrome(ctx context.Context, req *ppb.SAMLTestIdPEnrollUsingChromeRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Enrolling using Chrome with username: %s, dmserver: %s", string(req.Username), string(req.DmserverURL))

	if err := c.newChrome(
		ctx,
		chrome.SAMLTestIDPEnterpriseEnroll(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.NoLogin(),
		chrome.DMSPolicy(req.DmserverURL),
	); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	return &empty.Empty{}, nil
}

// EnrollUsingChrome starts a FakeDMS insstance that serves the provided policies and
// enrolls the device. Specified user is logged in after this function completes.
func (c *PolicyService) EnrollUsingChrome(ctx context.Context, req *ppb.EnrollUsingChromeRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Enrolling using Chrome with policy %s", string(req.PolicyJson))

	var opts []chrome.Option

	ok := false

	for _, extension := range req.Extensions {
		extDir, err := ioutil.TempDir("", "tast-extensions-")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create temp dir")
		}
		defer func(ctx context.Context) {
			if !ok {
				if err := os.RemoveAll(extDir); err != nil {
					testing.ContextLogf(ctx, "Failed to delete %s: %v", extDir, err)
				}
			}
		}(ctx)

		c.extensionDirs = append(c.extensionDirs, extDir)

		for _, file := range extension.Files {
			if err := os.WriteFile(filepath.Join(extDir, file.Name), file.Contents, 0644); err != nil {
				return nil, errors.Wrapf(err, "failed to write %s for %s", file.Name, extension.Id)
			}
		}

		if extID, err := chrome.ComputeExtensionID(extDir); err != nil {
			return nil, errors.Wrap(err, "failed to compute extension id")
		} else if extID != extension.Id {
			return nil, errors.Errorf("unexpected extension id: got %s; want %s", extID, extension.Id)
		}

		opts = append(opts, chrome.UnpackedExtension(extDir))
	}

	if req.FakedmsDir == "" {
		tmpdir, err := ioutil.TempDir("", "fdms-")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create temp dir")
		}
		c.fakeDMSDir = tmpdir
		c.fakeDMSRemoval = true
	} else {
		c.fakeDMSDir = req.FakedmsDir
		c.fakeDMSRemoval = false
	}

	defer func() {
		if !ok {
			if err := os.RemoveAll(c.fakeDMSDir); err != nil {
				testing.ContextLogf(ctx, "Failed to delete %s: %v", c.fakeDMSDir, err)
			}
			c.fakeDMSDir = ""
		}
	}()

	// fakedms.New starts a background process that outlives the current context.
	fdms, err := fakedms.New(c.s.ServiceContext(), c.fakeDMSDir) // NOLINT
	if err != nil {
		return nil, errors.Wrap(err, "failed to start FakeDMS")
	}
	c.fakeDMS = fdms
	defer func() {
		if !ok {
			c.fakeDMS.Stop(ctx)
			c.fakeDMS = nil
		}
	}()

	if err := fdms.WritePolicyBlobRaw(req.PolicyJson); err != nil {
		return nil, errors.Wrap(err, "failed to write policy blob")
	}

	user := req.Username
	if user == "" {
		user = "tast-user@managedchrome.com"
	}

	opts = append(opts, chrome.FakeEnterpriseEnroll(chrome.Creds{User: user, Pass: "test0000"}))
	if req.SkipLogin {
		opts = append(opts, chrome.NoLogin())
	} else {
		opts = append(opts, chrome.CustomLoginTimeout(chrome.EnrollmentAndLoginTimeout))
		opts = append(opts, chrome.FakeLogin(chrome.Creds{User: user, Pass: "test0000", GAIAID: "gaiaid"}))
	}

	opts = append(opts, chrome.DMSPolicy(fdms.URL))
	opts = append(opts, chrome.ExtraArgs(req.ExtraArgs))
	opts = append(opts, chrome.EnableLoginVerboseLogs())

	// Make sure we have a network connection.
	if !vm.IsRunningOnVM() {
		if err := ping.VerifyInternetConnectivity(ctx, 10*time.Second); err != nil {
			dumpfile := "network_dump_ping_" + time.Now().Format("030405000") + ".txt"

			if err := dumputil.DumpNetworkInfo(ctx, dumpfile); err != nil {
				testing.ContextLog(ctx, "Failed to dump network info after a ping expectation failure: ", err)
			}
			testing.ContextLog(ctx, "Ping expectation failed, current network info dumped into ", dumpfile)

			return nil, errors.Wrap(err, "pre login network connection tests failed")
		}
	}

	// Make sure chrome.New does not take too long.
	ctx, cancel := context.WithTimeout(ctx, chrome.EnrollmentAndLoginTimeout)
	defer cancel()

	if err := c.newChrome(ctx, opts...); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	ok = true

	return &empty.Empty{}, nil
}

// UpdatePolicies updates the policies served by FakeDMS and refreshes them in Chrome.
func (c *PolicyService) UpdatePolicies(ctx context.Context, req *ppb.UpdatePoliciesRequest) (*empty.Empty, error) {
	if c.fakeDMS == nil {
		return nil, errors.New("fake DMS server not started")
	}

	// Write policies
	if err := c.fakeDMS.WritePolicyBlobRaw(req.PolicyJson); err != nil {
		return nil, errors.Wrap(err, "failed to write policy blob")
	}

	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	// Refresh policies.
	if err := tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.refreshEnterprisePolicies)();`, nil); err != nil {
		return nil, errors.Wrap(err, "failed to refresh policies")
	}

	return &empty.Empty{}, nil
}

// CheckChromeAndFakeDMS verifies that Chrome and FakeDMS are still running and responsive.
func (c *PolicyService) CheckChromeAndFakeDMS(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if c.fakeDMS == nil {
		return nil, errors.New("fake DMS server not started")
	}

	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	// Check Chrome
	result := false
	if err := tconn.Eval(ctx, "true", &result); err != nil {
		return nil, errors.Wrap(err, "failed to interact with Chrome")
	}
	if !result {
		return nil, errors.New("eval 'true' returned false")
	}

	// Check FakeDMS
	if err := c.fakeDMS.Ping(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to ping FakeDMS")
	}

	return &empty.Empty{}, nil
}

// StopChromeAndFakeDMS stops Chrome and FakeDMS.
func (c *PolicyService) StopChromeAndFakeDMS(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	var lastErr error

	for id, conn := range c.extensionConns {
		if err := conn.Close(); err != nil {
			testing.ContextLogf(ctx, "Failed to close connection to extension %s", id)
			lastErr = errors.Wrapf(err, "failed to close connection to extension %s", id)
		}
	}
	c.extensionConns = make(map[string]*chrome.Conn)

	if c.fakeDMS != nil {
		c.fakeDMS.Stop(ctx)
		c.fakeDMS = nil
	}

	if c.fakeDMSRemoval {
		if err := os.RemoveAll(c.fakeDMSDir); err != nil {
			testing.ContextLog(ctx, "Failed to remove temporary directory: ", err)
			lastErr = errors.Wrap(err, "failed to remove temporary directory")
		}
	}

	if c.sharedObject.Chrome != nil {
		func() {
			c.sharedObject.ChromeMutex.Lock()
			defer c.sharedObject.ChromeMutex.Unlock()

			if err := c.sharedObject.Chrome.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close chrome: ", err)
				lastErr = errors.Wrap(err, "failed to close chrome")
			}
			c.sharedObject.Chrome = nil
		}()
	}

	for _, extDir := range c.extensionDirs {
		if err := os.RemoveAll(extDir); err != nil {
			testing.ContextLog(ctx, "Failed to remove extension directory: ", err)
			lastErr = errors.Wrap(err, "failed to remove extension directory")
		}
	}

	return &empty.Empty{}, lastErr
}

func (c *PolicyService) StartChrome(ctx context.Context, req *ppb.StartChromeRequest) (*empty.Empty, error) {
	testing.ContextLogf(ctx, "Starting Chrome with policy %s", string(req.PolicyJson))

	if c.sharedObject.Chrome != nil {
		return nil, errors.New("Chrome is already started")
	}

	var opts []chrome.Option

	if req.KeepEnrollment {
		opts = append(opts, chrome.KeepEnrollment())
	}

	user := req.Username
	if user == "" {
		user = "tast-user@managedchrome.com"
	}
	opts = append(opts, chrome.FakeLogin(chrome.Creds{User: user, Pass: "test0000", GAIAID: "gaiaid"}))

	if req.SkipLogin {
		opts = append(opts, chrome.NoLogin())
	} else if req.DeferLogin {
		opts = append(opts, chrome.DeferLogin())
	} else {
		opts = append(opts, chrome.CustomLoginTimeout(chrome.LoginTimeout))
	}

	if c.fakeDMS != nil {
		opts = append(opts, chrome.DMSPolicy(c.fakeDMS.URL))
		if err := c.fakeDMS.WritePolicyBlobRaw(req.PolicyJson); err != nil {
			return nil, errors.Wrap(err, "failed to write policy blob")
		}
	}
	opts = append(opts, chrome.EnableLoginVerboseLogs())

	if err := c.newChrome(ctx, opts...); err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}

	return &empty.Empty{}, nil
}

func (c *PolicyService) StopChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	var lastErr error

	if c.sharedObject.Chrome == nil {
		return nil, errors.New("no active Chrome instance")
	}

	c.sharedObject.ChromeMutex.Lock()
	defer c.sharedObject.ChromeMutex.Unlock()

	for id, conn := range c.extensionConns {
		if err := conn.Close(); err != nil {
			testing.ContextLogf(ctx, "Failed to close connection to extension %s", id)
			lastErr = errors.Wrapf(err, "failed to close connection to extension %s", id)
		}
	}
	c.extensionConns = make(map[string]*chrome.Conn)

	if err := c.sharedObject.Chrome.Close(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close Chrome: ", err)
		lastErr = errors.Wrap(err, "failed to close Chrome")
	}

	c.sharedObject.Chrome = nil

	return &empty.Empty{}, lastErr
}

func (c *PolicyService) ContinueLogin(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if c.sharedObject.Chrome == nil {
		return nil, errors.New("no active Chrome instance")
	}

	if err := c.sharedObject.Chrome.ContinueLogin(ctx); err != nil {
		return nil, errors.Wrap(err, "Chrome login failed")
	}

	return &empty.Empty{}, nil
}

// CreateFakeDMSDir creates a directory. It needs to be removed with RemoveFakeDMSDir.
func (c *PolicyService) CreateFakeDMSDir(ctx context.Context, req *ppb.CreateFakeDMSDirRequest) (*empty.Empty, error) {
	// Remove existing data.
	os.RemoveAll(req.Path)

	if err := os.Mkdir(req.Path, 0755); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to create FakeDMS directory")
	}

	return &empty.Empty{}, nil
}

// CreateTempFakeDMSDir creates a temporary directory under the provided path. It needs to be removed with RemoveFakeDMSDir.
func (c *PolicyService) CreateTempFakeDMSDir(ctx context.Context, req *ppb.CreateTempFakeDMSDirRequest) (*ppb.CreateTempFakeDMSDirResponse, error) {
	if err := os.MkdirAll(req.Path, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create temporary directory root")
	}

	dir, err := os.MkdirTemp(req.Path, "fakedms-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temporary FakeDMS directory")
	}

	return &ppb.CreateTempFakeDMSDirResponse{
		Path: dir,
	}, nil
}

// RemoveFakeDMSDir removes a directory created with CreateFakeDMSDir.
func (c *PolicyService) RemoveFakeDMSDir(ctx context.Context, req *ppb.RemoveFakeDMSDirRequest) (*empty.Empty, error) {
	if err := os.RemoveAll(req.Path); err != nil {
		return &empty.Empty{}, errors.Wrapf(err, "failed to remove %q", req.Path)
	}

	return &empty.Empty{}, nil
}

// StartExternalDataServer starts  an instance of externaldata.Server.
func (c *PolicyService) StartExternalDataServer(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if c.eds != nil {
		return nil, errors.New("URL Policy server already started")
	}

	eds, err := externaldata.NewServer(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start policy HTTP server")
	}

	c.eds = eds

	return &empty.Empty{}, nil
}

// ServePolicyData serves the provided data and returns the url and hash that need to be providied to the policy.
func (c *PolicyService) ServePolicyData(ctx context.Context, req *ppb.ServePolicyDataRequest) (*ppb.ServePolicyDataResponse, error) {
	if c.eds == nil {
		return nil, errors.New("URL Policy server not started")
	}

	url, hash := c.eds.ServePolicyData(req.Contents)

	return &ppb.ServePolicyDataResponse{
		Url:  url,
		Hash: hash,
	}, nil
}

// StopExternalDataServer stops the instance of externaldata.Server.
func (c *PolicyService) StopExternalDataServer(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if c.eds == nil {
		return nil, errors.New("URL Policy server not started")
	}

	if err := c.eds.Stop(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to stop URL Policy server")
	}
	c.eds = nil

	return &empty.Empty{}, nil
}

func (c *PolicyService) connToExtension(ctx context.Context, id string) (*chrome.Conn, error) {
	if val, ok := c.extensionConns[id]; ok {
		return val, nil
	}

	bgURL := chrome.ExtensionBackgroundPageURL(id)
	conn, err := c.sharedObject.Chrome.NewConnForTarget(ctx, chrome.MatchTargetURL(bgURL))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to extension at %s", bgURL)
	}

	c.extensionConns[id] = conn

	return conn, nil
}

func (c *PolicyService) EvalStatementInExtension(ctx context.Context, req *ppb.EvalInExtensionRequest) (*empty.Empty, error) {
	conn, err := c.connToExtension(ctx, req.ExtensionId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create connection to extension")
	}

	if err := conn.Eval(ctx, req.Expression, nil); err != nil {
		return nil, errors.Wrap(err, "failed to run javascript")
	}

	return &empty.Empty{}, nil
}

func (c *PolicyService) EvalInExtension(ctx context.Context, req *ppb.EvalInExtensionRequest) (*ppb.EvalInExtensionResponse, error) {
	conn, err := c.connToExtension(ctx, req.ExtensionId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create connection to extension")
	}

	var result json.RawMessage
	if err := conn.Eval(ctx, req.Expression, &result); err != nil {
		return nil, errors.Wrap(err, "failed to run javascript")
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode result")
	}

	return &ppb.EvalInExtensionResponse{
		Result: encoded,
	}, nil
}

func (c *PolicyService) VerifyVisibleNotification(ctx context.Context, req *ppb.VerifyVisibleNotificationRequest) (*empty.Empty, error) {
	if c.sharedObject.Chrome == nil {
		return nil, errors.New("chrome is not available")
	}
	if req.NotificationId == "" {
		return nil, errors.New("request has empty notification id")
	}

	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	testing.ContextLogf(ctx, "Waiting for notification with id %s", req.NotificationId)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		notifications, err := ash.Notifications(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get notifications"))
		}

		for _, notification := range notifications {
			if strings.Contains(notification.ID, req.NotificationId) {
				return nil
			}
		}
		return errors.New("failed to find notification")
	}, &testing.PollOptions{
		Timeout: 15 * time.Second, // Checks for notification once per second by default.
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to wait for %q notification", req.NotificationId)
	}

	return &empty.Empty{}, nil
}

func (c *PolicyService) EvalExpressionInChromeURL(ctx context.Context, req *ppb.EvalExpressionInChromeUrlRequest) (*empty.Empty, error) {
	if c.sharedObject.Chrome == nil {
		return nil, errors.New("chrome is not available")
	}
	if req.Url == "" {
		return nil, errors.New("request has empty URL")
	}
	if req.Expression == "" {
		return nil, errors.New("request has empty expression")
	}

	conn, err := c.sharedObject.Chrome.NewConn(ctx, req.Url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to %s", req.Url)
	}
	defer conn.Close()

	if err := conn.WaitForExprFailOnErr(ctx, req.Expression); err != nil {
		return nil, errors.Wrapf(err, "failed to evaluate expression on %s", req.Url)
	}

	return &empty.Empty{}, nil
}

func (c *PolicyService) ClientID(ctx context.Context, req *empty.Empty) (*ppb.ClientIdResponse, error) {
	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create session_manager binding")
	}

	p, err := session.RetrievePolicyData(ctx, sm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve settings")
	} else if p == nil {
		return nil, errors.New("client ID not found")
	}

	return &ppb.ClientIdResponse{ClientId: *p.DeviceId}, nil
}

func (c *PolicyService) DirectoryAPIID(ctx context.Context, req *empty.Empty) (*ppb.DirectoryAPIIDResponse, error) {
	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create session_manager binding")
	}

	p, err := session.RetrievePolicyData(ctx, sm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve settings")
	} else if p == nil {
		return nil, errors.New("client ID not found")
	}

	return &ppb.DirectoryAPIIDResponse{DirectoryAPIID: *p.DirectoryApiId}, nil
}

// DeviceAndCustomerID retrieves the device id and customer id needed for deprovisioning.
func (c *PolicyService) DeviceAndCustomerID(ctx context.Context, req *empty.Empty) (*ppb.DeviceAndCustomerIDResponse, error) {
	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create session_manager binding")
	}

	p, err := session.RetrievePolicyData(ctx, sm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve settings")
	} else if p == nil {
		return nil, errors.New("client ID not found")
	}

	return &ppb.DeviceAndCustomerIDResponse{DeviceID: *p.DirectoryApiId, CustomerID: *p.ObfuscatedCustomerId}, nil
}

// LockDevice locks the device's screen.
func (c *PolicyService) LockDevice(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	if err := quicksettings.LockScreen(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to lock the screen with quick settings")
	}

	return &empty.Empty{}, nil
}

// UnlockDeviceWithPassword tries to unlock the lock screen with the password provided as a param.
func (c *PolicyService) UnlockDeviceWithPassword(ctx context.Context, req *ppb.UnlockDeviceWithPasswordRequest) (*empty.Empty, error) {
	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	if err := lockscreen.WaitForPasswordField(ctx, tconn, req.Username, 10*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the password field")
	}
	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get virtual keyboard")
	}
	defer keyboard.Close(ctx)

	if err := lockscreen.EnterPassword(ctx, tconn, req.Username, req.Password, keyboard); err != nil {
		return nil, errors.Wrap(err, "failed to enter password")
	}

	return &empty.Empty{}, nil
}

// Logout performs a logout on the current user session.
func (c *PolicyService) Logout(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}
	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to sign out with quick settings")
	}

	return &empty.Empty{}, nil
}

func (c *PolicyService) newChrome(ctx context.Context, opts ...chrome.Option) error {
	c.sharedObject.ChromeMutex.Lock()
	defer c.sharedObject.ChromeMutex.Unlock()
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	c.sharedObject.Chrome = cr
	return nil
}

// SendRemoteCommand sends a remote command request to the fake_dmserver.
func (c *PolicyService) SendRemoteCommand(ctx context.Context, req *ppb.SendRemoteCommandRequest) (*ppb.SendRemoteCommandResponse, error) {
	testing.ContextLog(ctx, "Sending remote command request")

	if c.fakeDMS == nil {
		return nil, errors.New("fakedms is not started")
	}

	rcReq := &empb.SendRemoteCommandRequest{RemoteCommand: &empb.RemoteCommand{}}
	if err := proto.Unmarshal(req.RemoteCommand, rcReq.RemoteCommand); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal the RemoteCommand proto")
	}

	resp, err := c.fakeDMS.SendRemoteCommand(ctx, rcReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send the remote command request")
	}

	return &ppb.SendRemoteCommandResponse{CommandId: *resp.CommandId}, nil
}

// WaitRemoteCommandResult waits for the result of the command with the specified id.
func (c *PolicyService) WaitRemoteCommandResult(ctx context.Context, req *ppb.WaitRemoteCommandResultRequest) (*ppb.WaitRemoteCommandResultResponse, error) {
	testing.ContextLog(ctx, "Waiting for remote command result")

	if c.fakeDMS == nil {
		return nil, errors.New("fakedms is not started")
	}

	resp, err := c.fakeDMS.WaitRemoteCommandResult(ctx, &req.CommandId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for the remote command result")
	}

	result, err := proto.Marshal(resp.Result)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal the WaitRemoteCommandResult proto")
	}

	return &ppb.WaitRemoteCommandResultResponse{Result: result}, nil
}

// WaitRemoteCommandAcked waits for the result of the command with the specified id.
func (c *PolicyService) WaitRemoteCommandAcked(ctx context.Context, req *ppb.WaitRemoteCommandAckedRequest) (*empty.Empty, error) {
	testing.ContextLog(ctx, "Waiting for remote command to be acknowledged")

	if c.fakeDMS == nil {
		return nil, errors.New("fakedms is not started")
	}

	err := c.fakeDMS.WaitRemoteCommandAcked(ctx, &req.CommandId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for the remote command acknowledgement")
	}

	return &empty.Empty{}, nil
}

// RefreshRemoteCommands triggers remote_command request on the fake_dmserver to fetch and execute all the pending command sent.
func (c *PolicyService) RefreshRemoteCommands(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	// Refresh remote commands.
	if err := tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.refreshRemoteCommands)();`, nil); err != nil {
		return nil, errors.Wrap(err, "failed to refresh remote commands")
	}

	return &empty.Empty{}, nil
}

// FindAndClickRestartNowButton finds and clicks the Restart now button that shows up after triggering DEVICE_REBOOT remote command.
func (c *PolicyService) FindAndClickRestartNowButton(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	tconn, err := c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	ui := uiauto.New(tconn)
	restartButton := nodewith.Name("Restart now").Role(role.Button)
	if err := ui.WaitUntilExists(restartButton)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find restart now button")
	}
	// Clicking on the restart button will restart immediately and disconnect the DUT, so we should ignore the error.
	ui.LeftClick(restartButton)(ctx)

	return &empty.Empty{}, nil
}
