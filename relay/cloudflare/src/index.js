/**
 * Single-tenant feedback relay for Cloudflare Workers.
 *
 * The target repository and GitHub token are server-side configuration. A
 * client can describe a report, but cannot redirect the credential to another
 * repository.
 */

const MAX_TITLE_BYTES = 200;
const MAX_BODY_BYTES = 12_000;
const MAX_METADATA_BYTES = 160;
const MAX_REQUEST_BYTES = 96 * 1024;
const ALLOWED_FIELDS = new Set([
  "schema",
  "user_approved",
  "install_id",
  "product",
  "version",
  "os",
  "arch",
  "agent",
  "title",
  "body",
]);

class RequestTooLargeError extends Error {}

function json(status, payload) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
    },
  });
}

function byteLength(value) {
  return new TextEncoder().encode(value).byteLength;
}

function validShortString(value, required = false) {
  if (value === undefined || value === null || value === "") {
    return !required;
  }
  return (
    typeof value === "string" &&
    byteLength(value) <= MAX_METADATA_BYTES &&
    !/[\u0000-\u001f\u007f]/.test(value)
  );
}

function validateSubmission(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return "submission must be an object";
  }
  for (const key of Object.keys(value)) {
    if (!ALLOWED_FIELDS.has(key)) {
      return `unknown field: ${key}`;
    }
  }
  if (value.schema !== 1) {
    return "unsupported schema";
  }
  if (value.user_approved !== true) {
    return "explicit user approval is required";
  }
  if (!validShortString(value.product, true)) {
    return "product is required or too large";
  }
  for (const field of ["install_id", "version", "os", "arch", "agent"]) {
    if (!validShortString(value[field])) {
      return `${field} is too large or invalid`;
    }
  }
  if (value.install_id && !/^[A-Za-z0-9._-]{1,64}$/.test(value.install_id)) {
    return "install_id has invalid characters";
  }
  if (typeof value.title !== "string" || value.title.trim() === "") {
    return "title is required";
  }
  if (/[\u0000-\u001f\u007f]/.test(value.title)) {
    return "title contains control characters";
  }
  if (typeof value.body !== "string" || value.body.trim() === "") {
    return "body is required";
  }
  if (/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(value.body)) {
    return "body contains control characters";
  }
  if (byteLength(value.title) > MAX_TITLE_BYTES || byteLength(value.body) > MAX_BODY_BYTES) {
    return "submission is too large";
  }
  return null;
}

function validateRepository(value) {
  return typeof value === "string" && /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(value);
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
  return (path, init = {}) =>
    fetch(`https://api.github.com${path}`, {
      ...init,
      headers: {
        authorization: `Bearer ${env.GITHUB_TOKEN}`,
        accept: "application/vnd.github+json",
        "content-type": "application/json",
        "user-agent": "awesome-agent-app-features-feedback-relay",
        "x-github-api-version": "2022-11-28",
        ...(init.headers || {}),
      },
    });
}

async function handle(request, env) {
  const url = new URL(request.url);
  if (request.method !== "POST" || url.pathname !== "/v1/feedback") {
    return json(404, { error: "not found" });
  }
  if (
    !env.GITHUB_TOKEN ||
    !validateRepository(env.GITHUB_REPO) ||
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

  const clientKey =
    (submission.install_id && submission.install_id.slice(0, 64)) ||
    request.headers.get("cf-connecting-ip") ||
    "unknown";
  if (await rateLimited(env, clientKey)) {
    return json(429, { error: "rate limited" });
  }

  const repository = env.GITHUB_REPO;
  const label = validLabel(env.GITHUB_LABEL) ? env.GITHUB_LABEL : "user-feedback";
  const github = githubClient(env);
  const marker = `aaf-fp:${await fingerprint(`${submission.product}\n${submission.title}`)}`;
  const environmentLine = [
    submission.product,
    submission.version || "?",
    `${submission.os || "?"}/${submission.arch || "?"}`,
    submission.agent || "?",
  ].join(" · ");

  // Best-effort deduplication. GitHub search is eventually consistent, so the
  // relay never presents it as a strict uniqueness guarantee.
  try {
    const query = encodeURIComponent(`repo:${repository} is:issue is:open label:"${label}" "${marker}"`);
    const searchResponse = await github(`/search/issues?q=${query}&per_page=1`);
    if (searchResponse.ok) {
      const found = await searchResponse.json();
      const existing = found.items?.[0];
      if (existing && Number.isInteger(existing.number) && typeof existing.html_url === "string") {
        const commentResponse = await github(`/repos/${repository}/issues/${existing.number}/comments`, {
          method: "POST",
          body: JSON.stringify({ body: `+1 — ${environmentLine}` }),
        });
        if (commentResponse.ok && validIssueURL(existing.html_url, repository)) {
          return json(200, { issue_url: existing.html_url, deduplicated: true });
        }
        logError("dedup comment failed", { status: commentResponse.status });
      }
    } else {
      logError("dedup search failed", { status: searchResponse.status });
    }
  } catch (error) {
    logError("dedup lookup failed", {
      error: error instanceof Error ? error.message : String(error),
    });
  }

  const issueResponse = await github(`/repos/${repository}/issues`, {
    method: "POST",
    body: JSON.stringify({
      title: submission.title,
      body: `${submission.body}\n\n<!-- ${marker} -->`,
      labels: [label],
    }),
  });
  if (!issueResponse.ok) {
    logError("GitHub issue creation failed", { status: issueResponse.status });
    return json(502, { error: "issue creation failed" });
  }
  const issue = await issueResponse.json();
  if (!validIssueURL(issue.html_url, repository)) {
    return json(502, { error: "unexpected GitHub response" });
  }
  return json(200, { issue_url: issue.html_url, deduplicated: false });
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
      value.pathname.startsWith(`/${repository}/issues/`)
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
  fingerprint,
  readRequestJSON,
  validateRepository,
  validateSubmission,
};
