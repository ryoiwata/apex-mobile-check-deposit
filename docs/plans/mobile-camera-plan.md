# Mobile Check Capture — Implementation Plan

## Current State

The deposit form (`web/src/components/DepositForm.jsx`) uses two standard HTML file input elements for the front and back check images:

```jsx
<input type="file" accept="image/*" onChange={e => setFrontFile(e.target.files[0] || null)} />
<input type="file" accept="image/*" onChange={e => setBackFile(e.target.files[0] || null)} />
```

Both inputs are optional — if no file is selected, the form falls back to a hardcoded 1×1 white PNG placeholder (`createPlaceholderFile()`), allowing the deposit to proceed without a real image. This is sufficient for the demo since the vendor stub ignores image content entirely (it keys on account ID suffix only).

The app is already accessible on mobile devices without any changes. Vite's dev server binds to `0.0.0.0` by default, so any device on the same WiFi network can open `http://<machine-ip>:5173` and use the full app. The backend at `:8080` is similarly accessible.

On mobile, the standard `<input type="file" accept="image/*">` element opens a system picker that lets the user choose between the camera roll and the camera. It does not open the rear camera directly or provide a live preview.

---

## Tier 1: HTML5 Camera Capture (30 minutes)

**Goal:** Tapping the image input on a mobile device opens the rear camera directly.

### Changes required

- In `DepositForm.jsx`, add `capture="environment"` to both file input elements alongside the existing `accept="image/*"`:

```jsx
<input type="file" accept="image/*" capture="environment" onChange={...} />
```

- Add a toggle or separate button pair: **"Upload File"** (existing behavior, no `capture` attribute) vs **"Take Photo"** (adds `capture="environment"`). This lets desktop users keep normal file selection while mobile users get a direct camera shortcut.
- On desktop browsers, the `capture` attribute is ignored — behavior falls back to the normal file picker with no regression.
- No JavaScript changes beyond the toggle state, no new dependencies, no backend changes.

### Testing plan

- **Desktop:** verify file upload still works normally (no capture attribute in effect)
- **Mobile (same WiFi):** open `http://<ip>:5173`, tap "Take Photo", verify rear camera opens immediately without going through the system photo picker
- Snap a photo of the front, then the back, submit the deposit
- Verify the captured image flows through the full pipeline: vendor stub → funding rules → ledger posting → `funds_posted` status returned

### Limitations

- No live camera preview within the app — the native camera app opens, captures, and returns to the browser
- No edge detection, auto-crop, or framing guide for the check
- No image quality feedback before submission (blur, glare detection must rely on the vendor stub)
- Image quality depends entirely on the phone's default camera settings and the user's technique

---

## Tier 2: Progressive Web App (2–3 hours)

**Goal:** App can be installed to the phone's home screen and launches fullscreen like a native app.

### Changes required

**1. Web App Manifest** — create `web/public/manifest.json`:

```json
{
  "name": "Mobile Check Deposit",
  "short_name": "Check Deposit",
  "description": "Deposit checks into your brokerage account",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#1e293b",
  "theme_color": "#3b82f6",
  "icons": [
    { "src": "/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/icon-512.png", "sizes": "512x512", "type": "image/png" }
  ]
}
```

**2. Service worker** — create `web/public/sw.js` for offline shell caching (cache the HTML/JS/CSS app shell; do not cache API responses, which must be live):

```js
const CACHE = 'mcd-shell-v1'
const SHELL = ['/', '/index.html', '/assets/...']
self.addEventListener('install', e => e.waitUntil(caches.open(CACHE).then(c => c.addAll(SHELL))))
self.addEventListener('fetch', e => { /* network-first for /api, cache-first for shell */ })
```

**3. Register service worker** — in `web/src/main.jsx`:

```js
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js')
}
```

**4. HTML meta tags** — in `web/index.html`:

```html
<link rel="manifest" href="/manifest.json" />
<meta name="theme-color" content="#3b82f6" />
<meta name="apple-mobile-web-app-capable" content="yes" />
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
<meta name="apple-mobile-web-app-title" content="Check Deposit" />
```

**5. Icons** — generate simple 192×192 and 512×512 PNG icons with "MCD" text on a blue background. Can be created with a Canvas snippet or any image tool; commit as `web/public/icon-192.png` and `web/public/icon-512.png`.

**6. Tier 1 changes** — include the `capture="environment"` additions from Tier 1.

### Testing plan

- Open the app on mobile Chrome → verify Chrome's "Add to Home Screen" banner or install prompt appears
- Install → verify the app launches fullscreen without the browser address bar or tab chrome
- Verify camera capture and deposit submission work correctly from the installed PWA context
- Disconnect from WiFi → verify the app shell loads (the UI renders) even when the backend is unreachable; API calls should show a clear error, not a blank screen
- Verify the app icon and splash screen appear correctly on the home screen

### Limitations

- Not an app store listing — distributed by sharing a URL, not through the App Store or Google Play
- iOS Safari PWA support is more limited than Android Chrome: no install prompt, must use "Add to Home Screen" manually from the share sheet; some PWA features unavailable
- No push notifications without a separate push service (FCM/APNs) and notification permission flow
- Service worker caching is basic — does not provide an offline deposit queue or background sync

---

## Tier 3: React Native App (1–2 weeks)

**Goal:** Native mobile app with real-time camera preview, edge detection, and app store distribution.

### Architecture

- **Backend:** existing Go API unchanged — all endpoints stay identical; no new backend contracts required
- **Frontend:** new React Native app using Expo managed workflow for faster development iteration
- **API client:** port `web/src/api.js` to React Native `fetch` calls; the request/response contracts are identical to the web app
- **Auth:** same `Authorization: Bearer tok_investor_test` and `X-Operator-ID: OP-001` headers initially; can upgrade to device-stored token with biometric unlock later

### Key new components

- **`CameraScreen.tsx`** — uses `expo-camera` for a live in-app camera preview with:
  - Auto-focus tuned for document capture
  - Rectangle framing guide overlay to align the check
  - Capture front image, then prompt to flip and capture back
  - Client-side image sharpness heuristic (Laplacian variance) before submission to catch blur before the vendor stub does
- **`DepositScreen.tsx`** — amount input, account selector, submit button (mirrors `DepositForm.jsx` logic)
- **`StatusScreen.tsx`** — transfer state tracker with polling (mirrors `TransferStatus.jsx`)
- **`QueueScreen.tsx`** — operator review queue with image display, approve/reject (mirrors `ReviewQueue.jsx`)
- **Navigation:** React Navigation tab navigator with the same 4 tabs (Deposit, My Deposits, Operator Queue, Ledger)

### Dependencies

```
expo (~51.0)
expo-camera
expo-image-picker
expo-file-system
@react-navigation/native
@react-navigation/bottom-tabs
react-native-paper or nativewind (styling)
```

### New backend requirements

- **CORS:** add the Expo Go development origin (`exp://192.168.x.x:8081`) and the production app origin to the Gin CORS allowlist
- **Image compression:** React Native's camera produces large JPEGs; may need to tune `expo-camera` quality setting or add a backend image compression step to stay within upload limits
- **Push notifications (optional):** new endpoint to store FCM/APNs device tokens per account; trigger on state transitions (approved, rejected, returned)

### Testing plan

- Use **Expo Go** for development testing on a physical device — scan QR code, full hot-reload cycle
- Use **EAS Build** for production builds: iOS TestFlight for internal testing, Android internal track in Google Play Console
- E2E testing via **Maestro** (simpler YAML-based flows) or **Detox** (Jest-based) against the Go backend in Docker

### Estimated effort

| Component | Days |
|---|---|
| Camera capture + edge detection overlay | 3–4 |
| Port existing screens (Deposit, Status, Queue, Ledger) | 2–3 |
| Navigation + polish + error states | 1–2 |
| Testing + EAS Build + deployment setup | 1–2 |
| **Total** | **7–11 days** |

---

## Recommendation for This Project

**Implement Tier 1 only.** It is a 30-minute change to two JSX attributes that demonstrates mobile awareness in the demo without consuming time better spent on backend correctness and test coverage. The rubric allocates 25 points to deposit pipeline correctness and 10 points to tests — zero points are explicitly allocated to mobile UX.

Mention Tier 2 and Tier 3 in `docs/decision_log.md` under "with one more week, we would" to show that the team has thought through the full mobile product trajectory.
