import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";

import worker, { _test } from "../src/index.js";

function rateLimiter(limit = 5) {
  let count = 0;
  return {
    async limit() {
      count += 1;
      return { success: count <= limit };
    },
  };
}

function submission(overrides = {}) {
  return {
    schema: 1,
    user_approved: true,
    install_id: "install-1",
    product: "Example Agent",
    version: "v1.0.0",
    os: "darwin",
    arch: "arm64",
    agent: "codex",
    title: "[feedback] Improve startup diagnostics",
    body: "The final redacted report.",
    ...overrides,
  };
}

function request(body) {
  return new Request("https://relay.example/v1/feedback", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });
}

let originalFetch;
let env;

beforeEach(() => {
  originalFetch = globalThis.fetch;
  env = {
    GITHUB_TOKEN: "test-token",
    GITHUB_REPO: "owner/repository",
    GITHUB_LABEL: "user-feedback",
    RATE_LIMITER: rateLimiter(),
  };
});

afterEach(() => {
  globalThis.fetch = originalFetch;
});

test("requires explicit approval and rejects client-selected repositories", async () => {
  const noApproval = await worker.fetch(request(submission({ user_approved: false })), env);
  assert.equal(noApproval.status, 400);
  assert.match((await noApproval.json()).error, /approval/);

  const clientRepository = await worker.fetch(request(submission({ repo: "attacker/repository" })), env);
  assert.equal(clientRepository.status, 400);
  assert.match((await clientRepository.json()).error, /unknown field/);
});

test("creates an issue only in the configured repository", async () => {
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url: String(url), init });
    if (String(url).includes("/search/issues")) {
      return Response.json({ items: [] });
    }
    if (String(url).endsWith("/repos/owner/repository/issues")) {
      return Response.json({ html_url: "https://github.com/owner/repository/issues/7" });
    }
    return new Response("unexpected request", { status: 500 });
  };

  const response = await worker.fetch(request(submission()), env);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), {
    issue_url: "https://github.com/owner/repository/issues/7",
    deduplicated: false,
  });
  assert.equal(calls.length, 2);
  assert.match(calls[1].url, /\/repos\/owner\/repository\/issues$/);
  const issue = JSON.parse(calls[1].init.body);
  assert.equal(issue.title, submission().title);
  assert.match(issue.body, /<!-- aaf-fp:[a-f0-9]{16} -->/);
});

test("deduplicates onto an open issue", async () => {
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url: String(url), init });
    if (String(url).includes("/search/issues")) {
      return Response.json({
        items: [{ number: 4, html_url: "https://github.com/owner/repository/issues/4" }],
      });
    }
    if (String(url).endsWith("/issues/4/comments")) {
      return Response.json({ id: 1 });
    }
    return new Response("unexpected request", { status: 500 });
  };

  const response = await worker.fetch(request(submission()), env);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), {
    issue_url: "https://github.com/owner/repository/issues/4",
    deduplicated: true,
  });
  assert.equal(calls.length, 2);
  assert.match(JSON.parse(calls[1].init.body).body, /Example Agent · v1\.0\.0 · darwin\/arm64 · codex/);
});

test("enforces byte limits and rate limiting", async () => {
  const oversized = await worker.fetch(request(submission({ title: "界".repeat(70) })), env);
  assert.equal(oversized.status, 413);

  globalThis.fetch = async (url) => {
    if (String(url).includes("/search/issues")) {
      return Response.json({ items: [] });
    }
    return Response.json({ html_url: "https://github.com/owner/repository/issues/1" });
  };
  for (let index = 0; index < 5; index += 1) {
    const allowed = await worker.fetch(request(submission()), env);
    assert.equal(allowed.status, 200);
  }
  const limited = await worker.fetch(request(submission()), env);
  assert.equal(limited.status, 429);
});

test("rejects an oversized request before JSON buffering", async () => {
  const oversized = new Request("https://relay.example/v1/feedback", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: "x".repeat(100 * 1024),
  });
  const response = await worker.fetch(oversized, env);
  assert.equal(response.status, 413);
  assert.deepEqual(await response.json(), { error: "request is too large" });
});

test("fails closed when server-side destination is missing", async () => {
  const response = await worker.fetch(request(submission()), {
    GITHUB_TOKEN: "test-token",
    GITHUB_REPO: "not-a-repository",
    RATE_LIMITER: rateLimiter(),
  });
  assert.equal(response.status, 500);
});
