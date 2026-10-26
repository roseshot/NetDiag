"use strict";

const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const context = {};
vm.runInNewContext(readFileSync(path.join(__dirname, "../web/assets/metrics.js"), "utf8"), context);
const metrics = context.NetDiagMetrics;

test("latency percentiles and jitter use valid samples", () => {
  assert.equal(metrics.percentile([30, 10, 20], 0.5), 20);
  assert.equal(metrics.percentile([30, 10, 20], 0.95), 29);
  assert.equal(metrics.jitter([10, 20, 15]), 7.5);
  const result = metrics.summarizeLatency([30, 10, 20, -1, NaN]);
  assert.equal(result.samples, 3);
  assert.equal(result.medianMs, 20);
  assert.equal(result.p95Ms, 29);
  assert.equal(result.jitterMs, 15);
});

test("unavailable samples do not become artificial zero latency", () => {
  assert.equal(metrics.percentile([], 0.5), null);
  const result = metrics.summarizeLatency([]);
  assert.equal(result.medianMs, null);
  assert.equal(result.p95Ms, null);
  assert.equal(result.jitterMs, null);
  assert.equal(metrics.addedLatency(null, 12), null);
});

test("loaded latency is relative to the idle baseline", () => {
  assert.equal(metrics.addedLatency(20, 80), 60);
  assert.equal(metrics.addedLatency(20, 12), 0);
});

test("throughput converts measured bytes and actual elapsed time to Mbps", () => {
  assert.equal(metrics.throughputMbps(125000, 1000), 1);
  assert.equal(metrics.throughputMbps(125000, 500), 2);
  assert.equal(metrics.throughputMbps(125000, 0), 0);
});
