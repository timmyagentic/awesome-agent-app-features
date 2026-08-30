import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";

import worker from "../src/index.js";
import { _test } from "../src/relay.js";

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
    environment: {
      product: "Example Agent",
      version: "v1.0.0",
      os: "darwin",
      arch: "arm64",
      agent: "codex",
    },
    description: "Improve startup diagnostics",
    recent_error: {
      text: "startup returned a redacted failure",
      occurred_at: "2026-08-23T09:00:00Z",
    },
    capability_gaps: ["doctor.explain"],
    ...overrides,
  };
}

function request(body, path = "/v1/feedback", contentType = "application/json") {
  return new Request(`https://relay.example${path}`, {
    method: "POST",
    headers: { "content-type": contentType },
    body: JSON.stringify(body),
  });
}

let originalFetch;
let originalConsoleError;
let env;

beforeEach(() => {
  originalFetch = globalThis.fetch;
  originalConsoleError = console.error;
  env = {
    GITHUB_TOKEN: "test-token",
    GITHUB_REPO: "owner/repository",
    GITHUB_LABEL: "user-feedback",
    RATE_LIMITER: rateLimiter(),
  };
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  console.error = originalConsoleError;
});

test("requires explicit approval and rejects client-selected presentation or repositories", async () => {
  const noApproval = await worker.fetch(request(submission({ user_approved: false })), env);
  assert.equal(noApproval.status, 400);
  assert.match((await noApproval.json()).error, /approval/);

  for (const field of ["repo", "title", "body"]) {
    const response = await worker.fetch(request(submission({ [field]: "attacker controlled" })), env);
    assert.equal(response.status, 400);
    assert.match((await response.json()).error, /unknown .*field/);
  }
});

test("validates the complete structured schema", async () => {
  const unknownEnvironment = submission();
  unknownEnvironment.environment.chat_id = "private";
  const unknownResponse = await worker.fetch(request(unknownEnvironment), env);
  assert.equal(unknownResponse.status, 400);
  assert.match((await unknownResponse.json()).error, /environment field/);

	const empty = submission();
	delete empty.description;
	delete empty.recent_error;
	delete empty.capability_gaps;
  const noContent = await worker.fetch(request(empty), env);
  assert.equal(noContent.status, 400);
  assert.match((await noContent.json()).error, /reportable content/);

  const invalidTime = await worker.fetch(
    request(submission({ recent_error: { text: "failure", occurred_at: "yesterday" } })),
    env,
  );
  assert.equal(invalidTime.status, 400);
  assert.match((await invalidTime.json()).error, /occurred_at/);

  const impossibleTime = await worker.fetch(
    request(submission({ recent_error: { text: "failure", occurred_at: "2026-02-30T09:00:00Z" } })),
    env,
  );
  assert.equal(impossibleTime.status, 400);
  assert.match((await impossibleTime.json()).error, /occurred_at/);

  const blankProduct = submission();
  blankProduct.environment.product = "   ";
  const blankProductResponse = await worker.fetch(request(blankProduct), env);
  assert.equal(blankProductResponse.status, 400);
  assert.match((await blankProductResponse.json()).error, /product/);
});

test("GitHub adapter renders an issue only in the configured repository", async () => {
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
    reference_url: "https://github.com/owner/repository/issues/7",
    deduplicated: false,
  });
  assert.equal(calls.length, 2);
  assert.ok(calls.every((call) => call.init.signal instanceof AbortSignal));
  assert.match(calls[1].url, /\/repos\/owner\/repository\/issues$/);
  const issue = JSON.parse(calls[1].init.body);
  assert.equal(issue.title, "[feedback] Example Agent: Improve startup diagnostics");
  assert.match(issue.body, /\*\*Description\*\*/);
  assert.match(issue.body, /startup returned a redacted failure/);
  assert.match(issue.body, /doctor\.explain/);
  assert.match(issue.body, /Example Agent/);
  assert.match(issue.body, /<!-- aaf-fp:[a-f0-9]{16} -->/);
});

test("deduplicates using adapter-rendered identity", async () => {
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
    reference_url: "https://github.com/owner/repository/issues/4",
    deduplicated: true,
  });
  assert.equal(calls.length, 2);
  assert.match(JSON.parse(calls[1].init.body).body, /Example Agent · v1\.0\.0 · darwin\/arm64 · codex/);
});

test("enforces field byte limits and rate limiting", async () => {
  const oversized = await worker.fetch(request(submission({ description: "界".repeat(1400) })), env);
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

test("uses only the Feedback v1 route and requires JSON", async () => {
  const futureRoute = await worker.fetch(request(submission(), "/v2/feedback"), env);
  assert.equal(futureRoute.status, 404);
  const queryRoute = await worker.fetch(request(submission(), "/v1/feedback?target=other"), env);
  assert.equal(queryRoute.status, 404);

  const wrongContentType = await worker.fetch(
    request(submission(), "/v1/feedback", "text/plain"),
    env,
  );
  assert.equal(wrongContentType.status, 415);

  const wrongMethod = await worker.fetch(
    new Request("https://relay.example/v1/feedback", { method: "GET" }),
    env,
  );
  assert.equal(wrongMethod.status, 405);
  assert.equal(wrongMethod.headers.get("allow"), "POST");
});

test("fails closed when server-side destination is missing", async () => {
  const response = await worker.fetch(request(submission()), {
    GITHUB_TOKEN: "test-token",
    GITHUB_REPO: "not-a-repository",
    RATE_LIMITER: rateLimiter(),
  });
  assert.equal(response.status, 500);

  const invalidLabel = await worker.fetch(request(submission()), {
    GITHUB_TOKEN: "test-token",
    GITHUB_REPO: "owner/repository",
    GITHUB_LABEL: `invalid"label`,
    RATE_LIMITER: rateLimiter(),
  });
  assert.equal(invalidLabel.status, 500);
});

test("maps GitHub failures and untrusted issue URLs to bounded 502 responses", async () => {
  const logs = [];
  console.error = (value) => logs.push(JSON.parse(value));
  globalThis.fetch = async () => {
    throw new Error("simulated upstream outage");
  };
  const unavailable = await worker.fetch(request(submission()), env);
  assert.equal(unavailable.status, 502);
  assert.deepEqual(await unavailable.json(), { error: "issue creation failed" });
  assert.equal(unavailable.headers.get("cache-control"), "no-store");

  globalThis.fetch = async (url) => {
    if (String(url).includes("/search/issues")) {
      return Response.json({ items: [] });
    }
    return Response.json({ html_url: "https://github.com/attacker/other/issues/7" });
  };
  const wrongRepository = await worker.fetch(request(submission()), env);
  assert.equal(wrongRepository.status, 502);
  assert.deepEqual(await wrongRepository.json(), { error: "unexpected GitHub response" });
  assert.ok(logs.every((entry) => typeof entry.message === "string"));
});

test("renderer bounds multibyte titles and escapes Markdown metadata", () => {
  const issue = _test.renderGitHubIssue(
    submission({
      environment: { ...submission().environment, product: "示例*[应用] @team" },
      description: `${"界".repeat(300)} @maintainer`,
    }),
  );
  assert.ok(new TextEncoder().encode(issue.title).byteLength <= 200);
  assert.match(issue.body, /示例\\\*\\\[应用\\\]/);
  assert.doesNotMatch(issue.title, /@team/);
  assert.doesNotMatch(issue.body, /@maintainer/);
});

test("dedupe identity is stable across set and object field order", () => {
  const first = submission({
    capability_gaps: ["doctor.explain", "logs.export"],
    recent_error: { text: "failure", occurred_at: "2026-08-23T09:00:00Z" },
  });
  const second = submission({
    capability_gaps: ["logs.export", "doctor.explain"],
    recent_error: { occurred_at: "2026-08-23T09:00:00Z", text: "failure" },
  });
  assert.equal(_test.feedbackIdentity(first), _test.feedbackIdentity(second));
});
