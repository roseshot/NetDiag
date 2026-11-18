"use strict";

// Human-readable interpretation. These are guidance thresholds, not a quality
// certification or a comparison with the user's subscribed line rate.
(function (scope) {
  function describe({ downloadMbps, uploadMbps, latencyMs, downloadAddedMs, uploadAddedMs, streams }) {
    if (![downloadMbps, uploadMbps, latencyMs].every((value) =>
      Number.isFinite(value) && value >= 0)) {
      return "No complete speed test is available. Run the test again to compare download, upload and latency.";
    }
    const details = [
      "Measured " + downloadMbps.toFixed(1) + " Mbps down, " +
      uploadMbps.toFixed(1) + " Mbps up, and " +
      latencyMs.toFixed(1) + " ms idle HTTP latency using " +
      streams + " concurrent request(s)."
    ];
    const added = Math.max(
      Number.isFinite(downloadAddedMs) ? downloadAddedMs : 0,
      Number.isFinite(uploadAddedMs) ? uploadAddedMs : 0
    );
    if (added >= 100) {
      details.push("HTTP latency increased considerably under load; calls and games may feel less responsive while large transfers run.");
    } else if (downloadMbps < 10 || uploadMbps < 3) {
      details.push("The observed transfer rate may constrain simultaneous streaming or large file transfers.");
    } else {
      details.push("Compare these results with your plan and repeat at different times to spot changes.");
    }
    details.push("This is one HTTP measurement path, not a guaranteed full-line capacity.");
    return details.join(" ");
  }

  scope.NetDiagUX = Object.freeze({ describe });
})(globalThis);
