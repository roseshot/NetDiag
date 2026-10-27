"use strict";

const $ = (id) => document.getElementById(id);
const set = (id, value) => { $(id).textContent = String(value); };
const fmt = (value) => Number.isFinite(value) ? value.toFixed(1) : "—";
const STUN_PROVIDERS = [
  ["Google", "stun:stun.l.google.com:19302"],
  ["Google 1", "stun:stun1.l.google.com:19302"],
  ["Google 2", "stun:stun2.l.google.com:19302"],
  ["Google 3", "stun:stun3.l.google.com:19302"],
  ["Google 4", "stun:stun4.l.google.com:19302"],
  ["Cloudflare", "stun:stun.cloudflare.com:3478"],
  ["Nextcloud", "stun:stun.nextcloud.com:443"],
  ["Linphone", "stun:stun.linphone.org:3478"],
  ["FreeSWITCH", "stun:stun.freeswitch.org:3478"],
  ["Sonetel", "stun:stun.sonetel.net:3478"],
];
// Review STUN provider terms and privacy implications before public deployment.
const ENABLE_PUBLIC_STUN = true;
let currentTest = null;
const stunController = new AbortController();
let testConfig = { durationSeconds: 8, downloadRequestBytes: 4194304, uploadRequestBytes: 4194304 };

async function getJSON(path, options = {}) {
  const response = await fetch(path, { cache: "no-store", ...options });
  if (!response.ok) throw new Error("HTTP " + response.status);
  return response.json();
}

async function checkIP() {
  try {
    const result = await getJSON("/api/ip");
    set("public-ip", result.ip);
    set("ip-family", result.family);
    const asnLabel = {
      not_configured: "Not configured",
      unavailable: "Database unavailable",
      not_found: "No ASN match",
      not_applicable: "Not applicable"
    };
    set("asn", result.asnStatus === "completed" ? "AS" + result.asn :
      (asnLabel[result.asnStatus] || "Unavailable"));
    set("asn-organization", result.asnStatus === "completed" ?
      (result.asnOrganization || "Unknown organization") : "—");
    set("asn-network", result.asnStatus === "completed" ?
      (result.asnNetwork || "—") : "—");
    set("asn-domain", result.asnStatus === "completed" ?
      (result.asnDomain || "—") : "—");
    set("asn-source", result.asnSource ?
      result.asnSource + (result.asnReloadStatus === "stale" ? " (stale)" : "") :
      "Not configured");
    set("ip-state", "Completed");
  } catch {
    set("ip-state", "Unavailable");
  }
}

async function checkTCP() {
  try {
    const result = await getJSON("/api/diag/tcp");
    if (result.status !== "completed") throw new Error("TCP_INFO unavailable");
    set("tcp-rtt", fmt(result.rttMs) + " ms");
    set("tcp-retrans", String(result.totalRetransmissions) + " (lifetime)");
    set("tcp-mss", result.sendMssBytes + " B");
    set("tcp-receive-mss", result.receiveMssBytes + " B");
    set("tcp-pmtu", result.pathMtuBytes + " B");
    set("tcp-rttvar", fmt(result.rttVarianceMs) + " ms");
    set("tcp-state", "Server-side");
  } catch {
    set("tcp-state", "Unavailable");
  }
}

function probeSTUN(url, signal) {
  return new Promise((resolve) => {
    let pc;
    try {
      pc = new RTCPeerConnection({ iceServers: [{ urls: url }], iceTransportPolicy: "all" });
    } catch {
      resolve({ state: "blocked" });
      return;
    }
    let settled = false;
    let endpoint = null;
    let observedSrflx = false;
    const finish = (state) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      signal.removeEventListener("abort", abortProbe);
      pc.close();
      resolve({ state, endpoint });
    };
    const timeout = setTimeout(() => finish(observedSrflx ? "reachable" : "timeout / inconclusive"), 3200);
    const abortProbe = () => finish("paused");
    signal.addEventListener("abort", abortProbe, { once: true });
    if (signal.aborted) {
      finish("paused");
      return;
    }
    pc.onicecandidate = (event) => {
      if (!event.candidate) return;
      const candidate = event.candidate;
      const text = candidate.candidate || "";
      if (candidate.type === "srflx" || /\btyp srflx\b/.test(text)) {
        observedSrflx = true;
        const match = text.match(/\s(\S+)\s(\d+)\s+typ srflx\b/);
        endpoint = candidate.address && candidate.port ? candidate.address + ":" + candidate.port :
          (match ? match[1] + ":" + match[2] : null);
      }
    };
    pc.onicegatheringstatechange = () => {
      if (pc.iceGatheringState === "complete") finish(observedSrflx ? "reachable" : "inconclusive");
    };
    try {
      pc.createDataChannel("diagnostics");
      pc.createOffer().then((offer) => pc.setLocalDescription(offer)).catch(() => finish("blocked"));
    } catch {
      finish("blocked");
    }
  });
}

async function checkSTUN() {
  if (!ENABLE_PUBLIC_STUN) {
    set("udp-state", "Disabled");
    return;
  }
  if (typeof RTCPeerConnection !== "function") {
    set("udp-state", "WebRTC unavailable");
    return;
  }
  let next = 0;
  const results = new Array(STUN_PROVIDERS.length);
  const list = $("stun-results");
  const render = () => {
    const success = results.filter((r) => r && r.state === "reachable").length;
    const done = results.filter(Boolean).length;
    set("stun-count", success + " / " + STUN_PROVIDERS.length);
    list.replaceChildren();
    results.forEach((result, i) => {
      if (!result) return;
      const item = document.createElement("li");
      const name = document.createElement("span");
      const state = document.createElement("span");
      name.textContent = STUN_PROVIDERS[i][0];
      state.textContent = result.state;
      item.append(name, state);
      list.append(item);
    });
    const mapped = results.find((r) => r && r.endpoint);
    if (mapped) set("udp-endpoint", mapped.endpoint);
    set("udp-state", stunController.signal.aborted ? "Paused for speed test" :
      (done === STUN_PROVIDERS.length ? "Completed" : "Checking " + done + "/10"));
  };
  async function worker() {
    while (!stunController.signal.aborted && next < STUN_PROVIDERS.length) {
      const index = next++;
      results[index] = await probeSTUN(STUN_PROVIDERS[index][1], stunController.signal);
      render();
    }
  }
  await Promise.all([worker(), worker(), worker()]);
}

function checkCancel(signal) {
  if (signal.aborted) throw new DOMException("Test cancelled", "AbortError");
}

let pingSequence = 0;

async function pingSample(signal) {
  const controller = new AbortController();
  const abort = () => controller.abort();
  signal.addEventListener("abort", abort, { once: true });
  const timeout = setTimeout(() => controller.abort(), 2000);
  const start = performance.now();
  try {
    const response = await fetch("/api/ping?sample=" + (++pingSequence), {
      cache: "no-store",
      signal: controller.signal
    });
    if (!response.ok) throw new Error("HTTP latency probe failed");
    return performance.now() - start;
  } finally {
    clearTimeout(timeout);
    signal.removeEventListener("abort", abort);
  }
}

async function measureLatency(signal) {
  const samples = [];
  for (let i = 0; i < 12; i++) {
    checkCancel(signal);
    const ms = await pingSample(signal);
    if (i >= 2) samples.push(ms);
  }
  return NetDiagMetrics.summarizeLatency(samples);
}

function delayUntilNextProbe(signal) {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const stop = () => {
      clearTimeout(timer);
      resolve();
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", stop);
      resolve();
    }, 250);
    signal.addEventListener("abort", stop, { once: true });
  });
}

async function measureLoadedLatency(signal, deadline) {
  const samples = [];
  while (!signal.aborted && performance.now() < deadline) {
    try {
      samples.push(await pingSample(signal));
    } catch {
      if (signal.aborted) break;
      // A timeout is not a successful RTT sample.
    }
    await delayUntilNextProbe(signal);
  }
  return NetDiagMetrics.summarizeLatency(samples);
}

function makePayload(size) {
  const data = new Uint8Array(size);
  for (let offset = 0; offset < data.length; offset += 65536) {
    crypto.getRandomValues(data.subarray(offset, Math.min(offset + 65536, data.length)));
  }
  return data;
}

// Only successful responses with an exact server byte acknowledgement
// contribute to the final upload result. Progress is for the live UI only.
function uploadOnce(payload, signal, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/upload");
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    let last = 0;
    const abort = () => xhr.abort();
    const cleanup = () => signal.removeEventListener("abort", abort);
    signal.addEventListener("abort", abort, { once: true });
    xhr.upload.onprogress = (event) => {
      const delta = Math.max(0, event.loaded - last);
      last = event.loaded;
      onProgress(delta);
    };
    xhr.onload = () => {
      cleanup();
      if (xhr.status !== 200) {
        reject(new Error("Upload returned HTTP " + xhr.status));
        return;
      }
      let received;
      try {
        received = JSON.parse(xhr.responseText).receivedBytes;
      } catch {
        reject(new Error("Invalid upload confirmation"));
        return;
      }
      if (received !== payload.byteLength) {
        reject(new Error("Upload byte confirmation mismatch"));
        return;
      }
      onProgress(Math.max(0, payload.byteLength - last));
      resolve(received);
    };
    xhr.onerror = () => { cleanup(); reject(new Error("Upload connection failed")); };
    xhr.onabort = () => { cleanup(); reject(new DOMException("Upload cancelled", "AbortError")); };
    xhr.send(payload);
    if (signal.aborted) xhr.abort();
  });
}

// Each phase uses a common deadline. The upload result intentionally excludes
// bytes from incomplete/aborted requests, so it can understate link capacity.
async function timedTransfer(kind, streams, seconds, parentSignal) {
  checkCancel(parentSignal);
  const controller = new AbortController();
  const cancel = () => controller.abort();
  parentSignal.addEventListener("abort", cancel, { once: true });
  const started = performance.now();
  const deadline = started + seconds * 1000;
  const timeout = setTimeout(() => controller.abort(), seconds * 1000);
  let confirmedBytes = 0;
  let progressBytes = 0;
  let lastProgress = 0;
  let lastTick = started;
  let completedRequests = 0;
  let fatalError = null;
  const interval = setInterval(() => {
    const now = performance.now();
    const deltaMs = now - lastTick;
    if (deltaMs > 0) {
      set("live-speed", NetDiagMetrics.throughputMbps(progressBytes - lastProgress, deltaMs).toFixed(2));
    }
    lastProgress = progressBytes;
    lastTick = now;
  }, 250);
  const payload = kind === "upload" ? makePayload(testConfig.uploadRequestBytes) : null;
  const latencyTask = measureLoadedLatency(controller.signal, deadline);

  async function worker() {
    while (!controller.signal.aborted && performance.now() < deadline) {
      try {
        if (kind === "download") {
          const response = await fetch("/api/download?bytes=" + testConfig.downloadRequestBytes, {
            cache: "no-store",
            signal: controller.signal
          });
          if (!response.ok || !response.body) throw new Error("Download returned HTTP " + response.status);
          const reader = response.body.getReader();
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            confirmedBytes += value.byteLength;
            progressBytes += value.byteLength;
          }
        } else {
          confirmedBytes += await uploadOnce(payload, controller.signal, (delta) => {
            progressBytes += delta;
          });
        }
        completedRequests++;
      } catch (error) {
        if (controller.signal.aborted) break;
        fatalError = error;
        controller.abort();
        break;
      }
    }
  }

  let latency;
  let elapsedMs;
  try {
    await Promise.all(Array.from({ length: streams }, () => worker()));
  } finally {
    elapsedMs = performance.now() - started;
    controller.abort();
    latency = await latencyTask;
    clearTimeout(timeout);
    clearInterval(interval);
    parentSignal.removeEventListener("abort", cancel);
  }

  checkCancel(parentSignal);
  if (fatalError) throw fatalError;
  if (confirmedBytes <= 0 || (kind === "upload" && completedRequests === 0)) {
    throw new Error(kind === "upload" ?
      "No uploads completed within the measurement window" :
      "Download did not return any data");
  }
  return {
    mbps: NetDiagMetrics.throughputMbps(confirmedBytes, elapsedMs),
    confirmedBytes,
    elapsedMs,
    completedRequests,
    latency
  };
}

function showLoadedLatency(kind, idle, loaded) {
  const added = NetDiagMetrics.addedLatency(idle.medianMs, loaded.medianMs);
  set(kind + "-loaded", fmt(added));
  set(kind + "-p95", fmt(loaded.p95Ms));
}

async function startTest() {
  if (currentTest) return;
  stunController.abort(); // Stop third-party probes before throughput or loaded-latency tests.
  const controller = new AbortController();
  currentTest = controller;
  $("start").disabled = true;
  $("stop").disabled = false;
  for (const id of ["ping", "jitter", "download", "upload", "download-loaded",
    "upload-loaded", "download-p95", "upload-p95"]) set(id, "—");
  set("live-speed", "0.00");
  const streams = Number($("streams").value);
  try {
    set("phase", "Measuring idle HTTP latency...");
    const idle = await measureLatency(controller.signal);
    set("ping", fmt(idle.medianMs));
    set("jitter", fmt(idle.jitterMs));

    set("phase", "Measuring download and loaded latency with " + streams + " stream(s)...");
    const download = await timedTransfer("download", streams, testConfig.durationSeconds, controller.signal);
    set("download", fmt(download.mbps));
    showLoadedLatency("download", idle, download.latency);

    set("phase", "Measuring upload and loaded latency with " + streams + " stream(s)...");
    const upload = await timedTransfer("upload", streams, testConfig.durationSeconds, controller.signal);
    set("upload", fmt(upload.mbps));
    showLoadedLatency("upload", idle, upload.latency);

    set("phase", "Test completed · upload counts only server-confirmed requests");
    set("live-speed", fmt(upload.mbps));
  } catch (error) {
    set("phase", error.name === "AbortError" ? "Test cancelled" : "Test failed: " + error.message);
    set("live-speed", "0.00");
  } finally {
    currentTest = null;
    $("start").disabled = false;
    $("stop").disabled = true;
  }
}

$("start").addEventListener("click", startTest);
$("stop").addEventListener("click", () => currentTest?.abort());

// An independent operator-provided endpoint is required for each address family.
// The Go endpoint returns HTTP 409 if the request arrives on the wrong family.
async function probeAddressFamily(family, origin) {
  if (!origin) {
    set(family + "-state", "Not configured");
    return;
  }
  const requested = family === "ipv4" ? "4" : "6";
  const expected = family === "ipv4" ? "IPv4" : "IPv6";
  const samples = [];
  try {
    for (let i = 0; i < 4; i++) {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 4000);
      const started = performance.now();
      let result;
      try {
        const response = await fetch(origin + "/api/diag/address?family=" +
          requested + "&sample=" + i, {
          mode: "cors",
          credentials: "omit",
          cache: "no-store",
          signal: controller.signal
        });
        if (!response.ok) throw new Error("probe returned HTTP " + response.status);
        result = await response.json();
      } finally {
        clearTimeout(timeout);
      }
      if (result.status !== "completed" || result.family !== expected ||
          result.scope !== "direct_peer" || !result.ip) {
        throw new Error("unexpected address-family response");
      }
      if (i === 0) {
        set(family + "-address", result.ip);
      } else {
        samples.push(performance.now() - started);
      }
    }
    const summary = NetDiagMetrics.summarizeLatency(samples);
    set(family + "-rtt", fmt(summary.medianMs) + " ms");
    set(family + "-state", "Available");
  } catch {
    set(family + "-address", "—");
    set(family + "-rtt", "—");
    set(family + "-state", "Unavailable");
  }
}

getJSON("/api/config").then((config) => {
  testConfig = { ...testConfig, ...config };
  const origins = config.probeOrigins || {};
  void Promise.all([
    probeAddressFamily("ipv4", origins.ipv4),
    probeAddressFamily("ipv6", origins.ipv6)
  ]);
}).catch(() => {
  set("ipv4-state", "Configuration unavailable");
  set("ipv6-state", "Configuration unavailable");
});
void checkIP();
void checkTCP();
void checkSTUN();
