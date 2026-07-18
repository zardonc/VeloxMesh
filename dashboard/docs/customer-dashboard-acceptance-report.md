# Customer Dashboard Final Acceptance Report

Date: 2026-07-17  
Scope: Customer filtering, pagination, usage ranges, tenant isolation, UI states, responsive behavior, authentication, and persistence.

## Result

Status: **Passed**

- Go test suite: passed.
- Frontend unit tests: 38 passed, 0 failed.
- Production build: passed.
- Playwright acceptance tests: 4 passed, 0 failed.
- Admin Dashboard, Benchmarks, CSV/HTML export, Provider Health, and Requests/Logs regression checks: passed.

## Accepted Behavior

- My Requests uses server-side `page`, `pageSize`, `status`, `model`, `from`, and `to` parameters.
- Page sizes 25, 50, and 100, previous/next navigation, totals, and Clear filters are available.
- Usage supports the last 24 hours, 7 days, 30 days, and a custom time range.
- Usage shows requests, input/output/total tokens, average/P95 latency, daily trend, and model distribution.
- Customer A and Customer B use independent browser sessions and cannot read each other's requests, usage, or API keys.
- Query, header, and request-body tenant injection does not override the authenticated session tenant.
- Cross-tenant API key deletion returns 404; unauthenticated Customer API access returns 401; Customer access to Admin APIs returns 403.
- API key secrets are shown once and subsequent lists contain only masked values.
- Loading, Empty, Error, No Permission, and Partial Data states are covered.
- Refresh restores the active session. Logout invalidates the previous session cookie.
- Users, tenants, and API keys persist after a BFF restart. Sessions intentionally require a new login after restart.
- 1440x900, 1024x768, and 390x844 layouts have no page-level horizontal overflow and keep controls usable.

## Repeatable Commands

```powershell
cd "C:\Users\USER\Desktop\capstone\dashboard"
go test ./...

cd "C:\Users\USER\Desktop\capstone\dashboard\web\admin-console"
npm.cmd test
npm.cmd run build
npm.cmd run test:e2e
```

`npm.cmd run test:e2e` starts an isolated Redis container, BFF, and Vite server, uses unique state/outbox files, and removes the Redis container after completion.

## Evidence

- Playwright HTML report: `web/admin-console/playwright-report/index.html`
- Acceptance artifacts: `web/admin-console/test-results/`
- Desktop screenshot: `customer-requests-desktop-1440x900.png`
- Tablet screenshot: `customer-requests-tablet-1024x768.png`
- Mobile screenshot: `customer-requests-mobile-390x844.png`

## Remaining Risks

- The acceptance data is intentionally small and isolated; it does not establish performance at the full MMLU/LMSYS dataset scale.
- Production SMTP delivery and upstream provider reliability require deployment-environment testing.
- Large request-history performance should be measured separately with the final benchmark workload and production-like Redis capacity.
