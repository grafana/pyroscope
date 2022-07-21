local d = import 'doc-util/main.libsonnet';

{
  local container = $.core.v1.container,
  local volumeMount = $.core.v1.volumeMount,
  local volume = $.core.v1.volume,

  local patch = {
    local volumeMountDescription =
      |||
        This helper function can be augmented with a `volumeMountsMixin`. For example,
        passing "k.core.v1.volumeMount.withSubPath(subpath)" will result in a subpath
        mixin.
      |||,


    '#configVolumeMount': d.fn(
      |||
        `configVolumeMount` mounts a ConfigMap by `name` on `path`.

        If `containers` is specified as an array of container names it will only be mounted
        to those containers, otherwise it will be mounted on all containers.

        This helper function can be augmented with a `volumeMixin`. For example,
        passing "k.core.v1.volume.configMap.withDefaultMode(420)" will result in a 
        default mode mixin.
      |||
      + volumeMountDescription,
      [
        d.arg('name', d.T.string),
        d.arg('path', d.T.string),
        d.arg('volumeMountMixin', d.T.object),
        d.arg('volumeMixin', d.T.object),
        d.arg('containers', d.T.array),
      ]
    ),
    configVolumeMount(name, path, volumeMountMixin={}, volumeMixin={}, containers=null)::
      local addMount(c) = c + (
        if containers == null || std.member(containers, c.name)
        then container.withVolumeMountsMixin(
          volumeMount.new(name, path) +
          volumeMountMixin,
        )
        else {}
      );

      super.mapContainers(addMount) +
      super.spec.template.spec.withVolumesMixin([
        volume.fromConfigMap(name, name) +
        volumeMixin,
      ]),


    '#configMapVolumeMount': d.fn(
      |||
        `configMapVolumeMount` mounts a `configMap` on `path`. It will
        also add an annotation hash to ensure the pods are re-deployed when the config map
        changes.

        If `containers` is specified as an array of container names it will only be mounted
        to those containers, otherwise it will be mounted on all containers.

        This helper function can be augmented with a `volumeMixin`. For example,
        passing "k.core.v1.volume.configMap.withDefaultMode(420)" will result in a 
        default mode mixin.
      |||
      + volumeMountDescription,
      [
        d.arg('configMap', d.T.object),
        d.arg('path', d.T.string),
        d.arg('volumeMountMixin', d.T.object),
        d.arg('volumeMixin', d.T.object),
        d.arg('containers', d.T.array),
      ]
    ),
    configMapVolumeMount(configMap, path, volumeMountMixin={}, volumeMixin={}, containers=null)::
      local name = configMap.metadata.name,
            hash = std.md5(std.toString(configMap));
      local addMount(c) = c + (
        if containers == null || std.member(containers, c.name)
        then container.withVolumeMountsMixin(
          volumeMount.new(name, path) +
          volumeMountMixin,
        )
        else {}
      );

      super.mapContainers(addMount) +
      super.spec.template.spec.withVolumesMixin([
        volume.fromConfigMap(name, name) +
        volumeMixin,
      ]) +
      super.spec.template.metadata.withAnnotationsMixin({
        ['%s-hash' % name]: hash,
      }),


    '#hostVolumeMount': d.fn(
      |||
        `hostVolumeMount` mounts a `hostPath` on `path`.

        If `containers` is specified as an array of container names it will only be mounted
        to those containers, otherwise it will be mounted on all containers.

        This helper function can be augmented with a `volumeMixin`. For example,
        passing "k.core.v1.volume.hostPath.withType('Socket')" will result in a 
        socket type mixin.
      |||
      + volumeMountDescription,
      [
        d.arg('name', d.T.string),
        d.arg('hostPath', d.T.string),
        d.arg('path', d.T.string),
        d.arg('readOnly', d.T.bool),
        d.arg('volumeMountMixin', d.T.object),
        d.arg('volumeMixin', d.T.object),
        d.arg('containers', d.T.array),
      ]
    ),
    hostVolumeMount(name, hostPath, path, readOnly=false, volumeMountMixin={}, volumeMixin={}, containers=null)::
      local addMount(c) = c + (
        if containers == null || std.member(containers, c.name)
        then container.withVolumeMountsMixin(
          volumeMount.new(name, path, readOnly=readOnly) +
          volumeMountMixin,
        )
        else {}
      );

      super.mapContainers(addMount) +
      super.spec.template.spec.withVolumesMixin([
        volume.fromHostPath(name, hostPath) +
        volumeMixin,
      ]),


    '#pvcVolumeMount': d.fn(
      |||
        `hostVolumeMount` mounts a PersistentVolumeClaim by `name` on `path`.

        If `containers` is specified as an array of container names it will only be mounted
        to those containers, otherwise it will be mounted on all containers.

        This helper function can be augmented with a `volumeMixin`. For example,
        passing "k.core.v1.volume.persistentVolumeClaim.withReadOnly(true)" will result in a 
        mixin that forces all container mounts to be read-only.
      |||
      + volumeMountDescription,
      [
        d.arg('name', d.T.string),
        d.arg('path', d.T.string),
        d.arg('readOnly', d.T.bool),
        d.arg('volumeMountMixin', d.T.object),
        d.arg('volumeMixin', d.T.object),
        d.arg('containers', d.T.array),
      ]
    ),
    pvcVolumeMount(name, path, readOnly=false, volumeMountMixin={}, volumeMixin={}, containers=null)::
      local addMount(c) = c + (
        if containers == null || std.member(containers, c.name)
        then container.withVolumeMountsMixin(
          volumeMount.new(name, path, readOnly=readOnly) +
          volumeMountMixin,
        )
        else {}
      );

      super.mapContainers(addMount) +
      super.spec.template.spec.withVolumesMixin([
        volume.fromPersistentVolumeClaim(name, name) +
        volumeMixin,
      ]),


    '#secretVolumeMount': d.fn(
      |||
        `secretVolumeMount` mounts a Secret by `name` into all container on `path`.'

        If `containers` is specified as an array of container names it will only be mounted
        to those containers, otherwise it will be mounted on all containers.

        This helper function can be augmented with a `volumeMixin`. For example,
        passing "k.core.v1.volume.secret.withOptional(true)" will result in a 
        mixin that allows the secret to be optional.
      |||
      + volumeMountDescription,
      [
        d.arg('name', d.T.string),
        d.arg('path', d.T.string),
        d.arg('defaultMode', d.T.string),
        d.arg('volumeMountMixin', d.T.object),
        d.arg('volumeMixin', d.T.object),
        d.arg('containers', d.T.array),
      ]
    ),
    secretVolumeMount(name, path, defaultMode=256, volumeMountMixin={}, volumeMixin={}, containers=null)::
      local addMount(c) = c + (
        if containers == null || std.member(containers, c.name)
        then container.withVolumeMountsMixin(
          volumeMount.new(name, path) +
          volumeMountMixin,
        )
        else {}
      );

      super.mapContainers(addMount) +
      super.spec.template.spec.withVolumesMixin([
        volume.fromSecret(name, secretName=name) +
        volume.secret.withDefaultMode(defaultMode) +
        volumeMixin,
      ]),

    '#secretVolumeMountAnnotated': d.fn(
      'same as `secretVolumeMount`, adding an annotation to force redeploy on change.'
      + volumeMountDescription,
      [
        d.arg('name', d.T.string),
        d.arg('path', d.T.string),
        d.arg('defaultMode', d.T.string),
        d.arg('volumeMountMixin', d.T.object),
        d.arg('volumeMixin', d.T.object),
        d.arg('containers', d.T.array),
      ]
    ),
    secretVolumeMountAnnotated(name, path, defaultMode=256, volumeMountMixin={}, volumeMixin={}, containers=null)::
      local annotations = { ['%s-secret-hash' % name]: std.md5(std.toString(name)) };

      self.secretVolumeMount(name, path, defaultMode, volumeMountMixin, volumeMixin, containers)
      + super.spec.template.metadata.withAnnotationsMixin(annotations),

    '#emptyVolumeMount': d.fn(
      |||
        `emptyVolumeMount` mounts empty volume by `name` into all container on `path`.

        If `containers` is specified as an array of container names it will only be mounted
        to those containers, otherwise it will be mounted on all containers.

        This helper function can be augmented with a `volumeMixin`. For example,
        passing "k.core.v1.volume.emptyDir.withSizeLimit('100Mi')" will result in a 
        mixin that limits the size of the volume to 100Mi.
      |||
      + volumeMountDescription,
      [
        d.arg('name', d.T.string),
        d.arg('path', d.T.string),
        d.arg('volumeMountMixin', d.T.object),
        d.arg('volumeMixin', d.T.object),
        d.arg('containers', d.T.array),
      ]
    ),
    emptyVolumeMount(name, path, volumeMountMixin={}, volumeMixin={}, containers=null)::
      local addMount(c) = c + (
        if containers == null || std.member(containers, c.name)
        then container.withVolumeMountsMixin(
          volumeMount.new(name, path) +
          volumeMountMixin,
        )
        else {}
      );

      super.mapContainers(addMount) +
      super.spec.template.spec.withVolumesMixin([
        volume.fromEmptyDir(name) + volumeMixin,
      ]),
  },

  batch+: {
    v1+: {
      job+: patch,
    },
  },
  apps+: { v1+: {
    daemonSet+: patch,
    deployment+: patch,
    replicaSet+: patch,
    statefulSet+: patch,
  } },
}
