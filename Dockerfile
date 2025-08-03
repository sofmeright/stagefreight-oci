FROM alpine:3.22.1

LABEL maintainer="SoFMeRight <sofmeright@gmail.com>" \
      org.opencontainers.image.title="StageFreight" \
      description="A general-purpose DevOps automation image built to accelerate CI/CD pipelines." \
      org.opencontainers.image.description="A general-purpose DevOps automation image built to accelerate CI/CD pipelines." \
      org.opencontainers.image.source="https://gitlab.prplanit.com/precisionplanit/stagefreight-oci.git" \
      org.opencontainers.image.licenses="GPL-3.0"

# Install dependencies & useful tools.
RUN apk add --no-cache \
      bash \
      coreutils \
      curl \
      docker-cli \
      git \
      jq \
      python3 \
      py3-pip \
      py3-yaml \
      rsync \
      tree

# Install yq
ENV YQ_VERSION=v4.44.1 \
    YQ_BINARY=yq_linux_amd64
RUN curl -Ls "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${YQ_BINARY}" -o /usr/bin/yq \
    && chmod +x /usr/bin/yq

# Installs the latest buildx release at build time.
RUN mkdir -p ~/.docker/cli-plugins && \
    LATEST_BUILDX_VERSION=$(curl -s https://api.github.com/repos/docker/buildx/releases/latest | jq -r .tag_name) && \
    curl -Lo ~/.docker/cli-plugins/docker-buildx "https://github.com/docker/buildx/releases/download/${LATEST_BUILDX_VERSION}/buildx-${LATEST_BUILDX_VERSION}.linux-amd64" && \
    chmod +x ~/.docker/cli-plugins/docker-buildx

# Create required directory structure
RUN mkdir -p /opt/stagefreight
WORKDIR /opt/stagefreight

COPY README.md README.md

ENTRYPOINT ["/bin/sh"]