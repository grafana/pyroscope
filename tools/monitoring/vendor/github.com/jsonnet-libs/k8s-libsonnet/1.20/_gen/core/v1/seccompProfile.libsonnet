{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='seccompProfile', url='', help="\"SeccompProfile defines a pod/container's seccomp profile settings. Only one profile source may be set.\""),
  '#withLocalhostProfile':: d.fn(help="\"localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is \\\"Localhost\\\".\"", args=[d.arg(name='localhostProfile', type=d.T.string)]),
  withLocalhostProfile(localhostProfile): { localhostProfile: localhostProfile },
  '#withType':: d.fn(help='"type indicates which kind of seccomp profile will be applied. Valid options are:\\n\\nLocalhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied."', args=[d.arg(name='type', type=d.T.string)]),
  withType(type): { type: type },
  '#mixin': 'ignore',
  mixin: self,
}
