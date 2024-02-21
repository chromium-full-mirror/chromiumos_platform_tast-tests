// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Aliases for type enums.
const Configurability = chrome.documentScan.Configurability;
const ConnectionType = chrome.documentScan.ConnectionType;
const ConstraintType = chrome.documentScan.ConstraintType;
const OptionType = chrome.documentScan.OptionType;
const OptionUnit = chrome.documentScan.OptionUnit;
const OperationResult = chrome.documentScan.OperationResult;

// UI elements.
var simpleScanButton = document.getElementById('doSimpleScan');
var scanButton = document.getElementById('scanButton');
var cancelButton = document.getElementById('cancelButton');
var scanStatus = document.getElementById('scanStatus');
var imageContainer = document.getElementById('imageContainer');
var waitAnimation = document.getElementById('waitAnimation');
var getListButton = document.getElementById('getListButton');
var openScannerButton = document.getElementById('getScannerConfigButton');
var closeScannerButton = document.getElementById('closeScannerButton');
var scannerList = document.getElementById('scannerList');
var scannerConfig = document.getElementById('scannerConfig');
var sizeA4 = document.getElementById('sizeA4');
var sizeLetter = document.getElementById('sizeLetter');
var sizeLegal = document.getElementById('sizeLegal');
var sizeA3 = document.getElementById('sizeA3');
var sizeFull = document.getElementById('sizeFull');
var scanSource = document.getElementById('scanSource');
var scanResolution = document.getElementById('scanResolution');
var toggleInactiveButton = document.getElementById('toggleInactive');
var toggleAdvancedButton = document.getElementById('toggleAdvanced');
var bytesReceivedValue = document.getElementById('bytesReceived');
var pageCountValue = document.getElementById('pageCount');
var scannerInfoContainer = document.getElementById('scannerInfo');
var scannerInfoMfgr = document.getElementById('mfgr');
var scannerInfoModel = document.getElementById('model');
var scannerInfoConnection = document.getElementById('connection');
var scannerInfoSecure = document.getElementById('secure');
var scannerInfoProtocolType = document.getElementById('protocolType');

// State.
var currentScanner;
var currentOptionGroups;
var currentConfig;
var currentScanJob;
var showInactive = false;
var showAdvanced = false;
var bytesReceived = 0;
var pageCount = 0;

function setOnlyChild(parent, child) {
  while (parent.firstChild) {
    parent.removeChild(parent.firstChild);
  }
  parent.appendChild(child);
}

var beforeUnloadHandler = function(event) {
  if (currentScanner) {
    chrome.documentScan.closeScanner(currentScanner);
  }
};

var updateScannerList = function(scanners) {
  let e = document.createElement("select");
  e.disabled = true;
  e.id = 'scannerList';
  e.name = 'scannerList';
  scannerList.replaceWith(e);
  scannerList = e;
  for (const scanner of scanners) {
    console.log("Got scanner: " + JSON.stringify(scanner));
    var opt = document.createElement('option');
    opt.text = scanner.name;
    opt.value = JSON.stringify(scanner);
    if (scanner.manufacturer == "DavieV") {
      opt.selected = true;
    }
    scannerList.appendChild(opt);
  }
  if (scannerList.value) {
    updateScannerInfo();
    scanStatus.innerText = "No scanner";
    openScannerButton.disabled = false;
  }
  getListButton.disabled = false;
  scannerList.disabled = false;
};

var updateScannerInfo = function() {
  if (!scannerList.value) {
    return;
  }
  const info = JSON.parse(scannerList.value);
  scannerInfoMfgr.innerText = info.manufacturer;
  scannerInfoModel.innerText = info.model;
  scannerInfoConnection.innerText = info.connectionType;
  scannerInfoSecure.innerText = info.secure;
  scannerInfoProtocolType.innerText = info.protocolType;
  scannerInfoContainer.style.display = 'initial';
};

var GetOptionVal = function(option) {
  switch (option.type) {
    case OptionType.BOOL:
      return option.value;

    case OptionType.INT:
      return option.value;

    case OptionType.FIXED:
      return option.value.toFixed(1);

    case OptionType.STRING:
      return option.value;

    default:
      console.log("Unexpected option type in option " + option.name +
                  ": " + option.type);
      return null;
  }
};

var OptionValue = function(option) {
  if (!option.isActive || option.value === null) {
    return "";
  }

  var ret = "";
  var val = GetOptionVal(option);
  var unit;
  switch (option.unit) {
    case OptionUnit.PIXEL:
      unit = " px";
      break;

    case OptionUnit.BIT:
      unit = " bit";
      break;

    case OptionUnit.MM:
      unit = " mm";
      break;

    case OptionUnit.DPI:
      unit = " dpi";
      break;

    case OptionUnit.PERCENT:
      unit = "%";
      break;

    case OptionUnit.MICROSEC:
      unit = " μs";
      break;

    case OptionUnit.UNITLESS:
    case null:
      unit = "";
      break;
  }

  if (option.constraint) {
    switch (option.constraint.type) {
      case ConstraintType.FIXED_RANGE:
        ret = val + unit + " [" + option.constraint.min.toFixed(1) + "-" +
              option.constraint.max.toFixed(1) + "]";
        if (option.constraint.quant &&
            option.constraint.quant.toFixed(1) != "1.0" &&
            option.constraint.quant.toFixed(3) != "0.000") {
          ret += "/" + option.constraint.quant.toFixed(3);
        }
        break;

      case ConstraintType.INT_RANGE:
        ret = val + unit + " [" + option.constraint.min + "-" +
              option.constraint.max + "]";
        if (option.constraint.quant && option.constraint.quant != 1) {
          ret += "/" + option.constraint.quant;
        }
        break;

      case ConstraintType.FIXED_LIST:
        for (num of option.constraint.list) {
          if (num.toFixed(4) == val.toFixed(4)) {
            ret += "[" + val.toFixed(1) + unit + "] ";
          } else {
            ret += num.toFixed(1) + " "
          }
        }
        break;

      case ConstraintType.INT_LIST:
        for (num of option.constraint.list) {
          if (num == val) {
            ret += "[" + val + unit + "] ";
          } else {
            ret += num + " "
          }
        }
        break;

      case ConstraintType.STRING_LIST:
        for (str of option.constraint.list) {
          if (str == val) {
            ret += "[" + val + unit + "], ";
          } else {
            ret += str + ", "
          }
        }
        break;
    }
  }
  if (!ret) {
    ret = val + unit;
  }
  return ret;
};

var disableOptions = function() {
  scanSource.disabled = true;
  scanResolution.disabled = true;
  sizeA4.disabled = true;
  sizeLetter.disabled = true;
  sizeLegal.disabled = true;
  sizeA3.disabled = true;
  sizeFull.disabled = true;

  while (scanSource.options.length > 0) {
    scanSource.options.remove(0);
  }
  while (scanResolution.options.length > 0) {
    scanResolution.options.remove(0);
  }
};

var enableSizes = function(options) {
  const tl_x = GetMinValue(options["tl-x"]);
  const br_x = GetMaxValue(options["br-x"]);
  const tl_y = GetMinValue(options["tl-y"]);
  const br_y = GetMaxValue(options["br-y"]);
  const max_width = br_x - tl_x;
  const max_height = br_y - tl_y;

  sizeA4.disabled = !(max_width >= 209.5 && max_height >= 296.5);
  sizeLetter.disabled = !(max_width >= 215.8 && max_height >= 279.3);
  sizeLegal.disabled = !(max_width >= 215.8 && max_height >= 355.5);
  sizeA3.disabled = !(max_width >= 296.5 && max_height >= 419.5);
  sizeFull.disabled = false;
};

var GetOptionFlags = function(option) {
  var flags = [];
  if (!option.isActive) {
    flags.push("inactive");
  }
  if (!option.isDetectable) {
    flags.push("!detect");
  }
  switch (option.configurability) {
    case Configurability.HARDWARE_CONFIGURABLE:
      flags.push("hw");
      break;
    case Configurability.NOT_CONFIGURABLE:
      flags.push("ro");
      break;
    case Configurability.SOFTWARE_CONFIGURABLE:
      break;
  }
  if (option.isEmulated) {
    flags.push("emu");
  }
  if (option.isAutoSettable) {
    flags.push("auto");
  }
  if (option.isAdvanced) {
    flags.push("adv");
  }
  if (option.isInternal) {
    flags.push("int");
  }

  if (flags.length) {
    return "[" + flags.join(",") + "] ";
  } else {
    return "";
  }
};

var updateDisplay = function(config) {
  currentConfig = config;
  display = "";
  if (currentOptionGroups && currentOptionGroups.length > 0) {
    for (const group of currentOptionGroups) {
      display += group.title + " options\n";
      for (const member of group.members) {
        const option = config.options[member];
        if (!showAdvanced && option.isAdvanced) {
          continue;
        }
        if (!showInactive && !option.isActive) {
          continue;
        }
        const flags = GetOptionFlags(option);
        display += "  " + flags + option.title + "(" + option.name + "): " +
                   OptionValue(option) + "\n";
      }
    }
  } else {
    for (const [name, option] of Object.entries(config.options)) {
      if (!showAdvanced && option.isAdvanced) {
        continue;
      }
      if (!showInactive && !option.isActive) {
        continue;
      }
      const flags = GetOptionFlags(option);
      display += flags + option.title + "(" + option.name + "): " +
                 OptionValue(option) + "\n";
    }
  }
  scannerConfig.textContent = display;

  while (scanSource.options.length > 0) {
    scanSource.options.remove(0);
  }
  if (config.options["source"]) {
    const source_opt = config.options["source"];
    if (source_opt.constraint &&
        source_opt.constraint.type == ConstraintType.STRING_LIST) {
      for (const val of source_opt.constraint.list) {
        var opt = document.createElement('option');
        opt.text = val;
        opt.value = val;
        if (val == source_opt.value) {
          opt.selected = true;
        }
        scanSource.options.add(opt);
      }
      scanSource.disabled = false;
    } else {
      var opt = document.createElement('option');
      opt.text = source_opt.value;
      opt.value = source_opt.value;
      opt.selected = true;
      scanSource.options.add(opt);
    }
  }
  while (scanResolution.options.length > 0) {
    scanResolution.options.remove(0);
  }
  if (config.options["resolution"]) {
    const source_opt = config.options["resolution"];
    if (source_opt.constraint &&
        source_opt.constraint.type == ConstraintType.INT_LIST) {
      for (const val of source_opt.constraint.list) {
        var opt = document.createElement('option');
        opt.text = val;
        opt.value = val;
        if (val == source_opt.value) {
          opt.selected = true;
        }
        scanResolution.options.add(opt);
      }
      scanResolution.disabled = false;
    } else {
      var opt = document.createElement('option');
      opt.text = source_opt.value;
      opt.value = source_opt.value;
      opt.selected = true;
      scanResolution.options.add(opt);
    }
  }

  enableSizes(config.options);

  scanButton.disabled = false;
  scanStatus.innerText = "Ready";
};

var GetMinValue = function(option) {
  if (!option.constraint) {
    return null;
  }

  if (option.constraint.type != ConstraintType.INT_RANGE &&
      option.constraint.type != ConstraintType.FIXED_RANGE) {
    return null;
  }

  return option.constraint.min;
};

var GetMaxValue = function(option) {
  if (!option.constraint) {
    return null;
  }

  if (option.constraint.type != ConstraintType.INT_RANGE &&
      option.constraint.type != ConstraintType.FIXED_RANGE) {
    return null;
  }

  return option.constraint.max;
};

var MakeOptionValue = function(option, value) {
  var option = {
      name: option.name,
      type: option.type,
      value: value
  };
  return option;
};

var paperSizeOptions = function(width, height) {
  const tl_x = GetMinValue(currentConfig.options["tl-x"]);
  const br_x = tl_x + width;
  const tl_y = GetMinValue(currentConfig.options["tl-y"]);
  const br_y = tl_y + height;
  var options = [
      MakeOptionValue(currentConfig.options["tl-x"], tl_x),
      MakeOptionValue(currentConfig.options["tl-y"], tl_y),
      MakeOptionValue(currentConfig.options["br-x"], br_x),
      MakeOptionValue(currentConfig.options["br-y"], br_y)
  ];
  console.log("Paper size options: " + JSON.stringify(options));
  return options;
};

var setPaperSize = async function(width, height) {
  scanButton.disabled = true;
  var response = await chrome.documentScan.setOptions(
      currentScanner, paperSizeOptions(width, height));
  console.log("setOptions response: " + JSON.stringify(response));
  updateDisplay(response);
};

scanButton.addEventListener('click', async function() {
  scanButton.disabled = true;
  currentScanJob = null;
  imageContainer.innerHTML = '';
  scanStatus.innerText = "Starting";
  waitAnimation.style.display = 'block';
  const multi_page = scanSource.value.includes('ADF');
  pageCount = 0;
  bytesReceived = 0;
  while (true) {
    currentScanJob = null;
    cancelButton.disabled = true;
    await new Promise(r => setTimeout(r, 1000));
    let response = await chrome.documentScan.startScan(
        currentScanner, {format: "image/jpeg"});
    console.log("StartScan response: " + JSON.stringify(response));
    waitAnimation.style.display = 'none';
    if (response.result != OperationResult.SUCCESS || !response.job) {
      if (pageCount > 0 && response.result == OperationResult.ADF_EMPTY) {
        scanStatus.innerText = "Completed";
      } else {
        scanStatus.innerText = "Failed: " + response.result;
      }
      break;
    }
    currentScanJob = response.job;
    cancelButton.disabled = false;
    scanStatus.innerText = "Waiting";

    response = await chrome.documentScan.readScanData(currentScanJob);
    console.log("First ReadScanData result: " + response.result);
    let parts = [];
    while (response.result == OperationResult.SUCCESS) {
      scanStatus.innerText = "Reading: " + response.estimatedCompletion + "%";

      if (response.data && response.data.byteLength > 0) {
        parts.push(response.data);
      }
      response = await chrome.documentScan.readScanData(currentScanJob);
      if (response.result == OperationResult.CANCELLED) {
        console.log("Scan canceled");
        break;
      } else if (response.result != OperationResult.SUCCESS) {
        console.log("ReadScanData result: " + response.result);
      } else {
        if (response.data && response.data.byteLength > 0) {
          console.log("ReadScanData bytes: " + response.data.byteLength);
          bytesReceived += response.data.byteLength;
        } else {
          console.log("ReadScanData no data");
          await new Promise(r => setTimeout(r, 100));
        }
      }
      bytesReceivedValue.innerText = bytesReceived;
    }
    console.log("Final result: " + response.result);
    cancelButton.disabled = true;
    if (response.result == OperationResult.EOF) {
      // Pick up the final bytes.
      console.log("Bytes at EOF: " + response.data.byteLength);
      parts.push(response.data);
      bytesReceived += response.data.byteLength;
      bytesReceivedValue.innerText = bytesReceived;
    } else if (response.result == OperationResult.CANCELLED) {
      break;
    } else {
      scanStatus.innerText = "Failed";
      if (response.result == OperationResult.ADF_EMPTY && pageCount == 0) {
        alert("Document feeder is empty.");
      }
      break;
    }
    pageCount += 1;
    pageCountValue.innerText = pageCount;
    scanStatus.innerText = "Page completed";

    let imgData = new Blob(parts, { type: "image/jpeg" });
    let image = document.createElement('img');
    image.src = URL.createObjectURL(imgData);
    imageContainer.appendChild(image);

    if (!multi_page || response.result == OperationResult.ADF_EMPTY) {
      scanStatus.innerText = "Completed";
      break;
    }
  }
  scanButton.disabled = false;
});

getListButton.addEventListener('click', async function() {
  waitAnimation.style.display = 'block';
  if (currentScanner) {
    window.removeEventListener("beforeunload", beforeUnloadHandler);
    await chrome.documentScan.closeScanner(currentScanner);
  }
  currentScanner = null;
  openScannerButton.disabled = true;
  getListButton.disabled = true;
  scanButton.disabled = true;
  cancelButton.disabled = true;
  scannerConfig.innerHTML = "";
  disableOptions();
  scannerInfoContainer.style.display = 'none';
  while (scannerList.options.length > 0) {
    scannerList.options.remove(0);
  }
  await new Promise(r => setTimeout(r, 1000));

  var response = await chrome.documentScan.getScannerList({
    local: true, secure: false});
  console.log("Response: " + JSON.stringify(response));
  if (response.result != OperationResult.SUCCESS) {
    scanStatus.innerText = "Failed to get scanner list: " + response.result;
  }
  updateScannerList((response.scanners && response.scanners.length > 0) ?
                    response.scanners :
                    []);
  waitAnimation.style.display = 'none';
});

scannerList.addEventListener('change', async function() {
  updateScannerInfo();
});

openScannerButton.addEventListener('click', async function() {
  scanButton.disabled = true;
  cancelButton.disabled = true;
  disableOptions();
  if (currentScanner) {
    window.removeEventListener("beforeunload", beforeUnloadHandler);
    await chrome.documentScan.closeScanner(currentScanner);
    currentScanner = null;
  }
  currentOptionGroups = [];
  var info = JSON.parse(scannerList.value);
  var scanner = info.scannerId;
  console.log("Scanner request: " + scannerList.value);
  let response = await chrome.documentScan.openScanner(scanner);
  console.log("openScanner response: " + JSON.stringify(response));
  if (response.result != OperationResult.SUCCESS) {
    alert("Open failed: " + response.result);
    return;
  }
  currentScanner = response.scannerHandle;
  currentConfig = null;
  let config = response;
  if (!config.options) {
    return;
  }
  window.addEventListener("beforeunload", beforeUnloadHandler);

  if (chrome.documentScan.getOptionGroups != null) {
    response = await chrome.documentScan.getOptionGroups(currentScanner);
    console.log("getOptionGroups response: " + JSON.stringify(response));
    if (response.groups) {
      currentOptionGroups = response.groups;
    }
  }

  // Set up special options up front.
  var options = [];
  if ('page-width' in config.options) {
    options.push(
      MakeOptionValue(config.options['page-width'],
                      GetMaxValue(config.options['page-width']))
    );
  }
  if ('page-height' in config.options) {
    options.push(
      MakeOptionValue(config.options['page-height'],
                      GetMaxValue(config.options['page-height']))
    );
  }
  if ('resolution' in config.options) {
    options.push(
        MakeOptionValue(config.options['resolution'], 100.0)
    );
  }
  console.log("Modes: " + JSON.stringify(config.options['mode']));
  if ('mode' in config.options) {
    options.push(
        MakeOptionValue(config.options['mode'], 'Color')
    );
  }
  if (options.length > 0) {
    config = await chrome.documentScan.setOptions(currentScanner, options);
  }

  closeScannerButton.disabled = false;
  updateDisplay(config);
});

closeScannerButton.addEventListener('click', async function() {
  scanButton.disabled = true;
  cancelButton.disabled = true;
  closeScannerButton.disabled = true;
  scannerConfig.innerHTML = "";
  disableOptions();
  if (currentScanner) {
    window.removeEventListener("beforeunload", beforeUnloadHandler);
    await chrome.documentScan.closeScanner(currentScanner);
    currentScanner = null;
  }
  currentConfig = null;
});

scanSource.addEventListener('change', async function() {
  const option = {
    name: "source",
    type: OptionType.STRING,
    value: scanSource.value
  };
  const options = {
    options: [option]
  };
  console.log("Setting source: " + JSON.stringify(option));
  scanButton.disabled = true;
  var response = await chrome.documentScan.setOptions(currentScanner, [option]);
  updateDisplay(response);
});

scanResolution.addEventListener('change', async function() {
  const option = {
    name: "resolution",
    type: OptionType.INT,
    value: parseInt(scanResolution.value)
  };
  const options = {
    options: [option]
  };
  console.log("Setting resolution: " + JSON.stringify(option));
  scanButton.disabled = true;
  var response = await chrome.documentScan.setOptions(currentScanner, [option]);
  updateDisplay(response);
});

sizeA4.addEventListener('click', async function() {
  await setPaperSize(210.0, 297.0);
});

sizeLetter.addEventListener('click', async function() {
  await setPaperSize(215.9, 279.4);
});

sizeLegal.addEventListener('click', async function() {
  await setPaperSize(215.9, 355.6);
});

sizeA3.addEventListener('click', async function() {
  await setPaperSize(210.0, 420.0);
});

sizeFull.addEventListener('click', async function() {
  const width = GetMaxValue(currentConfig.options["br-x"]) -
                GetMinValue(currentConfig.options["tl-x"]);
  const height = GetMaxValue(currentConfig.options["br-y"]) -
                 GetMinValue(currentConfig.options["tl-y"]);
  await setPaperSize(width, height);
});

toggleInactiveButton.addEventListener('click', function() {
  showInactive = !showInactive;
  toggleInactiveButton.innerText = showInactive ?
                                   'Hide inactive' :
                                   'Show inactive';

  if (currentConfig) {
    updateDisplay(currentConfig);
  }
});

toggleAdvancedButton.addEventListener('click', function() {
  showAdvanced = !showAdvanced;
  toggleAdvancedButton.innerText = showAdvanced ?
                                   'Hide advanced' :
                                   'Show advanced';

  if (currentConfig) {
    updateDisplay(currentConfig);
  }
});

simpleScanButton.addEventListener('click', async function() {
  try {
    simpleScanButton.disabled = true;
    waitAnimation.style.display = 'block';
    imageContainer.innerHTML = '';
    await new Promise(r => setTimeout(r, 1000));

    var scan_results = await chrome.documentScan.scan({mimeTypes: ["testing"]});
    if (chrome.runtime.lastError) {
      console.log('Scan failed: ' + chrome.runtime.lastError.message);
      return;
    }

    numImages = scan_results.dataUrls.length;
    console.log('Scan completed with ' + numImages + ' images.');
    urlData = scan_results.dataUrls[0]
    console.log('Scan data length ' + urlData.length + '.');
    console.log('URL is ' + urlData);

    let image = document.createElement('img');
    image.id = 'scannedImage';  // Simple scan only expects one image.
    image.src = scan_results.dataUrls[0];
    imageContainer.appendChild(image);
  } catch (err) {
    scanStatus.innerText = err;
  } finally {
    waitAnimation.style.display = 'none';
    simpleScanButton.disabled = false;
  }
});

cancelButton.addEventListener('click', async function() {
  if (!currentScanJob) {
    return;
  }

  scanStatus.innerText = 'Cancelling';
  let response = await chrome.documentScan.cancelScan(currentScanJob);
  console.log("Cancel response: " + response.result);
  while (response.result != OperationResult.SUCCESS) {
    if (response.result == OperationResult.DEVICE_BUSY) {
        scanStatus.innerText = 'Waiting for cancellation';
    } else {
        scanStatus.innerTest = 'Cancel failed: ' + response.result;
        return;
    }
    response = await chrome.documentScan.cancelScan(currentScanJob);
    console.log("Cancel response: " + response.result);
  }
  scanStatus.innerText = 'Cancelled';
});

async function getScannedImageBytes(i) {
  let src = document.getElementById('imageContainer').children[i].src;
  console.log(src);
  let buf = await fetch(src).then(response => response.arrayBuffer());
  return Array.from(new Uint8Array(buf));
}
