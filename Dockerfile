FROM cgr.dev/chainguard/static:latest@sha256:fc42af33e643f20d08baf03cddcc1c02ba57052dfb02edb0d79f3fc47bcea8d9
ARG TARGETPLATFORM
ENV SMTPD_ADDR=":2525" SMTPD_METRICS=":8080"
EXPOSE 2525 8080
ENTRYPOINT [ "/usr/bin/graph-smtpd" ]
COPY $TARGETPLATFORM/graph-smtpd /usr/bin/
USER 65532
