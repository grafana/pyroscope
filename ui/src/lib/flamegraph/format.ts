// Replacements for @grafana/data getValueFormat + getDisplayProcessor.
// Only covers the three units this lib uses: 'ns' (duration),
// 'bytes' (size), 'short' (counts with K/Mil/Bil/Tri suffixes).
//
// Each formatter returns { text, suffix, numeric } where text+suffix is the
// display string and `numeric` is the original input — used for percent
// math in the tooltip and table.

export type Formatted = { text: string; suffix: string; numeric: number };

const KIB = 1024;
const MIB = KIB * 1024;
const GIB = MIB * 1024;
const TIB = GIB * 1024;

const NS_PER_US = 1_000;
const NS_PER_MS = 1_000_000;
const NS_PER_S = 1_000_000_000;
const NS_PER_MIN = 60 * NS_PER_S;
const NS_PER_HR = 60 * NS_PER_MIN;
const NS_PER_DAY = 24 * NS_PER_HR;

/** Regex-escape a string so it can be used as a literal pattern. */
export function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/** Format a count with K/Mil/Bil/Tri suffix. Matches @grafana/data short. */
export function formatShort(n: number): Formatted {
  const abs = Math.abs(n);
  if (abs >= 1e12) return scaled(n, 1e12, ' Tri');
  if (abs >= 1e9) return scaled(n, 1e9, ' Bil');
  if (abs >= 1e6) return scaled(n, 1e6, ' Mil');
  if (abs >= 1e3) return scaled(n, 1e3, ' K');
  return { text: trimZeros(n.toFixed(2)), suffix: '', numeric: n };
}

/** Format a nanosecond duration. */
export function formatDuration(ns: number): Formatted {
  const abs = Math.abs(ns);
  if (abs >= NS_PER_DAY) return scaled(ns, NS_PER_DAY, ' day');
  if (abs >= NS_PER_HR) return scaled(ns, NS_PER_HR, ' hour');
  if (abs >= NS_PER_MIN) return scaled(ns, NS_PER_MIN, ' min');
  if (abs >= NS_PER_S) return scaled(ns, NS_PER_S, ' s');
  if (abs >= NS_PER_MS) return scaled(ns, NS_PER_MS, ' ms');
  if (abs >= NS_PER_US) return scaled(ns, NS_PER_US, ' µs');
  return { text: String(Math.round(ns)), suffix: ' ns', numeric: ns };
}

/** Format a byte count with binary (1024-based) suffixes. */
export function formatBytes(b: number): Formatted {
  const abs = Math.abs(b);
  if (abs >= TIB) return scaled(b, TIB, ' TiB');
  if (abs >= GIB) return scaled(b, GIB, ' GiB');
  if (abs >= MIB) return scaled(b, MIB, ' MiB');
  if (abs >= KIB) return scaled(b, KIB, ' KiB');
  return { text: String(Math.round(b)), suffix: ' B', numeric: b };
}

/** Pick a formatter by unit string. Falls back to short for unknown units. */
export function formatByUnit(
  value: number,
  unit: string | undefined,
): Formatted {
  switch (unit) {
    case 'ns':
      return formatDuration(value);
    case 'bytes':
      return formatBytes(value);
    case 'short':
    default:
      return formatShort(value);
  }
}

function scaled(value: number, divisor: number, suffix: string): Formatted {
  return {
    text: trimZeros((value / divisor).toFixed(2)),
    suffix,
    numeric: value,
  };
}

/** "11.60" → "11.6"; "100.00" → "100"; "11" → "11". */
function trimZeros(s: string): string {
  if (!s.includes('.')) return s;
  return s.replace(/\.?0+$/, '');
}
