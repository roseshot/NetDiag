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
let stunController = null;
let stunRunning = false;
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
    NetDiagASN.render(result);
    set("ip-state", "Completed");
  } catch {
    NetDiagASN.render(null);
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
  if (stunRunning || currentTest) return;
  if (!ENABLE_PUBLIC_STUN) {
    set("udp-state", "Not enabled");
    $("stun-start").disabled = true;
    return;
  }
  if (typeof RTCPeerConnection !== "function") {
    set("udp-state", "WebRTC unavailable");
    return;
  }

  const controller = new AbortController();
  stunController = controller;
  stunRunning = true;
  $("stun-start").disabled = true;
  $("stun-results").replaceChildren();
  set("udp-endpoint", "—");
  set("stun-count", "0 / " + STUN_PROVIDERS.length);
  set("udp-state", "Checking");

  let next = 0;
  const results = new Array(STUN_PROVIDERS.length);
  const list = $("stun-results");
  const render = () => {
    const success = results.filter((entry) => entry && entry.state === "reachable").length;
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
    const mapped = results.find((entry) => entry && entry.endpoint);
    if (mapped) set("udp-endpoint", mapped.endpoint);
    set("udp-state", controller.signal.aborted ? "Paused by speed test" :
      (done === STUN_PROVIDERS.length ? "Completed" : "Checking " + done + "/" + STUN_PROVIDERS.length));
  };
  async function worker() {
    while (!controller.signal.aborted && next < STUN_PROVIDERS.length) {
      const index = next++;
      results[index] = await probeSTUN(STUN_PROVIDERS[index][1], controller.signal);
      render();
    }
  }
  try {
    await Promise.all([worker(), worker(), worker()]);
  } finally {
    if (controller.signal.aborted) set("udp-state", "Paused by speed test");
    if (stunController === controller) stunController = null;
    stunRunning = false;
    $("stun-start").disabled = Boolean(currentTest);
    $("stun-start").textContent = "Run UDP check again";
  }
}

function checkCancel(signal) {
  if (signal.aborted) throw new DOMException("Test cancelled", "AbortError");
}

const TEST_PHASES = ["latency", "download", "upload"];
function showTestPhase(phase) {
  const activeIndex = TEST_PHASES.indexOf(phase);
  for (const item of document.querySelectorAll("[data-test-phase]")) {
    const index = TEST_PHASES.indexOf(item.dataset.testPhase);
    item.dataset.state = activeIndex < 0 ? "complete" :
      (index < activeIndex ? "complete" : index === activeIndex ? "active" : "pending");
    if (index === activeIndex) item.setAttribute("aria-current", "step");
    else item.removeAttribute("aria-current");
  }
}
function updateTestProgress(phase, fraction) {
  const value = Math.max(0, Math.min(1, fraction));
  const ranges = { latency: [0, 20], download: [20, 60], upload: [60, 100] };
  const range = ranges[phase];
  if (range) $("test-progress").value = range[0] + value * (range[1] - range[0]);
}
function stopTestProgress() {
  for (const item of document.querySelectorAll("[data-test-phase]")) {
    if (item.dataset.state === "active") item.dataset.state = "stopped";
    item.removeAttribute("aria-current");
  }
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
    updateTestProgress("latency", (i + 1) / 12);
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
function uploadOnce(payload, signal, onProgress, windowToken) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/upload");
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    if (windowToken) xhr.setRequestHeader("X-NetDiag-Window", windowToken);
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

// A TCP_INFO observation is optional and never prevents throughput tests.
// Read the summary only after transfers settle; a pending result is not zero.
async function beginTCPWindow(signal) {
  try {
    const created = await getJSON("/api/diag/tcp/window", { method: "POST", signal });
    return /^[A-Z2-7]{39}$/.test(created.token || "") ? created.token : null;
  } catch {
    return null;
  }
}

async function readTCPWindow(token, signal) {
  if (!token || signal.aborted) return null;
  for (let i = 0; i < 6; i++) {
    try {
      const result = await getJSON("/api/diag/tcp/window?token=" + encodeURIComponent(token), { signal });
      if (result.status !== "pending") return result;
    } catch {
      return null;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  return null;
}

function showTCPWindow(kind, result) {
  const element = kind === "download" ? "tcp-download-delta" : "tcp-upload-delta";
  if (result?.status === "completed" && Number.isSafeInteger(result.retransmissions)) {
    set(element, String(result.retransmissions) + " (observed on " + result.connections + " socket(s))");
  } else {
    set(element, "Unavailable");
  }
}

// Each phase uses a common deadline. The upload result intentionally excludes
// bytes from incomplete/aborted requests, so it can understate link capacity.
async function timedTransfer(kind, streams, seconds, parentSignal) {
  checkCancel(parentSignal);
  const windowToken = await beginTCPWindow(parentSignal);
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
      updateTestProgress(kind, (now - started) / (seconds * 1000));
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
            headers: windowToken ? { "X-NetDiag-Window": windowToken } : {},
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
          }, windowToken);
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
  updateTestProgress(kind, 1);
  const tcp = await readTCPWindow(windowToken, parentSignal);
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
    latency,
    tcp
  };
}

function showLoadedLatency(kind, idle, loaded) {
  const added = NetDiagMetrics.addedLatency(idle.medianMs, loaded.medianMs);
  set(kind + "-loaded", fmt(added));
  set(kind + "-p95", fmt(loaded.p95Ms));
}

async function startTest() {
  if (currentTest) return;
  stunController?.abort(); // Suspend external STUN probing before the bandwidth test.
  const controller = new AbortController();
  currentTest = controller;
  $("start").disabled = true;
  $("stop").disabled = false;
  $("streams").disabled = true;
  $("stun-start").disabled = true;
  $("test-status").classList.remove("muted");
  set("test-status", "Running");
  set("result-summary", "Testing your connection. Results will appear as each stage finishes.");
  $("test-progress").value = 0;
  for (const id of ["ping", "jitter", "download", "upload", "download-loaded",
    "upload-loaded", "download-p95", "upload-p95",
    "tcp-download-delta", "tcp-upload-delta"]) set(id, "—");
  set("live-speed", "—");
  set("speed-label", "Live speed");
  const streams = Number($("streams").value);
  let completed = false;
  try {
    showTestPhase("latency");
    set("phase", "1 of 3 · Measuring idle HTTP latency...");
    const idle = await measureLatency(controller.signal);
    set("ping", fmt(idle.medianMs));
    set("jitter", fmt(idle.jitterMs));

    showTestPhase("download");
    set("phase", "2 of 3 · Measuring download speed and loaded latency...");
    const download = await timedTransfer("download", streams, testConfig.durationSeconds, controller.signal);
    set("download", fmt(download.mbps));
    showLoadedLatency("download", idle, download.latency);
    showTCPWindow("download", download.tcp);

    showTestPhase("upload");
    set("phase", "3 of 3 · Measuring upload speed and loaded latency...");
    const upload = await timedTransfer("upload", streams, testConfig.durationSeconds, controller.signal);
    set("upload", fmt(upload.mbps));
    showLoadedLatency("upload", idle, upload.latency);
    showTCPWindow("upload", upload.tcp);

    showTestPhase(null);
    $("test-progress").value = 100;
    set("speed-label", "Download speed");
    set("live-speed", fmt(download.mbps));
    set("test-status", "Complete");
    set("phase", "Test completed · Upload rate includes only server-confirmed requests.");
    set("result-summary", NetDiagUX.describe({
      downloadMbps: download.mbps,
      uploadMbps: upload.mbps,
      latencyMs: idle.medianMs,
      downloadAddedMs: NetDiagMetrics.addedLatency(idle.medianMs, download.latency.medianMs),
      uploadAddedMs: NetDiagMetrics.addedLatency(idle.medianMs, upload.latency.medianMs),
      streams
    }));
    completed = true;
  } catch (error) {
    stopTestProgress();
    const cancelled = error.name === "AbortError";
    set("test-status", cancelled ? "Stopped" : "Incomplete");
    set("phase", cancelled ? "Test stopped. Partial measurements are not a full result." :
      "Test failed: " + error.message);
    set("speed-label", "Speed test");
    set("live-speed", "—");
    set("result-summary", cancelled ?
      "You stopped the test before all stages completed. Run it again for comparable results." :
      "The test could not finish. Check your connection and run it again. Any values shown are partial.");
  } finally {
    if (!completed) $("test-status").classList.add("muted");
    currentTest = null;
    $("start").disabled = false;
    $("start").textContent = "Run test again";
    $("stop").disabled = true;
    $("streams").disabled = false;
    if (!stunRunning) $("stun-start").disabled = false;
  }
}

$("start").addEventListener("click", startTest);
$("stop").addEventListener("click", () => currentTest?.abort());

// An independent operator-provided endpoint is required for each address family.
// The Go endpoint returns HTTP 409 if the request arrives on the wrong family.
async function probeAddressFamily(family, origin) {
  if (!origin) {
    set(family + "-state", "Not enabled");
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
  dnsEnabled = config.dnsResolvers === "available";
  $("dns-start").disabled = !dnsEnabled;
  set("dns-state", dnsEnabled ? "Ready" : "Not enabled");
  if (dnsEnabled) set("dns-message", "Click Check DNS to request an opt-in resolver observation.");
  const origins = config.probeOrigins || {};
  void Promise.all([
    probeAddressFamily("ipv4", origins.ipv4),
    probeAddressFamily("ipv6", origins.ipv6)
  ]);
}).catch(() => {
  set("ipv4-state", "Configuration unavailable");
  set("ipv6-state", "Configuration unavailable");
});
let dnsRunning = false;
const dnsProbeImage = { current: null };

async function checkDNS() {
  if (dnsRunning) return;
  dnsRunning = true;
  $("dns-start").disabled = true;
  $("dns-results").replaceChildren();
  set("dns-state", "Checking");
  set("dns-message", "Requesting a unique DNS challenge...");
  try {
    const created = await getJSON("/api/dns/start", { method: "POST" });
    if (typeof created.token !== "string" || !/^([a-z2-7]{39})$/.test(created.token) ||
        typeof created.probeURL !== "string" || !created.probeURL.startsWith("https://")) {
      throw new Error("Unexpected DNS challenge response");
    }
    const probe = new Image();
    dnsProbeImage.current = probe;
    probe.referrerPolicy = "no-referrer";
    probe.src = created.probeURL;
    set("dns-message", "Waiting for recursive DNS servers to contact the observer...");
    let matched = [];
    for (let i = 0; i < 10; i++) {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      const result = await getJSON("/api/dns/result?token=" + encodeURIComponent(created.token));
      if (Array.isArray(result.resolvers)) {
        matched = result.resolvers;
        if (matched.length) break;
      }
    }
    if (!matched.length) {
      set("dns-state", "Inconclusive");
      set("dns-message", "No matching DNS query was observed. Network policies, DNS caching, or browser restrictions can prevent a result.");
      return;
    }
    for (const resolver of matched) {
      const item = document.createElement("li");
      const ip = document.createElement("strong");
      ip.textContent = String(resolver.ip || "Unknown resolver");
      item.append(ip);
      if (resolver.asn && resolver.organization) {
        const meta = document.createElement("span");
        meta.textContent = "AS" + resolver.asn + " · " + resolver.organization;
        item.append(meta);
      }
      $("dns-results").append(item);
    }
    set("dns-state", "Observed");
    set("dns-message", "Observed authoritative DNS peers. These addresses may belong to forwarders or shared recursive services.");
  } catch (error) {
    set("dns-state", "Unavailable");
    set("dns-message", "DNS discovery failed: " + error.message);
  } finally {
    dnsProbeImage.current = null;
    dnsRunning = false;
    $("dns-start").disabled = !dnsEnabled;
  }
}

let dnsEnabled = false;
$("dns-start").addEventListener("click", () => { void checkDNS(); });

void checkIP();
void checkTCP();
$("stun-start").addEventListener("click", () => { void checkSTUN(); });

// Reflect MMDB hot reload on an already-open page without reloading it.
setInterval(() => {
  if (!document.hidden) void checkIP();
}, 15000);
