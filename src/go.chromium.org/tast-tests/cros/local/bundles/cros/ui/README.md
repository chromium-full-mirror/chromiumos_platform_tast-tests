# Performance CUJ Documentation and Guide
**Author: ramsaroop@google.com, cros-sw-perf@google.com**
**Updated: Jan 8, 2023**

The purpose of this document is to provide a guide on how to locally run Performance CUJs, such as DesksCUJ and MeetCUJ.

## Prerequisites
- A connected ChromeOS Device with a test image ([leased from the lab](http://go/crosfleet) or [locally available](http://go/cros-direct-dut)).
- A valid, updated ChromeOS chroot.
    - Follow [ChromiumOS Developer Guide](https://chromium.googlesource.com/chromiumos/docs/+/HEAD/developer_guide.md) to get the ChromiumOS source code and chroot.
    - Ensure the chroot is updated using the following command:
~~~
# Outside the chroot
$ cros_sdk update_chroot
~~~

## Running a Performance CUJ
### Commands
The following command assumes that the chromiumos checkout is found at `~/chromiumos`. If this path is different, the commands must be appropriately adjusted.

~~~
# Outside the chroot
$ cd ~/chromiumos                     # Move to the ChromiumOS directory.
$ cros_sdk                            # Enter the chroot.
$ tast run <DUT IP> <TEST NAME>       # Run test.
~~~

### Additional Flags
1. `-var=cujrecorder.isLocal=true -var=cuj.isLocal=true` Skips temperature and CPU cooldown checks. This is useful for local development to speed up test run time.

2. `var=cujrecorder.record=true` Records a video and saves a file `record.webm` in the test output.

### Reading Test Output
Once the test completes, the last line of the test output should have a filepath in the form:
~~~
/tmp/tast/results/20231103-110702
~~~

**From within the chroot**, you can view the test metrics as follows:
~~~
$ vim /tmp/tast/results/20231103-110702/tests/{test-name}/results-chart.json
~~~

**From outside the chroot**, you can view the test metrics as follows:
~~~
$ vim chromiumos/out/tast/results/20231103-110702/tests/{test-name}/results-chart.json
~~~

## Running CUJ Examples
### Running One Test
~~~
$ tast run <DUT IP> ui.MeetCUJ.docs
~~~

### Running Multiple Tests
~~~
$ tast run <DUT IP> ui.GoogleSheetsCUJ ui.GoogleSheetsCUJ.battery_saver
~~~

### Running Test with Additional Flags
~~~
$ tast run -var=cujrecorder.isLocal=true -var=cuj.isLocal=true <DUT IP> ui.DesksCUJ
~~~

## Performance CUJ Documentation
[go/cuj-documentation](http://go/cuj-documentation) contains a list of valid Performance CUJs that are supported by the Software performance Team.

[go/cuj-videos](http://go/cuj-videos) contains video demos of each of the CUJs.