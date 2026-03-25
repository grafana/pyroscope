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

## Linting

Run `yarn lint` and fix all errors before considering work complete. The default response to a lint error is to fix the code, not suppress the rule.

**Before adding any `eslint-disable` comment, ask the user for clarification.** Do not apply an ignore unilaterally.

Suppressing a rule is only acceptable in these scenarios:

- **Async initialization from external data** — a `useEffect` that sets state once in response to async data arriving (e.g. selecting a default service after the services list loads), where restructuring would require changing unrelated API boundaries.
- **Set-loading-true before a fetch** — calling `setLoading(true)` synchronously at the start of a fetch effect, where the loading flag must flip before the async work begins and there is no cleaner structural alternative.
- **Intentionally impure render values** — calling an impure function like `Date.now()` during render where the impurity is the entire point (e.g. "show the current time as the end of a relative range"), and using a state/ref workaround would add complexity with no real benefit.

In all other cases, restructure the code to satisfy the rule.

## Code style

Banner comments are not allowed. Do not use decorative section dividers such as:

```ts
// ─── Section name ────────────────────────
```

## Dependency workarounds

Several packages are patched via `yarn patch` due to incompatibilities with our stack. Before debugging a dependency issue, read `WORKAROUNDS.md` — it documents each patch, why it exists, and when it can be removed. Patch files live in `.yarn/patches/`.

## Components

Components should be hand built and purpose driven. It is okay to make them generic and extensible, but only when necessary. For example, if a button is needed, make a "Button" component, but don't add size variants until a size variant is required.
