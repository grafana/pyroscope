import { useEffect, useMemo, useRef, useState } from 'react';
import { fetchLabelNames, fetchLabelValues } from '@api/client';
import { getCursorContext, type CursorContext } from '../queryLang';
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
        ? fetchLabelNames(ctx.otherMatchers, s, e, controller.signal)
        : ctx.kind === 'value'
          ? fetchLabelValues(
              ctx.labelName,
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
        cache.set(debouncedKey, []);
        setFetchTick((v) => v + 1);
      });

    return () => controller.abort();
  }, [debouncedKey]);

  const suggestions = useMemo(() => {
    if (context.kind === 'none') return [];
    const results = cache.get(debouncedKey) ?? [];
    const prefix = context.prefix.toLowerCase();
    const filtered = prefix
      ? results.filter((n) => n.toLowerCase().includes(prefix))
      : results.slice();
    return filtered.slice(0, MAX_SUGGESTIONS);
  }, [debouncedKey, context]);

  const loading = !!debouncedKey && !cache.has(debouncedKey);

  // We have a "definite" answer only when the user has stopped typing long
  // enough for the debounce to settle (requestKey === debouncedKey) and the
  // cache has been populated for that key. Without this, "No matches"
  // would flash during every keystroke.
  const definitelyEmpty =
    context.kind !== 'none' &&
    requestKey === debouncedKey &&
    cache.has(debouncedKey) &&
    suggestions.length === 0;

  return { context, suggestions, loading, definitelyEmpty };
}
