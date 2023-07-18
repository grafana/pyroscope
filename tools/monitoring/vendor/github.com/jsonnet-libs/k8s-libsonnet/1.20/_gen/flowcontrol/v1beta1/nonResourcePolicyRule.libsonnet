{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='nonResourcePolicyRule', url='', help='"NonResourcePolicyRule is a predicate that matches non-resource requests according to their verb and the target non-resource URL. A NonResourcePolicyRule matches a request if and only if both (a) at least one member of verbs matches the request and (b) at least one member of nonResourceURLs matches the request."'),
  '#withNonResourceURLs':: d.fn(help='"`nonResourceURLs` is a set of url prefixes that a user should have access to and may not be empty. For example:\\n  - \\"/healthz\\" is legal\\n  - \\"/hea*\\" is illegal\\n  - \\"/hea\\" is legal but matches nothing\\n  - \\"/hea/*\\" also matches nothing\\n  - \\"/healthz/*\\" matches all per-component health checks.\\n\\"*\\" matches all non-resource urls. if it is present, it must be the only entry. Required."', args=[d.arg(name='nonResourceURLs', type=d.T.array)]),
  withNonResourceURLs(nonResourceURLs): { nonResourceURLs: if std.isArray(v=nonResourceURLs) then nonResourceURLs else [nonResourceURLs] },
  '#withNonResourceURLsMixin':: d.fn(help='"`nonResourceURLs` is a set of url prefixes that a user should have access to and may not be empty. For example:\\n  - \\"/healthz\\" is legal\\n  - \\"/hea*\\" is illegal\\n  - \\"/hea\\" is legal but matches nothing\\n  - \\"/hea/*\\" also matches nothing\\n  - \\"/healthz/*\\" matches all per-component health checks.\\n\\"*\\" matches all non-resource urls. if it is present, it must be the only entry. Required."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='nonResourceURLs', type=d.T.array)]),
  withNonResourceURLsMixin(nonResourceURLs): { nonResourceURLs+: if std.isArray(v=nonResourceURLs) then nonResourceURLs else [nonResourceURLs] },
  '#withVerbs':: d.fn(help='"`verbs` is a list of matching verbs and may not be empty. \\"*\\" matches all verbs. If it is present, it must be the only entry. Required."', args=[d.arg(name='verbs', type=d.T.array)]),
  withVerbs(verbs): { verbs: if std.isArray(v=verbs) then verbs else [verbs] },
  '#withVerbsMixin':: d.fn(help='"`verbs` is a list of matching verbs and may not be empty. \\"*\\" matches all verbs. If it is present, it must be the only entry. Required."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='verbs', type=d.T.array)]),
  withVerbsMixin(verbs): { verbs+: if std.isArray(v=verbs) then verbs else [verbs] },
  '#mixin': 'ignore',
  mixin: self,
}
