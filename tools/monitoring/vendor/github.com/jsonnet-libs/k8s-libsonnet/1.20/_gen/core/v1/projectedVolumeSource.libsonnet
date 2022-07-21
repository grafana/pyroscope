{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='projectedVolumeSource', url='', help='"Represents a projected volume source"'),
  '#withDefaultMode':: d.fn(help='"Mode bits used to set permissions on created files by default. Must be an octal value between 0000 and 0777 or a decimal value between 0 and 511. YAML accepts both octal and decimal values, JSON requires decimal values for mode bits. Directories within the path are not affected by this setting. This might be in conflict with other options that affect the file mode, like fsGroup, and the result can be other mode bits set."', args=[d.arg(name='defaultMode', type=d.T.integer)]),
  withDefaultMode(defaultMode): { defaultMode: defaultMode },
  '#withSources':: d.fn(help='"list of volume projections"', args=[d.arg(name='sources', type=d.T.array)]),
  withSources(sources): { sources: if std.isArray(v=sources) then sources else [sources] },
  '#withSourcesMixin':: d.fn(help='"list of volume projections"\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='sources', type=d.T.array)]),
  withSourcesMixin(sources): { sources+: if std.isArray(v=sources) then sources else [sources] },
  '#mixin': 'ignore',
  mixin: self,
}
