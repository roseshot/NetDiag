"use strict";

(function (scope) {
  // Linear interpolation between order statistics, in milliseconds.
  function percentile(samples, fraction) {
    if (!Array.isArray(samples) || samples.length === 0) return null;
    if (!Number.isFinite(fraction) || fraction < 0 || fraction > 1) return null;
    const sorted = samples.filter((value) => Number.isFinite(value) && value >= 0).sort((a, b) => a - b);
    if (sorted.length === 0) return null;
    const position = (sorted.length - 1) * fraction;
    const lower = Math.floor(position);
    const weight = position - lower;
    return sorted[lower] + (sorted[Math.min(lower + 1, sorted.length - 1)] - sorted[lower]) * weight;
  }

  function jitter(samples) {
    if (!Array.isArray(samples) || samples.length < 2) return null;
    let sum = 0;
    for (let i = 1; i < samples.length; i++) {
      if (!Number.isFinite(samples[i]) || !Number.isFinite(samples[i - 1])) return null;
      sum += Math.abs(samples[i] - samples[i - 1]);
    }
    return sum / (samples.length - 1);
  }

  function summarizeLatency(samples) {
    const valid = samples.filter((value) => Number.isFinite(value) && value >= 0);
    return {
      samples: valid.length,
      medianMs: percentile(valid, 0.5),
      p95Ms: percentile(valid, 0.95),
      jitterMs: jitter(valid)
    };
  }

  function addedLatency(idleMs, loadedMs) {
    if (!Number.isFinite(idleMs) || !Number.isFinite(loadedMs)) return null;
    return Math.max(0, loadedMs - idleMs);
  }

  function throughputMbps(bytes, elapsedMs) {
    if (!Number.isFinite(bytes) || bytes < 0 || !Number.isFinite(elapsedMs) || elapsedMs <= 0) {
      return 0;
    }
    return bytes * 8 / (elapsedMs * 1000);
  }

  scope.NetDiagMetrics = Object.freeze({
    percentile,
    jitter,
    summarizeLatency,
    addedLatency,
    throughputMbps
  });
})(globalThis);
