FROM clearlinux/mixer-ci:latest
COPY --chown=clr:clr . /home/clr/go/src/github.com/clearlinux/mixer-tools/
WORKDIR /home/clr/go/src/github.com/clearlinux/mixer-tools
ENTRYPOINT ["/bin/sh", "-c", "make && sudo -E make install && make lint && make check"]
