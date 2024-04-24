// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reportingutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	grpc "google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/tape"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ManagedChromeCustomerIDPath is the path to the secret customer ID var for managedchrome.
const ManagedChromeCustomerIDPath = "policy.managedchrome_obfuscated_customer_id"

// EventsAPIKeyPath is the path to the secret api key var for the events API.
const EventsAPIKeyPath = "policy.events_api_key"

// ReportingServerURL is the URL to the autopush reporting server.
const ReportingServerURL = "https://autopush-chromereporting-pa.sandbox.googleapis.com/v1"

// ReportingDirectory is the directory on the device which contains directories in which records are stored.
const ReportingDirectory = "/var/spool/reporting"

// HeartbeatPriority is the priority with which heartbeat events are enqueued.
const HeartbeatPriority = "FastBatch"

// UpdatePolicy is used to identify which policies need to updated for a test.
type UpdatePolicy int

// EnableAll and DisableAll enable/disable all reporting.
// Custom enables reporting with an allowlist.
const (
	EnableAll UpdatePolicy = iota
	DisableAll
	Custom
)

// VerifyEventTypeCallback is passed to the PruneEvents function. If this function returns false for an event
// then PruneEvents will not include the event in the returned list.
type VerifyEventTypeCallback func(InputEvent) bool

// Interface for calling methods in tape.client.
type tapeClient interface {
	SetPolicy(context.Context, tape.PolicySchema, []string, interface{}, string) error
}

// SaveCrosboltEventCountMetric reports the number of events that were queried on a test.
func SaveCrosboltEventCountMetric(metricName string, count int, outDir string) error {
	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      metricName,
		Unit:      "events",
		Direction: perf.SmallerIsBetter,
		Multiple:  false,
	}, float64(count))
	return pv.Save(outDir)
}

// PruneEvents reduces the events response to only memory events after test began.
func PruneEvents(ctx context.Context, events []InputEvent, correctEventType VerifyEventTypeCallback) ([]InputEvent, error) {
	var prunedEvents []InputEvent
	for _, event := range events {
		if !correctEventType(event) {
			continue
		}
		prunedEvents = append(prunedEvents, event)
		j, err := json.Marshal(event)
		if err != nil {
			testing.ContextLog(ctx, "Reporting: Failed to marshall event: ", err)
			return []InputEvent{}, errors.Wrap(err, "failed to marshal event")
		}
		testing.ContextLog(ctx, "Reporting: Found a valid event ", string(j))
	}

	return prunedEvents, nil
}

// ChronoSortEvents sorts events in-place in the chronologically ascending order.
func ChronoSortEvents(events []InputEvent) error {
	sortErr := error(nil)
	getTime := func(e InputEvent) int64 {
		t, err := strconv.ParseInt(e.APIEvent.ReportingRecordEvent.Time, 0, 64)
		if err != nil {
			sortErr = err
		}
		return t
	}
	less := func(i, j int) bool {
		return getTime(events[i]) < getTime(events[j])
	}
	sort.Slice(events, less)
	return sortErr
}

// LookupEventsByRequestPath calls the Reporting API Server's ChromeReportingDebugService.LookupEvents
// endpoint to get a list of events received by the server based on the arguments in `requestPath`.
func LookupEventsByRequestPath(ctx context.Context, requestPath string, testStartTime time.Time) ([]InputEvent, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", requestPath, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to craft event query request to the Reporting Server")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to issue debug query request to the Reporting Server")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.Errorf("reporting server encountered an error with the event query %q %v %q", requestPath, resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the response body")
	}
	var resData InputEventsResponse
	if err := json.Unmarshal(resBody, &resData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response")
	}
	var filteredEvents []InputEvent
	for _, event := range resData.Event {
		us, err := strconv.ParseInt(event.APIEvent.ReportingRecordEvent.Time, 10, 64)
		if err != nil {
			return filteredEvents, errors.Wrap(err, "failed to parse int64 Spanner timestamp from event")
		}
		if time.UnixMicro(us).After(testStartTime) {
			filteredEvents = append(filteredEvents, event)
		} else {
			testing.ContextLogf(ctx, "Found event with timestamp before test start time. event timestamp = %s. test start time=%s", time.UnixMicro(us), testStartTime)
		}
	}
	testing.ContextLogf(ctx, "Reporting: Found %d events after querying the reporting server", len(filteredEvents))
	return filteredEvents, nil
}

// LookupEvents calls the Reporting API Server's ChromeReportingDebugService.LookupEvents
// endpoint to get a list of events received by the server from a managed device.
func LookupEvents(ctx context.Context, reportingServerURL, obfuscatedCustomerID, clientID, apiKey, destination string, testStartTime time.Time) ([]InputEvent, error) {
	reqPath := fmt.Sprintf("%v/test/events?key=%v&obfuscatedCustomerId=%v&deviceId=%v&destination=%v", reportingServerURL, apiKey, obfuscatedCustomerID, clientID, destination)
	testing.ContextLog(ctx, "Reporting: Querying the reporting server for device events")
	return LookupEventsByRequestPath(ctx, reqPath, testStartTime)
}

// LookupUserEvents calls the Reporting API Server's ChromeReportingDebugService.LookupEvents
// endpoint to get a list of events received by the server from a managed user.
func LookupUserEvents(ctx context.Context, reportingServerURL, obfuscatedCustomerID, apiKey, destination, userEmail string, testStartTime time.Time) ([]InputEvent, error) {
	reqPath := fmt.Sprintf("%v/test/events?key=%v&obfuscatedCustomerId=%v&destination=%v&userEmail=%v", reportingServerURL, apiKey, obfuscatedCustomerID, destination, userEmail)
	testing.ContextLog(ctx, "Reporting: Querying the reporting server for user events")
	return LookupEventsByRequestPath(ctx, reqPath, testStartTime)
}

// Deprovision deprovisions the DUT. This should be used after the test is over.
func Deprovision(ctx context.Context, cc grpc.ClientConnInterface, serviceAccountVar []byte) error {
	tapeClient, err := tape.NewClient(ctx, serviceAccountVar)
	if err != nil {
		return errors.Wrap(err, "failed to create tape client")
	}

	policyClient := pspb.NewPolicyServiceClient(cc)
	deviceAndCustomerIDResponse, err := policyClient.DeviceAndCustomerID(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get device and customer id")
	}
	deviceID := deviceAndCustomerIDResponse.DeviceID
	customerID := deviceAndCustomerIDResponse.CustomerID

	if err = tapeClient.Deprovision(ctx, tape.WithDeviceAndCustomerID(deviceID, customerID)); err != nil {
		return errors.Wrap(err, "failed to deprovision device")
	}
	return nil
}

// DisableUpdatingDeviceAttribute disallows updating device attribute. Can be used to disable Asset
// ID screen on enrollment.
func DisableUpdatingDeviceAttribute(ctx context.Context, client tapeClient, requestID string) error {
	assetPolicy := &tape.AllowPopulateAssetIdentifierUsers{
		AllowToUpdateDeviceAttribute: false,
	}

	if err := client.SetPolicy(ctx, assetPolicy, []string{"allowToUpdateDeviceAttribute"}, nil, requestID); err != nil {
		return errors.Wrap(err, "failed to disable the AllowToUpdateDeviceAttribute policy")
	}
	return nil
}

// SleepWithContextLog sleeps for the given number of minutes and log it to the context.
func SleepWithContextLog(ctx context.Context, minutes int) error {
	testing.ContextLogf(ctx,
		"Reporting: Waiting for %d minutes to check for reported telemetry", minutes)

	// GoBigSleepLint: Waiting for reported telemetry.
	if err := testing.Sleep(ctx, time.Duration(minutes)*time.Minute); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}

// SetTelemetryPolicies makes a call to the DMServer API to update the policies of a device.
func SetTelemetryPolicies(ctx context.Context, client tapeClient, requestID string, updatePolicy UpdatePolicy, allowlist []string, skipAssetIDScreen bool) error {
	// Make the device skip the asset id screen for enrollment.
	if skipAssetIDScreen {
		if err := DisableUpdatingDeviceAttribute(ctx, client, requestID); err != nil {
			return errors.Wrap(err, "failed to set the asset policy")
		}
	}

	// Enable or disable the policies depending on the param.
	var telemetryBehavior tape.ReportingTelemetryBehaviorEnum
	if updatePolicy == EnableAll {
		telemetryBehavior = tape.REPORTINGTELEMETRYBEHAVIORENUM_REPORTING_TELEMETRY_BEHAVIOR_ENUM_REPORTING_ENABLE_ALL
	} else if updatePolicy == DisableAll {
		telemetryBehavior = tape.REPORTINGTELEMETRYBEHAVIORENUM_REPORTING_TELEMETRY_BEHAVIOR_ENUM_REPORTING_DISABLE_ALL
	} else if updatePolicy == Custom && len(allowlist) == 0 {
		telemetryBehavior = tape.REPORTINGTELEMETRYBEHAVIORENUM_REPORTING_TELEMETRY_BEHAVIOR_ENUM_REPORTING_DISABLE_ALL
	} else if updatePolicy == Custom {
		telemetryBehavior = tape.REPORTINGTELEMETRYBEHAVIORENUM_REPORTING_TELEMETRY_BEHAVIOR_ENUM_REPORTING_CUSTOM_WITH_ALLOWLIST
	}

	policy := &tape.EnableGranularDeviceTelemetryReportingDevices{
		ReportingTelemetryBehavior:     telemetryBehavior,
		ReportTelemetryCustomAllowlist: allowlist,
	}

	// Change the UpdateMask depending on the changes needed.
	var updateMask []string
	if updatePolicy == Custom && len(allowlist) > 0 {
		updateMask = []string{"reportingTelemetryBehavior", "reportTelemetryCustomAllowlist"}
	} else {
		updateMask = []string{"reportingTelemetryBehavior"}
	}

	if err := client.SetPolicy(ctx, policy, updateMask, nil, requestID); err != nil {
		return errors.Wrap(err, "failed to set the policy")
	}
	return nil
}

// MissiveConfirmedAllRecords looks for all records in '/var/spool/reporting' with the given priority and returns an error if there are any unconfirmed records. Returns nil otherwise.
// TODO(b/315535216): This function requires the test to run as root user in order to work. Do not use until this is fixed.
func MissiveConfirmedAllRecords(ctx context.Context, priority string, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		// Record files have the format: /var/spool/reporting/<Priority>/P_<PRIORITY>.<generation_id>.<sequencing_id>
		recordFilePatternMatcher := path.Join(ReportingDirectory, priority, "P_*")
		// Find all record files.
		recordFilePaths, err := filepath.Glob(recordFilePatternMatcher)
		if err != nil {
			return err
		}
		if recordFilePaths == nil {
			// Missive creates empty record files to hold the next record. If no record files exist, then no records were sent.
			return errors.New("no records were sent from this device")
		}
		// Verify all record files have size 0 bytes. Record files that have size > 0 bytes have not been confirmed by the server.
		for _, filePath := range recordFilePaths {
			file, err := os.Stat(filePath)
			if err != nil {
				return errors.Wrapf(err, "failed to get filesize of record file %s", filePath)
			}
			// Get file size and verify it's zero, i.e. doesn't contain any record data, and doesn't have negative size due to some error.
			if file.Size() != 0 {
				return errors.Wrapf(err, "record file %s contains unsent records", filePath)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

// ReplaceCrashType replaces the crash type in a meta file with kernel. Only
// used by crash events tests.
func ReplaceCrashType(metaPath string) error {
	input, err := os.ReadFile(metaPath)
	if err != nil {
		return errors.Wrap(err, "failed to read file")
	}

	lines := strings.Split(string(input), "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "upload_var_collector=") {
			lines[i] = "upload_var_collector=kernel"
		}
	}
	output := strings.Join(lines, "\n")
	err = os.WriteFile(metaPath, []byte(output), 0000 /* perm, ignored */)
	if err != nil {
		return errors.Wrapf(err, "failed to write back to meta file %s", metaPath)
	}

	return nil
}
