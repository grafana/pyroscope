{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='keyToPath', url='', help='"Maps a string key to a path within a volume."'),
  '#withKey':: d.fn(help='"The key to project."', args=[d.arg(name='key', type=d.T.string)]),
  withKey(key): { key: key },
  '#withMode':: d.fn(help='"Optional: mode bits used to set permissions on this file. Must be an octal value between 0000 and 0777 or a decimal value between 0 and 511. YAML accepts both octal and decimal values, JSON requires decimal values for mode bits. If not specified, the volume defaultMode will be used. This might be in conflict with other options that affect the file mode, like fsGroup, and the result can be other mode bits set."', args=[d.arg(name='mode', type=d.T.integer)]),
  withMode(mode): { mode: mode },
  '#withPath':: d.fn(help="\"The relative path of the file to map the key to. May not be an absolute path. May not contain the path element '..'. May not start with the string '..'.\"", args=[d.arg(name='path', type=d.T.string)]),
  withPath(path): { path: path },
  '#mixin': 'ignore',
  mixin: self,
}
