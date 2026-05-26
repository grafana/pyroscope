import { useEffect, useState } from 'react';

/**
 * Tracks whether the app is in light mode by observing the html `data-theme`
 * attribute (the same hook the rest of the app uses to switch themes). The
 * vendored flamegraph code originally took a `theme.isLight` from
 * `@grafana/data` createTheme; this replaces that single bit of JS-side state.
 */
export function useIsLight(): boolean {
  const [isLight, setIsLight] = useState(getIsLight);

  useEffect(() => {
    const observer = new MutationObserver(() => setIsLight(getIsLight()));
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    });
    return () => observer.disconnect();
  }, []);

  return isLight;
}

function getIsLight(): boolean {
  if (typeof document === 'undefined') return false;
  return document.documentElement.getAttribute('data-theme') === 'light';
}

/** Reads a `--var` from :root computed style. Returns trimmed string. */
export function cssVar(name: string): string {
  if (typeof getComputedStyle === 'undefined') return '';
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}
