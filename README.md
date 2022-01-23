# k8s-image-controller

This repository implements a Kubernetes controller for watching and building
Image resources defined with a CustomResourceDefinition (CRD).

# Development
It makes use of the generators in [k8s.io/code-generator](https://github.com/kubernetes/code-generator)
to generate a typed client, informers, listers and deep-copy functions. You can
do this yourself using the `./hack/update-codegen.sh` script.

The `update-codegen` script will automatically generate the following files &
directories:

* `pkg/apis/imagecontroller/v1alpha1/zz_generated.deepcopy.go`
* `pkg/generated/`

Changes should not be made to these files manually, and when creating your own
controller based off of this implementation you should not copy these files and
instead run the `update-codegen` script to generate your own.

## Running

**Prerequisite**: Since the image-controller uses `apps/v1` deployments, the Kubernetes cluster version should be greater than 1.9.

```sh
# assumes you have a working kubeconfig, not required if operating in-cluster
go build  .
./k8s-image-controller -kubeconfig=$HOME/.kube/config

# create the CustomResourceDefinition
kubectl create -f deploy/crd-status-subresource.yaml

# create a custom resource of type Image
kubectl create -f examples/example-image.yaml

# check images created
kubectl get images
```

### TODO
- [x] Updating image should create new job
- [x] Support build args
- [ ] Support build context
- [ ] Add ready status
