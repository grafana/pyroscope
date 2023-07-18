local d = import 'doc-util/main.libsonnet';

{
  core+: {
    v1+: {
      configMap+: {
        local withData(data) = if data != {} then super.withData(data) else {},
        withData:: withData,

        local withDataMixin(data) = if data != {} then super.withDataMixin(data) else {},
        withDataMixin:: withDataMixin,

        '#new': d.fn('new creates a new `ConfigMap` of given `name` and `data`', [d.arg('name', d.T.string), d.arg('data', d.T.object)]),
        new(name, data={})::
          super.new(name)
          + super.metadata.withName(name)
          + withData(data),
      },

      container+: {
        '#new': d.fn('new returns a new `container` of given `name` and `image`', [d.arg('name', d.T.string), d.arg('image', d.T.string)]),
        new(name, image):: super.withName(name) + super.withImage(image),

        withEnvMixin(env)::
          // if an envvar has an empty value ("") we want to remove that property
          // because k8s will remove that and then it would always
          // show up as a difference.
          local removeEmptyValue(obj) =
            if std.objectHas(obj, 'value') && std.length(obj.value) == 0 then
              {
                [k]: obj[k]
                for k in std.objectFields(obj)
                if k != 'value'
              }
            else
              obj;
          super.withEnvMixin([
            removeEmptyValue(envvar)
            for envvar in env
          ]),

        '#withEnvMap': d.fn(
          '`withEnvMap` works like `withEnvMixin` but accepts a key/value map, this map is converted a list of core.v1.envVar(key, value)`',
          [d.arg('env', d.T.object)]
        ),
        withEnvMap(env)::
          self.withEnvMixin([
            $.core.v1.envVar.new(k, env[k])
            for k in std.objectFields(env)
          ]),

        withResourcesRequests(cpu, memory)::
          self.resources.withRequests(
            (if cpu != null
             then { cpu: cpu }
             else {}) +
            (if memory != null
             then { memory: memory }
             else {})
          ),

        withResourcesLimits(cpu, memory)::
          self.resources.withLimits(
            (if cpu != null
             then { cpu: cpu }
             else {}) +
            (if memory != null
             then { memory: memory }
             else {})
          ),
      },

      containerPort+: {
        // using a local here to re-use new, because it is lexically scoped,
        // while `self` is not
        local new(containerPort) = super.withContainerPort(containerPort),
        local newNamed(containerPort, name) = new(containerPort) + super.withName(name),
        '#new': d.fn('new returns a new `containerPort`', [d.arg('containerPort', d.T.int)]),
        new:: new,
        '#newNamed': d.fn('newNamed works like `new`, but also sets the `name`', [d.arg('containerPort', d.T.int), d.arg('name', d.T.string)]),
        newNamed:: newNamed,
        '#newUDP': d.fn('newUDP works like `new`, but also sets protocal to UDP', [d.arg('containerPort', d.T.int)]),
        newUDP(containerPort):: new(containerPort) + super.withProtocol('UDP'),
        '#newNamedUDP': d.fn('newNamedUDP works like `newNamed`, but also sets protocal to UDP', [d.arg('containerPort', d.T.int), d.arg('name', d.T.string)]),
        newNamedUDP(containerPort, name):: newNamed(containerPort, name) + super.withProtocol('UDP'),
      },

      envVar+: {
        '#new': d.fn('new returns a new `envVar` of given `name` and `value`', [d.arg('name', d.T.string), d.arg('value', d.T.string)]),
        new(name, value):: super.withName(name) + super.withValue(value),

        '#fromSecretRef': d.fn('fromSecretRef creates a `envVar` from a secret reference', [
          d.arg('name', d.T.string),
          d.arg('secretRefName', d.T.string),
          d.arg('secretRefKey', d.T.string),
        ]),
        fromSecretRef(name, secretRefName, secretRefKey)::
          super.withName(name)
          + super.valueFrom.secretKeyRef.withName(secretRefName)
          + super.valueFrom.secretKeyRef.withKey(secretRefKey),

        '#fromFieldPath': d.fn('fromFieldPath creates a `envVar` from a field path', [
          d.arg('name', d.T.string),
          d.arg('fieldPath', d.T.string),
        ]),
        fromFieldPath(name, fieldPath)::
          super.withName(name)
          + super.valueFrom.fieldRef.withFieldPath(fieldPath),
      },

      keyToPath+:: {
        '#new': d.fn('new creates a new `keyToPath`', [d.arg('key', d.T.string), d.arg('path', d.T.string)]),
        new(key, path):: super.withKey(key) + super.withPath(path),
      },

      persistentVolume+: {
        '#new':: d.fn(help='new returns an instance of Persistentvolume', args=[d.arg(name='name', type=d.T.string)]),
        new(name=''): {
          apiVersion: 'v1',
          kind: 'PersistentVolume',
        } + (
          if name != ''
          then self.metadata.withName(name=name)
          else {}
        ),
      },

      secret+:: {
        '#new'+: d.func.withArgs([
          d.arg('name', d.T.string),
          d.arg('data', d.T.object),
          d.arg('type', d.T.string, 'Opaque'),
        ]),
        new(name, data, type='Opaque')::
          super.new(name)
          + super.withData(data)
          + super.withType(type),
      },

      service+:: {
        '#new'+: d.func.withArgs([
          d.arg('name', d.T.string),
          d.arg('selector', d.T.object),
          d.arg('ports', d.T.array),
        ]),
        new(name, selector, ports)::
          super.new(name)
          + super.spec.withSelector(selector)
          + super.spec.withPorts(ports),
        '#newWithoutSelector'+: d.fn('newWithoutSelector works like `new`, but creates a Service without ports and selector', [
          d.arg('name', d.T.string),
        ]),
        newWithoutSelector(name)::
          super.new(name),
      },

      servicePort+:: {
        local new(port, targetPort) = super.withPort(port) + super.withTargetPort(targetPort),
        '#new': d.fn('new returns a new `servicePort`', [
          d.arg('port', d.T.int),
          d.arg('targetPort', d.T.any),
        ]),
        new:: new,

        '#newNamed': d.fn('newNamed works like `new`, but also sets the `name`', [
          d.arg('name', d.T.string),
          d.arg('port', d.T.int),
          d.arg('targetPort', d.T.any),
        ]),
        newNamed(name, port, targetPort)::
          new(port, targetPort) + super.withName(name),
      },

      volume+:: {
        '#fromConfigMap': d.fn('Creates a new volume from a `ConfigMap`', [
          d.arg('name', d.T.string),
          d.arg('configMapName', d.T.string),
          d.arg('configMapItems', d.T.array),
        ]),
        fromConfigMap(name, configMapName, configMapItems=[])::
          super.withName(name)
          + super.configMap.withName(configMapName)
          + (
            if configMapItems != []
            then super.configMap.withItems(configMapItems)
            else {}
          ),

        '#fromEmptyDir': d.fn('Creates a new volume of type `emptyDir`', [
          d.arg('name', d.T.string),
          d.arg('emptyDir', d.T.object, {}),
        ]),
        fromEmptyDir(name, emptyDir={})::
          super.withName(name) + { emptyDir: emptyDir },

        '#fromPersistentVolumeClaim': d.fn('Creates a new volume using a `PersistentVolumeClaim`.', [
          d.arg('name', d.T.string),
          d.arg('claimName', d.T.string),
        ]),
        fromPersistentVolumeClaim(name, claimName='', emptyDir='')::
          // Note: emptyDir is inherited from ksonnet-lib, this provides backwards compatibility
          local claim =
            if (claimName == '' && emptyDir != '')
            then emptyDir
            else
              if claimName == ''
              then error 'claimName not set'
              else claimName;
          super.withName(name) + super.persistentVolumeClaim.withClaimName(claim),

        '#fromHostPath': d.fn('Creates a new volume using a `hostPath`', [
          d.arg('name', d.T.string),
          d.arg('hostPath', d.T.string),
        ]),
        fromHostPath(name, hostPath)::
          super.withName(name) + super.hostPath.withPath(hostPath),

        '#fromSecret': d.fn('Creates a new volume from a `Secret`', [
          d.arg('name', d.T.string),
          d.arg('secretName', d.T.string),
        ]),
        fromSecret(name, secretName)::
          super.withName(name) + super.secret.withSecretName(secretName),
      },

      volumeMount+:: {
        '#new': d.fn('new creates a new `volumeMount`', [
          d.arg('name', d.T.string),
          d.arg('mountPath', d.T.string),
          d.arg('readOnly', d.T.bool),
        ]),
        new(name, mountPath, readOnly=false)::
          super.withName(name)
          + super.withMountPath(mountPath)
          + (if readOnly then self.withReadOnly(readOnly) else {}),
      },
    },
  },
}
