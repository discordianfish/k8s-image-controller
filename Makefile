all: k8s-webhook-handler deploy/build/

deploy/imagecontroller.5pi.de_images.json: crds/imagecontroller.5pi.de_images.yaml
deploy/imagecontroller.5pi.de_imagebuilders.json: crds/imagecontroller.5pi.de_imagebuilders.yaml

deploy/%.json: crds/%.yaml
	yaml2json $< > $@

.PHONY: crds
crds:
	controller-gen paths=./... crd:generateEmbeddedObjectMeta=true output:dir=crds/

.PHONY: github.com/discordianfish/k8s-image-controller/pkg/generated
github.com/discordianfish/k8s-image-controller/pkg/generated:
	./hack/update-codegen.sh

.PHONY: deploy/build/
deploy/build/: crds deploy/imagecontroller.5pi.de_images.json deploy/imagecontroller.5pi.de_imagebuilders.json
	cd deploy && ./generate

.PHONY: k8s-webhook-handler
k8s-webhook-handler: github.com/discordianfish/k8s-image-controller/pkg/generated
	go build
