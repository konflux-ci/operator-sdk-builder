FROM registry.access.redhat.com/ubi9/go-toolset:9.5-1739801907 as osdk-builder

COPY --chown=default ./operator-sdk/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN  ls -l && CGO_ENABLED=0 GOOS=linux go build -a -o operator-sdk cmd/operator-sdk/main.go

FROM registry.access.redhat.com/ubi9/go-toolset:9.5-1739801907 as opm-builder

COPY --chown=default ./operator-registry/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -o opm cmd/opm/main.go

FROM registry.access.redhat.com/ubi9/go-toolset:9.5-1739801907 as kustomize-builder

COPY --chown=default ./kustomize/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -o kustomize ./kustomize

FROM registry.access.redhat.com/ubi9/ubi-minimal@sha256:dee813b83663d420eb108983a1c94c614ff5d3fcb5159a7bd0324f0edbe7fca1

COPY LICENSE /licenses
COPY --from=osdk-builder /opt/app-root/src/operator-sdk /bin
COPY --from=kustomize-builder /opt/app-root/src/kustomize /bin

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
