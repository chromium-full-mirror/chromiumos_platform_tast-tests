## Title: Geekbench5CUJ/Geekbench6CUJ Documentation and Guide (public)
**Author: vincentchiang@chromium.org , cros-sw-perf@google.com**
**Updated: Aug 17, 2023**
### Objective

The purpose of this document is to provide a guide to automate running Geekbench5 and Geekbench6 for public ChromeOS developers. For Googlers, please read the [internal guide](http://go/geekbenchcuj-guide).

### Prerequisites
- Ubuntu 20.04
- A 64 bit system
- An account with sudo access
- Git configured
- A ChromeOS device
- A valid Geekbench 5 or 6 license: https://www.primatelabs.com/store/.
- For ARM devices, you need an ARM binary from [Primatelabs](https://www.primatelabs.com/store/) as part of the site/source/dev license.

### Install the necessary dependencies and tools
[ChromeOS Developer Guide Prequisites](https://chromium.googlesource.com/chromiumos/docs/+/main/developer_guide.md#Prerequisites)

In summary:
Install the git and subversion revision control systems, the curl download helper, and lvm tools.
~~~
$ sudo apt-get install git-core gitk git-gui subversion curl lvm2 thin-provisioning-tools python-pkg-resources python-virtualenv python-oauth2client
~~~

Install depot_tools and add it to path:
~~~
$ git clone https://chromium.googlesource.com/chromium/tools/depot_tools.git
$ echo 'PATH="$PATH:$HOME/depot_tools"' >> ~/.bashrc
$ source ~/.bashrc
~~~

### Fetch ChromiumOS code and create chroot
Fetch and create the chromiumos source code and chroot environment.
~~~
$ mkdir -p ~/chromiumos
$ cd ~/chromiumos
$ repo init -u https://chromium.googlesource.com/chromiumos/manifest -b main
$ repo sync -j4
$ cros_sdk --create
~~~
### ChromeOS device setup
Please follow developer guide: [ChromiumOS Developer Guide](https://chromium.googlesource.com/chromiumos/docs/+/main/developer_guide.md#Installing-ChromiumOS-on-your-Device)

In summary:
1. Set the device to developer mode.
2. Flash it with a valid ChromeOS test image

~~~
$ export BOARD=amd64-generic
$ cros build-packages --board=${BOARD}
$ cros build-image --board=${BOARD}
$ cros flash --board=${BOARD} usb://
~~~

On DUT:
Enable boot from USB:
~~~
(dut) # crossystem dev_boot_usb=1
(dut) # reboot
~~~

Boot and install the image:
~~~
(dut booted from usb) # chromeos-install
~~~

### Running the Benchmark: Tast command (x86_64 architecture only)
Note: This public test uses the Geekbench 5 and 6 binaries directly downloaded from geekbench.com.
If you have acquired a site license or above and gained access to Linux/AArch64 binaries, please refer to the python script section below.

Step 1:  Set up the environment.
Navigate to your chromiumos checkout and enter chroot via cros_sdk

~~~
$ cd ~/chromiumos/src
$ cros_sdk
~~~

Step 2: Run test via the command line.
Run with:
~~~
$ tast run -var=geekbench.email=<email> -var=geekbench.key=<key> <DUT> <test names>
~~~

\<test names\> include the following:
1. ui.Geekbench5CUJ
2. ui.Geekbench5CUJ.crostini
3. ui.Geekbench6CUJ
4. ui.Geekbench6CUJ.crostini

Examples

Run Geekbench5CUJ with:
~~~
$ tast run -var=geekbench.email=<email> -var=geekbench.key=<key> <DUT> ui.Geekbench5CUJ
~~~

Run Geekbench5CUJ.crostini with:
~~~
$ tast run -var=geekbench.email=<email> -var=geekbench.key=<key> <DUT> ui.Geekbench5CUJ.crostini
~~~

Run multiple tests together (only works for tests using the same Geekbench Version)
~~~
$ tast run -var=geekbench.email=<email> -var=geekbench.key=<key> <DUT> ui.Geekbench5CUJ ui.Geekbench5CUJ.crostini
~~~

### Running the Benchmark: Python script (Custom binary support)
This section will go over how to run Geekbench(5|6)CUJ using the python script located in: ```/path/to/src/platform/tast-tests/tools/geekbench.py```
Documentation can also be seen by running the following command:
~~~
$ /path/to/src/platform/tast-tests/tools/geekbench.py -h
~~~

The goal of this section is to run non-x86_64 binaries like ARM64 or ARM32.

The python script requires the following positional arguments:
1. ```<email>``` Geekbench email
2. ```<key>``` Geekbench key
3. ```<dut>``` Address where test device can be reached

Optional arguments are the following:
1. ```--binary (-b)``` Custom binary path
2. ```--gb-version -v``` Geekbench version. Default 5
3. ```--env -e``` Execution environment; native, crostini, or both. Default native

Run example:
~~~
$ /path/to/src/platform/tast-tests/tools/geekbench.py <email> <key> <DUT> -b /path/to/geekbench/binary -v <5|6> -e <native|crostini|both>
~~~

### Viewing the Results
Results will be saved in the output directory as part of ```results-chart.json```, or the binary output result can be directly viewed in results.txt.

In ```results-chart.json```, benchmark scores will be separated by MultiCore or SingleCore using the following notation:

MultiCore:
	```Benchmark.Geekbench.MultiCore.<Suite Name | Score>```
SingleCore:
	```Benchmark.Geekbench.SingleCore.<Suite Name | Score>```
