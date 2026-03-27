# Design System

The design system lives in a single file: `src/theme.css`. Import it once at the app entry point and then reference its CSS custom properties anywhere in the codebase.

```ts
// src/main.tsx
import './theme.css';
```

No build step, preprocessor, or runtime dependency required.

---

## Core concept: two token tiers

The system has two layers, and knowing which to use is the main thing to internalize.

**Primitives** are raw palette values — the color blue at a given lightness, a spacing unit, a radius value. They live in `:root` and are named after what they _are_.

```css
--blue-500: #3d71d9;
--space-4: 1rem;
```

**Semantic tokens** are role-based aliases that reference primitives (or define their own values). They are named after how they are _used_.

```css
--color-primary: var(--blue-500);
--bg-primary: #212a44;
```

**The rule:** components always use semantic tokens. Primitives exist only to define semantics — never reference `--blue-500` in a component. This is what allows the light/dark theme swap to work: semantic tokens are redefined per theme, primitives are not.

---

## Theming

Dark mode is the default. Light mode is activated by setting `data-theme="light"` on `<html>`. Remove the attribute to return to dark.

```ts
// Enable light mode
document.documentElement.setAttribute('data-theme', 'light');

// Return to dark mode
document.documentElement.removeAttribute('data-theme');
```

Any component using semantic tokens automatically updates — no JavaScript, no class toggling on individual elements.

---

## Semantic tokens reference

### Backgrounds

Four depth layers, from furthest back to foremost. Use them in order — don't skip layers.

| Token            | Use                                    |
| ---------------- | -------------------------------------- |
| `--bg-canvas`    | The outermost page background          |
| `--bg-primary`   | Panels, cards, main content surfaces   |
| `--bg-secondary` | Sidebars, form inputs, nested surfaces |
| `--bg-elevated`  | Dropdowns, popovers, tooltips, dialogs |

```css
.panel {
  background: var(--bg-primary);
  border: 1px solid var(--border-medium);
  border-radius: var(--radius-lg);
}

.dropdown {
  background: var(--bg-elevated);
  box-shadow: var(--shadow-md);
}
```

In light mode, `--bg-elevated` and `--bg-primary` both resolve to `#ffffff`. Elevation is communicated by shadow rather than color difference — so always pair `--bg-elevated` with an appropriate `--shadow-*`.

### Borders

Alpha-based so they blend correctly on any background layer.

| Token             | Use                                             |
| ----------------- | ----------------------------------------------- |
| `--border-weak`   | Dividers, row separators, subtle section breaks |
| `--border-medium` | Default element borders (inputs, cards, panels) |
| `--border-strong` | Focused elements, prominent outlines            |

```css
.input {
  border: 1px solid var(--border-medium);
}

.input:focus {
  border-color: var(--color-primary-border);
  box-shadow: 0 0 0 3px var(--action-focus);
}
```

### Text

| Token                 | Use                                                                             |
| --------------------- | ------------------------------------------------------------------------------- |
| `--text-primary`      | All main body and UI text                                                       |
| `--text-secondary`    | Labels, hints, descriptions, metadata                                           |
| `--text-disabled`     | Non-interactive / disabled text                                                 |
| `--text-link`         | Anchor text                                                                     |
| `--text-link-hover`   | Anchor text on hover                                                            |
| `--text-max-contrast` | Text placed on top of colored backgrounds (e.g. inside a filled badge or alert) |

### Action states

These are overlay colors — apply them via `background` on hover/selected states, not as solid colors.

| Token               | Use                                                       |
| ------------------- | --------------------------------------------------------- |
| `--action-hover`    | Background overlay when a row, item, or button is hovered |
| `--action-selected` | Background overlay for the currently active/selected item |
| `--action-focus`    | Focus ring color (used with `box-shadow` or `outline`)    |

```css
.menu-item:hover {
  background: var(--action-hover);
}

.menu-item[aria-current='true'] {
  background: var(--action-selected);
}
```

### Semantic color roles

Each role (`primary`, `secondary`, `success`, `error`, `warning`) exposes five tokens:

| Suffix        | Use                                                                        |
| ------------- | -------------------------------------------------------------------------- |
| _(none)_      | Solid fill — primary button background, alert background                   |
| `-hover`      | Solid fill on hover                                                        |
| `-subtle`     | Low-opacity tinted background — for badges, highlights, alert banners      |
| `-border`     | Border and ring color when referencing this role                           |
| `-text`       | Readable text _in this color_ on a neutral background                      |
| `-foreground` | Text placed _on top of_ the solid fill (e.g. label inside a filled button) |

```css
/* A filled primary button */
.btn-primary {
  background: var(--color-primary);
  color: var(--color-primary-foreground); /* white */
  border: 1px solid transparent;
}

.btn-primary:hover {
  background: var(--color-primary-hover);
}

/* An outlined primary button */
.btn-primary-outline {
  background: transparent;
  color: var(--color-primary-text);
  border: 1px solid var(--color-primary-border);
}

/* An inline status badge */
.badge-error {
  background: var(--color-error-subtle);
  color: var(--color-error-text);
  border: 1px solid var(--color-error-border);
}
```

The `-text` tokens are theme-aware: in dark mode they resolve to a light shade of the color (readable on dark backgrounds), in light mode they resolve to a darker shade (readable on white).

### Shadows

| Token         | Typical use                                |
| ------------- | ------------------------------------------ |
| `--shadow-xs` | Subtle lift for small interactive elements |
| `--shadow-sm` | Cards and panels in light mode             |
| `--shadow-md` | Dropdowns, popovers                        |
| `--shadow-lg` | Modals, dialogs                            |
| `--shadow-xl` | Full-screen overlays, drawers              |

Shadow opacity is automatically heavier in dark mode and lighter in light mode — the same token works correctly in both themes.

---

## Primitive tokens reference

Use these only when defining new semantic tokens, not in components directly.

### Color palette

All hues follow a numeric lightness scale (`100` = lightest, `700` = darkest). Available hues: `--neutral`, `--blue`, `--green`, `--red`, `--orange`.

```
--blue-100  #d6e4ff   ← very light
--blue-300  #6e9fff
--blue-500  #3d71d9   ← mid (used as --color-primary)
--blue-700  #1e449e   ← dark
```

The neutral scale runs from `--neutral-0` (#ffffff) to `--neutral-1000` (#000000) with stops at 50, 100, 200 … 950. Most dark-mode backgrounds are derived from `--neutral-850` through `--neutral-900`.

### Spacing

4 px base scale in `rem`. Choosing `rem` over `px` means the spacing scales proportionally when a user adjusts their browser's base font size.

| Token         | Value    | px equivalent |
| ------------- | -------- | ------------- |
| `--space-0-5` | 0.125rem | 2 px          |
| `--space-1`   | 0.25rem  | 4 px          |
| `--space-2`   | 0.5rem   | 8 px          |
| `--space-3`   | 0.75rem  | 12 px         |
| `--space-4`   | 1rem     | 16 px         |
| `--space-5`   | 1.25rem  | 20 px         |
| `--space-6`   | 1.5rem   | 24 px         |
| `--space-8`   | 2rem     | 32 px         |
| `--space-10`  | 2.5rem   | 40 px         |
| `--space-12`  | 3rem     | 48 px         |
| `--space-16`  | 4rem     | 64 px         |

### Border radius

| Token           | Value  | Use                                |
| --------------- | ------ | ---------------------------------- |
| `--radius-sm`   | 3px    | Chips, badges, tags                |
| `--radius-md`   | 5px    | Buttons, inputs, standard elements |
| `--radius-lg`   | 8px    | Cards, panels, dropdowns           |
| `--radius-xl`   | 12px   | Dialogs, modals                    |
| `--radius-full` | 9999px | Pills, avatars, toggles            |

### Typography

**Font families:**

```css
font-family: var(--font-sans); /* Roboto, with system fallbacks */
font-family: var(--font-mono); /* Roboto Mono, with system fallbacks */
```

**Size scale** — base UI text is `--text-md` (14 px). The `html` element is set to `16px`, so `1rem = 16px`.

| Token        | Value                     |
| ------------ | ------------------------- |
| `--text-xs`  | 11 px                     |
| `--text-sm`  | 12 px                     |
| `--text-md`  | 14 px ← default body size |
| `--text-lg`  | 16 px                     |
| `--text-xl`  | 18 px                     |
| `--text-2xl` | 20 px                     |
| `--text-3xl` | 24 px                     |
| `--text-4xl` | 32 px                     |

**Weights:** `--weight-light` (300), `--weight-regular` (400), `--weight-medium` (500), `--weight-bold` (700)

**Line heights:** `--leading-tight` (1.25), `--leading-base` (1.5), `--leading-relaxed` (1.7)

**Letter spacing:** `--tracking-tight` (-0.01em), `--tracking-normal` (0), `--tracking-wide` (0.02em), `--tracking-caps` (0.08em)

### Z-index

| Token          | Value | Use                                |
| -------------- | ----- | ---------------------------------- |
| `--z-raised`   | 10    | Slightly elevated in-flow elements |
| `--z-dropdown` | 1000  | Dropdown menus                     |
| `--z-sticky`   | 1100  | Sticky headers, toolbars           |
| `--z-overlay`  | 1200  | Background overlays                |
| `--z-modal`    | 1300  | Modal dialogs                      |
| `--z-popover`  | 1400  | Popovers, command palettes         |
| `--z-toast`    | 1500  | Toast notifications                |
| `--z-tooltip`  | 1600  | Tooltips (always on top)           |

### Motion

**Durations:** Use shorter durations for small elements, longer for large ones.

| Token               | Value | Use                                   |
| ------------------- | ----- | ------------------------------------- |
| `--duration-fast`   | 100ms | Icon swaps, simple visibility toggles |
| `--duration-base`   | 150ms | Default — buttons, badges, borders    |
| `--duration-slow`   | 200ms | Dropdowns, expanding panels           |
| `--duration-slower` | 300ms | Modals, drawers, page transitions     |

**Easing:**

| Token           | Use                                                              |
| --------------- | ---------------------------------------------------------------- |
| `--ease-out`    | Most interactions — elements that enter or respond to user input |
| `--ease-in-out` | Elements that travel from one place to another                   |
| `--ease-smooth` | General-purpose smooth curve (Material-style)                    |
| `--ease-spring` | Playful overshoot — tooltips, popovers appearing                 |

```css
.dropdown {
  transition:
    opacity var(--duration-slow) var(--ease-out),
    transform var(--duration-slow) var(--ease-spring);
}
```

---

## Patterns

### Building a card component

```css
.card {
  background: var(--bg-primary);
  border: 1px solid var(--border-medium);
  border-radius: var(--radius-lg);
  padding: var(--space-5);
  box-shadow: var(--shadow-sm);
}

.card-title {
  font-size: var(--text-lg);
  font-weight: var(--weight-medium);
  color: var(--text-primary);
  margin-bottom: var(--space-3);
}

.card-body {
  font-size: var(--text-md);
  color: var(--text-secondary);
  line-height: var(--leading-base);
}
```

### Building a status badge

```css
.badge {
  display: inline-flex;
  align-items: center;
  padding: var(--space-0-5) var(--space-2);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
}

.badge-success {
  background: var(--color-success-subtle);
  color: var(--color-success-text);
  border: 1px solid var(--color-success-border);
}
```

### Building a form input

```css
.input {
  background: var(--bg-secondary);
  color: var(--text-primary);
  border: 1px solid var(--border-medium);
  border-radius: var(--radius-md);
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-md);
  transition:
    border-color var(--duration-base) var(--ease-out),
    box-shadow var(--duration-base) var(--ease-out);
}

.input::placeholder {
  color: var(--text-disabled);
}

.input:focus {
  outline: none;
  border-color: var(--color-primary-border);
  box-shadow: 0 0 0 3px var(--action-focus);
}

.input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
```

### Adding a new semantic token

> **This is highly atypical.** The token set is intentionally minimal and was designed to cover the common cases without bloat. Before adding a new semantic token, check whether an existing one covers the need — and if not, seek sign-off from the team first. Expanding the token set has a maintenance cost: every new token requires a light-mode override if it is theme-dependent, and it needs to be documented here.

If a new token is genuinely warranted, add it to the `:root` block in `theme.css` alongside the existing semantic tokens, and add a `[data-theme="light"]` override if its value should differ between themes.

```css
/* In :root (dark default) */
--sidebar-width: 240px;
--topbar-height: var(--space-12);
```

---

## What not to do

**Don't use primitives in components.**
`--blue-500` is a palette building block, not a component color. Use `--color-primary` instead, which adapts across themes.

**Don't hardcode hex values.**
If a color doesn't exist in the token system, extend the system — don't reach for raw hex in a component stylesheet.

**Don't skip background layers.**
If content should appear above the page background, use `--bg-primary`. If it should appear above that (as in a sidebar or nested surface), use `--bg-secondary`. Skipping layers breaks the visual depth model.

**Don't use `px` for spacing.**
The spacing scale uses `rem` for accessibility. Users who set a larger browser font size get proportionally larger spacing. Hardcoded `px` spacing won't scale with them.
