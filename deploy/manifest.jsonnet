local manifests = (import 'deploy.libsonnet') + {
  _config+: {
    image_registry: 'fish',
  },
};

{
  [name + '.yaml']: std.manifestYamlDoc(manifests[name])
  for name in std.objectFields(manifests)
}
