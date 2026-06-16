---
name: "Dark Terminal"
description: "Emerald on near-black. Monospace developer aesthetic with thin borders."
tags: [dark, minimal, brutalist]
colors:
  primary:   "#E8E8E8"
  secondary: "#7A7A7A"
  tertiary:  "#10B981"
  neutral:   "#0A0A0A"
  surface:   "#141414"
typography:
  display: "JetBrains Mono"
  body:    Inter
  mono:    "JetBrains Mono"
radius:
  sm: 2px
  md: 4px
  lg: 4px
buttons:
  primary:
    background: #00FF88
    color: #000000
    border: none
    shape: sharp
    padding: 10px 20px
    font: mono / 700 / 0.06em
    uppercase: true
  secondary:
    background: #0A0A0A
    color: #00FF88
    border: 1px solid #00FF88
    shape: sharp
    padding: 10px 20px
    font: mono / 600 / 0.06em
    uppercase: true
  outline:
    background: transparent
    color: #7A7A7A
    border: 1px solid #2A2A2A
    shape: sharp
    padding: 10px 20px
    font: mono / 500 / 0.06em
    uppercase: true
  ghost:
    background: transparent
    color: #00FF88
    border: none
    shape: sharp
    padding: 10px 0
    font: mono / 500
    hover: underline
charts:
  variant: "thin-bars"
  gridlines: true
  bar_gap: 10px
  highlight: all
fonts_url: "https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700&family=Inter:wght@400;500;600&display=swap"
dependencies: ["lucide-react"]
---

# Dark Terminal

The Xeme OS design system. Single source of truth for every UI surface —
the local dashboard (`xeme-os`), the web frontend (`frontend/`), and any
future marketing site.

## AI Build Instructions

> **Read this section before writing any code.** The rules below
> are non-negotiable. Every value used in the UI must come from this
> file's frontmatter — never substitute, approximate, or invent new
> colors, fonts, radii, or shadows. If a value is missing, ask the
> user before adding one.

### 1 · Your role

You are building UI for a project that has adopted **Dark Terminal** as its
design system. Treat `DESIGN.md` as the single source of truth.
Your job is to translate the user's product requirements into
components and pages that look like they were designed by the same
person who authored this file.

### 2 · Token compliance

- Pull every color, font family, radius, shadow, and spacing value
  from the frontmatter at the top of this file.
- Use semantic roles (e.g. `primary`, `accent`, `muted`) — never
  hard-code hex values that bypass the system.
- When a token can be expressed as a CSS variable, declare it once
  in your global stylesheet and reference it everywhere downstream.
- The Google Fonts `<link>` is provided in the Typography section.
  Add it to `<head>` before any component renders.

### 3 · Component recipes

Use these recipes verbatim when building the corresponding component.

#### Buttons

Four variants are defined. Pick one — never blend variants or invent a fifth.

- **Primary** — sharp shape, bg `#00FF88`, text `#000000`, padding `10px 20px`, weight `700`, uppercased.
- **Secondary** — sharp shape, bg `#0A0A0A`, text `#00FF88`, border `1px solid #00FF88`, padding `10px 20px`, weight `600`, uppercased.
- **Outline** — sharp shape, text `#7A7A7A`, border `1px solid #2A2A2A`, padding `10px 20px`, weight `500`, uppercased.
- **Ghost** — sharp shape, text `#00FF88`, padding `10px 0`, weight `500`.

Reach for **primary** as the single dominant CTA per screen.
**Secondary** for the supporting action. **Outline** for tertiary
actions in toolbars. **Ghost** for inline links and table actions.

#### Cards

- Background: `#141414`
- Radius: `radius.lg` (`4px`)
- Internal padding: `20px` for compact cards, `24–28px` for content cards.

#### Tabs

Variant: `boxed`. Each tab is a bordered card. Active tab gets the accent border and a subtle fill.
Tabs are uppercased with `0.06em` tracking.

#### Charts

- Bar/line variant: `thin-bars`
- Highlight strategy: `all` — emphasize a single bar/point per chart.

#### Typography pairings

- **Display (`JetBrains Mono`)** — h1, h2, hero headlines, brand wordmarks.
- **Body (`Inter`)** — paragraphs, labels, button text, form inputs.
- **Mono (`JetBrains Mono`)** — code, eyebrows, metadata, numerals in tables.

### 4 · Hard constraints

Never do any of the following without explicit instruction from the user:

- Introduce a new color, font, radius, or shadow that isn't declared above.
- Mix this system with another (e.g. don't paste in Material or Bootstrap defaults).
- Use generic gradient defaults (purple→blue, peach→pink) — they break the system's voice.
- Reach for emoji icons. Use a consistent icon library and size icons in line with body type.
- Add motion that exceeds the system's restraint — keep transitions short (≤200ms) and subtle.

### 5 · Before you finish — verify

Run through this checklist for every screen you produce:

- [ ] Every color used appears in the Colors table above.
- [ ] Headlines use the display font; body copy uses the body font.
- [ ] Buttons match one of the declared variants exactly (shape, padding, weight).
- [ ] Border-radius values come from `radius.sm` / `radius.md` / `radius.lg` / `radius.pill`.
- [ ] Cards and dividers use the declared border + shadow tokens.
- [ ] No values were invented; if you needed something missing, you stopped and asked.

---

## Overview

Inspired by terminal emulators and developer tooling. Quiet near-black background, emerald accent for action and status, monospace for headers.

## Colors

- **Primary `#E8E8E8`** — body text.
- **Secondary `#7A7A7A`** — meta.
- **Tertiary `#10B981`** — emerald. Buttons, success, links.
- **Neutral `#0A0A0A`** — page background.
- **Surface `#141414`** — cards.

## Typography

**JetBrains Mono** for headlines and code. **Inter** for prose to keep readability.

## Spacing

4px grid, 80px sections.

## Components

Thin 1px borders (`#262626`). 4px radii. No shadows on dark surfaces — hard offset shadows only on hover/active.

## Icons

`lucide-react`, stroke width 1.5, inheriting text color. Use emerald only on status icons (check, success).

## Do's and Don'ts

- ✅ Use emerald for active/success states.
- ✅ Keep contrast high — never gray-on-gray text.
- ❌ Don't introduce a second accent color.
- ❌ Don't use soft shadows on dark surfaces.

---

## Tokens

> Generated from the same source the live preview renders from.
> Treat the values below as the contract — never substitute approximations.

### Colors

| Role      | Value |
|-----------|-------|
| primary   | `#E8E8E8` |
| secondary | `#7A7A7A` |
| tertiary  | `#10B981` |
| neutral   | `#0A0A0A` |
| surface   | `#141414` |

### Typography

- **Display:** JetBrains Mono
- **Body:** Inter
- **Mono:** JetBrains Mono

### Radius

- sm: `2px`
- md: `4px`
- lg: `4px`

### Buttons

Four variants, each fully tokenized.

#### Primary

| Property | Value |
|----------|-------|
| shape | `sharp` |
| background | `#00FF88` |
| color | `#000000` |
| border | `none` |
| padding | `10px 20px` |
| fontFamily | `mono` |
| fontWeight | `700` |
| tracking | `0.06em` |
| uppercase | `true` |

#### Secondary

| Property | Value |
|----------|-------|
| shape | `sharp` |
| background | `#0A0A0A` |
| color | `#00FF88` |
| border | `1px solid #00FF88` |
| padding | `10px 20px` |
| fontFamily | `mono` |
| fontWeight | `600` |
| tracking | `0.06em` |
| uppercase | `true` |

#### Outline

| Property | Value |
|----------|-------|
| shape | `sharp` |
| background | `transparent` |
| color | `#7A7A7A` |
| border | `1px solid #2A2A2A` |
| padding | `10px 20px` |
| fontFamily | `mono` |
| fontWeight | `500` |
| tracking | `0.06em` |
| uppercase | `true` |

#### Ghost

| Property | Value |
|----------|-------|
| shape | `sharp` |
| background | `transparent` |
| color | `#00FF88` |
| border | `none` |
| padding | `10px 0` |
| fontFamily | `mono` |
| fontWeight | `500` |
| hoverHint | `underline` |

### Charts

| Property | Value |
|----------|-------|
| variant | `thin-bars` |
| gridlines | `true` |
| barGap | `10px` |
| highlight | `all` |

---

## Pro tokens

> Production-fidelity tokens. States, density, motion, elevation,
> content rules and a measured WCAG contract — derived from the
> resting tokens unless explicitly authored.

### States

#### Button

- **hover** — shadow: `4px 4px 0 0 #E8E8E8`, transform: `translate(-2px, -2px)`
- **focus** — outline: `2px solid #E8E8E8`, outline-offset: `3px`
- **active** — shadow: `none`, transform: `translate(0, 0)`
- **disabled** — opacity: `0.4`, filter: `grayscale(0.4)`
- **loading** — opacity: `0.6`
- **selected** — bg: `#E8E8E8`, color: `#141414`

#### Input

- **hover** — border: `2px solid #E8E8E8`
- **focus** — border: `2px solid #E8E8E8`, shadow: `4px 4px 0 0 #E8E8E8`
- **disabled** — bg: `rgba(232, 232, 232, 0.05)`, opacity: `0.4`
- **error** — border: `2px solid #B91C1C`, shadow: `4px 4px 0 0 #B91C1C`

#### Card

- **hover** — shadow: `6px 6px 0 0 #E8E8E8`, transform: `translate(-3px, -3px)`
- **selected** — border: `3px solid #E8E8E8`
- **dragging** — shadow: `8px 8px 0 0 #E8E8E8`, transform: `rotate(-1deg) scale(1.02)`

#### Tab

- **hover** — bg: `rgba(232, 232, 232, 0.08)`
- **focus** — outline: `2px solid #E8E8E8`, outline-offset: `2px`
- **selected** — bg: `#E8E8E8`, color: `#141414`

### Density

| Mode | padding × | row × | body | radius × | Use for |
|------|-----------|-------|------|----------|---------|
| compact | 0.72 | 0.78 | 0.8125rem | 0.85 | Information-dense — tables, IDEs, dashboards |
| comfortable | 1 | 1 | 0.9375rem | — | Default — most product UI |
| spacious | 1.35 | 1.3 | 1rem | 1.15 | Editorial — marketing, long-form, settings |

### Motion

**Signature — Hard cut.** No animation. Transitions are cuts — the state switches, the eye follows. Brutalism means no softening.

```css
transition: none;
```

| Token | Value |
|-------|-------|
| duration.instant | `0ms` |
| duration.fast | `50ms` |
| duration.base | `100ms` |
| duration.slow | `150ms` |
| easing.standard | `linear` |
| easing.decelerate | `linear` |
| easing.accelerate | `linear` |
| easing.spring | `steps(3, end)` |

### Elevation

Five-level scale, system-specific recipe.

| Level | Shadow | Recipe |
|-------|--------|--------|
| level0 | `none` | Flat — the border carries the separation. |
| level1 | `2px 2px 0 0 #E8E8E8` | Hard offset 2/2, no blur. |
| level2 | `4px 4px 0 0 #E8E8E8` | Hard offset 4/4 — cards. |
| level3 | `6px 6px 0 0 #E8E8E8` | Hard offset 6/6 — dialogs. |
| level4 | `8px 8px 0 0 #E8E8E8` | Hard offset 8/8 — modals, thicker border. |

### Content

- **measure:** `64ch` (max line length for body prose)
- **paragraph spacing:** `1.2em`
- **list indent:** `1.5em`
- **list gap:** `0.5em`
- **link:** color `#E8E8E8`, underline `always`
- **blockquote:** border `4px solid #E8E8E8`, padding `0.8em 1em`
- **code:** background `#E8E8E8`, color `#141414`

### Accessibility (WCAG 2.1)

**Overall:** AA-Large

| Pair | Ratio | Required | Grade | Suggested fix |
|------|-------|----------|-------|---------------|
| Body text on surface | 15.04:1 | AA | AAA | — |
| Body text on canvas | 16.16:1 | AA | AAA | — |
| Muted text on surface | 4.29:1 | AA | AA-Large | `#7f7f7f` → 4.6:1 (AA) |
| Accent on surface | 7.26:1 | AA-Large | AAA | — |
| Accent on canvas | 7.8:1 | AA-Large | AAA | — |
