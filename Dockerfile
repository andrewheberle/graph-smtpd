FROM golang:1.24@sha256:fa145a3c13f145356057e00ed6f66fbd9bf017798c9d7b2b8e956651fe4f52da AS builder

COPY . /build

RUN cd /build && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' ./cmd/graph-smtpd

FROM gcr.io/distroless/base-debian12:nonroot@sha256:5c9b112e85b26632c6ba9ac874be9c6b20d61599f6087534ce2b9feeb7f6babf

COPY --from=builder /build/graph-smtpd /app/graph-smtpd

ENV SMTPD_ADDR=":2525" \
    SMTPD_METRICS=":8080"

EXPOSE 2525 8080

ENTRYPOINT [ "/app/graph-smtpd" ]
