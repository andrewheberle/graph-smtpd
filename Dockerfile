FROM golang:1.24@sha256:14fd8a55e59a560704e5fc44970b301d00d344e45d6b914dda228e09f359a088 AS builder

COPY . /build

RUN cd /build && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' ./cmd/graph-smtpd

FROM gcr.io/distroless/base-debian12:nonroot@sha256:dca858878977f01b80685240ca2cd9c1bb38958e2ab6f4ed5698c0ea23307143

COPY --from=builder /build/graph-smtpd /app/graph-smtpd

ENV SMTPD_ADDR=":2525" \
    SMTPD_METRICS=":8080"

EXPOSE 2525 8080

ENTRYPOINT [ "/app/graph-smtpd" ]
