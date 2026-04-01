# Dependency Workarounds

This document explains why certain packages are patched via `yarn patch` (`.yarn/patches/`).
These patches exist because we need the `@grafana/flamegraph` component for business requirements
and the package has incompatibilities with our stack that upstream has not yet resolved.

---

## `react-custom-scrollbars-2` — ESM entry point

**Patch file:** `.yarn/patches/react-custom-scrollbars-2-npm-4.5.0-*.patch`

### Problem

`react-custom-scrollbars-2` ships only a CommonJS build (`lib/index.js`).
Rolldown (the bundler used by Vite 8) wraps CJS modules with `__toESM`, and
Vite's `optimizeDeps` pre-bundler adds another layer on top of that. The net
result is **two levels of indirection**: the default import resolves to the
whole CJS exports object rather than the `Scrollbars` class itself.

```js
import Scrollbars from 'react-custom-scrollbars-2';
// Without patch: Scrollbars = { default: <class>, Scrollbars: <class> }  ← wrong
// With patch:    Scrollbars = <class>  ← correct
```

`@grafana/ui` (a transitive dependency of `@grafana/flamegraph`) imports
`Scrollbars` as the default export, so this breakage manifests as a runtime
crash inside the flamegraph header's custom scrollbar.

### Fix

The patch adds:

- `lib/index.mjs` — a native ESM re-export that handles both possible wrapping
  outcomes from `__toESM` defensively.
- `exports` and `module` fields to `package.json` — directs ESM-aware bundlers
  to `lib/index.mjs`, bypassing the CJS pre-bundling path entirely.

### When to remove

Remove this patch if `react-custom-scrollbars-2` ships a native ESM build
(i.e., gains a `module` or `exports.import` field in its own `package.json`),
or if it is replaced by a package that does.

---

## `@grafana/flamegraph` — remove `@grafana/assistant` dependency

**Patch file:** `.yarn/patches/@grafana-flamegraph-npm-12.4.2-*.patch`

### Problem

`@grafana/flamegraph` imports `OpenAssistantButton` from `@grafana/assistant`
in its `FlameGraphHeader` component. `@grafana/assistant` is a Grafana Labs
internal package for AI assistant integration. It is not published to the
public npm registry in a form that works outside the Grafana application
platform, so there is no viable version we can install.

```
// Before patch (in dist/esm/FlameGraphHeader.mjs):
import { OpenAssistantButton } from '@grafana/assistant';
```

The import causes a build error because the module cannot be resolved.

### Fix

The patch replaces the `@grafana/assistant` import with an inline no-op:

```js
const OpenAssistantButton = () => null;
```

The `OpenAssistantButton` is only rendered when `assistantContext` is non-empty,
and we never populate `assistantContext` in our usage of `FlameGraph`. The no-op
is therefore functionally invisible — no AI assistant button will appear, which
is the correct behavior for our deployment.

### When to remove

Remove this patch if `@grafana/assistant` becomes available as a public package
or if `@grafana/flamegraph` makes the dependency optional/tree-shakeable such
that the import is no longer emitted in the distributed build.
