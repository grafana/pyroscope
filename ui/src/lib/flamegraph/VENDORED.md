# Vendored: `@grafana/flamegraph` v13.0.1

These files were copied verbatim from
[grafana/grafana @ v13.0.1](https://github.com/grafana/grafana/tree/v13.0.1/packages/grafana-flamegraph/src),
Apache-2.0 licensed (see `LICENSE`).

We vendored the source so we can iteratively strip it down — removing the
`@grafana/ui` and `@grafana/data` dependencies and the heavy machinery we
don't need for our single-page UI.

Local modifications from the upstream tag:

- `@grafana/assistant` integration (the `OpenAssistantButton`, `assistantContext`
  prop, and `showAnalyzeWithAssistant` flag) was removed wholesale. The
  package is not publicly available and the feature is not used here.
- `CallTree/` subdir, `PaneView.CallTree` option, and the related code paths
  removed — only Top Table + Flame Graph views are surfaced.
- `enableNewUI` opt-in (and the entire NewUI rendering path it gated)
  removed. `FlameGraphPane.tsx`, `NewUIContainer`, the new-UI branch of
  `FlameGraphHeader`/`FlameGraph`, and the `viewMode` / `paneView` plumbing
  through `FlameGraphCanvas` and `FlameGraphContextMenu`'s callback state
  are gone. `ViewMode` and `PaneView` types deleted; the only top-level
  view selector left is `SelectedView` (TopTable / FlameGraph / Both).
- Diff-mode support removed. `valueRight` / `selfRight` fields on
  `LevelItem` and `FlameGraphDataContainer`, `isDiffFlamegraph()`,
  `getValueRight()` / `getSelfRight()`, `ColorSchemeDiff`,
  `getBarColorByDiff` + diff color gradients, the diff-mode tooltip table,
  the diff-mode top-table columns (Baseline/Comparison/Diff), and the
  `isDiffMode` prop chain are all gone. `d3` is no longer required.

To diff against upstream:

```
git -C ../../grafana show v13.0.1:packages/grafana-flamegraph/src/<file>
```
