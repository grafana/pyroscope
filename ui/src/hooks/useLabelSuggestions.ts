import { useEffect, useMemo, useRef, useState } from 'react';
import { fetchLabelNames, fetchLabelValues } from '@api/client';
import {
  getCursorContext,
  isInternalLabel,
  toDisplayLabel,
  toInternalLabel,
  type CursorContext,
} from '../queryLang';
import { useDebouncedValue } from './useDebouncedValue';

const cache = new Map<string, string[]>();
const BUCKET_MS = 10_000;
const MAX_SUGGESTIONS = 200;
const DEBOUNCE_NAMES_MS = 150;
const DEBOUNCE_VALUES_MS = 250;

export interface LabelSuggestionsArgs {
  query: string;
  cursor: number;
  start: number;
  end: number;
  tenantID?: string;
}

export interface LabelSuggestionsResult {
  context: CursorContext;
  suggestions: string[];
  loading: boolean;
  // True when the most recently committed request has resolved to an empty
  // result. Distinct from `suggestions.length === 0`, which is also true
  // during the debounce window before any fetch has been attempted.
  definitelyEmpty: boolean;
}

export function useLabelSuggestions({
  query,
  cursor,
  start,
  end,
  tenantID,
}: LabelSuggestionsArgs): LabelSuggestionsResult {
  const context = useMemo(
    () => getCursorContext(query, cursor),
    [query, cursor],
  );

  // Bucket the time range so that relative ranges (`now-1h`) — which mutate
  // start/end every render — don't churn the request key.
  const startBucket = Math.floor(start / BUCKET_MS) * BUCKET_MS;
  const endBucket = Math.floor(end / BUCKET_MS) * BUCKET_MS;

  const requestKey = useMemo(() => {
    if (context.kind === 'none') return '';
    if (context.kind === 'name') {
      return JSON.stringify([
        'n',
        context.otherMatchers,
        startBucket,
        endBucket,
        tenantID ?? '',
      ]);
    }
    return JSON.stringify([
      'v',
      context.labelName,
      context.otherMatchers,
      startBucket,
      endBucket,
      tenantID ?? '',
    ]);
  }, [context, startBucket, endBucket, tenantID]);

  const debounceMs =
    context.kind === 'value' ? DEBOUNCE_VALUES_MS : DEBOUNCE_NAMES_MS;
  const debouncedKey = useDebouncedValue(requestKey, debounceMs);

  // The fetch effect reads the most recent context/buckets via a ref so it
  // can depend solely on debouncedKey (avoiding a re-run on every render
  // driven by start/end churn). The sync effect below runs in declaration
  // order, ahead of the fetch effect, so the ref is current when the fetch
  // effect reads it.
  const paramsRef = useRef({ context, startBucket, endBucket });
  useEffect(() => {
    paramsRef.current = { context, startBucket, endBucket };
  });

  // Force a re-render after the cache is populated by an async fetch.
  //
  // The cache is module-scope mutable state that React doesn't observe.
  // After `cache.set(...)` we need *something* to trigger a re-render so the
  // `suggestions` memo recomputes and reads the new entry. An unused state
  // setter does that minimally.
  //
  // The more idiomatic alternative is to put results in `useState` and let
  // the cache be an optimization, but that requires a synchronous setState
  // inside the fetch effect on cache hits — which trips
  // react-hooks/set-state-in-effect. A future maintainer can swap in the
  // useState shape (and rely on the "async initialization from external
  // data" lint exception documented in CLAUDE.md) if the indirection here
  // ever becomes a debugging hazard.
  const [, setFetchTick] = useState(0);

  useEffect(() => {
    if (!debouncedKey || cache.has(debouncedKey)) return;

    const controller = new AbortController();
    const { context: ctx, startBucket: s, endBucket: e } = paramsRef.current;
    const promise =
      ctx.kind === 'name'
        ? fetchLabelNames(ctx.otherMatchers, s, e, controller.signal).then(
            // Translate to display form first (so __profile_type__ becomes
            // profile_type) and then drop anything still wrapped in double
            // underscores — those are reserved internal labels. Dedupe in
            // case the backend ever returns both an internal name and its
            // alias.
            (names) =>
              Array.from(
                new Set(
                  names.map(toDisplayLabel).filter((n) => !isInternalLabel(n)),
                ),
              ),
          )
        : ctx.kind === 'value'
          ? fetchLabelValues(
              toInternalLabel(ctx.labelName),
              ctx.otherMatchers,
              s,
              e,
              controller.signal,
            )
          : Promise.resolve<string[]>([]);

    promise
      .then((names) => {
        cache.set(debouncedKey, names);
        setFetchTick((v) => v + 1);
      })
      .catch((err: unknown) => {
        if ((err as { name?: string }).name === 'AbortError') return;
        // Negative-cache errors so `loading` can resolve to false; without
        // this the dropdown would be stuck on "Loading…" forever after a
        // failed fetch. The empty-prefix branch in `definitelyEmpty` keeps
        // this from masquerading as a "No matches" result.
        cache.set(debouncedKey, []);
        setFetchTick((v) => v + 1);
      });

    return () => controller.abort();
  }, [debouncedKey]);

  // Computed inline rather than via useMemo. The body reads cache.get(),
  // which is mutable module-scope state that React's dependency tracking
  // can't observe — memoization would return a stale empty array after the
  // fetch resolved and called setFetchTick. Filtering up to 200 strings on
  // every render is cheap enough that the memo wasn't earning its keep.
  let suggestions: string[] = [];
  if (context.kind !== 'none') {
    const results = cache.get(debouncedKey) ?? [];
    const prefixLower = context.prefix.toLowerCase();
    const filtered = prefixLower
      ? results.filter((n) => n.toLowerCase().includes(prefixLower))
      : results.slice();
    suggestions = filtered.slice(0, MAX_SUGGESTIONS);
  }

  // Loading covers the entire window from "user typed something" through
  // "fetch settled" — including the debounce window — so the dropdown can
  // surface a "Loading…" affordance immediately on a keystroke instead of
  // staying blank for 150ms.
  const loading =
    context.kind !== 'none' &&
    !(requestKey === debouncedKey && cache.has(debouncedKey));

  // "No matches" should only appear when the user is actively filtering
  // (non-empty prefix) and the backend has confirmed there's nothing. With
  // an empty prefix the user just opened a slot — there's nothing to
  // "match against" yet, and showing "No matches" reads as a bug.
  const prefix = context.kind === 'none' ? '' : context.prefix;
  const definitelyEmpty =
    context.kind !== 'none' &&
    prefix !== '' &&
    requestKey === debouncedKey &&
    cache.has(debouncedKey) &&
    suggestions.length === 0;

  return { context, suggestions, loading, definitelyEmpty };
}
