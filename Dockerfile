FROM golang:1.24@sha256:fa145a3c13f145356057e00ed6f66fbd9bf017798c9d7b2b8e956651fe4f52da AS builder

COPY . /build

RUN cd /build && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' ./cmd/graph-smtpd

FROM gcr.io/distroless/base-debian12:nonroot@sha256:97d15218016debb9b6700a8c1c26893d3291a469852ace8d8f7d15b2f156920f

COPY --from=builder /build/graph-smtpd /app/graph-smtpd

ENV SMTPD_ADDR=":2525" \
    SMTPD_METRICS=":8080"

EXPOSE 2525 8080

ENTRYPOINT [ "/app/graph-smtpd" ]
