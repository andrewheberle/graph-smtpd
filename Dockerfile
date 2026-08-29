FROM gcr.io/distroless/base-debian13:nonroot@sha256:d199d20fb09c898d8822ae5cbd5cf3c6d424e9b5e1fc2eb9a719a7752cd9d861
ARG TARGETPLATFORM
ENV SMTPD_ADDR=":2525" SMTPD_METRICS=":8080"
EXPOSE 2525 8080
ENTRYPOINT [ "/usr/bin/graph-smtpd" ]
COPY $TARGETPLATFORM/graph-smtpd /usr/bin/
