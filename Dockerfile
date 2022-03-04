FROM registry.access.redhat.com/ubi8/go-toolset:1.16.12 as builder
COPY . .
RUN make gobuild

FROM registry.access.redhat.com/ubi8-minimal:8.5
RUN microdnf update -y && microdnf install -y ca-certificates && rm -rf /var/cache/yum
COPY --from=builder /opt/app-root/src/signalfx-prometheus-exporter /
ENTRYPOINT ["/signalfx-prometheus-exporter"]
CMD ["serve"]
