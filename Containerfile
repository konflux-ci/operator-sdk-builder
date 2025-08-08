FROM registry.access.redhat.com/ubi9/go-toolset:1.24.4-1754467841 as osdk-builder

COPY --chown=default ./operator-sdk/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN  ls -l && CGO_ENABLED=0 GOOS=linux go build -a -tags=containers_image_openpgp -o operator-sdk cmd/operator-sdk/main.go

FROM registry.access.redhat.com/ubi9/go-toolset:1.24.4-1754467841 as opm-builder

COPY --chown=default ./operator-registry/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -tags=containers_image_openpgp -o opm cmd/opm/main.go

FROM registry.access.redhat.com/ubi9/go-toolset:1.24.4-1754467841 as kustomize-builder

COPY --chown=default ./kustomize/. /opt/app-root/src/
WORKDIR /opt/app-root/src/kustomize
RUN ls -l && CGO_ENABLED=0 GOOS=linux GOWORK=off go build -a -o kustomize .

FROM registry.access.redhat.com/ubi9/go-toolset:1.24.4-1754467841 as controller-gen-builder

COPY --chown=default ./controller-tools/. /opt/app-root/src/
WORKDIR /opt/app-root/src
RUN ls -l && CGO_ENABLED=0 GOOS=linux go build -a -o controller-gen ./cmd/controller-gen

FROM registry.access.redhat.com/ubi9/ubi-minimal@sha256:0d7cfb0704f6d389942150a01a20cb182dc8ca872004ebf19010e2b622818926

COPY LICENSE /licenses
COPY --from=osdk-builder /opt/app-root/src/operator-sdk /bin
COPY --from=opm-builder /opt/app-root/src/opm /bin
COPY --from=kustomize-builder /opt/app-root/src/kustomize/kustomize /bin
COPY --from=controller-gen-builder /opt/app-root/src/controller-gen /bin
COPY files/policy.json /etc/containers/policy.json

ARG INSTALLED_RPMS="gettext make"
RUN microdnf install -y ${INSTALLED_RPMS} && microdnf clean all

ENTRYPOINT ["/bin/operator-sdk"]

LABEL \
  description="Konflux image containing rebuilds for tooling to assist in building OLM operators, bundles, and catalogs." \
  io.k8s.description="Konflux image containing rebuilds for tooling to assist in building OLM operators, bundles, and catalogs." \
  summary="Konflux operator-sdk builder" \
  io.k8s.display-name="Konflux operator-sdk builder" \
  io.openshift.tags="konflux build operator-sdk OLM tekton pipeline security" \
  name="Konflux operator-sdk builder" \
  com.redhat.component="operator-sdk-builder"
