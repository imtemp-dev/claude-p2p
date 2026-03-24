#!/usr/bin/env node

const { execFileSync } = require("child_process");
const path = require("path");

const BIN_NAME = process.platform === "win32" ? "claude-p2p.exe" : "claude-p2p";
const BIN_PATH = path.join(__dirname, BIN_NAME);

try {
  execFileSync(BIN_PATH, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  if (err.status !== null) {
    process.exit(err.status);
  }
  throw err;
}
