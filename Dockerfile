FROM quay.io/app-sre/golang:1.17.8 as builder
WORKDIR /build
COPY . .
RUN make gobuild

FROM registry.access.redhat.com/ubi8-minimal
RUN microdnf update -y && microdnf install -y ca-certificates && rm -rf /var/cache/yum
COPY --from=builder /build/signalfx-prometheus-exporter /
ENTRYPOINT ["/signalfx-prometheus-exporter"]
CMD ["serve"]
