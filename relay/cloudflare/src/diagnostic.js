import { protocol } from "./relay.js";

/** @typedef {import("./relay.js").FeedbackSubmission & {report_id: string, diagnostic?: Diagnostic}} DiagnosticSubmission */
/** @typedef {{after_ms: number, kind: string, name?: string}} Activity */
/** @typedef {{backend?: string, agent_version?: string, platform?: string, model?: string, effort?: string, service_tier?: string, mode?: string, settings_source?: string, idle_timeout_ms: number, max_turn_time_ms: number}} Runtime */
/** @typedef {{process_state?: string, read_state?: string, exit_code?: number, last_protocol_ms?: number, last_core_ms?: number, last_delivery_ms?: number, pending_rpc?: number, queue_depth?: number, queue_high_water?: number, dropped_events?: number, terminal_received?: boolean, terminal_delivered?: boolean}} Transport */
/** @typedef {{capture_version: number, started_at: string, occurred_at: string, phase: string, error_code: string, request?: string, response?: string, error?: string, runtime: Runtime, transport: Transport, activity?: Activity[], missing?: string[], truncated?: string[]}} Diagnostic */
/** @typedef {{reference_url: string, deduplicated: boolean}} Receipt */
/** @typedef {{hash: string, state: "dispatching" | "failed" | "submitted", receipt?: Receipt}} StoredReport */
/** @typedef {{get: () => Promise<StoredReport | undefined>, put: (record: StoredReport) => Promise<void>}} ReportStore */
/** @typedef {{status: number, body: Receipt | {error: string}}} SubmissionResult */
/** @typedef {import("./relay.js").RelayEnv} RelayEnv */

const {isObject, unknownField, validText, validTimestamp, byteLength} = protocol;
const TOP_FIELDS = new Set(["schema", "user_approved", "environment", "description", "recent_error", "capability_gaps", "report_id", "diagnostic"]);
const DIAGNOSTIC_FIELDS = new Set(["capture_version", "started_at", "occurred_at", "phase", "error_code", "request", "response", "error", "runtime", "transport", "activity", "missing", "truncated"]);
const RUNTIME_TEXT = ["backend", "agent_version", "platform", "model", "effort", "service_tier", "mode", "settings_source"];
const RUNTIME_FIELDS = new Set([...RUNTIME_TEXT, "idle_timeout_ms", "max_turn_time_ms"]);
const TRANSPORT_TIME = ["last_protocol_ms", "last_core_ms", "last_delivery_ms"];
const TRANSPORT_COUNT = ["pending_rpc", "queue_depth", "queue_high_water", "dropped_events"];
const TRANSPORT_BOOL = ["terminal_received", "terminal_delivered"];
const TRANSPORT_FIELDS = new Set(["process_state", "read_state", "exit_code", ...TRANSPORT_TIME, ...TRANSPORT_COUNT, ...TRANSPORT_BOOL]);
const MAX_TIME = 30 * 24 * 60 * 60 * 1000;
const MAX_ISSUE_BYTES = 60 * 1024;
// JSON may escape every '<', '>' or '&' as six ASCII bytes. Keep a bounded
// envelope that can carry all of the independently bounded decoded fields.
export const MAX_DIAGNOSTIC_REQUEST_BYTES = 256 * 1024;

/** @param {unknown} value @param {number} maximum @param {number} [minimum] */
function integer(value, maximum, minimum = 0) {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= minimum && value <= maximum;
}

/** @param {unknown} value @param {number} maximum */
function line(value, maximum) {
  return validText(value, maximum) && !/[\r\n\t]/.test(/** @type {string} */ (value));
}

/** @param {unknown} value @returns {string | null} */
export function validateDiagnosticSubmission(value) {
  if (!isObject(value)) return "submission must be an object";
  const unexpected = unknownField(value, TOP_FIELDS, "submission");
  if (unexpected) return unexpected;
  if (value.schema !== 2) return "unsupported schema";
  if (typeof value.report_id !== "string" || !/^[a-f0-9]{32}$/.test(value.report_id)) return "report_id is invalid";
  const {report_id: _id, diagnostic, ...base} = value;
  const baseError = protocol.validateSubmission({...base, schema: 1});
  if (baseError) return baseError;
  if (diagnostic !== undefined) {
    const error = validateDiagnostic(diagnostic);
    if (error) return error;
  }
  if (byteLength(renderDiagnosticIssue(/** @type {DiagnosticSubmission} */ (value)).body) + 200 > MAX_ISSUE_BYTES) return "submission is too large";
  return null;
}

/** @param {unknown} value @returns {string | null} */
function validateDiagnostic(value) {
  if (!isObject(value) || unknownField(value, DIAGNOSTIC_FIELDS, "diagnostic")) return "diagnostic fields are invalid";
  if (value.capture_version !== 1 || !validTimestamp(value.started_at) || !validTimestamp(value.occurred_at)) return "diagnostic capture is invalid";
  if (!line(value.phase, 64) || !line(value.error_code, 64)) return "diagnostic phase or error_code is invalid";
  for (const [name, maximum] of /** @type {[string, number][]} */ ([["request", 4000], ["response", 1200], ["error", 4000]])) {
    if (value[name] !== undefined && !validText(value[name], maximum)) return `diagnostic ${name} is invalid`;
  }
  const runtime = value.runtime;
  if (!isObject(runtime) || unknownField(runtime, RUNTIME_FIELDS, "runtime")) return "diagnostic runtime is invalid";
  for (const field of RUNTIME_TEXT) {
    if (runtime[field] !== undefined && !line(runtime[field], 160)) return `runtime ${field} is invalid`;
  }
  if (!integer(runtime.idle_timeout_ms, MAX_TIME) || !integer(runtime.max_turn_time_ms, MAX_TIME)) return "runtime time limits are invalid";
  const transport = value.transport;
  if (!isObject(transport) || unknownField(transport, TRANSPORT_FIELDS, "transport")) return "diagnostic transport is invalid";
  for (const field of ["process_state", "read_state"]) {
    if (transport[field] !== undefined && !line(transport[field], 64)) return `transport ${field} is invalid`;
  }
  for (const field of TRANSPORT_TIME) {
    if (transport[field] !== undefined && !integer(transport[field], MAX_TIME)) return `transport ${field} is invalid`;
  }
  for (const field of TRANSPORT_COUNT) {
    if (transport[field] !== undefined && !integer(transport[field], 1_000_000_000)) return `transport ${field} is invalid`;
  }
  for (const field of TRANSPORT_BOOL) {
    if (transport[field] !== undefined && typeof transport[field] !== "boolean") return `transport ${field} is invalid`;
  }
  if (transport.exit_code !== undefined && !integer(transport.exit_code, 65535, -65535)) return "transport exit_code is invalid";
  if (value.activity !== undefined) {
    if (!Array.isArray(value.activity) || value.activity.length > 24 || value.activity.length === 0) return "diagnostic activity is invalid";
    for (const event of value.activity) {
      if (!isObject(event) || unknownField(event, new Set(["after_ms", "kind", "name"]), "activity") || !integer(event.after_ms, MAX_TIME) || !line(event.kind, 64) || (event.name !== undefined && !line(event.name, 96))) return "diagnostic activity entry is invalid";
    }
  }
  for (const name of ["missing", "truncated"]) {
    const values = value[name];
    if (values !== undefined && (!Array.isArray(values) || values.length < 1 || values.length > 24 || values.some(v => !line(v, 160)) || new Set(values).size !== values.length)) return `diagnostic ${name} is invalid`;
  }
  return null;
}

/** A dynamic fence keeps even hostile backticks inside code, where mentions
 * are inert. No user text is concatenated into HTML or Markdown structure.
 * @param {string} text @param {string} [language] */
function code(text, language = "text") {
  const longest = Math.max(2, ...[...text.matchAll(/`+/g)].map(match => match[0].length));
  const fence = "`".repeat(longest + 1);
  return `${fence}${language}\n${text}\n${fence}`;
}

/** @param {DiagnosticSubmission} submission @returns {import("./relay.js").RenderedIssue} */
export function renderDiagnosticIssue(submission) {
  const base = protocol.renderGitHubIssue(submission);
  const sections = [base.body];
  if (submission.diagnostic) {
    const {request, response, error, ...facts} = submission.diagnostic;
    if (request) sections.push(`**Original request**\n\n${code(request)}`);
    if (error) sections.push(`**Failure at capture**\n\n${code(error)}`);
    if (response) sections.push(`**Response excerpt**\n\n${code(response)}`);
    sections.push(`**Captured diagnostics**\n\n${code(JSON.stringify(facts, null, 2), "json")}`);
  }
  sections.push(`Report: ${submission.report_id} · Diagnostic protocol: 2`);
  return {title: base.title, body: sections.join("\n\n")};
}

/** @param {unknown} value @returns {string} */
function canonical(value) {
  if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
  if (isObject(value)) return `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(",")}}`;
  return JSON.stringify(value);
}

/** @param {unknown} value @returns {Promise<string>} */
export async function reportHash(value) {
  const data = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(canonical(value)));
  return [...new Uint8Array(data)].map(v => v.toString(16).padStart(2, "0")).join("");
}

/** @param {Request} request @param {RelayEnv} env @returns {Promise<Response>} */
export async function diagnosticFetchHandler(request, env) {
  const url = new URL(request.url);
  if (url.pathname !== "/v2/feedback" || url.search !== "") return protocol.json(404, {error: "not found"});
  if (request.method !== "POST") return protocol.json(405, {error: "method not allowed"});
  if (!/^application\/json(?:\s*;|$)/i.test(request.headers.get("content-type") || "")) return protocol.json(415, {error: "content type must be application/json"});
  try {
    const value = await protocol.readRequestJSON(request, MAX_DIAGNOSTIC_REQUEST_BYTES);
    const error = validateDiagnosticSubmission(value);
    if (error) return protocol.json(error === "submission is too large" ? 413 : 400, {error});
    if (!env.FEEDBACK_REPORTS || typeof env.GITHUB_TOKEN !== "string" || !env.GITHUB_TOKEN.trim() || env.GITHUB_TOKEN !== env.GITHUB_TOKEN.trim() || !protocol.validateRepository(env.GITHUB_REPO) || (env.GITHUB_LABEL && !protocol.validLabel(env.GITHUB_LABEL)) || !env.RATE_LIMITER) return protocol.json(500, {error: "relay is not configured"});
    const client = request.headers.get("cf-connecting-ip")?.trim().slice(0,128);
    if (!(await env.RATE_LIMITER.limit({key: client ? `ip:${client}` : "unknown"})).success) return protocol.json(429, {error: "rate limited"});
    const submission = /** @type {DiagnosticSubmission} */ (value);
    const result = await env.FEEDBACK_REPORTS.getByName(`${env.GITHUB_REPO}:${submission.report_id}`).submit(submission, env.GITHUB_TOKEN);
    return protocol.json(result.status, result.body);
  } catch (error) {
    if (error instanceof protocol.RequestTooLargeError) return protocol.json(413, {error: "request is too large"});
    if (error instanceof SyntaxError || error instanceof TypeError) return protocol.json(400, {error: "invalid JSON"});
    // Never log the submitted payload, credential, or upstream error body.
    return protocol.json(503, {error: "feedback submission unavailable"});
  }
}

/** Persist intent before mutation. An unknown outcome is only reconciled by
 * positive marker evidence; an empty eventually-consistent search is never
 * permission to create a second Issue.
 * @param {ReportStore} store @param {DiagnosticSubmission} submission
 * @param {import("./relay.js").GitHubClient} github
 * @param {string} repository @param {string} label @param {string} hash
 * @returns {Promise<SubmissionResult>}
 */
export async function submitReport(store, submission, github, repository, label, hash) {
  const record = await store.get();
  if (record && record.hash !== hash) return {status: 409, body: {error: "report_id is already bound to another payload"}};
  if (record?.receipt) return {status: 200, body: {...record.receipt, deduplicated: true}};
  const marker = `<!-- aaf-report:${submission.report_id}:${hash} -->`;
  if (record?.state === "dispatching") {
    const query = encodeURIComponent(`repo:${repository} is:issue "aaf-report:${submission.report_id}"`);
    try {
      const response = await github(`/search/issues?q=${query}&per_page=10`);
      if (!response.ok) { await protocol.discardResponse(response); return unknownOutcome(); }
      const found = await protocol.readGitHubJSON(response);
      if (isObject(found) && Array.isArray(found.items)) {
        for (const item of found.items) {
          if (isObject(item) && typeof item.body === "string" && item.body.endsWith(marker) && protocol.validIssueURL(item.html_url, repository)) {
            const receipt = {reference_url: item.html_url, deduplicated: true};
            await store.put({hash, state: "submitted", receipt});
            return {status: 200, body: receipt};
          }
        }
      }
    } catch { /* Keep the durable unknown state; never re-create. */ }
    return unknownOutcome();
  }
  const issue = renderDiagnosticIssue(submission);
  await store.put({hash, state: "dispatching"});
  try {
    const response = await github(`/repos/${repository}/issues`, {method: "POST", body: JSON.stringify({title: issue.title, body: `${issue.body}\n\n${marker}`, labels: [label || "user-feedback"]})});
    if (!response.ok) {
      await protocol.discardResponse(response);
      if ([400, 401, 403, 404, 422, 429].includes(response.status)) {
        await store.put({hash, state: "failed"});
        return {status: 502, body: {error: "issue creation rejected"}};
      }
      return unknownOutcome();
    }
    const created = await protocol.readGitHubJSON(response);
    if (!isObject(created) || !protocol.validIssueURL(created.html_url, repository)) return unknownOutcome();
    const receipt = {reference_url: created.html_url, deduplicated: false};
    await store.put({hash, state: "submitted", receipt});
    return {status: 200, body: receipt};
  } catch { return unknownOutcome(); }
}

/** @returns {SubmissionResult} */
function unknownOutcome() { return {status: 503, body: {error: "submission outcome is not yet confirmed"}}; }
