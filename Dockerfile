FROM alpine:3.22.1

LABEL maintainer="SoFMeRight <sofmeright@gmail.com>" \
      org.opencontainers.image.title="StageFreight" \
      description="A general-purpose DevOps automation image built to accelerate CI/CD pipelines." \
      org.opencontainers.image.description="A general-purpose DevOps automation image built to accelerate CI/CD pipelines." \
      org.opencontainers.image.source="https://gitlab.prplanit.com/precisionplanit/stagefreight-oci.git" \
      org.opencontainers.image.licenses="GPL-3.0"

ENV YQ_VERSION=v4.44.1 \
    YQ_BINARY=yq_linux_amd64

# Install dependencies
RUN apk add --no-cache \
      bash \
      curl \
      git \
      jq \
      coreutils \
  && curl -Ls "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${YQ_BINARY}" -o /usr/bin/yq \
  && chmod +x /usr/bin/yq

# Create required directory structure
RUN mkdir -p /opt/stagefreight
WORKDIR /opt/stagefreight

COPY README.md README.md

ENTRYPOINT ["/bin/sh"]
