export type LoadingType = 'pristine' | 'loading' | 'reloading' | 'loaded';

/**
 * returns true if at least one source is in 'loading' or 'reloading' state
 */
export function isLoadingOrReloading(sources: LoadingType[]) {
  return sources.some((state) => {
    return state === 'loading' || state === 'reloading';
  });
}
