"use strict";

const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const script = readFileSync(path.join(__dirname, "../web/assets/asn-ui.js"), "utf8");

function createView() {
  const ids = ["asn", "asn-organization", "asn-domain", "asn-network",
    "asn-note", "asn-source-tooltip", "asn-info-button"];
  const elements = Object.fromEntries(ids.map((id) => [id, {
    textContent: "",
    title: "",
    hidden: id === "asn-note",
    attributes: {},
    setAttribute(key, value) { this.attributes[key] = value; }
  }]));
  const rows = Array.from({ length: 4 }, () => ({ hidden: true }));
  const context = {
    document: {
      getElementById(id) { return elements[id]; },
      querySelectorAll(selector) {
        assert.equal(selector, ".asn-detail");
        return rows;
      }
    }
  };
  vm.runInNewContext(script, context);
  return { elements, rows, renderer: context.NetDiagASN };
}

test("missing MMDB, missing record, and invalid ASN hide every ASN row", () => {
  const view = createView();
  const states = [
    null,
    { asnStatus: "not_configured" },
    { asnStatus: "unavailable" },
    { asnStatus: "not_found", asnSource: "IPinfo" },
    { asnStatus: "not_applicable" },
    { asnStatus: "completed", asn: null, asnSource: "IPinfo" },
    { asnStatus: "completed", asn: 0, asnSource: "IPinfo" }
  ];
  for (const result of states) {
    view.renderer.render(result);
    assert.ok(view.rows.every((row) => row.hidden));
    assert.equal(view.elements["asn-note"].hidden, true);
    assert.equal(view.elements["asn-source-tooltip"].textContent, "");
    assert.equal(view.elements["asn"].textContent, "—");
  }
});

test("valid ASN is displayed with provider attribution in accessible tooltip", () => {
  const view = createView();
  view.renderer.render({
    asnStatus: "completed",
    asn: 13335,
    asnOrganization: "CLOUDFLARENET",
    asnDomain: "cloudflare.com",
    asnNetwork: "1.1.1.0/24",
    asnSource: "IPinfo",
    asnDatabaseType: "ipinfo bundle_asn_asn.mmdb",
    asnDatabaseBuiltAt: "2026-10-27T00:00:00Z"
  });
  assert.ok(view.rows.every((row) => !row.hidden));
  assert.equal(view.elements["asn-note"].hidden, false);
  assert.equal(view.elements.asn.textContent, "AS13335");
  assert.equal(view.elements["asn-organization"].textContent, "CLOUDFLARENET");
  assert.equal(view.elements["asn-domain"].textContent, "cloudflare.com");
  const source = view.elements["asn-source-tooltip"].textContent;
  assert.match(source, /Data source: IPinfo/);
  assert.match(source, /Built:/);
  assert.equal(view.elements["asn-info-button"].attributes["aria-label"], source);
});

test("stale DB retains record but discloses stale source; removed data hides it", () => {
  const view = createView();
  view.renderer.render({
    asnStatus: "completed",
    asn: 64512,
    asnSource: "MaxMind GeoLite2 ASN",
    asnReloadStatus: "stale"
  });
  assert.ok(view.rows.every((row) => !row.hidden));
  assert.match(view.elements["asn-source-tooltip"].textContent, /last verified snapshot/);
  view.renderer.render({ asnStatus: "not_configured" });
  assert.ok(view.rows.every((row) => row.hidden));
  assert.equal(view.elements["asn-source-tooltip"].textContent, "");
});
