# Copyright 2026 OpenSSF Scorecard Authors.
# SPDX-License-Identifier: Apache-2.0

# syntax=docker/dockerfile:1

FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/scorecard-api ./cmd/scorecard-api

FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 scorecard && \
    mkdir -p /data && chown scorecard:scorecard /data

COPY --from=builder /out/scorecard-api /usr/local/bin/scorecard-api

USER scorecard
VOLUME /data
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/scorecard-api"]
