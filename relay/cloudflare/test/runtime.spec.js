import { exports } from "cloudflare:workers";
import { describe, expect, it, vi } from "vitest";

import { _test, fetchHandler } from "../src/relay.js";

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

  it("never follows redirects with a GitHub credential or approved report", async () => {
    const outgoing = [];
    const mock = vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      outgoing.push(new Request(url, init));
      return String(url).includes("/search/issues")
        ? Response.json({items: []})
        : Response.json({html_url: "https://github.com/owner/repository/issues/1"});
    });
    try {
      const response = await fetchHandler(new Request("https://relay.example/v1/feedback", {
        method: "POST",
        headers: {"content-type": "application/json"},
        body: JSON.stringify({
          schema: 1, user_approved: true,
          environment: {product: "Example"}, description: "A bounded report",
        }),
      }), {
        GITHUB_TOKEN: "test-token", GITHUB_REPO: "owner/repository",
        RATE_LIMITER: {async limit() { return {success: true}; }},
      });
      expect(response.status).toBe(200);
      expect(outgoing).toHaveLength(2);
      for (const request of outgoing) {
        expect(request.headers.get("authorization")).toBe("Bearer test-token");
        expect(request.redirect).toBe("manual");
      }
    } finally {
      mock.mockRestore();
    }
  });
});
