import { createRequire } from "node:module";
import { describe, expect, it, vi } from "vitest";

// Resolve the actual SDK dependency, bypassing Vitest's Obsidian runtime stub.
// This is development-tooling coverage; Moment is not added to the plugin bundle.
const requirePlugin = createRequire(new URL("../package.json", import.meta.url));
const requireSDK = createRequire(requirePlugin.resolve("obsidian/package.json"));
const moment = requireSDK("moment") as typeof import("moment");

describe("Obsidian SDK dependency security", () => {
  it("installs the reviewed Moment fix for GHSA-4p3w-j4w9-5jqw", () => {
    expect(moment.version).toBe("2.31.0");
  });

  it("normalizes locale objects without trusting their match method", () => {
    const match = vi.fn(() => { throw new Error("untrusted locale matcher called"); });
    const candidate = {
      toString: () => "stele-invalid-locale",
      toLowerCase: () => "en",
      match,
    };
    // No traversal path or fixture module is used. The old vulnerable version
    // invokes candidate.match; the patched loader uses the canonical string.
    const actual: unknown = Reflect.apply(moment.localeData, moment, [candidate]);
    expect(actual).toBe(moment.localeData("en"));
    expect(match).not.toHaveBeenCalled();
  });
});
