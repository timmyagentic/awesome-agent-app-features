/**
 * Single-tenant GitHub Issues adapter for structured feedback reports.
 *
 * The client sends only provider-neutral, redacted fields. This adapter owns
 * GitHub title/body rendering, and the target repository and token remain
 * server-side configuration.
 */

const MAX_DESCRIPTION_BYTES = 4_000;
const MAX_ERROR_BYTES = 4_000;
const MAX_GAP_BYTES = 160;
const MAX_GAPS = 20;
const MAX_METADATA_BYTES = 160;
const MAX_ISSUE_TITLE_BYTES = 200;
const MAX_ISSUE_BODY_BYTES = 32_000;
const MAX_REQUEST_BYTES = 96 * 1024;
const MAX_GITHUB_RESPONSE_BYTES = 1024 * 1024;
const GITHUB_TIMEOUT_MS = 15_000;

const ALLOWED_FIELDS = new Set([
  "schema",
  "user_approved",
  "environment",
  "description",
  "recent_error",
  "capability_gaps",
]);
const ALLOWED_ENVIRONMENT_FIELDS = new Set(["product", "version", "os", "arch", "agent"]);
const ALLOWED_ERROR_FIELDS = new Set(["text", "occurred_at"]);

class RequestTooLargeError extends Error {}

function json(status, payload) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
      "content-security-policy": "default-src 'none'; frame-ancestors 'none'",
      "x-content-type-options": "nosniff",
    },
  });
}

function byteLength(value) {
  return new TextEncoder().encode(value).byteLength;
}

function isObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function unknownField(value, allowed, label) {
  for (const key of Object.keys(value)) {
    if (!allowed.has(key)) {
      return `unknown ${label} field: ${key}`;
    }
  }
  return null;
}

function validShortString(value) {
  return (
    typeof value === "string" &&
    value.trim() !== "" &&
    byteLength(value) <= MAX_METADATA_BYTES &&
    !/[\u0000-\u001f\u007f]/.test(value)
  );
}

function validText(value, maxBytes) {
  return (
    typeof value === "string" &&
    value.trim() !== "" &&
    byteLength(value) <= maxBytes &&
    !/[\u0000-\u0008\u000b-\u001f\u007f]/.test(value)
  );
}

function validTimestamp(value) {
  if (typeof value !== "string" || value.length > 64) {
    return false;
  }
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-](\d{2}):(\d{2}))$/.exec(
    value,
  );
  if (!match) {
    return false;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (
    month < 1 ||
    month > 12 ||
    day < 1 ||
    day > days[month - 1] ||
    hour > 23 ||
    minute > 59 ||
    second > 59
  ) {
    return false;
  }
  if (match[7] !== "Z" && (Number(match[8]) > 23 || Number(match[9]) > 59)) {
    return false;
  }
  return Number.isFinite(Date.parse(value));
}

function validateSubmission(value) {
  if (!isObject(value)) {
    return "submission must be an object";
  }
  const topLevelError = unknownField(value, ALLOWED_FIELDS, "submission");
  if (topLevelError) {
    return topLevelError;
  }
  if (value.schema !== 1) {
    return "unsupported schema";
  }
  if (value.user_approved !== true) {
    return "explicit user approval is required";
  }
  if (!isObject(value.environment)) {
    return "environment is required";
  }
  const environmentError = unknownField(value.environment, ALLOWED_ENVIRONMENT_FIELDS, "environment");
  if (environmentError) {
    return environmentError;
  }
  if (!validShortString(value.environment.product)) {
    return "environment product is required or too large";
  }
  for (const field of ["version", "os", "arch", "agent"]) {
    if (value.environment[field] !== undefined && !validShortString(value.environment[field])) {
      return `environment ${field} is too large or invalid`;
    }
  }

  if (value.description !== undefined) {
    if (!validText(value.description, MAX_DESCRIPTION_BYTES)) {
      return typeof value.description === "string" && byteLength(value.description) > MAX_DESCRIPTION_BYTES
        ? "submission is too large"
        : "description is invalid";
    }
  }

  if (value.recent_error !== undefined) {
    if (!isObject(value.recent_error)) {
      return "recent_error must be an object";
    }
    const recentError = unknownField(value.recent_error, ALLOWED_ERROR_FIELDS, "recent_error");
    if (recentError) {
      return recentError;
    }
    if (!validText(value.recent_error.text, MAX_ERROR_BYTES)) {
      return typeof value.recent_error.text === "string" && byteLength(value.recent_error.text) > MAX_ERROR_BYTES
        ? "submission is too large"
        : "recent_error text is required or invalid";
    }
    if (!validTimestamp(value.recent_error.occurred_at)) {
      return "recent_error occurred_at must be RFC3339";
    }
  }

  if (value.capability_gaps !== undefined) {
    if (
      !Array.isArray(value.capability_gaps) ||
      value.capability_gaps.length === 0 ||
      value.capability_gaps.length > MAX_GAPS
    ) {
      return "capability_gaps is invalid";
    }
    const seen = new Set();
    for (const gap of value.capability_gaps) {
      if (!validShortString(gap)) {
        return typeof gap === "string" && byteLength(gap) > MAX_GAP_BYTES
          ? "submission is too large"
          : "capability_gaps contains an invalid value";
      }
      if (byteLength(gap) > MAX_GAP_BYTES) {
        return "submission is too large";
      }
      if (seen.has(gap)) {
        return "capability_gaps contains a duplicate";
      }
      seen.add(gap);
    }
  }

  const hasContent =
    (typeof value.description === "string" && value.description.trim() !== "") ||
    value.recent_error !== undefined ||
    (Array.isArray(value.capability_gaps) && value.capability_gaps.length > 0);
  if (!hasContent) {
    return "feedback has no reportable content";
  }
  const issue = renderGitHubIssue(value);
  const maximumIssueBody = `${issue.body}\n\n<!-- aaf-fp:${"0".repeat(16)} -->`;
  if (byteLength(maximumIssueBody) > MAX_ISSUE_BODY_BYTES) {
    return "submission is too large";
  }
  return null;
}

function validateRepository(value) {
  if (typeof value !== "string" || !/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(value)) {
    return false;
  }
  return value.split("/").every((part) => part !== "." && part !== "..");
}

function validLabel(value) {
  return (
    typeof value === "string" &&
    byteLength(value) <= 50 &&
    /^[A-Za-z0-9][A-Za-z0-9 ._:-]*$/.test(value)
  );
}

async function rateLimited(env, key) {
  const result = await env.RATE_LIMITER.limit({ key });
  return !result.success;
}

async function readRequestJSON(request) {
  const contentLength = Number(request.headers.get("content-length"));
  if (Number.isFinite(contentLength) && contentLength > MAX_REQUEST_BYTES) {
    throw new RequestTooLargeError("request body is too large");
  }
  if (!request.body) {
    throw new SyntaxError("request body is empty");
  }
  const reader = request.body.getReader();
  const chunks = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    total += value.byteLength;
    if (total > MAX_REQUEST_BYTES) {
      await reader.cancel();
      throw new RequestTooLargeError("request body is too large");
    }
    chunks.push(value);
  }
  const data = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    data.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(data));
}

async function readGitHubJSON(response) {
  const contentLength = Number(response.headers.get("content-length"));
  if (Number.isFinite(contentLength) && contentLength > MAX_GITHUB_RESPONSE_BYTES) {
    throw new RequestTooLargeError("GitHub response is too large");
  }
  if (!response.body) {
    throw new SyntaxError("GitHub response body is empty");
  }
  const reader = response.body.getReader();
  const chunks = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    total += value.byteLength;
    if (total > MAX_GITHUB_RESPONSE_BYTES) {
      await reader.cancel();
      throw new RequestTooLargeError("GitHub response is too large");
    }
    chunks.push(value);
  }
  const data = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    data.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(data));
}

async function discardResponse(response) {
  if (response.body) {
    try {
      await response.body.cancel();
    } catch {
      // The upstream outcome is already known; cancellation failure must not
      // change a deterministic 2xx/502 relay result.
    }
  }
}

function logError(message, fields = {}) {
  console.error(JSON.stringify({ message, ...fields }));
}

async function fingerprint(input) {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(input));
  return [...new Uint8Array(digest)]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("")
    .slice(0, 16);
}

function githubClient(env) {
  return (path, init = {}) => {
    const { headers = {}, signal, ...requestInit } = init;
    return fetch(`https://api.github.com${path}`, {
      ...requestInit,
      signal: signal || AbortSignal.timeout(GITHUB_TIMEOUT_MS),
      headers: {
        ...headers,
        authorization: `Bearer ${env.GITHUB_TOKEN}`,
        accept: "application/vnd.github+json",
        "content-type": "application/json",
        "user-agent": "awesome-agent-app-features-feedback-relay",
        "x-github-api-version": "2022-11-28",
      },
    });
  };
}

function firstLine(value) {
  return value.trim().split("\n", 1)[0].trim().replace(/\s+/g, " ");
}

function truncateUTF8(value, maxBytes, suffix = "…") {
  if (byteLength(value) <= maxBytes) {
    return value;
  }
  let result = "";
  for (const character of value) {
    if (byteLength(result + character + suffix) > maxBytes) {
      break;
    }
    result += character;
  }
  return result + suffix;
}

function markdown(value) {
  return value.replaceAll("@", "@\u200b").replace(/([\\`*_[\]<>])/g, "\\$1");
}

function valueOrUnknown(value) {
  return value || "unknown";
}

function renderGitHubIssue(submission) {
  const environment = submission.environment;
  let seed = firstLine(submission.description || "");
  if (!seed && submission.recent_error) {
    seed = `error: ${firstLine(submission.recent_error.text)}`;
  }
  if (!seed) {
    seed = `unsupported capability: ${submission.capability_gaps[0]}`;
  }
  const title = truncateUTF8(
    `[feedback] ${environment.product}: ${seed}`.replaceAll("@", "@\u200b"),
    MAX_ISSUE_TITLE_BYTES,
  );
  const sections = [];
  if (submission.description) {
    sections.push(`**Description**\n\n${markdown(submission.description)}`);
  }
  if (submission.recent_error) {
    sections.push(
      `**Recent error** (${submission.recent_error.occurred_at})\n\n${markdown(submission.recent_error.text)}`,
    );
  }
  if (submission.capability_gaps?.length) {
    sections.push(
      `**Capability gaps**\n\n${submission.capability_gaps.map((gap) => `- ${markdown(gap)}`).join("\n")}`,
    );
  }
  sections.push(
    [
      "**Environment**",
      "",
      `- Product: ${markdown(environment.product)}`,
      `- Version: ${markdown(valueOrUnknown(environment.version))}`,
      `- OS/Arch: ${markdown(valueOrUnknown(environment.os))}/${markdown(valueOrUnknown(environment.arch))}`,
      `- Agent: ${markdown(valueOrUnknown(environment.agent))}`,
    ].join("\n"),
  );
  sections.push("_Submitted through a host-rendered feedback flow after explicit user approval._");
  return { title, body: sections.join("\n\n") };
}

function feedbackIdentity(submission) {
  return JSON.stringify({
    product: submission.environment.product,
    description: submission.description || "",
    recent_error: submission.recent_error
      ? {
          text: submission.recent_error.text,
          occurred_at: submission.recent_error.occurred_at,
        }
      : null,
    capability_gaps: [...(submission.capability_gaps || [])].sort(),
  });
}

async function handle(request, env) {
  const url = new URL(request.url);
  if (url.pathname !== "/v1/feedback" || url.search !== "") {
    return json(404, { error: "not found" });
  }
  if (request.method !== "POST") {
    const response = json(405, { error: "method not allowed" });
    response.headers.set("allow", "POST");
    return response;
  }
  const contentType = request.headers.get("content-type") || "";
  if (!/^application\/json(?:\s*;|$)/i.test(contentType)) {
    return json(415, { error: "content type must be application/json" });
  }
  if (
    typeof env.GITHUB_TOKEN !== "string" ||
    env.GITHUB_TOKEN === "" ||
    env.GITHUB_TOKEN !== env.GITHUB_TOKEN.trim() ||
    !validateRepository(env.GITHUB_REPO) ||
    (env.GITHUB_LABEL !== undefined && env.GITHUB_LABEL !== "" && !validLabel(env.GITHUB_LABEL)) ||
    !env.RATE_LIMITER ||
    typeof env.RATE_LIMITER.limit !== "function"
  ) {
    return json(500, { error: "relay is not configured" });
  }

  let submission;
  try {
    submission = await readRequestJSON(request);
  } catch (error) {
    if (error instanceof RequestTooLargeError) {
      return json(413, { error: "request is too large" });
    }
    return json(400, { error: "invalid JSON" });
  }
  const validationError = validateSubmission(submission);
  if (validationError) {
    const status = validationError === "submission is too large" ? 413 : 400;
    return json(status, { error: validationError });
  }

  const connectingIP = request.headers.get("cf-connecting-ip")?.trim().slice(0, 128);
  const clientKey = connectingIP ? `ip:${connectingIP}` : "unknown";
  if (await rateLimited(env, clientKey)) {
    return json(429, { error: "rate limited" });
  }

  const repository = env.GITHUB_REPO;
  const label = env.GITHUB_LABEL || "user-feedback";
  const github = githubClient(env);
  const issue = renderGitHubIssue(submission);
  const marker = `aaf-fp:${await fingerprint(feedbackIdentity(submission))}`;
  const issueBody = `${issue.body}\n\n<!-- ${marker} -->`;
  if (byteLength(issueBody) > MAX_ISSUE_BODY_BYTES) {
    return json(413, { error: "submission is too large" });
  }
  const environmentLine = [
    submission.environment.product,
    submission.environment.version || "?",
    `${submission.environment.os || "?"}/${submission.environment.arch || "?"}`,
    submission.environment.agent || "?",
  ]
    .map(markdown)
    .join(" · ");

  // Best-effort deduplication. GitHub search is eventually consistent, so the
  // adapter never presents it as a strict uniqueness guarantee.
  try {
    const query = encodeURIComponent(`repo:${repository} is:issue is:open label:"${label}" "${marker}"`);
    const searchResponse = await github(`/search/issues?q=${query}&per_page=1`);
    if (searchResponse.ok) {
      const found = await readGitHubJSON(searchResponse);
      const existing = found.items?.[0];
      if (existing && Number.isInteger(existing.number) && typeof existing.html_url === "string") {
        const commentResponse = await github(`/repos/${repository}/issues/${existing.number}/comments`, {
          method: "POST",
          body: JSON.stringify({ body: `+1 — ${environmentLine}` }),
        });
        const commentOK = commentResponse.ok;
        await discardResponse(commentResponse);
        if (commentOK && validIssueURL(existing.html_url, repository)) {
          return json(200, { reference_url: existing.html_url, deduplicated: true });
        }
        logError("dedup comment failed", { status: commentResponse.status });
      }
    } else {
      logError("dedup search failed", { status: searchResponse.status });
      await discardResponse(searchResponse);
    }
  } catch (error) {
    logError("dedup lookup failed", {
      error: error instanceof Error ? error.message : String(error),
    });
  }

  let issueResponse;
  try {
    issueResponse = await github(`/repos/${repository}/issues`, {
      method: "POST",
      body: JSON.stringify({
        title: issue.title,
        body: issueBody,
        labels: [label],
      }),
    });
  } catch (error) {
    logError("GitHub issue request failed", {
      error: error instanceof Error ? error.message : String(error),
    });
    return json(502, { error: "issue creation failed" });
  }
  if (!issueResponse.ok) {
    logError("GitHub issue creation failed", { status: issueResponse.status });
    await discardResponse(issueResponse);
    return json(502, { error: "issue creation failed" });
  }
  let created;
  try {
    created = await readGitHubJSON(issueResponse);
  } catch (error) {
    logError("GitHub issue response was invalid", {
      error: error instanceof Error ? error.message : String(error),
    });
    return json(502, { error: "unexpected GitHub response" });
  }
  if (!validIssueURL(created.html_url, repository)) {
    return json(502, { error: "unexpected GitHub response" });
  }
  return json(200, { reference_url: created.html_url, deduplicated: false });
}

function validIssueURL(raw, repository) {
  if (typeof raw !== "string") {
    return false;
  }
  try {
    const value = new URL(raw);
    return (
      value.protocol === "https:" &&
      value.hostname === "github.com" &&
      value.username === "" &&
      value.password === "" &&
      value.search === "" &&
      value.hash === "" &&
      new RegExp(`^/${repository.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/issues/[1-9][0-9]*$`).test(
        value.pathname,
      )
    );
  } catch {
    return false;
  }
}

async function fetchHandler(request, env) {
  try {
    return await handle(request, env);
  } catch (error) {
    logError("unhandled relay error", {
      error: error instanceof Error ? error.message : String(error),
      path: new URL(request.url).pathname,
    });
    return json(500, { error: "internal relay error" });
  }
}

export default { fetch: fetchHandler };

export const _test = {
  feedbackIdentity,
  fingerprint,
  readGitHubJSON,
  readRequestJSON,
  renderGitHubIssue,
  validateRepository,
  validateSubmission,
};
