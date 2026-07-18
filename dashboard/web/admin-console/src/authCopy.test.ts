import { describe, expect, it } from "vitest";
import { authCopy, canRegisterRole } from "./authCopy";

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
});
