FROM registry.access.redhat.com/ubi9/go-toolset:1.24.4-1753221510 as osdk-builder

COPY --chown=default ./operator-sdk/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN  ls -l && CGO_ENABLED=0 GOOS=linux go build -a -tags=containers_image_openpgp -o operator-sdk cmd/operator-sdk/main.go

FROM registry.access.redhat.com/ubi9/go-toolset:1.24.4-1753221510 as opm-builder

COPY --chown=default ./operator-registry/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -tags=containers_image_openpgp -o opm cmd/opm/main.go

FROM registry.access.redhat.com/ubi9/go-toolset:1.24.4-1753221510 as kustomize-builder

COPY --chown=default ./kustomize/. /opt/app-root/src/
WORKDIR /opt/app-root/src/kustomize
RUN ls -l && CGO_ENABLED=0 GOOS=linux GOWORK=off go build -a -o kustomize .

FROM registry.access.redhat.com/ubi9/go-toolset:1.24.4-1753221510 as controller-gen-builder

COPY --chown=default ./controller-tools/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -o controller-gen ./cmd/controller-gen

FROM registry.access.redhat.com/ubi9/ubi-minimal@sha256:6d5a6576c83816edcc0da7ed62ba69df8f6ad3cbe659adde2891bfbec4dbf187

COPY LICENSE /licenses
COPY --from=osdk-builder /opt/app-root/src/operator-sdk /bin
COPY --from=opm-builder /opt/app-root/src/opm /bin
COPY --from=kustomize-builder /opt/app-root/src/kustomize/kustomize /bin
COPY --from=controller-gen-builder /opt/app-root/src/controller-gen /bin

ARG INSTALLED_RPMS="gettext make"
RUN microdnf install -y gettext

ENTRYPOINT ["/bin/operator-sdk"]

LABEL \
  description="Konflux image containing rebuilds for tooling to assist in building OLM operators, bundles, and catalogs." \
  io.k8s.description="Konflux image containing rebuilds for tooling to assist in building OLM operators, bundles, and catalogs." \
  summary="Konflux operator-sdk builder" \
  io.k8s.display-name="Konflux operator-sdk builder" \
  io.openshift.tags="konflux build operator-sdk OLM tekton pipeline security" \
  name="Konflux operator-sdk builder" \
  com.redhat.component="operator-sdk-builder"
