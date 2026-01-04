FROM debian:stretch-slim
ARG build_arch

COPY k8s-plugin-${build_arch} /k8s-plugin
ENTRYPOINT ["/k8s-plugin"]
