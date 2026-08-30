import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

import { _test } from "../../relay/cloudflare/src/relay.js";

const repositoryRoot = new URL("../../", import.meta.url);
const relayRequire = createRequire(new URL("../../relay/cloudflare/package.json", import.meta.url));
const Ajv2020 = relayRequire("ajv/dist/2020").default;
const addFormats = relayRequire("ajv-formats").default;

async function readJSON(relativePath) {
  return JSON.parse(await readFile(new URL(relativePath, repositoryRoot), "utf8"));
}

function validator(schema) {
  const ajv = new Ajv2020({ allErrors: true, strict: true });
  addFormats(ajv);
  return ajv.compile(schema);
}

test("feature manifests satisfy the machine-readable schema", async () => {
  const validate = validator(await readJSON("features/feature.schema.json"));
  for (const name of ["feedback", "updater"]) {
    const manifest = await readJSON(`features/${name}/feature.json`);
    assert.equal(validate(manifest), true, `${name}: ${JSON.stringify(validate.errors)}`);
  }
});

test("remote entry and minimal host lock satisfy their schemas", async () => {
  const validateEntry = validator(await readJSON("features/index.schema.json"));
  const entry = await readJSON("features/index.json");
  assert.equal(validateEntry(entry), true, JSON.stringify(validateEntry.errors));
  assert.equal(entry.delivery.user_clone_required, false);
  assert.equal(entry.lock.host_path, "agent-app-features.lock.json");
  assert.equal(
    entry.lock.validator_package,
    "github.com/timmyagentic/awesome-agent-app-features/cmd/feature-lock",
  );

  for (const feature of entry.features) {
    const manifest = await readJSON(feature.manifest);
    assert.equal(manifest.id, feature.id);
    assert.equal(manifest.contract, feature.contract);
  }

  const validateLock = validator(await readJSON(entry.lock.schema_path));
  const lock = {
    schema: 1,
    source: {
      repository: "timmyagentic/awesome-agent-app-features",
      commit: "a".repeat(40),
    },
    features: [
      {
        id: "feedback",
        contract: "v1",
        deliveries: [
          {
            mode: "go-module",
            source: "github.com/timmyagentic/awesome-agent-app-features/feedback",
            target: "go.mod",
            version: "v0.0.0-20260824000000-aaaaaaaaaaaa",
          },
        ],
        files: ["internal/feedback/flow.go"],
        verified_at: "2026-08-24T00:00:00Z",
        checks: ["go test ./..."],
        unverified: [],
      },
    ],
  };
  assert.equal(validateLock(lock), true, JSON.stringify(validateLock.errors));

  const unsafePath = structuredClone(lock);
  unsafePath.features[0].files = ["../../outside"];
  assert.equal(validateLock(unsafePath), false, "lock accepted a traversal path");

  const secretField = structuredClone(lock);
  secretField.features[0].configuration_values = { endpoint: "secret" };
  assert.equal(validateLock(secretField), false, "lock accepted configuration values");

  const zeroCommit = structuredClone(lock);
  zeroCommit.source.commit = "0".repeat(40);
  assert.equal(validateLock(zeroCommit), false, "lock accepted an all-zero source commit");

  const emptyFiles = structuredClone(lock);
  emptyFiles.features[0].files = [];
  assert.equal(validateLock(emptyFiles), false, "lock accepted a feature with no claimed host files");
});

test("Feedback v1 fixtures satisfy both JSON Schema and the Worker validator", async () => {
  const validate = validator(await readJSON("protocol/feedback/v1/schema.json"));
  for (const name of ["valid-full.json", "valid-minimal.json"]) {
    const fixture = await readJSON(`protocol/feedback/v1/testdata/${name}`);
    assert.equal(validate(fixture), true, `${name}: ${JSON.stringify(validate.errors)}`);
    assert.equal(_test.validateSubmission(fixture), null, name);
  }
  for (const name of [
    "invalid-schema-v2.json",
    "invalid-unapproved.json",
    "invalid-presentation-field.json",
    "invalid-duplicate-gap.json",
    "invalid-blank-product.json",
  ]) {
    const fixture = await readJSON(`protocol/feedback/v1/testdata/${name}`);
    assert.equal(validate(fixture), false, `${name} unexpectedly satisfied the schema`);
    assert.notEqual(_test.validateSubmission(fixture), null, name);
  }
});
