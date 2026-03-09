#!/usr/bin/env node

"use strict";

const { execFileSync } = require("child_process");
const path = require("path");
const os = require("os");

// Map Node.js platform/arch to npm package names
const PLATFORMS = {
  "darwin arm64": "@coingecko/cg-darwin-arm64",
  "darwin x64": "@coingecko/cg-darwin-x64",
  "linux arm64": "@coingecko/cg-linux-arm64",
  "linux x64": "@coingecko/cg-linux-x64",
  "win32 arm64": "@coingecko/cg-win32-arm64",
  "win32 x64": "@coingecko/cg-win32-x64",
};

function getBinaryPath() {
  const platform = os.platform();
  const arch = os.arch();
  const key = `${platform} ${arch}`;
  const pkg = PLATFORMS[key];

  if (!pkg) {
    throw new Error(
      `Unsupported platform: ${platform} ${arch}. ` +
        `Supported: ${Object.keys(PLATFORMS).join(", ")}`
    );
  }

  const binary = platform === "win32" ? "cg.exe" : "cg";

  try {
    // resolve the platform package from this package's location
    const pkgDir = path.dirname(require.resolve(`${pkg}/package.json`));
    return path.join(pkgDir, binary);
  } catch {
    throw new Error(
      `The platform package ${pkg} is not installed. ` +
        `This usually means your package manager excluded optional dependencies. ` +
        `Try reinstalling with: npm install @coingecko/cg`
    );
  }
}

const binary = getBinaryPath();

try {
  execFileSync(binary, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  if (err.status !== undefined) {
    process.exit(err.status);
  }
  throw err;
}
