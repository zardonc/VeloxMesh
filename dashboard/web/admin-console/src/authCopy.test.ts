import { describe, expect, it } from "vitest";
import { authCopy, canRegisterRole, portalRoleForPathname, shouldHandlePortalClick } from "./authCopy";

describe("authCopy", () => {
  it("uses role-specific copy on the admin login screen", () => {
    const copy = authCopy("login", "Admin", false);

    expect(copy.title).toBe("Admin sign in");
    expect(copy.brandLabel).toBe("Admin Dashboard");
    expect(copy.description).toContain("provider management");
  });

  it("uses role-specific copy on the customer login screen", () => {
    const copy = authCopy("login", "Customer", false);

    expect(copy.title).toBe("Customer sign in");
    expect(copy.brandLabel).toBe("Customer Dashboard");
    expect(copy.description).toContain("gateway status");
  });

  it("offers public registration only for Customer accounts", () => {
		expect(canRegisterRole("Customer")).toBe(true);
		expect(canRegisterRole("Admin")).toBe(false);
		expect(authCopy("register", "Admin", false).title).toBe("Create customer account");
    expect(authCopy("register", "Customer", false).title).toBe("Create customer account");
  });

  it("maps only the admin login path to the Admin portal", () => {
    expect(portalRoleForPathname("/admin/login")).toBe("Admin");
    expect(portalRoleForPathname("/customer/login")).toBe("Customer");
    expect(portalRoleForPathname("/admin")).toBe("Customer");
    expect(portalRoleForPathname("/somewhere-else")).toBe("Customer");
    expect(portalRoleForPathname("/")).toBe("Customer");
  });

  it("intercepts only ordinary unmodified primary portal clicks", () => {
    const ordinaryClick = {
      altKey: false,
      button: 0,
      ctrlKey: false,
      defaultPrevented: false,
      metaKey: false,
      shiftKey: false
    };

    expect(shouldHandlePortalClick(ordinaryClick)).toBe(true);
    expect(shouldHandlePortalClick({ ...ordinaryClick, button: 1 })).toBe(false);
    expect(shouldHandlePortalClick({ ...ordinaryClick, altKey: true })).toBe(false);
    expect(shouldHandlePortalClick({ ...ordinaryClick, ctrlKey: true })).toBe(false);
    expect(shouldHandlePortalClick({ ...ordinaryClick, metaKey: true })).toBe(false);
    expect(shouldHandlePortalClick({ ...ordinaryClick, shiftKey: true })).toBe(false);
    expect(shouldHandlePortalClick({ ...ordinaryClick, defaultPrevented: true })).toBe(false);
  });
});
