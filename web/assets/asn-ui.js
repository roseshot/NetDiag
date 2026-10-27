"use strict";

(function (scope) {
  function model(result) {
    const valid = result && result.asnStatus === "completed" &&
      Number.isSafeInteger(result.asn) && result.asn > 0;

    if (!valid) {
      return {
        visible: false,
        asn: "—",
        organization: "—",
        domain: "—",
        network: "—",
        source: ""
      };
    }

    const source = typeof result.asnSource === "string" && result.asnSource.trim() ?
      result.asnSource.trim() : "Unknown provider";
    const metadata = ["Data source: " + source];
    if (result.asnDatabaseType) metadata.push("Database: " + result.asnDatabaseType);
    if (result.asnDatabaseBuiltAt) metadata.push("Built: " + result.asnDatabaseBuiltAt);
    if (result.asnReloadStatus === "stale") {
      metadata.push("Using the last verified snapshot; database reload failed");
    }

    return {
      visible: true,
      asn: "AS" + result.asn,
      organization: result.asnOrganization || "Unknown organization",
      domain: result.asnDomain || "—",
      network: result.asnNetwork || "—",
      source: metadata.join(" · ")
    };
  }

  function render(result) {
    const data = model(result);
    for (const [id, value] of Object.entries({
      asn: data.asn,
      "asn-organization": data.organization,
      "asn-domain": data.domain,
      "asn-network": data.network
    })) {
      document.getElementById(id).textContent = value;
    }

    for (const row of document.querySelectorAll(".asn-detail")) {
      row.hidden = !data.visible;
    }
    document.getElementById("asn-note").hidden = !data.visible;
    document.getElementById("asn-source-tooltip").textContent = data.source;
    const button = document.getElementById("asn-info-button");
    button.title = data.source;
    button.setAttribute("aria-label", data.source || "ASN data source");
  }

  scope.NetDiagASN = Object.freeze({ model, render });
})(globalThis);
