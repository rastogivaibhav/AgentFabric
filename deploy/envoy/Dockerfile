FROM envoyproxy/envoy:v1.32-latest

COPY envoy.yaml /etc/envoy/envoy.yaml

EXPOSE 15001 9901

ENTRYPOINT ["/usr/local/bin/envoy"]
CMD ["-c", "/etc/envoy/envoy.yaml", "--log-level", "info"]
