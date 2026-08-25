// Pure role/navigation helpers: deciding what a user may see without touching
// the DOM, so layouts stay declarative and the rules are unit-testable.

export interface NavigationUser {
  role?: string;
  tenant_role?: string;
}

export function isAdmin(user: NavigationUser | null | undefined): boolean {
  return user?.role === "admin";
}

/** Enterprise owners and admins manage team budget and members. */
export function isEnterpriseAdmin(user: NavigationUser | null | undefined): boolean {
  return user?.tenant_role === "owner" || user?.tenant_role === "admin";
}

/** Enterprise members have read-only spend: no self-service wallet. */
export function isEnterpriseMember(user: NavigationUser | null | undefined): boolean {
  return user?.tenant_role === "member";
}

/** Whether a view path is accessible: /admin* requires the admin role. */
export function canAccessView(path: string, user: NavigationUser | null | undefined): boolean {
  if (path.startsWith("/admin")) return isAdmin(user);
  return true;
}

/** Default landing view for a role (admin console vs user dashboard). */
export function defaultViewForRole(user: NavigationUser | null | undefined): string {
  return isAdmin(user) ? "/admin/models" : "/dashboard";
}

/** Filters a nav item list to entries the user can access. */
export function filterNavItems<T extends { to: string }>(
  items: T[],
  user: NavigationUser | null | undefined,
): T[] {
  return items.filter((item) => canAccessView(item.to, user));
}
