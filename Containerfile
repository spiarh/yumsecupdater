ARG FROM_IMAGE
ARG FROM_IMAGE_BUILDER

FROM ${FROM_IMAGE_BUILDER} AS builder

WORKDIR /go/bin
COPY . .

RUN make build

FROM ${FROM_IMAGE}

ADD selinux-module/ selinux-module/

RUN microdnf install -y util-linux && \
    rm -rf /var/cache/yum

COPY --from=builder /go/bin/yumsecupdater /usr/local/bin/yumsecupdater

ENTRYPOINT ["/usr/local/bin/yumsecupdater"]
