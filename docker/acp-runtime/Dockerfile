FROM golang:1.27.2@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8 AS certificates
FROM ubuntu:24.04@sha256:c4a8d5503dfb2a3eb8ab5f807da5bc69a85730fb49b5cfca2330194ebcc41c7b AS runtime-files
ARG ACP_SHA256=39816d49d5ab92229952987225c23ea3fed7580699dcb4a444e0388e0ac1ce43
COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --chmod=0555 antigravity /usr/local/bin/agy
RUN echo "${ACP_SHA256}  /usr/local/bin/agy" | sha256sum -c -
COPY --chmod=0555 acp-runtime /usr/local/bin/acp-runtime
RUN mkdir -p /profile /run/acp /run/egress \
    && chown 10001:10001 /profile /run/acp /run/egress \
    && chmod 0700 /profile /run/acp /run/egress \
    && printf 'butler:x:10001:10001:Butler ACP:/profile/home:/usr/sbin/nologin\n' >> /etc/passwd \
    && printf 'butler:x:10001:\n' >> /etc/group

# The official ELF needs glibc, not an entire distribution. Keep the pinned
# loader/libraries and a small operator-only shell/file-delivery toolset.
FROM scratch
COPY --from=runtime-files /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/libm.so.6 /lib/x86_64-linux-gnu/libresolv.so.2 /lib/x86_64-linux-gnu/libpthread.so.0 /lib/x86_64-linux-gnu/libdl.so.2 /lib/x86_64-linux-gnu/librt.so.1 /lib/x86_64-linux-gnu/libselinux.so.1 /lib/x86_64-linux-gnu/libpcre2-8.so.0 /lib/x86_64-linux-gnu/
COPY --from=runtime-files /lib/x86_64-linux-gnu/ld-linux-x86-64.so.2 /lib64/ld-linux-x86-64.so.2
COPY --from=runtime-files /lib/x86_64-linux-gnu/libacl.so.1 /lib/x86_64-linux-gnu/libacl.so.1
COPY --from=runtime-files /lib/x86_64-linux-gnu/libattr.so.1 /lib/x86_64-linux-gnu/libattr.so.1
COPY --from=runtime-files /bin/dash /bin/sh
COPY --from=runtime-files /usr/bin/cat /usr/bin/mv /usr/bin/chmod /usr/bin/chown /usr/bin/
COPY --from=runtime-files /etc/passwd /etc/group /etc/nsswitch.conf /etc/
COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=runtime-files /usr/local/bin/agy /usr/local/bin/agy
COPY --from=runtime-files /usr/local/bin/acp-runtime /usr/local/bin/acp-runtime
COPY --from=runtime-files --chown=10001:10001 /profile /profile
COPY --from=runtime-files --chown=10001:10001 /run/acp /run/acp
COPY --from=runtime-files --chown=10001:10001 /run/egress /run/egress
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/acp-runtime"]
