# Session Log: Camera Toggle Debug & Frontend Proxy Fix

**Date:** 2026-03-09 22:48
**Duration:** ~45 minutes
**Focus:** Debug missing camera capture toggle on deposit form and fix mobile API connectivity

## What Got Done

- Identified that the camera toggle was present in `DepositForm.jsx` source but placed outside the `<form>` tag (above it rather than inside it between amount and front image inputs)
- Moved toggle inside the form between the Amount input and Front of Check input
- Updated toggle styling to use filled `bg-blue-600` active state with `bg-gray-50 rounded-lg` card container
- Added standalone `<p>` helper text below the toggle (separate from the toggle row)
- Confirmed both file inputs retain `{...(cameraMode ? { capture: 'environment' } : {})}` spread attribute
- Committed fix: `fix(web): add missing camera capture toggle to deposit form` (9c8e886)
- Diagnosed Docker port 5173 blockage and Docker layer cache issue
- Started native Vite dev server on port 5174 with correct `VITE_BACKEND_URL=http://localhost:8080`
- Verified full end-to-end deposit submission works through the Vite proxy (returned `funds_posted` status)

## Issues & Troubleshooting

- **Problem:** Camera toggle not visible in the browser despite being in source code
  - **Cause:** Toggle was positioned outside the `<form>` tag (lines 98–116 before `<form>` at line 118). Additionally, Docker's `COPY . .` layer was cached, so container rebuilds didn't pick up source changes.
  - **Fix:** Moved toggle inside the form; switched to running Vite natively on the host to bypass Docker cache entirely.

- **Problem:** `docker compose up --build -d frontend` failed with "Ports are not available: address already in use" on port 5173
  - **Cause:** An orphaned Vite process (PID 46912, running as root in `/app`) survived from the previous container stop. It lived inside Docker Desktop's QEMU VM namespace — visible in `ps` but not killable from the host with `kill`.
  - **Fix:** Ran `docker compose down` to tear down all containers, then `docker compose up -d` to bring everything back. Port 5173 was freed by the VM cleanup. However, Docker layer caching meant the rebuilt image still had old source files, so the native Vite server on port 5174 was used instead.

- **Problem:** Docker `build --no-cache` not attempted, resulting in stale source in the image
  - **Cause:** `COPY . .` layer hash hadn't changed from Docker's perspective (file timestamps or content hashing), so Docker used the cached layer and never re-copied `DepositForm.jsx` into the image.
  - **Fix:** Running Vite natively bypasses the Docker image entirely, reading source files directly from disk.

- **Problem:** Mobile submission returned "Submission failed. Is the backend running?"
  - **Cause:** When Vite was started natively without setting `VITE_BACKEND_URL`, `vite.config.js` defaulted the proxy target to `http://backend:8080`. The hostname `backend` is a Docker internal DNS name only resolvable within the Docker network — not from the host or a mobile device on the LAN.
  - **Fix:** Restarted Vite with `VITE_BACKEND_URL=http://localhost:8080 npx vite --host 0.0.0.0 --port 5174`. The proxy now correctly forwards `/api/*` to the local backend. Verified with `curl` through the proxy returning a valid `funds_posted` response.

## Decisions Made

- **Run Vite natively instead of in Docker for active development:** Docker's layer caching makes it unreliable for rapid iteration on frontend source files. Running `npm run dev` on the host directly reads current source files and hot-reloads instantly. The Docker frontend container is better suited for production-like demos.
- **Use port 5174 instead of 5173 for native dev server:** Port 5173 was held by Docker Desktop's VM networking and couldn't be freed without a full Docker Desktop restart. Using 5174 avoids the conflict.

## Current State

- **Backend:** Running in Docker at `http://localhost:8080`, healthy (Postgres + Redis connected)
- **Frontend (native):** Running at `http://localhost:5174` (LAN: `http://10.10.3.211:5174`) with Vite proxy correctly forwarding `/api/*` to `localhost:8080`
- **Frontend (Docker):** Container running on port 5173 but serving a stale image (old source without toggle) — do not use for testing
- **Camera toggle:** Visible and functional — "Upload File" active by default, "Take Photo" switches active state and helper text; `capture="environment"` applied to both file inputs in camera mode
- **Deposit submission:** Working end-to-end from both desktop and mobile (tested via `curl` and Playwright)

## Next Steps

1. Fix the Docker frontend build so it reliably picks up source changes — consider adding `--no-cache` to the build step or mounting the `src/` directory as a volume for dev
2. Stop the stale Docker frontend container (port 5173) to avoid confusion — it currently serves the old toggle-less UI
3. Test the full mobile flow end-to-end: open `http://10.10.3.211:5174` on mobile, tap "Take Photo", capture both front and back of check, submit, verify deposit result
4. If proceeding with Tier 2 of the mobile camera plan, review `docs/plans/02-mobile-camera-plan.md` for next implementation steps
