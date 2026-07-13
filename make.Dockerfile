ARG GOLANGCI_LINT_VERSION

FROM registry.access.redhat.com/ubi8/ubi

ARG GOLANGCI_LINT_VERSION

RUN yum install -y ca-certificates git go-toolset make

ENV GOPATH=/go
ENV PATH="$GOPATH/bin:${PATH}"
# Official install URL; the abandoned master-branch script fails checksum verification.
# https://golangci-lint.run/docs/welcome/install/local/#binaries
RUN curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b ${GOPATH}/bin ${GOLANGCI_LINT_VERSION}

ADD https://password.corp.redhat.com/RH-IT-Root-CA.crt /etc/pki/ca-trust/source/anchors/
RUN update-ca-trust extract
