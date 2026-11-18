"use strict";

const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const read = (file) => readFileSync(path.join(__dirname, "..", file), "utf8");
const ci = read(".github/workflows/ci.yml");
const merge = read(".github/workflows/dependabot-auto-merge.yml");
const dependabot = read(".github/dependabot.yml");

test("full Go CI runs for pull requests to main", () => {
  assert.match(ci, /name: Go CI/);
  assert.match(ci, /  pull_request:\n    branches: \[main\]/);
  assert.match(ci, /run: go mod tidy -diff/);
  assert.match(ci, /run: go test -race \.\/\.\.\./);
  assert.match(ci, /run: docker build -t netdiag:ci \./);
  assert.match(ci, /node --test tests\/\*\.test\.cjs/);
});

test("merge requires successful PR CI and exact Dependabot head SHA", () => {
  assert.match(merge, /workflow_run:/);
  assert.match(merge, /workflows: \[Go CI\]/);
  assert.match(merge, /types: \[completed\]/);
  assert.match(merge, /workflow_run\.event == 'pull_request'/);
  assert.match(merge, /workflow_run\.conclusion == 'success'/);
  assert.match(merge, /\.user\.login == "dependabot\[bot\]"/);
  assert.match(merge, /\.head\.sha == \$sha/);
  assert.match(merge, /-f sha="\$TESTED_SHA"/);
  assert.match(merge, /-f merge_method=squash/);
  assert.match(merge, /commits\/\$TESTED_SHA\/pulls/);
  assert.doesNotMatch(merge, /actions\/checkout|pull_request_target:/);
});

test("no dependency ecosystem or version type is excluded", () => {
  for (const ecosystem of ["gomod", "docker", "github-actions"]) {
    assert.ok(dependabot.includes('package-ecosystem: "' + ecosystem + '"'));
  }
  assert.doesNotMatch(dependabot, /^\s*ignore:/m);
  assert.doesNotMatch(merge, /semver-(?:major|minor|patch)|dependency-name|update-type/);
});
