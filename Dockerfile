FROM golang:1.18-alpine AS builder

WORKDIR /build

COPY . .
RUN go install go.opentelemetry.io/collector/cmd/builder@latest


# Copy project files into container
COPY . .

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
RUN builder --config builder.yaml

FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM scratch

COPY --from=builder ["/tmp/otelcol-mindtastic/mindtastic-opentelemetry-collector", "/otelcol"]
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# Command to run when starting the container.
ENTRYPOINT ["/otelcol"]
