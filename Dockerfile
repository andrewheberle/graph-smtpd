FROM golang:1.25@sha256:5a79b94c34c299ac0361fbb7c7fca6dc552e166b42341050323fa3ab137d7be9 AS builder

COPY . /build

RUN cd /build && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' ./cmd/graph-smtpd

FROM gcr.io/distroless/base-debian12:nonroot@sha256:4b5196599229a5cf312a676cfe1ee8587ecf2371dcc22620f8c7a66d77d125c8

COPY --from=builder /build/graph-smtpd /app/graph-smtpd

ENV SMTPD_ADDR=":2525" \
    SMTPD_METRICS=":8080"

EXPOSE 2525 8080

ENTRYPOINT [ "/app/graph-smtpd" ]
