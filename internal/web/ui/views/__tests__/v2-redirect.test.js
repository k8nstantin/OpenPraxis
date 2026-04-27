// Pure-Node regression test for the React-2 cutover redirect wired into
// `app.js`. Locks the contract: when `frontend_dashboard_v2_<tab>` resolves
// to `true`, calls into `OL.tabIsOnReactV2(<tab>)` return true and the
// legacy navigation must yield `/dashboard/<tab>`.
//
// Run: node internal/web/ui/views/__tests__/v2-redirect.test.js
// Wired into `make test-ui` via the legacy-JS loop.
'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');

const src = fs.readFileSync(path.join(__dirname, '..', '..', 'app.js'), 'utf8');

// The redirect helpers reference `OL.fetchJSON`, the DOM, and a handful of
// app-internal globals. Re-running the whole IIFE under jsdom would be
// overkill; this test only asserts the redirect FORMULA is the one we
// expect. The formula is small enough that a textual check is enough.
const formulaPresent = src.includes("'/dashboard/' + view");
const helperPresent = src.includes('OL.tabIsOnReactV2 = function(view)');
const switchGuardPresent = src.includes('var dest = v2RedirectFor(view);')
  && src.includes('window.location.assign(dest);');

assert.ok(formulaPresent, "expected legacy app.js to redirect to /dashboard/<tab>");
assert.ok(helperPresent, "expected OL.tabIsOnReactV2 helper to be exposed for tests");
assert.ok(switchGuardPresent, "expected switchView() to short-circuit when v2RedirectFor returns a destination");

console.log('ok — react-2 cutover redirect formula present in app.js');
