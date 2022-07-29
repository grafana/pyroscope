{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='v1', url='', help=''),
  apiService: (import 'apiService.libsonnet'),
  apiServiceCondition: (import 'apiServiceCondition.libsonnet'),
  apiServiceSpec: (import 'apiServiceSpec.libsonnet'),
  apiServiceStatus: (import 'apiServiceStatus.libsonnet'),
  serviceReference: (import 'serviceReference.libsonnet'),
}
