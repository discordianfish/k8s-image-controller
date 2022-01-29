local k = import 'k.libsonnet';

{
  _config:: {
    name: 'imagecontroller',
    namespace: 'kube-system',
    image_registry: error 'Must define image_registry',
    image_repository: self.name,
    image_tag: 'latest',
    image_ref: 'main',
  },
  serviceaccount: k.core.v1.serviceAccount.new($._config.name) +
                  k.core.v1.serviceAccount.metadata.withNamespace($._config.namespace),

  policyRules:: [
    k.rbac.v1.policyRule.withApiGroups(['imagecontroller.k8s.io']) +
    k.rbac.v1.policyRule.withResources(['images']) +
    k.rbac.v1.policyRule.withVerbs(['get', 'list', 'watch']),
    k.rbac.v1.policyRule.withApiGroups(['imagecontroller.k8s.io']) +
    k.rbac.v1.policyRule.withResources(['images/status']) +
    k.rbac.v1.policyRule.withVerbs(['update']),
    k.rbac.v1.policyRule.withApiGroups(['batch']) +
    k.rbac.v1.policyRule.withResources(['jobs']) +
    k.rbac.v1.policyRule.withVerbs(['create', 'list', 'watch', 'create', 'update', 'patch']),
    k.rbac.v1.policyRule.withApiGroups(['']) +
    k.rbac.v1.policyRule.withResources(['events']) +
    k.rbac.v1.policyRule.withVerbs(['create', 'patch']),
  ],

  cluster_role: k.rbac.v1.clusterRole.new($._config.name) +
                k.rbac.v1.clusterRole.withRules($.policyRules),

  cluster_role_binding: k.rbac.v1.clusterRoleBinding.new($._config.name) +
                        k.rbac.v1.clusterRoleBinding.bindRole($.cluster_role) +
                        k.rbac.v1.clusterRoleBinding.withSubjects(k.rbac.v1.subject.fromServiceAccount($.serviceaccount)),

  crd: std.parseJson(importstr 'crd.json'),

  container:: k.core.v1.container.new($._config.name, $._config.image_registry + '/' + $._config.image_repository + ':' + $._config.image_tag),

  deployment: k.apps.v1.deployment.new($._config.name, containers=$.container) +
              k.apps.v1.deployment.metadata.withNamespace($._config.namespace) +
              k.apps.v1.deployment.spec.template.spec.withServiceAccountName($.serviceaccount.metadata.name),

  image: {
    apiVersion: 'imagecontroller.k8s.io/v1alpha1',
    kind: 'Image',
    metadata: {
      name: 'imagecontroller',
    },
    spec: {
      registry: $._config.image_registry,
      repository: $._config.image_repository,
      tag: $._config.image_tag,
      containerfile: |||
        FROM docker.io/library/golang:1.17 as builder
        WORKDIR /usr/src
        ADD https://github.com/discordianfish/k8s-image-controller/archive/%s.tar.gz k8s-image-controller.tar.gz
        RUN tar --strip-components=1 -xzvf k8s-image-controller.tar.gz && \
          CGO_ENABLED=0 go install

        FROM scratch
        COPY --from=builder /go/bin/k8s-image-controller /
        ENTRYPOINT [ "/k8s-image-controller" ]
      ||| % $._config.image_ref,
    },
  },
}
