export function isLoadingOrReloading(
  states: ('pristine' | 'loading' | 'reloading' | 'loaded')[]
) {
  return states.every((state) => {
    return state === 'loading' || state === 'reloading';
  });
}
