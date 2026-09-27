import {env, exports} from "cloudflare:workers";
import {evictDurableObject, runInDurableObject} from "cloudflare:test";
import {setupNetwork} from "@msw/cloudflare";
import {http, HttpResponse} from "msw";
import {afterAll, afterEach, beforeAll, describe, expect, it, vi} from "vitest";
import {reportHash} from "../src/diagnostic.js";

const repository = env.GITHUB_REPO;

function report() {
  return {schema: 2, user_approved: true, report_id: crypto.randomUUID().replaceAll("-", ""), environment: {product: "Runtime test"}, description: "a bounded turn report"};
}

const network = setupNetwork(http.all("*", () => { throw new Error("unexpected outbound request"); }));
beforeAll(() => network.enable());
afterEach(() => { vi.restoreAllMocks(); network.resetHandlers(); });
afterAll(() => network.disable());

describe("durable Feedback v2 in workerd", () => {
  it("rejects unapproved v2 before storage/network", async () => {
    const outgoing = vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("unexpected network"));
    const response = await exports.default.fetch("https://relay.example/v2/feedback", {method: "POST", headers: {"content-type": "application/json"}, body: JSON.stringify({...report(), user_approved: false})});
    expect(response.status).toBe(400);
    expect(outgoing).not.toHaveBeenCalled();
  });

  it("serializes concurrent approvals, binds exact payload and survives object eviction", async () => {
    const value = report();
    const stub = env.FEEDBACK_REPORTS.getByName(`${repository}:${value.report_id}`);
    let posts = 0;
    network.use(http.post(`https://api.github.com/repos/${repository}/issues`, ({request}) => {
      posts++;
      expect(request.redirect).toBe("manual");
      return HttpResponse.json({html_url: `https://github.com/${repository}/issues/123`});
    }));
    const responses = await Promise.all(Array.from({length: 8}, () => stub.submit(value, "runtime-test-token")));
    expect(responses.every(result => result.status === 200)).toBe(true);
    expect(posts).toBe(1);
    await evictDurableObject(stub);
    expect((await stub.submit(value, "runtime-test-token")).body.deduplicated).toBe(true);
    expect((await stub.submit({...value, description: "changed after approval"}, "runtime-test-token")).status).toBe(409);
    expect(posts).toBe(1);
    const stored = await runInDurableObject(stub, (_instance, state) => state.storage.get("report"));
    expect(Object.keys(stored).sort()).toEqual(["hash", "receipt", "state"]);
    expect(JSON.stringify(stored)).not.toContain("runtime-test-token");
  });

  it("reconciles an unknown outcome after eviction without repeating issue creation", async () => {
    const value = report();
    const stub = env.FEEDBACK_REPORTS.getByName(`${repository}:${value.report_id}`);
    const hash = await reportHash(value);
    await runInDurableObject(stub, (_instance, state) => state.storage.put("report", {hash, state: "dispatching"}));
    await evictDurableObject(stub);
    let searches = 0;
    network.use(http.get("https://api.github.com/search/issues", () => { searches++; return HttpResponse.json({items: []}); }));
    expect((await stub.submit(value, "runtime-test-token")).status).toBe(503);
    network.use(http.get("https://api.github.com/search/issues", () => { searches++; return HttpResponse.json({items: [{html_url: `https://github.com/${repository}/issues/123`, body: `body\n<!-- aaf-report:${value.report_id}:${hash} -->`}]}); }));
    expect((await stub.submit(value, "runtime-test-token")).status).toBe(200);
    expect(searches).toBe(2);
  });
});
