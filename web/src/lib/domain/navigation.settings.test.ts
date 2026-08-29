import { describe, it, expect } from "vitest";
import { filterNavItems } from "./navigation";

const items = [
  { to: "/dashboard" },
  { to: "/api-keys" },
  { to: "/models" },
  { to: "/admin/models" },
];

const user = { role: "admin", tenant_role: "owner" };

describe("filterNavItems hiddenPaths", () => {
  it("hides paths listed in hiddenPaths", () => {
    const visible = filterNavItems(items, user, ["/models"]);
    expect(visible.map((i) => i.to)).not.toContain("/models");
    expect(visible.map((i) => i.to)).toContain("/dashboard");
  });

  it("defaults to no hidden paths", () => {
    const visible = filterNavItems(items, user);
    expect(visible.map((i) => i.to)).toEqual(["/dashboard", "/api-keys", "/models", "/admin/models"]);
  });
});
