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
    set("asn", result.asn || "Not configured");
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

function probeSTUN(url) {
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
      pc.close();
      resolve({ state, endpoint });
    };
    const timeout = setTimeout(() => finish(observedSrflx ? "reachable" : "timeout / inconclusive"), 3200);
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
    set("udp-state", done === STUN_PROVIDERS.length ? "Completed" : "Checking " + done + "/10");
  };
  async function worker() {
    while (next < STUN_PROVIDERS.length) {
      const index = next++;
      results[index] = await probeSTUN(STUN_PROVIDERS[index][1]);
      render();
    }
  }
  await Promise.all([worker(), worker(), worker()]);
}

function checkCancel(signal) {
  if (signal.aborted) throw new DOMException("Test cancelled", "AbortError");
}

async function measureLatency(signal) {
  const times = [];
  for (let i = 0; i < 12; i++) {
    checkCancel(signal);
    const start = performance.now();
    const response = await fetch("/api/ping?sample=" + i, { cache: "no-store", signal });
    if (!response.ok) throw new Error("Ping probe failed");
    const elapsed = performance.now() - start;
    if (i > 1) times.push(elapsed); // Discard the first two samples.
  }
  const sorted = [...times].sort((a, b) => a - b);
  const median = (sorted[4] + sorted[5]) / 2;
  const jitter = times.slice(1).reduce((sum, value, i) => sum + Math.abs(value - times[i]), 0) / (times.length - 1);
  return { median, jitter };
}

function makePayload(size) {
  const data = new Uint8Array(size);
  for (let offset = 0; offset < data.length; offset += 65536) {
    crypto.getRandomValues(data.subarray(offset, Math.min(offset + 65536, data.length)));
  }
  return data;
}

function uploadOnce(payload, signal, addBytes) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/upload");
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    let last = 0;
    const onAbort = () => xhr.abort();
    signal.addEventListener("abort", onAbort, { once: true });
    const cleanup = () => signal.removeEventListener("abort", onAbort);
    xhr.upload.onprogress = (event) => {
      const delta = Math.max(0, event.loaded - last);
      last = event.loaded;
      addBytes(delta);
    };
    xhr.onload = () => {
      cleanup();
      if (xhr.status !== 200) {
        reject(new Error("Upload returned HTTP " + xhr.status));
        return;
      }
      addBytes(Math.max(0, payload.byteLength - last));
      resolve();
    };
    xhr.onerror = () => { cleanup(); reject(new Error("Upload connection failed")); };
    xhr.onabort = () => { cleanup(); reject(new DOMException("Upload cancelled", "AbortError")); };
    xhr.send(payload);
    if (signal.aborted) xhr.abort();
  });
}

async function timedTransfer(kind, streams, seconds, parentSignal) {
  const controller = new AbortController();
  const onAbort = () => controller.abort();
  parentSignal.addEventListener("abort", onAbort, { once: true });
  const end = performance.now() + seconds * 1000;
  const timer = setTimeout(() => controller.abort(), seconds * 1000);
  let bytes = 0;
  let lastBytes = 0;
  let errors = 0;
  const interval = setInterval(() => {
    const delta = bytes - lastBytes;
    lastBytes = bytes;
    set("live-speed", (delta * 8 / 250000).toFixed(2));
  }, 250);
  const payload = kind === "upload" ? makePayload(testConfig.uploadRequestBytes) : null;
  async function worker() {
    while (performance.now() < end && !controller.signal.aborted) {
      try {
        if (kind === "download") {
          const response = await fetch("/api/download?bytes=" + testConfig.downloadRequestBytes,
            { cache: "no-store", signal: controller.signal });
          if (!response.ok || !response.body) throw new Error("Download failed");
          const reader = response.body.getReader();
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            bytes += value.byteLength;
          }
        } else {
          await uploadOnce(payload, controller.signal, (delta) => { bytes += delta; });
        }
      } catch (error) {
        if (controller.signal.aborted) return;
        errors++;
        if (errors >= 3) return;
      }
    }
  }
  try {
    await Promise.all(Array.from({ length: streams }, () => worker()));
  } finally {
    clearInterval(interval);
    clearTimeout(timer);
    parentSignal.removeEventListener("abort", onAbort);
    controller.abort();
  }
  checkCancel(parentSignal);
  if (bytes === 0) throw new Error(kind + " transfer did not return any data");
  return bytes * 8 / (seconds * 1000000);
}

async function startTest() {
  if (currentTest) return;
  const controller = new AbortController();
  currentTest = controller;
  $("start").disabled = true;
  $("stop").disabled = false;
  set("ping", "—"); set("jitter", "—"); set("download", "—"); set("upload", "—");
  set("live-speed", "0.00");
  const streams = Number($("streams").value);
  try {
    set("phase", "Measuring idle HTTP latency...");
    const latency = await measureLatency(controller.signal);
    set("ping", fmt(latency.median));
    set("jitter", fmt(latency.jitter));
    set("phase", "Measuring download with " + streams + " stream(s)...");
    const download = await timedTransfer("download", streams, testConfig.durationSeconds, controller.signal);
    set("download", fmt(download));
    set("phase", "Measuring upload with " + streams + " stream(s)...");
    const upload = await timedTransfer("upload", streams, testConfig.durationSeconds, controller.signal);
    set("upload", fmt(upload));
    set("phase", "Test completed");
    set("live-speed", fmt(upload));
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

getJSON("/api/config").then((config) => { testConfig = { ...testConfig, ...config }; }).catch(() => {});
void checkIP();
void checkTCP();
void checkSTUN();
