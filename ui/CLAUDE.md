# Pyroscope UI

React + Vite + TypeScript frontend for Pyroscope. This frontend is a rewrite of the old UI (found in ../public/app). The objective of the app is to provide a single page query UI to query Pyroscope profiling data. The canonical implementation can be found in the old UI. When in doubt, try model behaviors found there.

## Philosophy and ethos

The app's philosophy is to be unobtrusive, fast, and easy to use for power users. There should be very limited tooltips or extraneous "helper text" to guide the user. In general, users of this app understand the objective it serves and are familiar with how to use it.

This project also has a strong resistance adding dependencies. When considering a dependency, think about how much effort it would be to implement yourself. If the effort and additional code is low, add a local implementation.

## Why a new UI?

The old UI relied on far too many dependencies which have caused a burden on the maintainers with the security risk. There are too many packages that need constant upgrading. The new UI's objective is to provide the same features with significantly fewer dependencies. This will make the app easier to maintain as there will be fewer 3rd party packages to upgrade.

There are also some design decisions made which have turned out to be very poor. As an example, using Redux as a backing state management system has lead to untold number of weird race conditions and state corruption. Simplify the state management is a large objective of the rewrite.

This is also a good time to give the UI a facelift and improve its look and feel.

## Tech stack

- **React 19** with functional components and hooks
- **Vite 8** as bundler (ESM, no Webpack)
- **TypeScript 5.9**
- **Yarn 4 (Berry)** as package manager — use `yarn`, not `npm`
- **No UI library** — styling via CSS custom properties from `src/theme.css`

## Commands

```bash
yarn dev       # Dev server at http://localhost:5173
yarn build     # tsc -b && vite build
yarn lint      # ESLint
yarn preview   # Preview production build
```

## Design system

The entire design system lives in `src/theme.css`. Read `DESIGN.md` for the full reference — it is the authoritative guide.

**Two token tiers:**

- **Primitives** (`--blue-500`, `--space-4`) — palette/scale values, defined in `:root`
- **Semantic tokens** (`--color-primary`, `--bg-primary`) — role-based aliases

**Rule:** components always use semantic tokens. Never reference primitives directly in component styles.

**Theming:** dark mode is default. Light mode: `document.documentElement.setAttribute('data-theme', 'light')`.

## Styling conventions

- Use CSS custom properties via inline `style` props or `<style>` tags
- Use `rem`-based spacing tokens (`--space-*`), not hardcoded `px`
- Background depth order: `--bg-canvas` → `--bg-primary` → `--bg-secondary` → `--bg-elevated`
- Pair `--bg-elevated` surfaces with an appropriate `--shadow-*`
- Transitions use `var(--duration-*)` and `var(--ease-*)` tokens

## File structure

```
src/
  main.tsx    # Entry point — imports theme.css once here
  App.tsx     # Current: design system kitchen sink
  theme.css   # All CSS custom properties (single source of truth)
```

## Code style

Banner comments are not allowed. Do not use decorative section dividers such as:

```ts
// ─── Section name ────────────────────────
```

## Components

Components should be hand built and purpose driven. It is okay to make them generic and extensible, but only when necessary. For example, if a button is needed, make a "Button" component, but don't add size variants until a size variant is required.
