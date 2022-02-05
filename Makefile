all: k8s-webhook-handler deploy/image-crd.json deploy/build/

.PHONY: deploy/image-crd.json
deploy/image-crd.json:
	controller-gen paths=./... crd output:stdout | yaml2json /dev/stdin > $@

.PHONY: github.com/discordianfish/k8s-image-controller/pkg/generated
github.com/discordianfish/k8s-image-controller/pkg/generated:
	./hack/update-codegen.sh

.PHONY: deploy/build/
deploy/build/:
	cd deploy && ./generate

.PHONY: k8s-webhook-handler
k8s-webhook-handler: github.com/discordianfish/k8s-image-controller/pkg/generated
	go build
