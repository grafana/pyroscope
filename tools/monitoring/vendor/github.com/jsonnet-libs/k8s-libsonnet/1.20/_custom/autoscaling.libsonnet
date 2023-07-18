local d = import 'doc-util/main.libsonnet';

local withApiVersion = {
  '#withApiVersion':: d.fn(help='API version of the referent', args=[d.arg(name='apiversion', type=d.T.string)]),
  withApiVersion(apiversion): { apiVersion: apiversion },
};


local withScaleTargetRef = {
  '#withScaleTargetRef':: d.fn(help='Set spec.ScaleTargetRef to `object`', args=[d.arg(name='object', type=d.T.object)]),
  withScaleTargetRef(object):
    { spec+: { scaleTargetRef+: {
      apiVersion: object.apiVersion,
      kind: object.kind,
      name: object.metadata.name,
    } } },
};

local patch = {
  crossVersionObjectReference+: withApiVersion,
  horizontalPodAutoscaler+: {
    spec+: withScaleTargetRef,
  },
};

{
  autoscaling+: {
    v1+: patch,
    v2beta1+: patch,
    v2beta2+: patch,
  },
}
