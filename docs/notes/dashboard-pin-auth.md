# Dashboard PIN authentication — specification and verification map

Status: current as of 2026-09-17 (spaxel-d223bfdd). Implementation:
`mothership/internal/auth/handler.go`. Regression tests:
`mothership/internal/auth/handler_test.go` (endpoint and install-secret
contracts) and `mothership/internal/auth/middleware_session_test.go` (session
lifecycle and route-protection matrix). The original promise is plan.md's
Authentication section ("Dashboard access is protected by a PIN…").

This is the durable copy of what the PIN layer promises, what it actually
enforces today, and where each promise is pinned by a test. When a fact here
and the code disagree, the code wins — re-verify and fix this file.

## 1. Scope and threat model

- Single-admin home deployment: "A single PIN protects the mothership
  dashboard" (plan.md Non-Goals). No user accounts, no roles, no email reset,
  no rate limiting, no 2FA.
- **Privacy promise:** anyone with network reach to mothership must not be
  able to read positioning data (presence, trajectories, CSI) without the
  PIN.
- CSRF posture: the session cookie is `SameSite=Strict`, which is the CSRF
  story; no separate CSRF tokens. Secure attribute: see §5 (deliberate
  deviation from plan.md, documented there).

## 2. Data model

(handler.go `initializeAuth`, lines 71–110)

- `auth` — singleton row (`id = 1`): `pin_bcrypt` (bcrypt, cost 12), plus
  `install_secret`. The singleton makes "PIN configured" a cheap existence
  check (`IsPINConfigured`, handler.go:760).
- `sessions` — `session_id TEXT PRIMARY KEY` (32 CSPRNG bytes → 64 hex
  chars), `created_at` / `expires_at` / `last_seen_at` (Unix ms), index
  `idx_sessions_expires` for the cleanup sweep.
- `install_secret` is a separate mechanism (32 bytes; generated on first run
  and printed once, or seeded from `SPAXEL_INSTALL_SECRET` as 64-hex) used to
  derive per-node HMAC-SHA256 tokens for `/ws/node`. It is not a user
  credential and does not gate dashboard routes; its contract is pinned by
  `TestInstallSecret_*` in handler_test.go.

## 3. First-run PIN setup — `POST /api/auth/setup`

`handleSetup` (handler.go:253). Contract:

| Case | Response |
|---|---|
| No PIN configured, valid body | `200` + session cookie, `{"ok":"true"}` |
| Malformed / invalid PIN (not 4–8 numeric digits) | `400` |
| PIN already configured | `409` "PIN already configured" |

- PIN policy: 4–8 digits, numeric only. Stored only as a bcrypt hash (cost
  12); the plaintext never persists.
- A successful setup immediately mints a session — the admin who set the PIN
  is logged in, no separate login step.
- **Onboarding window:** until a PIN exists, the auth middleware passes every
  request (pages, API, and WebSocket alike). This is what makes zero-config
  first run possible. The window closes the moment a PIN is set and never
  reopens (short of the destructive recovery in §7).

## 4. Login — `POST /api/auth/login`

`handleLogin` (handler.go:333). Contract:

| Case | Response |
|---|---|
| Valid PIN | `200` + session cookie |
| Wrong PIN | `401` "Invalid PIN" + `[WARN] Failed login attempt from <RemoteAddr>` log |
| No PIN configured | `404` "PIN not configured" |

- **Failed-login handling (pinned):** a wrong PIN must not set *any* cookie
  and must not create a sessions row — `TestLogin_FailureCreatesNoCookieAndNoSession`.
  There is no lockout/cooldown; the WARN log is the audit trail.

## 5. Session contract

(`createSession` handler.go:504, `setSessionCookie` handler.go:528,
`ValidateSession` handler.go:545)

- Cookie: name `spaxel_session`, `Path=/`, `Max-Age=604800` (7 days),
  `HttpOnly`, `SameSite=Strict`, `Secure=false`.
  - **Deliberate deviation from plan.md**, which says the cookie is "secure"
    and SameSite=Strict only behind Traefik+TLS. The implementation is
    unconditional: `SameSite=Strict` always, `Secure` always false
    (`isSecure := false` — the deployment may be plain-HTTP on a LAN, and a
    `Secure` cookie would be dropped there). TLS termination is the reverse
    proxy's job.
- The session itself is server-side state: the cookie carries only the random
  64-hex ID; nothing about the session is client-computable.
- Session IDs are unique per login (fresh 32 random bytes each time) and
  concurrent sessions coexist — `TestSessionIDs_UniqueAcrossLogins`.
- Cookie attributes are pinned on both mint paths (setup and login) by
  `TestSessionCookieAttributes`.
- **Expiration:** validation rejects any session whose `expires_at` is in the
  past (`TestValidateSession_ExpiredSessionRejected`).
- **Rolling extension:** a validated session within 24h of expiry is extended
  to now + 7 days; a validated session further out only bumps `last_seen_at`.
  Pinned by `TestValidateSession_RollingExtension` (both paths).
- **Cleanup:** an hourly ticker (`cleanupExpiredSessions`, handler.go:624)
  deletes rows with `expires_at < now`. Expiry is therefore enforced at
  validation time immediately, and storage is reclaimed within the hour.

## 6. Logout — `POST /api/auth/logout`

`handleLogout` (handler.go:391). Deletes the sessions row (server-side
invalidation — a stolen cookie dies with logout) and clears the cookie with
`Max-Age=-1`, `HttpOnly`, `SameSite=Strict`, `Path=/`. Logout is a public
path, so it is reachable even when every other route is gated.

## 7. Change PIN ("reset") — `POST /api/auth/change-pin`

`handleChangePIN` (handler.go:422), behind `RequireAuth`:

| Case | Response |
|---|---|
| Correct current PIN, valid new PIN | `200`, hash replaced (bcrypt cost 12) |
| Wrong current PIN | `403` "Incorrect current PIN" |
| Invalid new PIN | `400` |
| No session | `401` (RequireAuth) |

- **Existing sessions remain valid after a PIN change** (handler.go comment
  at :496: "session tokens are independent of PIN"). This is deliberate: a
  PIN change is a credential rotation, not a session revocation. If revocation
  is wanted, log out (per-session) or clear the `sessions` table (all).
- **Lost-PIN recovery:** there is no email/hosted reset by design. Recovery is
  destructive: delete the `auth` singleton row (or wipe the data volume) and
  re-run first-run setup on next visit. The onboarding window opens only
  because `IsPINConfigured()` returns false again. Everything else (recordings,
  install secret) survives if only the row is deleted rather than the volume.

## 8. Route protection — `auth.Middleware`

`Middleware` (handler.go:794) is chi-compatible and evaluates stages in this
exact order:

1. `IsPublicPath` (handler.go:704) → pass. Exact list: `/healthz`,
   `/api/auth/status`, `/api/auth/setup`, `/api/auth/login`,
   `/api/auth/logout`, `/api/provision`, `/ws/node`, and everything under
   `/firmware/` (firmware URLs embed a SHA256 for integrity; the exemption is
   deliberate — see the /firmware notes in docs/notes/).
2. Static assets (`/js/`, `/css/`, `/images/`, `/favicon*`) → pass, so the
   login page can load its own assets.
3. Demo mode (`SPAXEL_DEMO_MODE`) → pass everything (reads *and* writes are
   admitted here; write-blocking is the separate `DemoModeMiddleware`, which
   403s mutating verbs with a JSON body — see §9).
4. Onboarding window (no PIN) → pass everything (§3).
5. Valid session cookie → pass.
6. Enforce:
   - `/api/*` and `/ws/*` → `401` `application/json`
     `{"error":"authentication required"}`. For WebSocket routes this fires
     **before the upgrade handshake**, so an unauthenticated client gets an
     HTTP 401, never a 101.
   - anything else (pages) → `200` `text/html` login-only page (`loginPage`,
     handler.go:776) whose script tag loads only `auth.js` — deleting the
     overlay reveals a blank page, never dashboard markup.

Full matrix pinned by `TestMiddleware_ProtectedRoutes`: onboarding-window
pass-through, locked-out table (pages vs. API vs. WS vs. public vs. static),
the 401 JSON contract, the login-page contract, valid-session pass-through
including `/ws/dashboard`, and expired/unknown-session rejection.
`IsPublicPath` itself has a table test (`TestPublicPaths`, includes
`/ws/dashboard` = false).

**Protected route inventory (when the middleware is wired):** every `/api/*`
and `/ws/*` route, including `/ws/dashboard` (the Three.js live feed) — except
the public list above. `/api/doctor` is additionally wrapped in
`RequireAuth` at its registration site (main.go:5105), so it stays protected
even without the middleware installed.

## 9. Demo mode interplay

- `DemoModeMiddleware` (handler.go:731; `r.Use`d in main.go:754) blocks
  `POST/PUT/PATCH/DELETE` with `403` JSON in demo mode and passes GET/HEAD/
  OPTIONS.
- Inside the auth middleware, demo mode short-circuits *before* the
  PIN/session stages (stage 3 above): a demo instance is read-only-public by
  definition. Pinned by `TestMiddlewareDemoModePINBypass` and
  `TestDemoModeMiddleware`.

## 10. Deployment wiring — what is actually enforced where

This section is the load-bearing one. **`auth.Middleware` is built and fully
regression-tested but is NOT installed in the live router.**

- Commit `821b3823` (2026-04-13, "remove(auth): drop PIN-based auth — Google
  OAuth handles access") removed `r.Use(authHandler.Middleware)`, the
  `/api/auth/*` endpoint registrations, and `auth.js` from the HTML pages,
  because the production install sits behind Traefik forward-auth (Google
  OAuth). A later commit re-added `authHandler.RegisterRoutes(r)` and
  `DemoModeMiddleware`, so today (main.go:754, :811–817):
  - installed: `DemoModeMiddleware`; `/api/auth/*` routes; `/api/doctor`
    behind `RequireAuth`.
  - **not installed:** `authHandler.Middleware`.
  - `/ws/dashboard` is registered bare (main.go:4948):
    `HandleDashboardWS` upgrades the connection with **no session-cookie
    check of its own** (dashboard/server.go).
- Consequence matrix:

| Deployment | Page routes | REST API | `/ws/dashboard` |
|---|---|---|---|
| Behind Traefik forward-auth (ardenone) | Google SSO | Google SSO, minus forward-auth exemptions (`/healthz`, `/api/provision`, `/ws/node`, `/firmware/*` — deliberately the same set as `IsPublicPath`) | Google SSO |
| Bare / LAN, no proxy | open (or login overlay only where a page embeds `auth.js`, e.g. `ambient.html`) | open once the onboarding window closes… meaning **never gated** | open — upgrades unauthenticated |

- In other words: plan.md's promise that "`/ws/dashboard` verifies the
  session cookie before upgrading" holds **iff the middleware is installed**.
  On a bare deployment with only Traefik absent, the PIN layer does not
  gate REST/WS today. This is the documented gap between the plan and the
  default wiring — it is a deployment decision, not a missing feature.
- **How to enable the PIN layer** on a bare deployment: add
  `r.Use(authHandler.Middleware)` where the auth handler is constructed in
  `cmd/mothership/main.go`. One line; the middleware's behavior, including
  the 401-before-upgrade contract for `/ws/dashboard`, is fully pinned by
  `TestMiddleware_ProtectedRoutes`.
- **Why it is not re-wired by default:** production users authenticate via
  Google and many have never set a PIN. With the middleware installed and a
  PIN set, every one of them would be locked out of the dashboard at the app
  layer (the onboarding window only covers the pre-PIN state). Re-installing
  the middleware is a per-deployment choice, not a global default.

## 11. Client-side contract

- `dashboard/js/auth.js` (`SpaxelAuth`): on load, `GET /api/auth/status`
  (`pin_configured`, plus a demo-mode probe via `HEAD /api/settings`) → routes
  the browser to first-run setup (`POST /api/auth/setup`), the login form
  (`POST /api/auth/login`), or straight through. Covered by
  `dashboard/js/auth.test.js`.
- `dashboard/ambient.html` is the only HTML page still embedding `auth.js`;
  it redirects to `/` when the status check fails.
- `index.html` has no client-side gate — it relies entirely on the server
  layer (Traefik today; the middleware if enabled). Client-side gating is a
  UX affordance, never the enforcement point.

## 12. Verification matrix

| plan.md promise | Contract | Pinned by |
|---|---|---|
| one-time setup page on first run | §3 | `TestHandler_SetupPIN`, `TestHandler_SetupPINInvalid`, `TestHandler_SetupPINAlreadyConfigured`, onboarding window in `TestMiddleware_ProtectedRoutes` |
| bcrypt hash in SQLite | §3 | `TestHandler_LoginValidPIN` (verifies via login), change-PIN tests |
| login issues session cookie, 7-day TTL | §5 | `TestSessionCookieAttributes`, `TestHandler_LoginValidPIN` |
| secure/HttpOnly/SameSite cookie | §5 | `TestSessionCookieAttributes` |
| subsequent visits require the PIN | §8 | `TestMiddleware_ProtectedRoutes` (locked-out table) |
| `/ws/dashboard` verifies cookie before upgrade; 401 otherwise | §8, §10 | `TestMiddleware_ProtectedRoutes` (ws rows + valid-session pass) — **conditional on the middleware being installed**, see §10 |
| session expiration | §5 | `TestValidateSession_ExpiredSessionRejected`, middleware expired-session row |
| failed-login handling | §4 | `TestLogin_FailureCreatesNoCookieAndNoSession` |
| reset behavior | §7 | `TestHandler_ChangePIN_*` (success / wrong old / unauthenticated / invalid new); session-survival documented at handler.go:496 |
| protected REST routes | §8 | `TestMiddleware_ProtectedRoutes`, `TestPublicPaths`, RequireAuth tests |
| node ingestion stays token-gated | §2 | `TestInstallSecret_NodeTokenDerivation`, `TestValidateSession` untouched by it |
