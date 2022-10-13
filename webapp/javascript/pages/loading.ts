/**
 * returns true if at least one source is in 'loading' or 'reloading' state
 */
export function isLoadingOrReloading(
  sources: ('pristine' | 'loading' | 'reloading' | 'loaded')[]
) {
  return sources.some((state) => {
    return state === 'loading' || state === 'reloading';
  });
}
