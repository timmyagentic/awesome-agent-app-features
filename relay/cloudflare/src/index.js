import { fetchHandler } from "./relay.js";

/** @typedef {Env & {GITHUB_TOKEN: string}} RelayEnv */
/** @type {ExportedHandler<RelayEnv>} */
const worker = { fetch: fetchHandler };

export default worker;
