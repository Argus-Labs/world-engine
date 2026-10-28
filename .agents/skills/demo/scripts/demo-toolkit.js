// === Demo Recording Toolkit ===
// Injected from the demo skill's scripts/demo-toolkit.js file.
//
// ALL helpers are SYNCHRONOUS (fire-and-forget with internal setTimeout).
// Control pacing with `agent-browser wait <ms>` after each call.
// Do NOT use `await` - it is not supported in agent-browser eval.

(() => {
  // === OVERLAY ===
  // Keep demo chrome outside <body>; body transforms used for zoom create a
  // containing block that breaks position:fixed descendants.
  document.getElementById("demo-overlay")?.remove();
  document.getElementById("demo-toolkit-style")?.remove();

  const overlay = document.createElement("div");
  overlay.id = "demo-overlay";
  overlay.style.cssText = "position:fixed;inset:0;z-index:2147483647;pointer-events:none;";
  document.documentElement.appendChild(overlay);

  // === CURSOR ===
  const c = document.createElement("div");
  c.id = "demo-cursor";
  c.style.cssText =
    "position:fixed;z-index:999999;pointer-events:none;left:640px;top:80px;transition:left 0.5s cubic-bezier(0.25,0.1,0.25,1),top 0.5s cubic-bezier(0.25,0.1,0.25,1);filter:drop-shadow(0 2px 6px rgba(0,0,0,0.6));";
  c.innerHTML =
    '<svg viewBox="0 0 24 24" width="28" height="28"><path d="M5.5 1L5.5 20L10.2 15.1L15 23L18 21.2L13.2 13.5L19.5 13.5Z" fill="white" stroke="black" stroke-width="1.3" stroke-linejoin="round"/></svg>';
  overlay.appendChild(c);

  // === ANIMATION CSS ===
  const s = document.createElement("style");
  s.id = "demo-toolkit-style";
  s.textContent = `
  @keyframes demo-ring{0%{transform:scale(0.3);opacity:1}100%{transform:scale(2.5);opacity:0}}
  @keyframes demo-dot{0%{transform:scale(1);opacity:0.6}100%{transform:scale(0);opacity:0}}
  @keyframes demo-sub-in{0%{opacity:0;transform:translateX(-50%) translateY(8px)}100%{opacity:1;transform:translateX(-50%) translateY(0)}}
  @keyframes demo-sub-out{0%{opacity:1;transform:translateX(-50%) translateY(0)}100%{opacity:0;transform:translateX(-50%) translateY(-8px)}}
`;
  document.head.appendChild(s);

  // === STATE ===
  window._demoScale = 1;
  window._demoTx = 0;
  window._demoTy = 0;

  // === CURSOR HELPERS ===

  // Move cursor to the visual center of an element (reads live bounding rect).
  // Preferred because it reads live layout. Follow with `agent-browser wait 550`.
  window.demoMoveToEl = (selector) => {
    const elements = document.querySelectorAll(selector);
    if (elements.length !== 1) {
      throw new Error(`demoMoveToEl: expected one target for ${selector}, found ${elements.length}`);
    }
    const el = elements[0];
    if (!isVisibleDemoTarget(el)) {
      throw new Error(`demoMoveToEl: target is not visible: ${selector}`);
    }
    const r = el.getBoundingClientRect();
    const c = document.getElementById("demo-cursor");
    c.style.left = `${r.left + r.width / 2 - 4}px`;
    c.style.top = `${r.top + r.height / 2 - 2}px`;
    return true;
  };

  const normalizeDemoText = (value) => value.replace(/\s+/g, " ").trim();

  const isVisibleDemoTarget = (el) => {
    const r = el.getBoundingClientRect();
    const style = window.getComputedStyle(el);
    return (
      r.width > 0 &&
      r.height > 0 &&
      style.display !== "none" &&
      style.visibility !== "hidden" &&
      Number(style.opacity) !== 0
    );
  };

  const demoInputValueFor = (el) => (el instanceof HTMLInputElement ? el.value : "");

  const demoLabelsFor = (el) =>
    [el.getAttribute("aria-label"), demoInputValueFor(el), el.textContent]
      .map((value) => normalizeDemoText(value || ""))
      .filter(Boolean);

  const demoTextTargetSelector = [
    "button",
    "a[href]",
    "input[type='button']",
    "input[type='submit']",
    "input[type='reset']",
    "label",
    "summary",
    "[role='button']",
    "[role='link']",
    "[role='menuitem']",
    "[role='menuitemcheckbox']",
    "[role='menuitemradio']",
    "[role='option']",
    "[role='tab']",
    "[role='checkbox']",
    "[role='radio']",
    "[role='switch']",
  ].join(",");

  // Move cursor to a visible element by accessible label or text. This is useful
  // for demoing semantic `agent-browser find ... click` actions without hard-
  // coding pixel coordinates. Follow with `agent-browser wait 550`.
  window.demoMoveToText = (text, selector) => {
    const target = normalizeDemoText(String(text || ""));
    if (!target) {
      throw new Error("demoMoveToText: missing text");
    }

    const candidates = Array.from(document.querySelectorAll(selector || demoTextTargetSelector))
      .map((el) => ({
        el,
        labels: demoLabelsFor(el),
      }))
      .filter(({ labels }) => labels.some((label) => label === target || label.includes(target)))
      .filter(({ el }) => isVisibleDemoTarget(el));

    const exact = candidates.filter(({ labels }) => labels.some((label) => label === target));
    const matches = exact.length ? exact : candidates;
    if (matches.length !== 1) {
      throw new Error(`demoMoveToText: expected one target for ${target}, found ${matches.length}; narrow the selector`);
    }
    const match = matches[0];

    const r = match.el.getBoundingClientRect();
    const c = document.getElementById("demo-cursor");
    c.style.left = `${r.left + r.width / 2 - 4}px`;
    c.style.top = `${r.top + r.height / 2 - 2}px`;
    return true;
  };

  // Move cursor to arbitrary untransformed viewport coordinates. Use only for
  // parking the cursor or intentional non-element targets. For visible clicks,
  // prefer demoMoveToEl/demoMoveToText so the ripple follows live layout.
  window.demoMoveTo = (x, y) => {
    const screenX = x * window._demoScale + window._demoTx;
    const screenY = y * window._demoScale + window._demoTy;
    const c = document.getElementById("demo-cursor");
    c.style.left = `${screenX - 4}px`;
    c.style.top = `${screenY - 2}px`;
  };

  // === CLICK HELPERS ===

  // Show click ripple + dot at cursor's current position.
  // Uses a blue ring visible against both light and dark backgrounds.
  // Called automatically by demoClickEl; only use manually for low-level control.
  window.demoClick = () => {
    const c = document.getElementById("demo-cursor");
    const overlay = document.getElementById("demo-overlay");
    const x = parseFloat(c.style.left) + 4,
      y = parseFloat(c.style.top) + 2;
    const r = document.createElement("div");
    r.style.cssText =
      "position:fixed;z-index:999998;pointer-events:none;left:" +
      (x - 24) +
      "px;top:" +
      (y - 24) +
      "px;width:48px;height:48px;border-radius:50%;border:3px solid rgba(59,130,246,0.85);animation:demo-ring 0.55s ease-out forwards;";
    overlay.appendChild(r);
    const d = document.createElement("div");
    d.style.cssText =
      "position:fixed;z-index:999998;pointer-events:none;left:" +
      (x - 8) +
      "px;top:" +
      (y - 8) +
      "px;width:16px;height:16px;border-radius:50%;background:rgba(59,130,246,0.6);animation:demo-dot 0.45s ease-out forwards;";
    overlay.appendChild(d);
    setTimeout(() => {
      r.remove();
      d.remove();
    }, 700);
  };

  // Move cursor to element, then after the CSS transition completes, show the
  // click ripple and run a best-effort DOM click. Use agent-browser click for
  // behavior that must trigger framework handlers.
  // Prefer demoMoveToEl + demoClick + agent-browser click in new scripts.
  window.demoClickEl = (selector) => {
    demoMoveToEl(selector);
    setTimeout(() => {
      demoClick();
      setTimeout(() => {
        const el = document.querySelector(selector);
        if (!el) return;

        if (
          el instanceof HTMLInputElement ||
          el instanceof HTMLTextAreaElement ||
          el instanceof HTMLSelectElement ||
          el.isContentEditable
        ) {
          el.focus({ preventScroll: true });
        }

        el.click();
      }, 120);
    }, 550);
  };

  // Flash a key name as a brief subtitle (for Enter, Escape, Tab, etc.).
  // Follow with `agent-browser wait 800`.
  window.demoKey = (keyName) => {
    const symbols = { Enter: "Enter", Escape: "Esc", Tab: "Tab", Backspace: "Backspace" };
    demoSubtitle(symbols[keyName] || keyName, 600);
  };

  // === SUBTITLE HELPERS ===

  // Show a subtitle overlay at the bottom 20% of the screen.
  // Fades in, holds for durationMs, fades out. Fire-and-forget.
  // Follow with `agent-browser wait <durationMs + 500>`.
  window.demoSubtitle = (text, durationMs) => {
    const duration = durationMs || 2000;
    const prev = document.getElementById("demo-subtitle");
    if (prev) prev.remove();
    const el = document.createElement("div");
    el.id = "demo-subtitle";
    el.style.cssText =
      "position:fixed;bottom:20%;left:50%;transform:translateX(-50%);z-index:999999;padding:10px 28px;background:rgba(0,0,0,0.78);color:white;font:600 18px/1.4 -apple-system,system-ui,sans-serif;border-radius:10px;pointer-events:none;white-space:nowrap;animation:demo-sub-in 0.3s ease-out forwards;";
    el.textContent = text;
    document.getElementById("demo-overlay").appendChild(el);
    setTimeout(() => {
      el.style.animation = "demo-sub-out 0.3s ease-in forwards";
      setTimeout(() => el.remove(), 300);
    }, duration);
  };

  // === ZOOM (only use when elements are too small to see at 1x) ===

  window.demoZoom = (cx, cy, s) => {
    const vw = window.innerWidth,
      vh = window.innerHeight;
    const body = document.body;
    const bounds = body.getBoundingClientRect();
    const transform = new DOMMatrix(getComputedStyle(body).transform);
    // Remove the current translation to recover the untransformed body origin,
    // including its margins and the page's scroll offset.
    const originX = bounds.left - transform.e;
    const originY = bounds.top - transform.f;
    window._demoScale = s;
    window._demoTx = vw / 2 - cx * s;
    window._demoTy = vh / 2 - cy * s;
    const translateX = window._demoTx + originX * (s - 1);
    const translateY = window._demoTy + originY * (s - 1);
    body.style.transition = "transform 0.6s cubic-bezier(0.25,0.1,0.25,1)";
    body.style.transformOrigin = "0 0";
    body.style.transform = `translate(${translateX}px,${translateY}px) scale(${s})`;
  };

  window.demoReset = () => {
    window._demoScale = 1;
    window._demoTx = 0;
    window._demoTy = 0;
    document.body.style.transition = "transform 0.5s cubic-bezier(0.25,0.1,0.25,1)";
    document.body.style.transformOrigin = "0 0";
    document.body.style.transform = "none";
  };
  return "ok";
})();
