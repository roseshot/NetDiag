"use strict";

const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const code = readFileSync(path.join(__dirname, "../web/assets/ux.js"), "utf8");
const scope = {};
vm.runInNewContext(code, scope);
const describe = scope.NetDiagUX.describe;

test("unavailable and partial results do not suggest a valid speed test", () => {
  assert.match(describe({}), /No complete speed test/);
  assert.match(describe({ downloadMbps: 50, uploadMbps: NaN, latencyMs: 10 }), /No complete/);
});

test("the summary describes concrete measurements and their limits", () => {
  const result = describe({
    downloadMbps: 88.4, uploadMbps: 22.2, latencyMs: 15.8,
    downloadAddedMs: 8, uploadAddedMs: 18, streams: 4
  });
  assert.match(result, /88\.4 Mbps down/);
  assert.match(result, /15\.8 ms idle HTTP latency/);
  assert.match(result, /4 concurrent request/);
  assert.match(result, /one HTTP measurement path/i);
});

test("large loaded latency is reported before throughput guidance", () => {
  const result = describe({
    downloadMbps: 100, uploadMbps: 5, latencyMs: 25,
    downloadAddedMs: 120, uploadAddedMs: 2, streams: 1
  });
  assert.match(result, /increased considerably under load/);
});

test("low transfer rates provide cautiously worded guidance", () => {
  const result = describe({
    downloadMbps: 8, uploadMbps: 2.5, latencyMs: 20,
    downloadAddedMs: 10, uploadAddedMs: 20, streams: 4
  });
  assert.match(result, /may constrain/);
});

test("the homepage starts unmeasured and scripts load before the app", () => {
  const html = readFileSync(path.join(__dirname, "../web/assets/index.html"), "utf8");
  assert.match(html, /id="live-speed">—<\/span>/);
  assert.match(html, /id="test-status"[^>]*>Not tested/);
  assert.match(html, /id="stun-start"[^>]*>Run UDP check/);
  assert.match(html, /<script src="\/ux\.js" defer><\/script>/);
});

test("third-party STUN checks require an explicit click", () => {
  const app = readFileSync(path.join(__dirname, "../web/assets/app.js"), "utf8");
  const callSites = app.match(/checkSTUN\(\)/g) || [];
  assert.equal(callSites.length, 2); // declaration and explicit button handler
  assert.match(app, /\$\("stun-start"\)\.addEventListener\("click"/);
});

test("all fixed JS element references exist in the homepage", () => {
  const html = readFileSync(path.join(__dirname, "../web/assets/index.html"), "utf8");
  const app = readFileSync(path.join(__dirname, "../web/assets/app.js"), "utf8");
  const ids = new Set([...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]));
  const references = [
    ...app.matchAll(/\$\("([^"]+)"\)/g),
    ...app.matchAll(/set\("([^"]+)"/g)
  ];
  for (const match of references) assert.ok(ids.has(match[1]), "Missing ID: " + match[1]);
});
