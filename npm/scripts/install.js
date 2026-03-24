#!/usr/bin/env node

const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");
const http = require("http");

const VERSION = require("../package.json").version;
const REPO = "imtemp-dev/claude-p2p";
const BIN_DIR = path.join(__dirname, "..", "bin");
const BIN_NAME = process.platform === "win32" ? "claude-p2p.exe" : "claude-p2p";
const BIN_PATH = path.join(BIN_DIR, BIN_NAME);

function getPlatform() {
  const platformMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };
  const os = platformMap[process.platform];
  const cpu = archMap[process.arch];
  if (!os || !cpu) {
    console.error(`Unsupported platform: ${process.platform}/${process.arch}`);
    process.exit(1);
  }
  return { os, cpu };
}

function download(url) {
  return new Promise((resolve, reject) => {
    const client = url.startsWith("https") ? https : http;
    client
      .get(url, { headers: { "User-Agent": "claude-p2p-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return download(res.headers.location).then(resolve).catch(reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`HTTP ${res.statusCode}: ${url}`));
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

async function main() {
  if (fs.existsSync(BIN_PATH)) {
    return;
  }

  const { os, cpu } = getPlatform();
  const ext = os === "windows" ? "zip" : "tar.gz";
  const asset = `claude-p2p_${os}_${cpu}.${ext}`;
  const url = `https://github.com/${REPO}/releases/download/v${VERSION}/${asset}`;

  console.log(`Downloading claude-p2p v${VERSION} for ${os}/${cpu}...`);

  const buffer = await download(url);
  fs.mkdirSync(BIN_DIR, { recursive: true });

  // Write archive to temp file and extract with system tools
  const tmpFile = path.join(BIN_DIR, `tmp.${ext}`);
  fs.writeFileSync(tmpFile, buffer);

  try {
    if (ext === "tar.gz") {
      execSync(`tar xzf "${tmpFile}" -C "${BIN_DIR}"`, { stdio: "ignore" });
    } else {
      execSync(`unzip -o "${tmpFile}" -d "${BIN_DIR}"`, { stdio: "ignore" });
    }
  } finally {
    fs.unlinkSync(tmpFile);
  }

  if (process.platform !== "win32") {
    fs.chmodSync(BIN_PATH, 0o755);
  }

  console.log(`claude-p2p v${VERSION} installed successfully.`);
}

main().catch((err) => {
  console.error(`Failed to install claude-p2p: ${err.message}`);
  process.exit(1);
});
