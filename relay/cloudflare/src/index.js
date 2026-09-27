import { fetchHandler } from "./relay.js";
import { diagnosticFetchHandler } from "./diagnostic.js";
export { FeedbackReport } from "./report-store.js";

/** @typedef {Env & {GITHUB_TOKEN: string}} RelayEnv */
/** @type {ExportedHandler<RelayEnv>} */
const worker = { fetch(request, env) {
  return new URL(request.url).pathname === "/v2/feedback"
    ? diagnosticFetchHandler(request, env) : fetchHandler(request, env);
} };

export default worker;
