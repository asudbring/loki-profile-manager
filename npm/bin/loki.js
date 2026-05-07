#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

const supported = new Map([
  ['darwin:x64', 'darwin-x64'],
  ['darwin:arm64', 'darwin-arm64'],
  ['linux:x64', 'linux-x64'],
  ['linux:arm64', 'linux-arm64'],
  ['win32:x64', 'win32-x64'],
  ['win32:arm64', 'win32-arm64'],
]);

const platformKey = `${process.platform}:${process.arch}`;
const vendorKey = supported.get(platformKey);

if (!vendorKey) {
  const supportedList = Array.from(supported.keys()).sort().join(', ');
  console.error(`Unsupported Loki platform: ${platformKey}`);
  console.error(`Supported platforms: ${supportedList}`);
  process.exit(1);
}

const binaryName = process.platform === 'win32' ? 'loki.exe' : 'loki';
const binaryPath = path.join(__dirname, '..', 'vendor', vendorKey, binaryName);

if (!fs.existsSync(binaryPath)) {
  console.error(`Loki binary missing: ${binaryPath}`);
  console.error('Reinstall @asudbring/loki-profile-manager or use the release script installer.');
  process.exit(1);
}

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: 'inherit',
  windowsHide: false,
});

child.on('error', (error) => {
  console.error(`Failed to start Loki binary: ${error.message}`);
  process.exit(1);
});

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code === null ? 1 : code);
});
