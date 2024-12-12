FROM golang:1.22@sha256:9855006ddcf40a79e9a2d90df11870331d24bcf2354232482ae132a7ba7b624f AS builder

COPY . /build

RUN cd /build && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' ./cmd/graph-smtpd

FROM gcr.io/distroless/base-debian12:nonroot@sha256:e5260be292def77bc70d03003f788f3d32c0796972ea1412d72cc0c843ab139a

COPY --from=builder /build/graph-smtpd /app/graph-smtpd

ENV SMTPD_ADDR=":2525"

EXPOSE 2525

ENTRYPOINT [ "/app/graph-smtpd" ]
