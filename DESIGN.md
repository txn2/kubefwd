---
version: alpha
spec: https://github.com/google-labs-code/design.md
name: kubefwd-docs
description: Local design adoption record for the kubefwd.com documentation site. References txn2/www DESIGN.md as the canonical visual identity for tokens, typography, components, copyright voice, and accessibility rules. Records only the decisions and MkDocs Material learnings that the canonical does not cover.
upstream:
  design: https://github.com/txn2/www/blob/master/DESIGN.md
  tokens: https://github.com/txn2/www/blob/master/tokens.json
adoption: token-alignment
stack:
  generator: MkDocs
  theme: Material for MkDocs
  templates: docs/overrides/
  styles: docs/stylesheets/extra.css
---

## What is canonical

The canonical visual identity for txn2 lives in [`txn2/www/DESIGN.md`](https://github.com/txn2/www/blob/master/DESIGN.md) with tokens in [`txn2/www/tokens.json`](https://github.com/txn2/www/blob/master/tokens.json). This file defers to those for everything below. If a value here disagrees with upstream, upstream wins.

| Concern              | Source of truth |
|----------------------|-----------------|
| Color palette        | upstream `tokens.json` `color.*` |
| Typography stack     | upstream `tokens.json` `font.*` |
| Type scale           | upstream `DESIGN.md` Typography table |
| Spacing / measure    | upstream `tokens.json` `size.*` |
| Component contracts  | upstream `DESIGN.md` Components |
| Voice / copy rules   | upstream `DESIGN.md` Voice and Copy |
| Accessibility rules  | upstream `DESIGN.md` Do's and Don'ts |
| Mermaid theme        | upstream `DESIGN.md` `mcp__card--feature` block |

Tokens are mirrored as CSS custom properties in `docs/stylesheets/extra.css` `:root`. They are duplicated for runtime use, not as a divergence point. When upstream changes a token, update the value in `extra.css` and ship.

## Adoption level: token alignment

Per the upstream downstream contract, three levels are valid:

1. Reference. Link to upstream, no visual changes.
2. Token alignment. Keep MkDocs Material, re-skin via `extra.css` against upstream tokens.
3. Full re-skin. Replace MkDocs Material with custom layouts.

kubefwd runs at **level 2**. The site keeps Material's instant nav, search, sidebar, version selector, code copy, and content extensions. The visual layer is replaced. The homepage is a custom Material template that takes over `block header`, `block container`, and `block footer` for full-bleed treatment.

## File map

| Path | Role |
|------|------|
| `mkdocs.yml`                   | Single dark `slate` palette. `font: false` so CSS loads the upstream Google Fonts URL with trimmed axes. |
| `docs/index.md`                | Stub front matter with `template: home.html`. All homepage HTML lives in the template. |
| `docs/overrides/main.html`     | Adds the upstream Google Fonts `<link>` plus OG and Twitter meta. Inherited by every page. |
| `docs/overrides/home.html`     | Custom homepage template. Overrides `block header` (rail), `block tabs` (empty), `block container` (page--home shell with hero, sections, flagship cards, stack, coda), `block footer` (home-footer). |
| `docs/stylesheets/extra.css`   | All design rules. Two halves: homepage components scoped under `.page--home`, and Material chrome restyle for inner pages via `[data-md-color-scheme="slate"]` variable overrides. |

## Project-specific components

Components ported from upstream verbatim, with kubefwd content:

- `.rail` (replaces Material `.md-header` on the homepage). Brand links to `./`. Live UTC clock in meta. txn2.com link in meta as `part of <em class="serif">txn2</em> ↗`.
- `.hero` with three Fraunces rows (kubefwd / kubernetes / port forwards.).
- `.section`, `.section__index`, `.section__title`.
- `.flagship__card`. Two cards: `--kubefwd` (TUI mode demo) and a second variant for the API + MCP surfaces. Top accent line animates on hover per upstream spec.
- `.terminal`, `.terminal__bar`, `.terminal__body` with `.t-prompt`, `.t-ok`, `.t-mute` classes. The only block with shadow.
- `.stack`, `.stack__row`. Each row links to a real anchor in the docs (architecture/#ip-allocation, advanced-usage/#custom-hosts-file-path, user-guide/#auto-reconnect, user-guide/#interface-overview, api-reference/, mcp-integration/).
- `.coda` and `.home-footer` (renamed from upstream `.footer` to avoid collision with markdown that uses `class="footer"`; see Learning #5).

Components from upstream **not used** here, with reason:

- `.mcp__card--feature` and the MCP grid. kubefwd is a single project, not an MCP catalog.
- The `mcp-data-platform` mermaid hero. Not relevant.
- The 5-column footer's `sponsors / craig` columns. The kubefwd home-footer has `about / docs / interfaces / code / txn2 / org` columns instead, since this site is project-scoped.

Custom additions specific to kubefwd:

- `home-footer__col--meta` includes a `txn2 / org` panel that backlinks to txn2.com explicitly. Per the org-wide rule that every sister project must clearly link home.
- All CNCF Landscape mentions on the homepage and in `docs/attributions.md` link to the kubefwd entry on `landscape.cncf.io`. The Landscape listing is a load-bearing credential and earns a real link wherever it appears.

## MkDocs Material learnings

The upstream is built with Hugo. kubefwd is built with MkDocs Material. The two have different override surfaces, so the same visual outcome requires different mechanics. These are the lessons worth carrying forward when re-skinning the next sister project.

### 1. Override the homepage via a separate template, not via CSS hacks

MkDocs Material's content pipeline wraps every page body in `.md-content__inner` with hard-coded max-widths. Trying to break out with `:has()` selectors or full-bleed hacks inside the markdown body is fragile and fights Material's own layout.

The pattern that works: create `docs/overrides/home.html` that extends Material's `main.html` and overrides `block container`, `block header`, `block footer`. Set `template: home.html` in `docs/index.md` front matter. Material then renders the homepage with the custom blocks instead of the standard layout. The homepage gets full viewport control without any width-fighting.

```yaml
# docs/index.md
---
title: kubefwd
template: home.html
hide:
  - navigation
  - toc
  - footer
---
```

### 2. Re-skin inner pages via Material variable overrides

Material exposes its own design tokens as CSS custom properties on `[data-md-color-scheme="slate"]`. Mapping these to upstream tokens re-skins every inner page automatically without touching their markdown:

```css
[data-md-color-scheme="slate"] {
  --md-default-bg-color:        var(--ink);
  --md-default-fg-color:        var(--paper);
  --md-typeset-a-color:         var(--signal);
  --md-text-font:               var(--sans);
  --md-code-font:               var(--mono);
  /* ...etc */
}
```

Verify the body actually carries the attribute (`grep data-md-color-scheme site/getting-started/index.html`). Single-scheme palettes (`palette: scheme: slate`) do emit it. If they ever stop, fall back to `:root`.

### 3. `font: false` to load fonts directly from CSS

Material's `font:` directive loads fonts through Google Fonts but without axis control. To get the upstream's exact trimmed payload (Fraunces with `opsz/wght` italic axis, Instrument Sans roman 400-700, JetBrains Mono roman 400-700, **no SOFT axis**), set `font: false` in `mkdocs.yml` and load the upstream Google Fonts URL via a `<link>` in `main.html`'s `extrahead` block. Verify a single `fonts.googleapis.com` request in DevTools.

### 4. Scope every homepage component class under `.page--home`

The upstream uses `<main class="page page--home">` on its index page and inner pages don't inherit those classes. In MkDocs the same component class names will collide if any markdown happens to use `class="terminal"`, `class="footer"`, `class="section"`, etc. Material's admonitions, tabbed content, and `md_in_html` all generate divs with predictable class names, and contributors will write more.

Discipline: every homepage component rule must include `.page--home` as an ancestor, OR rename the class with a project-specific prefix (e.g., `home-footer`). The cost of getting this wrong is silent visual breakage on inner pages weeks later when someone uses one of the colliding names.

### 5. Rename `.footer` → `.home-footer`

The single most common collision. Markdown that includes a literal `<div class="footer">` is plausible and will inherit the homepage's 60px-padded, 5-column-grid footer styling. Rename rather than scope.

### 6. h3 and h4 are technical reference, not display type

Inner pages like `api-reference.md` use `### GET /api/health` and `### POST /api/v1/services` as section headings. Rendering these in Fraunces italic display style (the obvious choice for a "serif headings" design) makes the API reference unreadable.

The rule: h1 and h2 stay Fraunces italic for page and section titles. h3 and h4 switch to Instrument Sans bold so technical reference reads as text, not as type. Add a `:has(code)` block that flips any heading containing inline code to JetBrains Mono with signal-orange code text. Add a `@supports not selector(:has(*))` fallback for older browsers.

```css
.md-typeset h3 {
  font-family: var(--sans);
  font-weight: 700;
  font-size: clamp(18px, 1.6vw, 22px);
}
.md-typeset h1:has(code),
.md-typeset h2:has(code),
.md-typeset h3:has(code),
.md-typeset h4:has(code) {
  font-family: var(--mono);
  font-style: normal;
  font-weight: 700;
}
```

### 7. Tabbed content nests boxes, fix it

Material's `.md-typeset .tabbed-set` defaults to a bordered, backgrounded box. Code blocks inside also default to bordered backgrounded boxes. The result is nested rectangles with cluttered visual hierarchy.

The fix: strip the `.tabbed-set` background and border. Leave only a bottom underline on `.tabbed-labels`. Let the inner code blocks be the visual statement. Tighten `pre > code` padding to ~14px so single-line install commands don't render as 70px-tall rectangles.

### 8. Mermaid via Material's CSS variables, not via separate init

MkDocs Material includes a Mermaid integration that reads `--md-mermaid-*` CSS variables. Don't bring in `mermaid.esm.mjs` separately like the upstream Hugo site does. Instead, set the variables in the slate scheme block:

```css
[data-md-color-scheme="slate"] {
  --md-mermaid-node-bg-color:   var(--ink-3);
  --md-mermaid-node-fg-color:   var(--paper);
  --md-mermaid-edge-color:      var(--mute);
  /* ...full list in extra.css */
}
```

Plus a `.md-typeset .mermaid` container restyle (dashed border, ink-3 background) for the diagram block itself.

### 9. Guard inline scripts against `navigation.instant`

`navigation.instant` is enabled (`mkdocs.yml`). On every navigation back to the homepage, Material rehydrates the body. Any `setInterval`, `setTimeout`, or event listener registered in an inline `<script>` will register **again**, leaking handles indefinitely.

Pattern for the live UTC clock used in the rail and footer:

```html
<script>
  (function () {
    if (window.__kubefwdClock) {
      window.__kubefwdClock.tick();
      return;
    }
    function tick() { /* ... */ }
    tick();
    var handle = setInterval(tick, 30 * 1000);
    window.__kubefwdClock = { tick: tick, handle: handle };
  })();
</script>
```

The `tick()` call on re-entry refreshes the displayed time without registering a new interval.

### 10. Drop the light/dark toggle

The canonical txn2 identity is dark only. Collapse `palette` to a single `scheme: slate`. Remove the `toggle:` blocks. Audit `extra.css` and `index.md` for orphan light-mode references (`hero-logo-light`, `prefers-color-scheme: light`, `kf-green` etc).

### 11. Atmospheric overlays at low z-index

`body::before` (grain) and `body::after` (vignette) are fixed viewport overlays. Place them at `z-index: 1` so they sit above the page background but below the rail (z-50) and skiplink (z-200). Vignette has no blend mode, so a higher z-index would darken interactive chrome.

### 12. What Hugo does that MkDocs cannot match

For reference when the next sister project chooses its adoption level:

- Hugo can compile tokens.json into CSS custom properties at build time via `resources.Get` + `transform.Unmarshal`. MkDocs has no equivalent. We hand-mirror the tokens in `extra.css` `:root` and update them when upstream changes.
- Hugo can subscribe to upstream tokens through a small extra.css that imports a generated `:root { --... }` block. MkDocs cannot. Token sync is a manual PR.
- Hugo's partial system handles header/footer composition naturally. MkDocs uses Material template `{% block %}` overrides, which is similar but constrained to Material's block names (`announce`, `header`, `tabs`, `container`, `content`, `footer`, `scripts`).

## Voice and copy

Defers to upstream. Briefly:

- No em-dashes (U+2014) or en-dashes (U+2013) anywhere, including code comments and template comments. Use commas, periods, colons, parentheses, slashes, hyphens.
- No AI-tell vocabulary: `seamless`, `leverage`, `comprehensive`, `robust`, `delve`, `unleash`, `elevate`, `embark`, `tapestry`, `not just X but Y`, `as an AI`, `let me X`.
- Sentence case for body. Lowercase for rail and label text. Title case rare.
- Section indices: `§ 01 / title` with slash, never an em-dash.
- Year ranges use a hyphen: `2017-2026`.
- Verify before commit: `grep -RE "—|–" docs/ mkdocs.yml`.

## Updating

When the upstream `txn2/www/DESIGN.md` or `tokens.json` changes:

1. Read the upstream diff. Identify which tokens, components, or rules changed.
2. Update the matching CSS variables in `docs/stylesheets/extra.css` `:root`.
3. If a component contract changed (padding, border, hover behavior), update the homepage template in `docs/overrides/home.html` and the matching CSS rules.
4. Update this file's File map / Project-specific components / MkDocs Material learnings sections if a new learning emerges.
5. Run `mkdocs build --strict` and verify in the browser before committing. Verify the home page hero, flagship cards, terminal, stack, coda, and home-footer. Verify an inner page (`/getting-started/`, `/api-reference/`) still inherits the look.

Keep this file thin. If a section grows past 30 lines, ask whether it belongs upstream instead.

## Downstream contract for sister projects

This file is canonical only for kubefwd.com. Other sister projects (txeh, mcp-s3, mcp-trino, mcp-datahub, mcp-data-platform) considering the same adoption should:

1. Use this file as a template for their own thin local DESIGN.md.
2. Reference upstream `txn2/www/DESIGN.md` as the source of truth for tokens, typography, components, voice.
3. Replace the kubefwd-specific sections (project-specific components, file paths, custom additions) with their own.
4. Keep the MkDocs Material learnings section. Those apply to every MkDocs Material project.
5. Update upstream's downstream-contract list when adding a new project.
