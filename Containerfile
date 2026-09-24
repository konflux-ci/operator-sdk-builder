FROM registry.access.redhat.com/ubi9/go-toolset:1.26.7-1790174511@sha256:0a4666f7a4eb0644c97a73cba198eb268691b270d97831822689e7a2088f87be as osdk-builder

COPY --chown=default ./operator-sdk/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN  ls -l && CGO_ENABLED=0 GOOS=linux go build -a -tags=containers_image_openpgp -o operator-sdk cmd/operator-sdk/main.go

FROM registry.access.redhat.com/ubi9/go-toolset:1.26.7-1790174511@sha256:0a4666f7a4eb0644c97a73cba198eb268691b270d97831822689e7a2088f87be as opm-builder

COPY --chown=default ./operator-registry/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -tags=containers_image_openpgp -o opm cmd/opm/main.go

FROM registry.access.redhat.com/ubi9/go-toolset:1.26.7-1790174511@sha256:0a4666f7a4eb0644c97a73cba198eb268691b270d97831822689e7a2088f87be as kustomize-builder

COPY --chown=default ./kustomize/. /opt/app-root/src/
WORKDIR /opt/app-root/src/kustomize
RUN ls -l && CGO_ENABLED=0 GOOS=linux GOWORK=off go build -a -o kustomize .

FROM registry.access.redhat.com/ubi9/go-toolset:1.26.7-1790174511@sha256:0a4666f7a4eb0644c97a73cba198eb268691b270d97831822689e7a2088f87be as controller-gen-builder

COPY --chown=default ./controller-tools/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -o controller-gen ./cmd/controller-gen

FROM registry.access.redhat.com/ubi9/go-toolset:1.26.7-1790174511@sha256:0a4666f7a4eb0644c97a73cba198eb268691b270d97831822689e7a2088f87be as yq-builder

COPY --chown=default ./yq/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -o yq .

FROM registry.access.redhat.com/ubi9/go-toolset:1.26.7-1790174511@sha256:0a4666f7a4eb0644c97a73cba198eb268691b270d97831822689e7a2088f87be

COPY LICENSE /licenses
COPY --from=osdk-builder /opt/app-root/src/operator-sdk /bin
COPY --from=opm-builder /opt/app-root/src/opm /bin
COPY --from=kustomize-builder /opt/app-root/src/kustomize/kustomize /bin
COPY --from=controller-gen-builder /opt/app-root/src/controller-gen /bin
COPY --from=yq-builder /opt/app-root/src/yq /bin
COPY files/policy.json /etc/containers/policy.json
COPY files/registry.access.redhat.com.yaml /etc/containers/registries.d/registry.access.redhat.com.yaml
COPY files/registry.redhat.io.yaml /etc/containers/registries.d/registry.redhat.io.yaml

ENV GOROOT=/usr/lib/golang
ENV PATH=${PATH}:${GOROOT}/bin

USER root
ARG INSTALLED_RPMS="gettext make"
RUN dnf install -y ${INSTALLED_RPMS} && dnf clean all
USER 1001

ENTRYPOINT ["/bin/operator-sdk"]

LABEL \
  description="Konflux image containing rebuilds for tooling to assist in building OLM operators, bundles, and catalogs." \
  io.k8s.description="Konflux image containing rebuilds for tooling to assist in building OLM operators, bundles, and catalogs." \
  summary="Konflux operator-sdk builder" \
  io.k8s.display-name="Konflux operator-sdk builder" \
  io.openshift.tags="konflux build operator-sdk OLM tekton pipeline security" \
  name="Konflux operator-sdk builder" \
  com.redhat.component="operator-sdk-builder"
