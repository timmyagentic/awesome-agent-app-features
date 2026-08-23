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
