# Role-Specific Login Design

## Goal

Make the Admin and Customer sign-in experiences unambiguous while keeping authorization controlled by the BFF. An account may sign in only through the portal matching its stored server-side role.

## User Experience

- `/admin/login` displays the Admin sign-in screen and does not offer public registration.
- `/customer/login` displays the Customer sign-in screen and offers Customer registration.
- Unauthenticated visits to other Dashboard paths use the Customer portal as the default.
- Each portal includes a link to the other portal.
- A valid account used in the wrong portal receives a clear role-mismatch error and no verification challenge.
- After verification, the server session role still determines navigation and API access.

## BFF Design

Add two role-specific endpoints while retaining the existing generic endpoint for compatibility:

- `POST /bff/auth/admin/login`
- `POST /bff/auth/customer/login`
- `POST /bff/auth/login` remains available for existing clients.

All three routes use one internal login function. The role-specific routes compare the requested portal role with the role stored on the authenticated user. The comparison occurs only after the password is verified. A mismatch returns HTTP 403 and does not create or send a verification challenge.

The browser cannot assign a role. The BFF always reads the role from the stored user and binds the challenge and session to that user.

## Frontend Design

`LoginScreen` receives a fixed portal role instead of maintaining a role selected by segmented buttons. `loginAccount` accepts the portal role and maps it to the corresponding BFF endpoint. Customer registration remains unchanged.

The application derives the unauthenticated portal from `window.location.pathname`. Portal links use browser history and update the rendered login screen without creating a separate application bundle.

## Error Handling

- Missing credentials: HTTP 400.
- Invalid credentials: HTTP 401 with the existing generic message.
- Correct credentials in the wrong portal: HTTP 403 with a portal-specific message.
- Verification delivery failure and rate limiting retain existing behavior.

## Security

- Client-selected portal role is treated only as an expected role, never as authorization.
- Admin and Customer API middleware remains the final authorization boundary.
- Public registration creates Customer accounts only.
- Role mismatch does not issue a challenge or Session Cookie.

## Tests

- Go tests prove correct-role acceptance and wrong-role rejection for both portals.
- API service tests prove Admin and Customer calls use different endpoints.
- UI tests prove URL-to-portal mapping and registration availability.
- Existing unit, build, and Playwright suites must remain green.
