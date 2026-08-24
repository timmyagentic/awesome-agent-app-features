import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const repositoryRoot = new URL("../../../", import.meta.url);

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

test("remote entry, integration plan, and receipt satisfy their machine-readable schemas", async () => {
  const validateEntry = validator(await readJSON("features/index.schema.json"));
  const entry = await readJSON("features/index.json");
  assert.equal(validateEntry(entry), true, JSON.stringify(validateEntry.errors));

  const validatePlan = validator(await readJSON("features/integration-plan.schema.json"));
  const plan = await readJSON("features/integration-plan.example.json");
  assert.equal(validatePlan(plan), true, JSON.stringify(validatePlan.errors));

  const validateReceipt = validator(await readJSON("features/integration-receipt.schema.json"));
  const receipt = await readJSON("examples/host-receipt/.agent-app-features/feedback.json");
  assert.equal(validateReceipt(receipt), true, JSON.stringify(validateReceipt.errors));

  const blocked = structuredClone(receipt);
  blocked.invariants[0].status = "blocked";
  assert.equal(validateReceipt(blocked), false, "active receipt accepted a blocked invariant");

  const partial = structuredClone(blocked);
  partial.state = "partial";
  partial.verification.find((item) => item.scope === "host").status = "failed";
  assert.equal(validateReceipt(partial), true, JSON.stringify(validateReceipt.errors));

  const unsafePath = structuredClone(receipt);
  unsafePath.artifacts[0].path = "../../outside";
  assert.equal(validateReceipt(unsafePath), false, "receipt accepted a traversal path");

  const failedActive = structuredClone(receipt);
  failedActive.verification.find((item) => item.scope === "host").status = "failed";
  assert.equal(validateReceipt(failedActive), false, "active receipt accepted failed host verification");

  const fakeRemoval = structuredClone(receipt);
  fakeRemoval.state = "removed";
  assert.equal(validateReceipt(fakeRemoval), false, "removed receipt omitted removal evidence and history");

  const removed = structuredClone(receipt);
  removed.state = "removed";
  removed.removal = {
    status: "complete",
    removed_paths: ["internal/feedback/flow.go", "internal/feedback/flow_test.go"],
    retained_paths: ["go.mod"],
    evidence: "Host verification passed after removal.",
  };
  removed.history.push({
    action: "remove",
    from_commit: receipt.source.resolved_commit,
    to_commit: receipt.source.resolved_commit,
    at: receipt.updated_at,
    summary: "Removed integration-managed host adapter files.",
  });
  assert.equal(validateReceipt(removed), true, JSON.stringify(validateReceipt.errors));

  const secretField = structuredClone(receipt);
  secretField.host.configuration_values = { "feedback.endpoint": "secret" };
  assert.equal(validateReceipt(secretField), false, "receipt accepted configuration values");

  assert.equal(entry.delivery.user_clone_required, false);
  assert.equal(entry.receipt.contains_secrets, false);
  assert.deepEqual(
    entry.actions.map((action) => action.id),
    ["integrate", "inspect", "validate", "refine", "upgrade", "remove", "list"],
  );
  for (const feature of entry.features) {
    const manifest = await readJSON(feature.manifest);
    assert.equal(manifest.id, feature.id);
    assert.equal(manifest.contract, feature.contract);
  }
});

test("Feedback v1 fixtures satisfy or violate the JSON Schema as declared", async () => {
  const validate = validator(await readJSON("protocol/feedback/v1/schema.json"));
  for (const name of ["valid-full.json", "valid-minimal.json"]) {
    const fixture = await readJSON(`protocol/feedback/v1/testdata/${name}`);
    assert.equal(validate(fixture), true, `${name}: ${JSON.stringify(validate.errors)}`);
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
  }
});
