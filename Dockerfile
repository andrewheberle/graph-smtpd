FROM golang:1.23@sha256:cc637ce72c1db9586bd461cc5882df5a1c06232fd5dfe211d3b32f79c5a999fc AS builder

COPY . /build

RUN cd /build && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' ./cmd/graph-smtpd

FROM gcr.io/distroless/base-debian12:nonroot@sha256:e5260be292def77bc70d03003f788f3d32c0796972ea1412d72cc0c843ab139a

COPY --from=builder /build/graph-smtpd /app/graph-smtpd

ENV SMTPD_ADDR=":2525"

EXPOSE 2525

ENTRYPOINT [ "/app/graph-smtpd" ]
