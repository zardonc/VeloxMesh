export type AccountRole = "Admin" | "Customer";
export type AuthMode = "login" | "register";

export function portalRoleForPathname(pathname: string): AccountRole {
  return pathname === "/admin/login" ? "Admin" : "Customer";
}

type PortalClick = {
  altKey: boolean;
  button: number;
  ctrlKey: boolean;
  defaultPrevented: boolean;
  metaKey: boolean;
  shiftKey: boolean;
};

export function shouldHandlePortalClick(event: PortalClick): boolean {
  return event.button === 0
    && !event.defaultPrevented
    && !event.altKey
    && !event.ctrlKey
    && !event.metaKey
    && !event.shiftKey;
}

export function canRegisterRole(role: AccountRole): boolean {
  return role === "Customer";
}

export function authCopy(mode: AuthMode, role: AccountRole, isVerifying: boolean) {
  if (isVerifying) {
    return {
      brandLabel: "Email Verification",
      title: "Email verification",
      description: "Enter the 6-digit code sent to your email to finish signing in."
    };
  }

  if (mode === "login") {
    return {
      brandLabel: role === "Admin" ? "Admin Dashboard" : "Customer Dashboard",
      title: `${role} sign in`,
      description: role === "Admin"
        ? "Use an admin account to open provider management, routing controls, benchmarks, request logs, and audit records."
        : "Use a customer account to open gateway status, account role, request access, and usage summary."
    };
  }

  return {
		brandLabel: "Customer Dashboard",
		title: "Create customer account",
		description: "Create a customer account and organization tenant for gateway usage, requests, and API access."
  };
}
