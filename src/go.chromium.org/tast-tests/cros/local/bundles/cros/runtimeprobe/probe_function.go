// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtimeprobe

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	rppb "go.chromium.org/chromiumos/system_api/runtime_probe_proto"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/runtimeprobe/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/runtimeprobe/fixture"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
)

// componentConstraint constraints allowed runtime_probe component categories.
type componentConstraint interface {
	*rppb.Battery | *rppb.Camera | *rppb.Edid | *rppb.InputDevice | *rppb.Memory | *rppb.Network | *rppb.Storage
	GetName() string
	GetInformation() *rppb.Information
}

// componentType represents runtime_probe component interface.
type componentType interface {
	GetName() string
	GetInformation() *rppb.Information
}

// Skip the known concurrent D-Bus call at boot.
const defaultTryCount = 2

// dbusCall invokes runtime_probe methods via D-Bus with given input protobuf
// |in|.  If the method called successfully, |out| will be set to the replied
// message and return without errors.  Otherwise an error will be returned.
func dbusCall(ctx context.Context, method string, in, out proto.Message) error {
	const (
		jobName       = "runtime_probe"
		dbusName      = "org.chromium.RuntimeProbe"
		dbusPath      = "/org/chromium/RuntimeProbe"
		dbusInterface = "org.chromium.RuntimeProbe"
	)
	var dbusMethod = dbusInterface + "." + method

	if err := upstart.EnsureJobRunning(ctx, jobName); err != nil {
		return errors.Wrap(err, "runtime probe is not running")
	}
	defer upstart.StopJob(ctx, jobName)

	conn, obj, err := dbusutil.ConnectPrivateWithAuth(ctx, sysutil.ChronosUID, dbusName, dbus.ObjectPath(dbusPath))
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := dbusutil.CallProtoMethod(ctx, obj, dbusMethod, in, out); err != nil {
		return errors.Wrapf(err, "failed to call method %s", dbusMethod)
	}
	return nil
}

// dbusCallWithRetry wraps dbusCall with retries which try to skip known error
// caused by concurrent D-Bus calls.
func dbusCallWithRetry(ctx context.Context, method string, in, out proto.Message, tryCount int) error {
	var err error
	for i := 0; i < tryCount; i++ {
		if err = dbusCall(ctx, method, in, out); err == nil {
			return nil
		}
	}
	return errors.Wrapf(err, "retry failed for %d times", tryCount)
}

// probe uses D-Bus call to get result from runtime_probe with given request.
// Currently only users chronos and debugd are allowed to call this D-Bus function.
func probe(ctx context.Context, request *rppb.ProbeRequest) (*rppb.ProbeResult, error) {
	result := &rppb.ProbeResult{}
	err := dbusCallWithRetry(ctx, "ProbeCategories", request, result, defaultTryCount)
	return result, err
}

// hostInfoLabels get host info labels for tast tests.  If the tast variable
// is not found or is an invalid JSON string, this function returns an
// error.
func hostInfoLabels(s *testing.State) ([]string, error) {
	labelsStr, ok := s.Var("autotest_host_info_labels")
	if !ok {
		return nil, errors.New("no labels")
	}

	var labels []string
	if err := json.Unmarshal([]byte(labelsStr), &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// modelName returns the model name from |labels|.
func modelName(labels []string) (string, error) {
	const modelPrefix = "model:"

	for _, label := range labels {
		if strings.HasPrefix(label, modelPrefix) {
			model := strings.TrimPrefix(label, modelPrefix)
			return model, nil
		}
	}
	return "", errors.New("no model label found")
}

var mainBranchRe = regexp.MustCompile(`^\d+\.0\.0$`)

// isMainBranch returns whether the test environment is on main branch images.
func isMainBranch() (bool, error) {
	lsbContent, err := lsbrelease.Load()
	if err != nil {
		return false, err
	}
	version, ok := lsbContent[lsbrelease.Version]
	if !ok {
		return false, errors.Errorf("failed to find %s in /etc/lsb-release", lsbrelease.Version)
	}
	return mainBranchRe.MatchString(version), nil
}

// categoryAliases returns the aliases of given category.  The category
// "video" is a legacy usage of "camera" category and both follow the
// {category}_{cid}_{qid} name policy.
func categoryAliases(category string) []string {
	if category == "camera" {
		return []string{"camera", "video"}
	}
	return []string{category}
}

// normalizeComponentName returns an normalized identifier of the component
// name which is performed the following conversions:
//  1. Lowercase the component name as the hwid_component cros labels are
//     lowercased by the infra.
//  2. Trim the "_{qid}" suffix in the component name and append a
//     fixed string "_{Any}" because tast tests do not care about the mutable
//     fields (e.g. firmware) which usually differ in qid but just make sure
//     hardware components are probed by probe configs.
func normalizeComponentName(model, category, compName string) string {
	compName = strings.ToLower(compName)
	aliases := categoryAliases(category)
	aliasPatterns := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		aliasPatterns = append(aliasPatterns, regexp.QuoteMeta(alias))
	}
	pattern := regexp.MustCompile("^(" + regexp.QuoteMeta(model) + "_" + "(?:" + strings.Join(aliasPatterns, "|") + ")" + `_\d+)_\d+(?:#.*)?$`)
	if matches := pattern.FindStringSubmatch(compName); len(matches) > 0 {
		return matches[1] + "_{Any}"
	}
	return compName
}

// collectKnownComponents uses D-Bus call to get known components with category
// |category|.
func collectKnownComponents(ctx context.Context, model, category string, tryCount int) (map[string]struct{}, error) {
	categoryValue, found := rppb.ProbeRequest_SupportCategory_value[category]
	if !found {
		return nil, errors.Errorf("invalid category %q", category)
	}
	request := rppb.GetKnownComponentsRequest{
		Category: rppb.ProbeRequest_SupportCategory(categoryValue),
	}
	result := rppb.GetKnownComponentsResult{}
	err := dbusCallWithRetry(ctx, "GetKnownComponents", &request, &result, tryCount)
	if err != nil {
		return nil, err
	}
	var components = make(map[string]struct{})
	for _, name := range result.GetComponentNames() {
		trimmedName := normalizeComponentName(model, category, name)
		components[trimmedName] = struct{}{}
	}
	return components, nil
}

// countComponents returns a counter counting each component label and
// normalizes their names.
func countComponents(labels []string, category, model string) map[string]int {
	count := make(map[string]int)
	// Filter labels with prefix "hwid_component:<component type>/" and trim them.
	for _, alias := range categoryAliases(category) {
		categoryPrefix := "hwid_component:" + alias + "/"
		for _, label := range labels {
			if !strings.HasPrefix(label, categoryPrefix) {
				continue
			}
			label := strings.TrimPrefix(label, categoryPrefix)
			key := normalizeComponentName(model, category, model+"_"+label)
			count[key]++
		}
	}
	return count
}

// messagesToComponents convert components from protobuf message type for
// runtime_probe to the general type "componentType" for Tast test.
func messagesToComponents[T componentConstraint](rppbComps []T) []componentType {
	var comps []componentType
	for _, rppbComp := range rppbComps {
		comps = append(comps, rppbComp)
	}
	return comps
}

func getComponentsByCategory(x *rppb.ProbeResult, category string) ([]componentType, error) {
	switch category {
	case "battery":
		return messagesToComponents(x.GetBattery()), nil
	case "camera":
		return messagesToComponents(x.GetCamera()), nil
	case "display_panel":
		return messagesToComponents(x.GetDisplayPanel()), nil
	case "stylus":
		return messagesToComponents(x.GetStylus()), nil
	case "touchpad":
		return messagesToComponents(x.GetTouchpad()), nil
	case "touchscreen":
		return messagesToComponents(x.GetTouchscreen()), nil
	case "dram":
		return messagesToComponents(x.GetDram()), nil
	case "cellular":
		return messagesToComponents(x.GetCellular()), nil
	case "ethernet":
		return messagesToComponents(x.GetEthernet()), nil
	case "wireless":
		return messagesToComponents(x.GetWireless()), nil
	case "storage":
		return messagesToComponents(x.GetStorage()), nil
	default:
		return nil, errors.Errorf("unknown category %s", category)
	}
}

// decreaseComponentCount decreases the count of given component by 1.  If the
// count of given component is decreased to 0, it will be removed from |count|.
// The first returned value will be false on failure.  The second returned
// value is the display name of |component|.
func decreaseComponentCount(count map[string]int, model, category string, component componentType) (bool, string) {
	name := component.GetName()
	info := component.GetInformation()
	if info != nil {
		if compGroup := info.GetCompGroup(); compGroup != "" {
			name = model + "_" + compGroup
		}
	}
	if name == "generic" {
		return false, name
	}
	trimmedName := normalizeComponentName(model, category, name)
	if _, exists := count[trimmedName]; !exists {
		return false, name
	}
	count[trimmedName]--
	if count[trimmedName] == 0 {
		delete(count, trimmedName)
	}
	return true, name
}

type probeFunctionTestParam struct {
	categories           []string
	allowExtraComponents bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeFunction,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that probe results are expected",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"clarkchung@google.com",
		},
		BugComponent: "b:606088",
		Attr:         []string{"group:racc", "racc_config_installed"},
		SoftwareDeps: []string{"racc"},
		VarDeps:      []string{"autotest_host_info_labels"},
		Params: []testing.Param{
			{
				Name:              "battery",
				ExtraHardwareDeps: common.ReleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"battery"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "battery_private",
				Fixture:           fixture.DecryptProbeConfig,
				ExtraHardwareDeps: common.UnreleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"battery"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "camera",
				ExtraHardwareDeps: common.ReleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"camera"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "camera_private",
				Fixture:           fixture.DecryptProbeConfig,
				ExtraHardwareDeps: common.UnreleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"camera"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "edid",
				ExtraHardwareDeps: common.ReleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"display_panel"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "edid_private",
				Fixture:           fixture.DecryptProbeConfig,
				ExtraHardwareDeps: common.UnreleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"display_panel"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "input_device",
				ExtraHardwareDeps: common.ReleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"stylus", "touchpad", "touchscreen"},
					allowExtraComponents: true,
				},
			}, {
				Name:              "input_device_private",
				Fixture:           fixture.DecryptProbeConfig,
				ExtraHardwareDeps: common.UnreleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"stylus", "touchpad", "touchscreen"},
					allowExtraComponents: true,
				},
			}, {
				Name:              "memory",
				ExtraHardwareDeps: common.ReleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"dram"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "memory_private",
				Fixture:           fixture.DecryptProbeConfig,
				ExtraHardwareDeps: common.UnreleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"dram"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "network",
				ExtraHardwareDeps: common.ReleasedDeviceDeps,
				ExtraSoftwareDeps: []string{"no_kernel_upstream"},
				Val: probeFunctionTestParam{
					categories:           []string{"cellular", "ethernet", "wireless"},
					allowExtraComponents: true,
				},
			}, {
				Name:              "network_private",
				Fixture:           fixture.DecryptProbeConfig,
				ExtraHardwareDeps: common.UnreleasedDeviceDeps,
				ExtraSoftwareDeps: []string{"no_kernel_upstream"},
				Val: probeFunctionTestParam{
					categories:           []string{"cellular", "ethernet", "wireless"},
					allowExtraComponents: true,
				},
			}, {
				Name:              "storage",
				ExtraHardwareDeps: common.ReleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"storage"},
					allowExtraComponents: false,
				},
			}, {
				Name:              "storage_private",
				Fixture:           fixture.DecryptProbeConfig,
				ExtraHardwareDeps: common.UnreleasedDeviceDeps,
				Val: probeFunctionTestParam{
					categories:           []string{"storage"},
					allowExtraComponents: false,
				},
			},
		},
	})
}

// ProbeFunction probes components with the param |categories| on a device
// using Runtime Probe D-Bus call and checks if the result matches the host
// info labels.
// If there is not a valid component label, the test will be skipped (pass).
// If a valid component in the host info labels is not probed, the test will
// fail.  If a probed component is not in the host info labels, the test will
// fail if the param |allowExtraComponents| is false.  Otherwise, the test
// will pass.
func ProbeFunction(ctx context.Context, s *testing.State) {
	param := s.Param().(probeFunctionTestParam)
	hostInfoLabels, err := hostInfoLabels(s)
	if err != nil {
		s.Fatal("hostInfoLabels failed: ", err)
	}

	model, err := modelName(hostInfoLabels)
	if err != nil {
		s.Fatal("modelName failed: ", err)
	}

	failOnUnknownComponentLabels, err := isMainBranch()
	if err != nil {
		s.Fatal("isMainBranch failed: ", err)
	}

	categoryToExpectedComponentCounts := make(map[string]map[string]int)
	var requestCategories []rppb.ProbeRequest_SupportCategory
	allKnownComponents := make(map[string]struct{})
	for _, category := range param.categories {
		categoryValue, found := rppb.ProbeRequest_SupportCategory_value[category]
		if !found {
			s.Fatalf("Invalid category %q", category)
		}

		count := countComponents(hostInfoLabels, category, model)
		// Waived components are defined in |waived_comp_categories| on the
		// HWID service:
		// platform/factory-private/config/hwid/service/appengine/configurations.yaml
		// If a component in the dut labels is not found in the probe config on
		// the DUT, countComponents will also ignore it.
		if len(count) == 0 {
			s.Logf("%q components are not found in host info labels. Skipped", category)
			continue
		}
		categoryToExpectedComponentCounts[category] = count

		knownComponents, err := collectKnownComponents(ctx, model, category, defaultTryCount)
		if err != nil {
			s.Fatal("collectKnownComponents failed: ", err)
		}
		// The probe config on the DUT doesn't have any components with
		// category |category|.
		if len(knownComponents) == 0 {
			s.Logf("%q components are not found in the probe config. Skipped", category)
			continue
		}
		for compName := range knownComponents {
			allKnownComponents[compName] = struct{}{}
		}
		requestCategories = append(requestCategories, rppb.ProbeRequest_SupportCategory(categoryValue))
	}

	request := &rppb.ProbeRequest{
		Categories: requestCategories,
	}
	result, err := probe(ctx, request)
	if err != nil {
		s.Fatal("probe failed: ", err)
	}

	for category, expectedCompCounts := range categoryToExpectedComponentCounts {
		probedComponents, err := getComponentsByCategory(result, category)
		var extraComponents []string
		if err != nil {
			s.Errorf("getComponentsByCategory %s failed: %v", category, err)
			continue
		}
		for _, component := range probedComponents {
			result, name := decreaseComponentCount(expectedCompCounts, model, category, component)
			s.Logf("Probed %s component: %s", category, name)
			if !result && name != "generic" {
				extraComponents = append(extraComponents, category+"/"+name)
			}
		}

		var unprobedComponents []string
		for name := range expectedCompCounts {
			// Only raise an error when a component exists in host
			// info labels but not RACC configs on ToT images.
			if _, found := allKnownComponents[name]; !found {
				if !failOnUnknownComponentLabels {
					s.Logf("Unknown %s component label: %s. Skipped", category, name)
					continue
				}
			}
			unprobedComponents = append(unprobedComponents, category+"/"+name)
		}
		if len(unprobedComponents) > 0 {
			sort.Strings(unprobedComponents)
			s.Fatalf("Some expected %s components are not probed: %v", category, unprobedComponents)
		}

		if len(extraComponents) > 0 {
			sort.Strings(extraComponents)
			if param.allowExtraComponents {
				s.Logf("Some extra %s components are probed: %v", category, extraComponents)
			} else {
				s.Fatalf("Some extra %s components are probed: %v", category, extraComponents)
			}
		}
	}
}
