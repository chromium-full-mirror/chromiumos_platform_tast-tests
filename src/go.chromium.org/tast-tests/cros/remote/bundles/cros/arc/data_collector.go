// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/arc/cache"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/arc/dututils"
	"go.chromium.org/tast-tests/cros/services/cros/arc"
	arcpb "go.chromium.org/tast-tests/cros/services/cros/arc"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParam struct {
	vmEnabled bool
	// Android package name.
	androidPackage string
	// if set, collected data will be upload to the cloud.
	upload bool
	// if set, packages reference would be uploaded to the cloud.
	// This is required for T+ builds.
	uploadPackagesReference bool
	// if set, this verifies others uploads and creates pin to version if needed.
	uprevBranch bool
	// set of CPU ABIs required for uprev to pin to the next version. If caches for
	// some ABIs missing uprev are skipped.
	requiredCPUAbisForBranchUprev []string
	// if set, keep local data in this directory.
	dataDir string
	// if set, generates dex opt cache
	dexOptCacheGen bool
	// if set, copy non-ureadahead caches temporarily in this directory.
	tmpCachesDir string
}

const (
	// Name of gsutil
	gsUtil = "gsutil"

	// Base path for uploaded resources.
	runtimeArtifactsRoot = "gs://chromeos-arc-images/runtime_artifacts"

	// TTS cache bucket
	ttsCache = "tts_cache"

	// DexOpt cache bucket
	dexOptCache = "dex_opt_cache"

	// Temporary directory to store copy of non-ureadahead cache artifacts prior
	// to running genUreadaheadPack.
	tmpContainerCacheArtifactsRoot = "/mnt/stateful_partition/unencrypted/apkcache/data_collector"
	tmpVMCacheArtifactsRoot        = "/var/run/arcvm/testharness/data_collector"
)

type dataUploader struct {
	ctx             context.Context
	androidPackage  string
	androidVersion  string
	shouldUpload    bool
	buildDescriptor *dututils.BuildDescriptor
}

func (du *dataUploader) needUpload(bucket string) bool {
	if !du.shouldUpload {
		testing.ContextLog(du.ctx, "Cloud upload is disabled")
		return false
	}

	if !du.buildDescriptor.Official {
		testing.ContextLogf(du.ctx, "Version: %s is not official version and generated caches won't be uploaded to the server", du.androidVersion)
		return false
	}

	gsURL := du.remoteURL(bucket, du.androidPackage, du.androidVersion)
	// gsutil stat would return 1 for a non-existent object.
	if err := exec.Command(gsUtil, "stat", gsURL).Run(); err != nil {
		return true
	}

	// This test is scheduled to run once per build id and ARCH. So race should never happen.
	testing.ContextLogf(du.ctx, "%q exists and won't be uploaded to the server", gsURL)
	return false
}

func (du *dataUploader) remoteURL(bucket, androidPackage, androidVersion string) string {
	return fmt.Sprintf("%s/%s/%s_%s.tar", runtimeArtifactsRoot, androidPackage, bucket, androidVersion)
}

func (du *dataUploader) upload(src, bucket string) error {
	gsURL := du.remoteURL(bucket, du.androidPackage, du.androidVersion)

	// Use gsutil command to upload the file to the server.
	testing.ContextLogf(du.ctx, "Uploading %q to the server", gsURL)
	if out, err := exec.Command(gsUtil, "copy", src, gsURL).CombinedOutput(); err != nil {
		return errors.Wrapf(err, "failed to upload %q to the server %q", src, out)
	}

	testing.ContextLogf(du.ctx, "Set read permission for  %q", gsURL)

	// AllUsers read access is considered safe for two reasons.
	// First, this data is included into the image unmodified.
	// Second, we already practice setting this permission for other Android build
	// artifacts. For example from APPS bucket.
	if out, err := exec.Command(gsUtil, "acl", "ch", "-u", "AllUsers:READ", gsURL).CombinedOutput(); err != nil {
		return errors.Wrapf(err, "failed to set read permission for %q to the server %q", gsURL, out)
	}

	return nil
}

func (du *dataUploader) uploadIfNeeded(src, bucket string) error {
	if du.needUpload(bucket) {
		return du.upload(src, bucket)
	}
	return nil
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataCollector,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Signs in to DUT and performs ARC++ boot with various paramters. Captures required data and uploads it to Chrome binary server. This data is used by various tools. Normally, this test should be run during the Android PFQ, once per build/arch",
		Contacts: []string{
			"arc-performance@google.com",
			"khmel@chromium.org", // Original author.
			"alanding@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		SoftwareDeps: []string{"arc_android_data_cros_access", "chrome", "chrome_internal"},
		ServiceDeps: []string{"tast.cros.arc.UreadaheadPackService",
			"tast.cros.arc.GmsCoreCacheService", "tast.cros.arc.TTSCacheService", "tast.cros.arc.DexOptCacheService"},
		Timeout: 50 * time.Minute,
		// Note that arc.DataCollector is not a simple test. It collects data used to
		// produce test and release images. Not collecting this data leads to performance
		// regression and failure of other tests. Please consider fixing the issue rather
		// then disabling this in Android PFQ. At this time missing the data is allowed
		// for the grace period however it will be a build stopper after.
		Params: []testing.Param{{
			ExtraAttr:         []string{"group:arc-data-collector"},
			ExtraSoftwareDeps: []string{"android_p"},
			Val: testParam{
				vmEnabled:               false,
				androidPackage:          "android-container-pi",
				upload:                  true,
				uploadPackagesReference: false,
				uprevBranch:             false,
				dexOptCacheGen:          false,
				dataDir:                 "",
				tmpCachesDir:            "",
			},
		}, {
			Name:              "container_r",
			ExtraAttr:         []string{"group:arc-data-collector"},
			ExtraSoftwareDeps: []string{"android_container_r"},
			Val: testParam{
				vmEnabled:               false,
				androidPackage:          "android-container-rvc",
				upload:                  true,
				uploadPackagesReference: false,
				uprevBranch:             false,
				dexOptCacheGen:          false,
				dataDir:                 "",
				tmpCachesDir:            tmpContainerCacheArtifactsRoot,
			},
		}, {
			Name:              "vm_r",
			ExtraAttr:         []string{"group:arc-data-collector"},
			ExtraSoftwareDeps: []string{"android_vm_r"},
			Val: testParam{
				vmEnabled:               true,
				androidPackage:          "android-vm-rvc",
				upload:                  true,
				uploadPackagesReference: false,
				uprevBranch:             false,
				dexOptCacheGen:          false,
				dataDir:                 "",
				tmpCachesDir:            "",
			},
		}, {
			Name:              "vm_t",
			ExtraAttr:         []string{"group:arc-data-collector"},
			ExtraSoftwareDeps: []string{"android_vm_t"},
			Val: testParam{
				vmEnabled:               true,
				androidPackage:          "android-vm-tm",
				upload:                  true,
				uploadPackagesReference: true,
				uprevBranch:             false,
				dexOptCacheGen:          true,
				dataDir:                 "",
				tmpCachesDir:            tmpVMCacheArtifactsRoot,
			},
		}, {
			Name:              "local",
			ExtraSoftwareDeps: []string{"android_p"},
			Val: testParam{
				vmEnabled:               false,
				androidPackage:          "android-container-pi",
				upload:                  false,
				uploadPackagesReference: false,
				uprevBranch:             false,
				dexOptCacheGen:          false,
				dataDir:                 "/tmp/data_collector",
				tmpCachesDir:            "",
			},
		}, {
			Name:              "container_r_local",
			ExtraSoftwareDeps: []string{"android_container_r"},
			Val: testParam{
				vmEnabled:               false,
				androidPackage:          "android-container-rvc",
				upload:                  false,
				uploadPackagesReference: false,
				uprevBranch:             false,
				dexOptCacheGen:          false,
				dataDir:                 "/tmp/data_collector",
				tmpCachesDir:            tmpContainerCacheArtifactsRoot,
			},
		}, {
			Name:              "vm_r_local",
			ExtraSoftwareDeps: []string{"android_vm_r"},
			Val: testParam{
				vmEnabled:               true,
				androidPackage:          "android-vm-rvc",
				upload:                  false,
				uploadPackagesReference: false,
				uprevBranch:             false,
				dexOptCacheGen:          false,
				dataDir:                 "/tmp/data_collector",
				tmpCachesDir:            "",
			},
		}, {
			Name:              "vm_t_local",
			ExtraSoftwareDeps: []string{"android_vm_t"},
			Val: testParam{
				vmEnabled:               true,
				androidPackage:          "android-vm-tm",
				upload:                  false,
				uploadPackagesReference: true,
				uprevBranch:             false,
				dexOptCacheGen:          true,
				dataDir:                 "/tmp/data_collector",
				tmpCachesDir:            tmpVMCacheArtifactsRoot,
			},
		}, {
			// branch_uprev versions are designed to provide caches uprev functionality
			// on release branches. For the main branch, uprev is done automatically by
			// passing PFQ where data collector is scheduled for execution. Release
			// branches don't have PFQ running and these configurations provide a
			// workaround. This passes DataCollector as usual and as a result caches
			// for the particular version are uploaded. However, this itself does not
			// bring caches to the official build once this is generated post-factum.
			// Instead we use here pin caches functionality to force using caches for
			// particular version at specific branch. This should not be the problem
			// for the release branch once it has only minor changes. As a result, for
			// release branch builds, the most recent version of caches would be used.
			// Note, pin does not distinguish CPU ABI caches os uprev happens only in
			// case all possible CPU ABI caches are generated.
			// Limit the run for several key models only once caches are model
			// agnostic.
			// Follow the policy 2+ models per ARCH of different boards.
			// 8GB+ if possible for ureadahead generation.
			Name:              "branch_uprev",
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraSoftwareDeps: []string{"android_p"},
			// x86-64 ARC: pyro(reef-Intel), sand(reef-Intel), snappy(reef-Intel), careena(grunt-AMD), treeya(grunt-AMD)
			// arm64 ARC: elm(elm) - 1 model exception with all other devices either reached AUE or migrated to R.
			// TODO(b/293160140): Remove grunt models once they fully migrate to R-Container.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("pyro", "sand", "snappy", "elm", "careena", "treeya")),
			Val: testParam{
				vmEnabled:                     false,
				androidPackage:                "android-container-pi",
				upload:                        true,
				uploadPackagesReference:       false,
				uprevBranch:                   true,
				dexOptCacheGen:                false,
				requiredCPUAbisForBranchUprev: []string{"x86_64-houdini", "x86_64-ndk", "arm64-native"},
				dataDir:                       "/tmp/data_collector",
				tmpCachesDir:                  "",
			},
		}, {
			Name:              "container_r_branch_uprev",
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraSoftwareDeps: []string{"android_container_r"},
			// x86-64 ARC: careena(grunt-AMD), treeya(grunt-AMD)
			ExtraHardwareDeps: hwdep.D(hwdep.Model("treeya", "careena")),
			Val: testParam{
				vmEnabled:                     false,
				androidPackage:                "android-container-rvc",
				upload:                        true,
				uploadPackagesReference:       false,
				uprevBranch:                   true,
				dexOptCacheGen:                false,
				requiredCPUAbisForBranchUprev: []string{"x86_64-ndk"},
				dataDir:                       "/tmp/data_collector",
				tmpCachesDir:                  "",
			},
		}, {
			Name:              "vm_r_branch_uprev",
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraSoftwareDeps: []string{"android_vm_r"},
			// x86-64 ARC: kohaku(hatch-Intel), eve(eve-Intel), gimble(brya-Intel), dewatt(guybrush-AMD), nipperkin(guybrush-AMD)
			// arm64 ARC: hoglin(herobrine), steelix(corsola), krane(kukui)
			// TODO(b/293665738): Update ARM models since hoglin is no longer scheduled and krane is scarcely scheduled.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("kohaku", "eve", "gimble", "dewatt", "nipperkin", "steelix", "hoglin", "krane")),
			Val: testParam{
				vmEnabled:                     true,
				androidPackage:                "android-vm-rvc",
				upload:                        true,
				uploadPackagesReference:       false,
				uprevBranch:                   true,
				dexOptCacheGen:                false,
				requiredCPUAbisForBranchUprev: []string{"x86_64-houdini", "x86_64-ndk", "arm64-native"},
				dataDir:                       "/tmp/data_collector",
				tmpCachesDir:                  "",
			},
		}, {
			Name:              "vm_t_branch_uprev",
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraSoftwareDeps: []string{"android_vm_t"},
			// x86-64 ARC: kohaku(hatch-Intel), jinlon(hatch-Intel)
			ExtraHardwareDeps: hwdep.D(hwdep.Model("kohaku", "jinlon")),
			Val: testParam{
				vmEnabled:                     true,
				androidPackage:                "android-vm-tm",
				upload:                        true,
				uploadPackagesReference:       true,
				uprevBranch:                   true,
				dexOptCacheGen:                true,
				requiredCPUAbisForBranchUprev: []string{"x86_64-houdini"},
				dataDir:                       "/tmp/data_collector",
				tmpCachesDir:                  "",
			},
		}},
		VarDeps: []string{"arc.perfAccountPool"},
		// Runtime variables that could be useful for debugging issues with a specific type of
		// data being collected. For example, to debug a consistent failure in DexOpt cache in T,
		// the test can be run with DexOpt cache generation only with no retry: 'tast run
		// -var=arc.DataCollector.skipUreadahead=true -var=arc.DataCollector.skipGmscore=true
		// -var=arc.DataCollector.skipTts=true -var=arc.DataCollector.retryCount=0 $DUT
		// arc.DataCollector.vm_t_local'
		Vars: []string{
			"arc.DataCollector.skipUreadahead",
			"arc.DataCollector.skipGmscore",
			"arc.DataCollector.skipTts",
			"arc.DataCollector.skipDexopt",
			"arc.DataCollector.retryCount",
		},
	})
}

// DataCollector performs ARC++ boots in various conditions, grabs required data and uploads it to
// the binary server.
func DataCollector(ctx context.Context, s *testing.State) {
	const (
		// Packages reference bucket
		packagesReference = "packages_reference"

		// ureadahed packs bucket
		ureadaheadPack = "ureadahead_pack"

		// GMS Core caches bucket
		gmsCoreCache = "gms_core_cache"

		// Name of the pack in case of initial boot.
		initialPack = "initial_pack"

		// Name of the log for pack in case of initial boot.
		initialPackLog = "initial_pack.log"

		// Name of the pack in case of initial boot inside VM.
		vmInitialPack = "vm_initial_pack"

		// Name of the log for pack in case of initial boot inside VM.
		vmInitialPackLog = "vm_initial_pack.log"

		// Number of retries for each flow in case of failure.
		// Please see b/167697547, b/181832600 for more information. Retries are
		// needed for occasional OptIn instability on ARC development builds. Only
		// lower count if for sure OptIn is completely stable.
		defaultRetryCount = 2
	)

	d := s.DUT()

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	param := s.Param().(testParam)

	desc, err := dututils.GetBuildDescriptorRemotely(ctx, d, param.vmEnabled)
	if err != nil {
		s.Fatal("Failed to get ARC build desc: ", err)
	}

	v := fmt.Sprintf("%s_%s_%s", desc.CPUAbi, desc.BuildType, desc.BuildID)
	vUreadahead := fmt.Sprintf("host_%s_%s_%s_%s", desc.HostUreadaheadAbi, desc.BinaryTranslationType, desc.BuildType, desc.BuildID)
	s.Logf("Detected version %s(host %s_%s)", v, desc.HostUreadaheadAbi, desc.BinaryTranslationType)
	if desc.BuildType != "user" {
		s.Fatal("Data collector should only be run on a user build")
	}

	dataDir := param.dataDir
	// If data dir is not provided, use temp folder and remove after use.
	if dataDir == "" {
		dataDir, err = ioutil.TempDir("", "data_collector")
		if err != nil {
			s.Fatal("Failed to create temp dir: ", err)
		}
		os.Chmod(dataDir, 0744)
		defer os.RemoveAll(dataDir)
	} else {
		// Clean up before use.
		os.RemoveAll(dataDir)
		err := os.Mkdir(dataDir, 0744)
		if err != nil {
			s.Fatal("Failed to create local dir: ", err)
		}
	}

	du := dataUploader{
		ctx:             ctx,
		androidPackage:  param.androidPackage,
		androidVersion:  v,
		shouldUpload:    param.upload,
		buildDescriptor: desc,
	}
	duUreadahead := dataUploader{
		ctx:             ctx,
		androidPackage:  param.androidPackage,
		androidVersion:  vUreadahead,
		shouldUpload:    param.upload,
		buildDescriptor: desc,
	}

	// Create temp caches directory before starting generation.
	tmpCachesDir := param.tmpCachesDir
	// TODO(b/279554423): Eventually enable this for container-rvc, vm-rvc, pi-arc,
	// devices after initial experiments are conducted on local and vm-tm configs.
	useDevCaches := (tmpCachesDir != "" && (!param.upload || param.androidPackage == "android-vm-tm"))
	if useDevCaches {
		if err := dututils.MkdirRemote(ctx, d, tmpCachesDir); err != nil {
			s.Fatalf("Failed to create temp cache dir %q:  %v", tmpCachesDir, err)
		}
	}
	defer dututils.RemoveAllRemote(ctx, d, tmpCachesDir)

	genUreadaheadPack := func() (retErr error) {
		service := arc.NewUreadaheadPackServiceClient(cl.Conn)
		// First boot is needed to be initial boot with removing all user data.
		request := arcpb.UreadaheadPackRequest{
			Creds:        s.RequiredVar("arc.perfAccountPool"),
			UseDevCaches: useDevCaches,
		}

		// Shorten the total context by 5 seconds to allow for cleanup.
		shortCtx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		// Limit running in PFQ for VM devices to 8GB+ RAM spec only for x86_64-Intel (houdini)
		// devices to prevent possible dropping caches due to high memory pressure. On arm64 and
		// x86_64-AMD (ndk), we have very few devices in Uprev with >4GB so don't restrict.
		// No restrictions for local test configs (upload=false) and non-VM devices (vmEnabled=false).
		if param.vmEnabled && param.upload && desc.BinaryTranslationType == "houdini" {
			response, err := service.CheckMinMemory(shortCtx, &empty.Empty{})
			if err != nil {
				return errors.Wrap(err, "ureadaheadPackService.CheckMinMemory returned an error")
			}
			if response.Result == false {
				testing.ContextLog(shortCtx, "Did not meet minimum memory requirement for ureadahead, skipping generate")
				return nil
			}
		}

		// Pass initial boot and capture results.
		response, err := service.Generate(shortCtx, &request)
		if err != nil {
			return errors.Wrap(err, "ureadaheadPackService.Generate returned an error for initial boot pass")
		}

		// Prepare target directory on host for pack file.
		targetDir := filepath.Join(dataDir, ureadaheadPack)
		defer func() {
			// Cleanup in case of failure. Might be needed for next retry passes.
			if retErr != nil {
				os.RemoveAll(targetDir)
			}
		}()
		if err = os.Mkdir(targetDir, 0744); err != nil {
			s.Fatalf("Failed to create %q: %v", targetDir, err)
		}

		var filesToGet = map[string]string{}
		if param.vmEnabled {
			if response.VmPackPath == "" || response.VmLogPath == "" {
				s.Fatal("Failed to obtain VM file paths from ureadaheadPackService.Generate response")
			}
			filesToGet[response.VmPackPath] = vmInitialPack
			filesToGet[response.VmLogPath] = vmInitialPackLog
		} else {
			if response.PackPath == "" || response.LogPath == "" {
				s.Fatal("Failed to obtain file paths from ureadaheadPackService.Generate response")
			}
			filesToGet[response.PackPath] = initialPack
			filesToGet[response.LogPath] = initialPackLog
		}

		for source, targetShort := range filesToGet {
			target := filepath.Join(targetDir, targetShort)
			if err = linuxssh.GetFile(shortCtx, d.Conn(), source, target, linuxssh.PreserveSymlinks); err != nil {
				s.Fatalf("Failed to get %q from the device: %v", source, err)
			}
		}

		targetTar := filepath.Join(targetDir, vUreadahead+".tar")
		testing.ContextLogf(shortCtx, "Compressing ureadahead files to %q", targetTar)
		if err = exec.Command("tar", "-cvpf", targetTar, "-C", targetDir, ".").Run(); err != nil {
			s.Fatalf("Failed to compress %q: %v", targetDir, err)
		}
		if err := duUreadahead.uploadIfNeeded(targetTar, ureadaheadPack); err != nil {
			s.Fatalf("Failed to upload %q: %v", ureadaheadPack, err)
		}

		return nil
	}

	packAndUploadData := func(ctx context.Context, bucket, srcDir string, resources []string) {
		targetDir := filepath.Join(dataDir, bucket)
		if err = os.Mkdir(targetDir, 0744); err != nil {
			s.Fatalf("Failed to create %q: %v", targetDir, err)
		}

		for _, resource := range resources {
			if err = linuxssh.GetFile(
				ctx, d.Conn(),
				filepath.Join(srcDir, resource),
				filepath.Join(targetDir, resource),
				linuxssh.PreserveSymlinks); err != nil {
				s.Fatalf("Failed to get %q from the device: %v", resource, err)
			}
		}

		targetTar := filepath.Join(targetDir, v+".tar")
		testing.ContextLogf(ctx, "Compressing %s to %q", bucket, targetTar)
		if err = exec.Command("tar", "-cvpf", targetTar, "-C", targetDir, ".").Run(); err != nil {
			s.Fatalf("Failed to compress %q: %v", targetDir, err)
		}

		if err := du.uploadIfNeeded(targetTar, bucket); err != nil {
			s.Fatalf("Failed to upload %q: %v", bucket, err)
		}
	}

	genPackagesReferenceAndGmsCoreCache := func() error {
		service := arc.NewGmsCoreCacheServiceClient(cl.Conn)

		request := arcpb.GmsCoreCacheRequest{
			PackagesCacheEnabled:       true,
			GmsCoreEnabled:             false,
			CopyGeneratedPackagesCache: false,
		}

		// Shorten the total context by 5 seconds to allow for cleanup.
		shortCtx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		response, err := service.Generate(shortCtx, &request)
		if err != nil {
			return errors.Wrap(err, "failed to generate packages reference and GMS core caches")
		}
		defer dututils.RemoveAllRemote(ctx, d, response.TargetDir)

		resources := []string{response.GmsCoreCacheName,
			response.GmsCoreManifestName,
			response.GsfCacheName}
		packAndUploadData(shortCtx, gmsCoreCache, response.TargetDir, resources)

		tempDir, err := ioutil.TempDir("", "cachebuilder")
		if err != nil {
			s.Fatal("Failed to create temp dir: ", err)
		}
		defer os.RemoveAll(tempDir)
		os.Chmod(tempDir, 0744)

		testing.ContextLog(ctx, "Created temp dir for cache builder: ", tempDir)
		jarPath, err := cache.InstallCacheBuilderJar(ctx, d, param.vmEnabled, tempDir)
		if err != nil {
			s.Fatal("Failed to install cache builder library: ", err)
		}

		localXMLPath := filepath.Join(tempDir, response.PackagesCacheName)
		if err := linuxssh.GetFile(
			ctx, d.Conn(),
			filepath.Join(response.TargetDir, response.PackagesCacheName), localXMLPath,
			linuxssh.PreserveSymlinks); err != nil {
			s.Fatalf("Failed to get %q from the device: %v", response.PackagesCacheName, err)
		}

		if useDevCaches {
			testing.ContextLogf(ctx, "Installing GMS core caches into dev directory: %q", tmpCachesDir)
			if err := decompressSystemImage(ctx, d, param.vmEnabled, tempDir); err != nil {
				s.Fatal("Failed to decompress system image: ", err)
			}
			localGMSTar := filepath.Join(tempDir, response.GmsCoreCacheName)
			if err := linuxssh.GetFile(
				ctx, d.Conn(),
				filepath.Join(response.TargetDir, response.GmsCoreCacheName), localGMSTar,
				linuxssh.PreserveSymlinks); err != nil {
				s.Fatalf("Failed to get %q from the device: %v", response.GmsCoreCacheName, err)
			}
			localGMSManifest := filepath.Join(tempDir, response.GmsCoreManifestName)
			if err := linuxssh.GetFile(
				ctx, d.Conn(),
				filepath.Join(response.TargetDir, response.GmsCoreManifestName), localGMSManifest,
				linuxssh.PreserveSymlinks); err != nil {
				s.Fatalf("Failed to get %q from the device: %v", response.GmsCoreManifestName, err)
			}
			gmsCacheBaseName := strings.TrimSuffix(response.GmsCoreCacheName, filepath.Ext(response.GmsCoreCacheName))
			localGMSCoreCache := filepath.Join(tempDir, gmsCacheBaseName)
			localPackagesCache := filepath.Join(tempDir, response.PackagesCacheName)
			fileCacheBaseName := "file_hash_cache"
			localFileHashCache := filepath.Join(tempDir, fileCacheBaseName)

			if err := cache.InstallGmsCoreCaches(ctx, jarPath, tempDir, localGMSTar, localGMSManifest, localGMSCoreCache, true /* enforceMatchingTimestamp */); err != nil {
				s.Fatal("Failed to install GMS core caches: ", err)
			}

			testing.ContextLogf(ctx, "Generating Packages cache into dev directory: %q", tmpCachesDir)
			vendorTmpDir := filepath.Join(tempDir, "vendor_root")
			if err = os.Mkdir(vendorTmpDir, 0744); err != nil {
				s.Fatalf("Failed to create %q: %v", vendorTmpDir, err)
			}
			vendorDstDir := filepath.Join(vendorTmpDir, "vendor")
			if err = os.Mkdir(vendorDstDir, 0744); err != nil {
				s.Fatalf("Failed to create %q: %v", vendorDstDir, err)
			}
			if err := decompressVendorImage(ctx, d, param.vmEnabled, vendorDstDir); err != nil {
				s.Fatal("Failed to decompress vendor image: ", err)
			}

			if err := cache.GeneratePackagesCache(ctx, jarPath, tempDir, getAndroidPath(param.vmEnabled), vendorTmpDir, localPackagesCache, localFileHashCache, localXMLPath, param.uploadPackagesReference); err != nil {
				s.Fatal("Failed to generate packages cache: ", err)
			}

			testing.ContextLog(ctx, "Copying dev cache artifacts to remote device")
			if err := copyDevCacheArtifactsToRemote(ctx, d, param.vmEnabled, tempDir, tmpCachesDir, gmsCacheBaseName, response.PackagesCacheName, fileCacheBaseName); err != nil {
				s.Fatal("Failed to copy temp cache artifacts to remote device: ", err)
			}
		}

		// Do validity check to make sure we won't fail cache generation during the official build image.
		testing.ContextLog(ctx, "Validating packages cache reference")

		// Note, we validate packages cache reference with itself. This is to validate the structure of captured document.
		if err := cache.ValidatePackagesCache(ctx, jarPath, localXMLPath, localXMLPath, false /* validateAll */); err != nil {
			s.Fatal("Failed to validate Packages cache reference: ", err)
		}

		if param.uploadPackagesReference {
			resources = []string{response.PackagesCacheName}
			packAndUploadData(shortCtx, packagesReference, response.TargetDir, resources)
		}

		return nil
	}

	// Helper that dumps logcat on failure. Dumping is optional and error here does not break
	// DataCollector flow and retries on error.
	dumpLogcat := func(mode string, attempt int) {
		log, err := dututils.AndroidShellRemote(ctx, d, "/system/bin/logcat -d")
		if err != nil {
			s.Logf("Failed to dump logcat, continue after this error: %q", err)
			return
		}

		logcatPath := filepath.Join(s.OutDir(), fmt.Sprintf("logcat_%s_%d.txt", mode, attempt))
		err = ioutil.WriteFile(logcatPath, log, 0644)
		if err != nil {
			s.Logf("Failed to save logcat, continue after this error: %q", err)
			return
		}
	}

	retryCount := defaultRetryCount
	if val, ok := s.Var("arc.DataCollector.retryCount"); ok {
		if retryCount, err = strconv.Atoi(val); err != nil {
			s.Fatal("Cannot parse argument 'arc.DataCollector.retryCount' of type int: ", err)
		}
	}

	attempts := 0
	for {
		if val, ok := s.Var("arc.DataCollector.skipGmscore"); ok {
			skipGmsCore := false
			if skipGmsCore, err = strconv.ParseBool(val); err != nil {
				s.Fatal("Cannot parse argument 'arc.DataCollector.skipGmscore' of type bool: ", err)
			}
			if skipGmsCore {
				break
			}
		}
		err := genPackagesReferenceAndGmsCoreCache()
		if err == nil {
			break
		}
		attempts = attempts + 1
		dumpLogcat("gms_core", attempts)
		if attempts > retryCount {
			s.Fatal("Failed to generate GMS Core caches. No more retries left: ", err)
		}
		s.Log("Retrying generating GMS Core caches, previous attempt failed: ", err)
	}

	attempts = 0
	for {
		if val, ok := s.Var("arc.DataCollector.skipTts"); ok {
			skipTts := false
			if skipTts, err = strconv.ParseBool(val); err != nil {
				s.Fatal("Cannot parse argument 'arc.DataCollector.skipTts' of type bool: ", err)
			}
			if skipTts {
				break
			}
		}
		err := genTTSCache(ctx, s, cl, filepath.Join(dataDir, ttsCache), v, &du)
		if err == nil {
			break
		}
		attempts = attempts + 1
		dumpLogcat("tts", attempts)
		if attempts > retryCount {
			s.Fatal("Failed to generate TTS cache. No more retries left: ", err)
		}
		s.Log("Retrying generating TTS cache, previous attempt failed: ", err)
	}

	if param.dexOptCacheGen {
		attempts = 0
		for {
			if val, ok := s.Var("arc.DataCollector.skipDexopt"); ok {
				skipDexOpt := false
				if skipDexOpt, err = strconv.ParseBool(val); err != nil {
					s.Fatal("Cannot parse argument 'arc.DataCollector.skipDexopt' of type bool: ", err)
				}
				if skipDexOpt {
					break
				}
			}
			err = genDexOptCache(ctx, s, cl, filepath.Join(dataDir, dexOptCache), v, &du)
			if err == nil {
				break
			}

			attempts = attempts + 1
			dumpLogcat("dex_opt", attempts)
			if attempts > retryCount {
				s.Fatal("Failed to generate DexOpt cache. No more retries left: ", err)
			}
			s.Log("Retrying generating DexOpt cache, previous attempt failed: ", err)
		}
	}

	// Make sure ureadahead pack generation is the last data to be generated and
	// collected since it depends on other caches being pre-installed in the system.
	attempts = 0
	for {
		if val, ok := s.Var("arc.DataCollector.skipUreadahead"); ok {
			skipUreadahead := false
			if skipUreadahead, err = strconv.ParseBool(val); err != nil {
				s.Fatal("Cannot parse argument 'arc.DataCollector.skipUreadahead' of type bool: ", err)
			}
			if skipUreadahead {
				break
			}
		}
		err := genUreadaheadPack()
		if err == nil {
			break
		}
		attempts = attempts + 1
		dumpLogcat("ureadahead", attempts)
		if attempts > retryCount {
			s.Fatal("Failed to generate ureadahead packs. No more retries left: ", err)
		}
		s.Log("Retrying generating ureadahead, previous attempt failed: ", err)
	}

	if param.uprevBranch {
		if err = maybeUprevBranch(ctx, desc, du.androidPackage, s.OutDir(), param.requiredCPUAbisForBranchUprev); err != nil {
			s.Fatal("Failed to uprev branch: ", err)
		}
	}
}

func genTTSCache(ctx context.Context, s *testing.State, cl *rpc.Client, targetDir, androidVersion string, du *dataUploader) error {
	service := arc.NewTTSCacheServiceClient(cl.Conn)

	// Shorten the total context by 5 seconds to allow for cleanup.
	shortCtx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// TTS cache setup should be disabled for the genuine cache to be generated.
	request := arcpb.TTSCacheRequest{
		TtsCacheSetupEnabled: false,
	}

	response, err := service.Generate(shortCtx, &request)
	if err != nil {
		return errors.Wrap(err, "failed to generate TTS caches")
	}
	d := s.DUT()
	defer dututils.RemoveAllRemote(ctx, d, response.TargetDir)

	if err = os.Mkdir(targetDir, 0744); err != nil {
		s.Fatalf("Failed to create %q: %v", targetDir, err)
	}

	if err = linuxssh.GetFile(shortCtx, d.Conn(), filepath.Join(response.TargetDir, response.TtsStateCacheName), filepath.Join(targetDir, response.TtsStateCacheName), linuxssh.PreserveSymlinks); err != nil {
		s.Fatalf("Failed to get %q from the device: %v", response.TtsStateCacheName, err)
	}
	targetTar := filepath.Join(targetDir, androidVersion+".tar")
	testing.ContextLogf(shortCtx, "Compressing TTS cache to %q", targetTar)
	if err = exec.Command("tar", "-cvpf", targetTar, "-C", targetDir, ".").Run(); err != nil {
		s.Fatalf("Failed to compress %q: %v", targetDir, err)
	}

	if err := du.uploadIfNeeded(targetTar, ttsCache); err != nil {
		s.Fatalf("Failed to upload %q: %v", ttsCache, err)
	}

	return nil
}

func maybeUprevBranch(ctx context.Context, desc *dututils.BuildDescriptor, androidPackage, outDir string, requiredCPUAbis []string) error {
	testing.ContextLog(ctx, "Trying to uprev branch")

	if !desc.Official {
		testing.ContextLogf(ctx, "Build %s is not official. Branch is not uprev-ed", desc.BuildID)
		return nil
	}

	// Read existing pin if possible.
	pinName := fmt.Sprintf("M%d_pin_version", desc.Milestone)
	// Note, this is URL and not file path.
	pinURL := fmt.Sprintf("%s/%s/%s", runtimeArtifactsRoot, androidPackage, pinName)
	existingPinVersion := 0

	// gsutil stat would return 1 for a non-existent object.
	if err := exec.Command(gsUtil, "stat", pinURL).Run(); err == nil {
		// Existing pin for this branch is found. Check this version is newer.
		out, err := exec.Command(gsUtil, "cat", pinURL).CombinedOutput()
		if err != nil {
			return errors.Wrapf(err, "failed to read branch pin %q", pinURL)
		}

		mVersion := regexp.MustCompile(`(\d+)\n?$`).FindStringSubmatch(string(out))
		if mVersion == nil {
			return errors.Wrapf(err, "existing pin version %q from %q is invalid", string(out), pinURL)
		}

		existingPinVersion, err = strconv.Atoi(mVersion[1])
		if err != nil {
			return errors.Wrapf(err, "could not parse existing pin version %q from %q", string(out), pinURL)
		}

		if existingPinVersion >= desc.BuildVersion {
			testing.ContextLogf(ctx, "Pin %q has version %d. This build has version %d. Uprev is not needed", pinURL, existingPinVersion, desc.BuildVersion)
			return nil
		}

	} else {
		testing.ContextLogf(ctx, "Branch pin %q does not exist", pinURL)
	}

	// Make sure all caches are available for uprev.
	set := make(map[string]bool)
	for _, abi := range requiredCPUAbis {
		// Parse special token `-` to split arch (i.e. arm64) vs. binary translation type (i.e. ndk).
		s := strings.Split(abi, "-")
		if len(s) != 2 {
			return errors.Errorf("invalid ABI string: %s", abi)
		}
		arch := s[0]
		binaryTranslationType := s[1]
		// Only check for GMS Core cache if arch was not previously processed.
		if !set[arch] {
			gmsCoreURL := fmt.Sprintf("%s/%s/gms_core_cache_%s_user_%d.tar", runtimeArtifactsRoot, androidPackage, arch, desc.BuildVersion)
			if err := exec.Command(gsUtil, "stat", gmsCoreURL).Run(); err != nil {
				testing.ContextLogf(ctx, "Required GMS Core cache %q does not exist. Branch is not yet ready for uprev", gmsCoreURL)
				return nil
			}
			testing.ContextLogf(ctx, "Required GMS Core cache %q exists", gmsCoreURL)
		}
		set[arch] = true

		ureadaheadURL := fmt.Sprintf("%s/%s/host_%s_%s_%s_%s.tar", runtimeArtifactsRoot, androidPackage, arch, binaryTranslationType, desc.BuildType, desc.BuildID)
		if err := exec.Command(gsUtil, "stat", ureadaheadURL).Run(); err != nil {
			testing.ContextLogf(ctx, "Required ureadahead pack %q does not exist. Branch is not yet ready for uprev", ureadaheadURL)
			return nil
		}
		testing.ContextLogf(ctx, "Required ureadahead pack %q exists", ureadaheadURL)
	}

	// Create local copy of pin.
	localPinPath := filepath.Join(outDir, pinName)
	if err := ioutil.WriteFile(localPinPath, []byte(fmt.Sprintf("%d\n", desc.BuildVersion)), 0644); err != nil {
		return errors.Wrapf(err, "failed to create pin locally %q", localPinPath)
	}

	// Upload it remotely.
	if err := exec.Command(gsUtil, "copy", localPinPath, pinURL).Run(); err != nil {
		return errors.Wrapf(err, "failed to upload pin remotely %q", pinURL)
	}

	testing.ContextLogf(ctx, "Branch %d is pinned to %d. Pin URL: %q", desc.Milestone, desc.BuildVersion, pinURL)
	return nil
}

func genDexOptCache(ctx context.Context, s *testing.State, cl *rpc.Client, targetDir, androidVersion string, du *dataUploader) (retErr error) {
	service := arc.NewDexOptCacheServiceClient(cl.Conn)

	// Shorten the total context by 5 seconds to allow for cleanup.
	shortCtx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	response, err := service.Generate(shortCtx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to generate DexOpt cache")
	}
	d := s.DUT()
	defer dututils.RemoveAllRemote(ctx, d, response.TargetDir)

	if err = os.Mkdir(targetDir, 0744); err != nil {
		s.Fatalf("Failed to create %q: %v", targetDir, err)
	}

	targetFile := filepath.Join(targetDir, response.DexOptCacheName)
	if err = linuxssh.GetFile(shortCtx, d.Conn(), filepath.Join(response.TargetDir, response.DexOptCacheName), targetFile, linuxssh.PreserveSymlinks); err != nil {
		s.Fatalf("Failed to get %q from the device: %v", response.DexOptCacheName, err)
	}

	if err := du.uploadIfNeeded(targetFile, dexOptCache); err != nil {
		s.Fatalf("Failed to upload %q: %v", targetFile, err)
	}

	return nil
}

func getAndroidPath(vmEnabled bool) string {
	const (
		// Path where the container images are installed in the rootfs.
		arcPath = "/opt/google/containers/android"
		// Path where the VM images are installed in the rootfs.
		arcVMPath = "/opt/google/vms/android"
	)

	if vmEnabled {
		return arcVMPath
	}
	return arcPath
}

func decompressSystemImage(ctx context.Context, d *dut.DUT, vmEnabled bool, dstDir string) error {
	systemImg := "system.raw.img"
	return decompressImage(ctx, d, vmEnabled, systemImg, dstDir)
}

func decompressVendorImage(ctx context.Context, d *dut.DUT, vmEnabled bool, dstDir string) error {
	vendorImg := "vendor.raw.img"
	return decompressImage(ctx, d, vmEnabled, vendorImg, dstDir)
}

func decompressImage(ctx context.Context, d *dut.DUT, vmEnabled bool, imageName, dstDir string) error {
	localImg := filepath.Join(dstDir, imageName)
	if err := linuxssh.GetFile(
		ctx, d.Conn(),
		filepath.Join(getAndroidPath(vmEnabled), imageName), localImg,
		linuxssh.PreserveSymlinks); err != nil {
		return errors.Wrapf(err, "failed to get %q from the device", imageName)
	}
	// Decompress image file.
	if err := testexec.CommandContext(ctx, "unsquashfs", "-no-xattrs", "-f", "-d", dstDir, localImg).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to unsquashfs image: %v", imageName)
	}
	return nil
}

func copyDevCacheArtifactsToRemote(ctx context.Context, d *dut.DUT, vmEnabled bool, localDir, remoteDir, gmsCoreCacheName, packagesCacheName, fileHashCacheName string) error {
	localGMSCoreCache := filepath.Join(localDir, gmsCoreCacheName)
	localPackagesCache := filepath.Join(localDir, packagesCacheName)
	localFileHashCache := filepath.Join(localDir, fileHashCacheName)
	remoteGMSCoreCache := filepath.Join(remoteDir, gmsCoreCacheName)
	remotePackagesCache := filepath.Join(remoteDir, packagesCacheName)
	remoteFileHashCache := filepath.Join(remoteDir, fileHashCacheName)
	dataMap := map[string]string{
		localGMSCoreCache:  remoteGMSCoreCache,
		localPackagesCache: remotePackagesCache,
		localFileHashCache: remoteFileHashCache,
	}
	if _, err := linuxssh.PutFiles(ctx, d.Conn(), dataMap, linuxssh.DereferenceSymlinks); err != nil {
		return errors.Wrapf(err, "failed to send data from %q to remote data path %q", localGMSCoreCache, remoteGMSCoreCache)
	}

	// Update file ownership and permissions of remote files. This is mimicking what
	// GmsCoreCacheInstaller is already doing but on the remote host.
	var ugid string
	if vmEnabled {
		// ARCVM requires system user.
		ugid = "1000"
	} else {
		// Container can use Android system user instead.
		ugid = "656360"
	}
	if err := dututils.ChownRemote(ctx, d, ugid, ugid, remoteGMSCoreCache); err != nil {
		return errors.Wrap(err, "failed to chown GMS core caches path")
	}
	if err := dututils.ChmodRemote(ctx, d, "0700", remoteGMSCoreCache); err != nil {
		return errors.Wrap(err, "failed to chmod GMS core caches path")
	}
	return nil
}
