import { exports } from "cloudflare:workers";
import { describe, expect, it } from "vitest";

import { _test } from "../src/relay.js";

describe("Feedback relay in the Workers runtime", () => {
  it("runs the production entrypoint and returns hardened JSON", async () => {
    const response = await exports.default.fetch("https://relay.example/not-found");
    expect(response.status).toBe(404);
    expect(response.headers.get("content-type")).toContain("application/json");
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(response.headers.get("x-content-type-options")).toBe("nosniff");
  });

  it("uses the Workers Web Crypto implementation", async () => {
    const value = await _test.fingerprint("runtime-smoke-test");
    expect(value).toMatch(/^[a-f0-9]{16}$/);
  });
});
