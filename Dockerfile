FROM gcr.io/distroless/base-debian13:nonroot@sha256:6c591b4ac789b683ab3665327181e7cfb7a39b5834f42dcb73d7becb8d18a5d9
ARG TARGETPLATFORM
ENV SMTPD_ADDR=":2525" SMTPD_METRICS=":8080"
EXPOSE 2525 8080
ENTRYPOINT [ "/usr/bin/graph-smtpd" ]
COPY $TARGETPLATFORM/graph-smtpd /usr/bin/
