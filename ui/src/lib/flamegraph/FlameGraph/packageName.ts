// Package names are inferred from the shape of a symbol. C++ and Rust use
// `::` to separate the module path, while the other runtimes have dedicated
// symbol formats.
const matchers: Array<[string, RegExp]> = [
  [
    'phpspy',
    /^(?<packageName>([^/]*\/)*)(?<filename>.*\.php+)(?<line_info>.*)$/,
  ],
  ['pyspy', /^(?<packageName>([^/]*\/)*)(?<filename>.*\.py+)(?<line_info>.*)$/],
  ['rbspy', /^(?<packageName>([^/]*\/)*)(?<filename>.*\.rb+)(?<line_info>.*)$/],
  [
    'nodespy',
    /^(\.\/node_modules\/)?(?<packageName>[^/]*)(?<filename>.*\.?(jsx?|tsx?)?):(?<functionName>.*):(?<line_info>.*)$/,
  ],
  ['gospy', /^(?<packageName>.*?\/.*?\.|.*?\.|.+)(?<functionName>.*)$/], // also 'scrape'
  ['javaspy', /^(?<packageName>.+\/)(?<filename>.+\.)(?<functionName>.+)$/],
  ['dotnetspy', /^(?<packageName>.+)\.(.+)\.(.+)\(.*\)$/],
  ['tracing', /^(?<packageName>.+?):.*$/],
  ['ebpfspy', /^(?<packageName>.+)$/],
  ['unknown', /^(?<packageName>.+)$/],
];

// C++ and Rust demangled symbols use `::` to separate the module path. The
// first segment groups a Rust symbol under its crate and a C++ symbol under
// its top-level namespace (or class when there is no namespace). Without this
// the generic matchers below would treat `::` as Node.js's `file:line:column`
// separator and group by almost the whole symbol.
function getNamespacedPackageName(name: string): string | undefined {
  if (name.startsWith('(anonymous namespace)::')) {
    return '(anonymous namespace)';
  }

  const normalized = name.replace(/^(?:virtual|non-virtual) thunk to /, '');

  const namespaced = normalized.match(
    /^[<&*\s]*(?:(?:mut|dyn)\s+)*(?<packageName>[A-Za-z_]\w*)::/,
  );
  if (namespaced?.groups?.packageName) {
    return namespaced.groups.packageName;
  }

  // Rust renders trait implementations as `<Type as crate::Trait>`.
  const traitImpl = normalized.match(
    /^[<&*\s]*(?:(?:mut|dyn)\s+)*(?<implementor>[A-Za-z_]\w*)\s+as\s+/,
  );
  return traitImpl?.groups?.implementor;
}

// Unsymbolized frames are rendered as `binary.so+0x1234` (and
// `binary (deleted)+0x1234` for unlinked binaries). Group all addresses from
// the same binary together, ignoring the library version suffix.
function getBinaryPackageName(name: string): string | undefined {
  const match = name.match(
    /^(?<packageName>.+?)(?:\s+\(deleted\))?\+0x[0-9a-f]+$/i,
  );
  if (!match?.groups?.packageName) {
    return undefined;
  }
  return match.groups.packageName.replace(/\.so(?:\.[\d.]+)?$/, '.so');
}

export function getPackageName(name: string): string | undefined {
  if (name.includes('::')) {
    const namespaced = getNamespacedPackageName(name);
    if (namespaced) {
      return namespaced;
    }
  }

  const binary = getBinaryPackageName(name);
  if (binary) {
    return binary;
  }

  for (const [_, matcher] of matchers) {
    const match = name.match(matcher);
    if (match) {
      return match.groups?.packageName || '';
    }
  }
  return undefined;
}
