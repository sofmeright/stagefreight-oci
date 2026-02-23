# ---- Go build stage ----
FROM docker.io/library/golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum* ./
COPY src/ ./src/
RUN go mod tidy

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 go build \
      -ldflags "-s -w \
        -X github.com/sofmeright/stagefreight/src/version.Version=${VERSION} \
        -X github.com/sofmeright/stagefreight/src/version.Commit=${COMMIT} \
        -X github.com/sofmeright/stagefreight/src/version.BuildDate=${BUILD_DATE}" \
      -o /out/stagefreight ./src/cli

# ---- Runtime image ----
FROM docker.io/library/alpine:3.22.1

LABEL maintainer="SoFMeRight <sofmeright@gmail.com>" \
      org.opencontainers.image.title="StageFreight" \
      description="Declarative CI/CD automation CLI — detect, build, scan, and release container images from a single manifest." \
      org.opencontainers.image.description="Declarative CI/CD automation CLI — detect, build, scan, and release container images from a single manifest." \
      org.opencontainers.image.source="https://github.com/sofmeright/stagefreight.git" \
      org.opencontainers.image.licenses="AGPL-3.0-only"

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

# Pinned tool versions — bump these for updates.
ENV YQ_VERSION=v4.44.1 \
    BUILDX_VERSION=v0.31.1 \
    TRIVY_VERSION=0.69.1 \
    SYFT_VERSION=1.42.1

# Install yq
RUN curl -Ls "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64" -o /usr/bin/yq \
    && chmod +x /usr/bin/yq

# Install docker buildx
RUN mkdir -p ~/.docker/cli-plugins && \
    curl -Lo ~/.docker/cli-plugins/docker-buildx "https://github.com/docker/buildx/releases/download/${BUILDX_VERSION}/buildx-${BUILDX_VERSION}.linux-amd64" && \
    chmod +x ~/.docker/cli-plugins/docker-buildx

# Install trivy (vulnerability scanner)
RUN curl -Lo /tmp/trivy.tar.gz "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz" && \
    tar -xzf /tmp/trivy.tar.gz -C /usr/local/bin trivy && \
    rm /tmp/trivy.tar.gz

# Install syft (SBOM generator)
RUN curl -Lo /tmp/syft.tar.gz "https://github.com/anchore/syft/releases/download/v${SYFT_VERSION}/syft_${SYFT_VERSION}_linux_amd64.tar.gz" && \
    tar -xzf /tmp/syft.tar.gz -C /usr/local/bin syft && \
    rm /tmp/syft.tar.gz

# Copy the Go binary from builder stage.
COPY --from=builder /out/stagefreight /usr/local/bin/stagefreight

CMD ["/bin/sh"]
