import { describe, it, expect } from "vitest";
import {
  isAdmin,
  isEnterpriseAdmin,
  isEnterpriseMember,
  canAccessView,
  defaultViewForRole,
  filterNavItems,
} from "./navigation";

describe("role helpers", () => {
  it("isAdmin only for role=admin", () => {
    expect(isAdmin({ role: "admin" })).toBe(true);
    expect(isAdmin({ role: "user" })).toBe(false);
    expect(isAdmin(null)).toBe(false);
    expect(isAdmin(undefined)).toBe(false);
  });

  it("isEnterpriseAdmin for owner or admin tenant role", () => {
    expect(isEnterpriseAdmin({ tenant_role: "owner" })).toBe(true);
    expect(isEnterpriseAdmin({ tenant_role: "admin" })).toBe(true);
    expect(isEnterpriseAdmin({ tenant_role: "member" })).toBe(false);
  });

  it("isEnterpriseMember only for member tenant role", () => {
    expect(isEnterpriseMember({ tenant_role: "member" })).toBe(true);
    expect(isEnterpriseMember({ tenant_role: "owner" })).toBe(false);
  });
});

describe("navigation helpers", () => {
  it("canAccessView gates /admin paths on the admin role", () => {
    expect(canAccessView("/admin/models", { role: "admin" })).toBe(true);
    expect(canAccessView("/admin/models", { role: "user" })).toBe(false);
    expect(canAccessView("/admin/models", null)).toBe(false);
    expect(canAccessView("/dashboard", { role: "user" })).toBe(true);
    expect(canAccessView("/usage", null)).toBe(true);
  });

  it("defaultViewForRole lands admins in the console and users on the dashboard", () => {
    expect(defaultViewForRole({ role: "admin" })).toBe("/admin/models");
    expect(defaultViewForRole({ role: "user" })).toBe("/dashboard");
    expect(defaultViewForRole(null)).toBe("/dashboard");
  });

  it("filterNavItems removes admin-only entries for non-admins", () => {
    const items = [
      { to: "/usage", label: "用量" },
      { to: "/admin/audit", label: "审计" },
    ];
    expect(filterNavItems(items, { role: "admin" })).toHaveLength(2);
    expect(filterNavItems(items, { role: "user" }).map((i) => i.to)).toEqual(["/usage"]);
  });
});
