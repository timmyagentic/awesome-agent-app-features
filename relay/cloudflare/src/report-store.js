import { DurableObject } from "cloudflare:workers";
import { protocol } from "./relay.js";
import { reportHash, submitReport, validateDiagnosticSubmission } from "./diagnostic.js";

/** @extends {DurableObject<Env>} */
export class FeedbackReport extends DurableObject {
  /** @type {{hash: string, task: Promise<import("./diagnostic.js").SubmissionResult>} | null} */
  active = null;

  /** @param {unknown} value @param {string} token
   * @returns {Promise<import("./diagnostic.js").SubmissionResult>} */
  async submit(value, token) {
    const error = validateDiagnosticSubmission(value);
    if (error) return {status: 400, body: {error}};
    const submission = /** @type {import("./diagnostic.js").DiagnosticSubmission} */ (value);
    const hash = await reportHash(submission);
    if (this.active) {
      if (this.active.hash !== hash) return {status: 409, body: {error: "report_id is already bound to another payload"}};
      return this.active.task;
    }
    // In-memory state only coalesces concurrent requests. The durable record
    // owns recovery after eviction/crash and contains no diagnostic payload.
    const controller = new AbortController();
    const deadline = setTimeout(() => controller.abort(), 15_000);
    const github = protocol.githubClient({...this.env, GITHUB_TOKEN: token});
    const task = submitReport({
      get: () => this.ctx.storage.get("report"),
      put: record => this.ctx.storage.put("report", record),
    }, submission, (path, init = {}) => github(path, {...init, signal: controller.signal}), this.env.GITHUB_REPO, this.env.GITHUB_LABEL, hash);
    this.active = {hash, task};
    try { return await task; }
    finally { clearTimeout(deadline); this.active = null; }
  }
}
