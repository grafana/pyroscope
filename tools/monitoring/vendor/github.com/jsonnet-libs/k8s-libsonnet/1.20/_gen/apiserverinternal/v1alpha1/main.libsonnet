{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='v1alpha1', url='', help=''),
  serverStorageVersion: (import 'serverStorageVersion.libsonnet'),
  storageVersion: (import 'storageVersion.libsonnet'),
  storageVersionCondition: (import 'storageVersionCondition.libsonnet'),
  storageVersionSpec: (import 'storageVersionSpec.libsonnet'),
  storageVersionStatus: (import 'storageVersionStatus.libsonnet'),
}
