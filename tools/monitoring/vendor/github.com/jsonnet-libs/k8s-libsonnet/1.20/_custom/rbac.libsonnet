local d = import 'doc-util/main.libsonnet';

local bindRoleDoc = d.fn(
  '`bindRole` returns a roleRef for a Role or ClusterRole object.',
  [d.arg('role', d.T.object)]
);

local bindRole(role) = {
  roleRef: {
    name: role.metadata.name,
    kind: role.kind,
    apiGroup: std.split(role.apiVersion, '/')[0],
  },
};

local patch = {
  roleBinding+: {
    '#bindRole': bindRoleDoc,
    bindRole(role):: bindRole(role),
  },
  clusterRoleBinding+: {
    '#bindRole': bindRoleDoc,
    bindRole(role):: bindRole(role),
  },
  subject+: {
    '#fromServiceAccount': d.fn(
      '`fromServiceAccount` returns a subject for a service account.',
      [d.arg('service_account', d.T.object)]
    ),
    fromServiceAccount(service_account)::
      super.withKind('ServiceAccount') +
      super.withName(service_account.metadata.name) +
      super.withNamespace(service_account.metadata.namespace),
  },
};

{
  rbac+: {
    v1+: patch,
    v1alpha1+: patch,
    v1beta1+: patch,
  },
}
