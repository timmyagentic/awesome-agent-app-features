import assert from "node:assert/strict";
import {test} from "node:test";
import {diagnosticFetchHandler, renderDiagnosticIssue, reportHash, submitReport, validateDiagnosticSubmission} from "../src/diagnostic.js";

export function diagnosticSubmission() {
  return {
    schema: 2, user_approved: true, report_id: "1234567890abcdef1234567890abcdef",
    environment: {product: "Example", version: "1.0"}, description: "Export exceeded its turn deadline",
    diagnostic: {
      capture_version: 1, started_at: "2026-09-20T00:00:00Z", occurred_at: "2026-09-21T02:00:00Z",
      phase: "turn_deadline", error_code: "turn_deadline", request: "Complete the overnight export", error: "turn deadline exceeded",
      runtime: {backend: "app_server", model: "model", settings_source: "turn_start_request", idle_timeout_ms: 600000, max_turn_time_ms: 7200000},
      transport: {process_state: "alive", read_state: "eof", terminal_received: false, terminal_delivered: false, dropped_events: 0},
      missing: ["runtime.agent_version: unavailable"],
    },
  };
}

function memoryStore() {
  let record;
  return {async get() { return record; }, async put(value) { record = structuredClone(value); }};
}

test("diagnostic validation strictly bounds every nested field and protects approval", () => {
  const original = diagnosticSubmission();
  assert.equal(validateDiagnosticSubmission(original), null);
  for (const edit of [
    s => { s.user_approved = false; }, s => { s.body = "arbitrary issue body"; },
    s => { s.report_id = "not-random"; }, s => { s.diagnostic.logs = "raw"; },
    s => { s.diagnostic.runtime.env = {SECRET: "value"}; },
    s => { s.diagnostic.transport.queue_depth = -1; },
    s => { s.diagnostic.transport.terminal_received = "false"; },
    s => { s.diagnostic.request = "界".repeat(1400); },
    s => { s.diagnostic.activity = [{after_ms: 0, kind: "tool", arguments: "secret"}]; },
    s => { s.diagnostic.missing = Array(25).fill("missing"); },
  ]) {
    const value = structuredClone(original); edit(value);
    assert.notEqual(validateDiagnosticSubmission(value), null, JSON.stringify(value));
  }
});

test("issue renderer retains long-turn context, transport facts and inert user text", () => {
  const value = diagnosticSubmission();
  value.diagnostic.request += "\n```\n@maintainer <script>alert(1)</script>\n````";
  const issue = renderDiagnosticIssue(value);
  for (const text of ["Original request", "overnight export", '"read_state": "eof"', "runtime.agent_version: unavailable", "`````text"]) assert.ok(issue.body.includes(text), text);
  assert.ok(issue.title.includes("Export exceeded"));
});

test("durable intent precedes create and an acknowledged receipt replays without a mutation", async () => {
  const store = memoryStore(), value = diagnosticSubmission(), hash = await reportHash(value);
  let posts = 0;
  const github = async (_path, init) => {
    assert.equal((await store.get()).state, "dispatching");
    assert.equal(init.method, "POST"); posts++;
    return Response.json({html_url: "https://github.com/owner/repository/issues/1"});
  };
  assert.equal((await submitReport(store, value, github, "owner/repository", "user-feedback", hash)).status, 200);
  assert.equal((await submitReport(store, value, github, "owner/repository", "user-feedback", hash)).body.deduplicated, true);
  assert.equal(posts, 1);
  assert.equal((await submitReport(store, value, github, "owner/repository", "user-feedback", "f".repeat(64))).status, 409);
  assert.equal(posts, 1);
  assert.equal(JSON.stringify(await store.get()).includes("overnight export"), false);
});

test("a lost create response never permits a second blind POST, even after an empty search", async () => {
  const store = memoryStore(), value = diagnosticSubmission(), hash = await reportHash(value);
  let posts = 0, marker = "", searchable = false;
  const github = async (path, init) => {
    if (init?.method === "POST") {
      posts++; marker = JSON.parse(init.body).body;
      throw new Error("response lost after GitHub committed");
    }
    assert.ok(path.startsWith("/search/issues"));
    return Response.json({items: searchable ? [{body: marker, html_url: "https://github.com/owner/repository/issues/1"}] : []});
  };
  assert.equal((await submitReport(store, value, github, "owner/repository", "", hash)).status, 503);
  assert.equal((await submitReport(store, value, github, "owner/repository", "", hash)).status, 503);
  searchable = true;
  assert.equal((await submitReport(store, value, github, "owner/repository", "", hash)).status, 200);
  assert.equal(posts, 1);
});

test("an unpersisted dispatch cannot reach GitHub and a definite rejection may retry", async () => {
  const value = diagnosticSubmission(), hash = await reportHash(value);
  let posts = 0;
  await assert.rejects(submitReport({async get() {}, async put() { throw new Error("storage unavailable"); }}, value, async () => { posts++; return Response.json({}); }, "owner/repository", "", hash));
  assert.equal(posts, 0);
  const store = memoryStore();
  const github = async () => { posts++; return posts === 1 ? new Response(null, {status: 403}) : Response.json({html_url: "https://github.com/owner/repository/issues/1"}); };
  assert.equal((await submitReport(store, value, github, "owner/repository", "", hash)).status, 502);
  assert.equal((await store.get()).state, "failed");
  assert.equal((await submitReport(store, value, github, "owner/repository", "", hash)).status, 200);
  assert.equal(posts, 2);
});

test("schema, approval and rate checks precede durable dispatch", async () => {
  let calls = 0;
  const env = {GITHUB_TOKEN: "test-token", GITHUB_REPO: "owner/repository", RATE_LIMITER: {async limit() { return {success: false}; }}, FEEDBACK_REPORTS: {getByName() { calls++; throw new Error("unexpected dispatch"); }}};
  const request = value => new Request("https://relay.example/v2/feedback", {method: "POST", headers: {"content-type": "application/json"}, body: JSON.stringify(value)});
  const value = diagnosticSubmission();
  assert.equal((await diagnosticFetchHandler(request({...value, user_approved: false}), env)).status, 400);
  assert.equal((await diagnosticFetchHandler(request(value), env)).status, 429);
  assert.equal(calls, 0);
});
