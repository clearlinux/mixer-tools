FROM clearlinux/mixer-ci:latest
ENV LC_ALL="en_US.UTF-8"
COPY --chown=clr:clr . /home/clr/go/src/github.com/clearlinux/mixer-tools/
WORKDIR /home/clr/go/src/github.com/clearlinux/mixer-tools
ENTRYPOINT ["/bin/sh", "-c", "make && sudo -E make install && make lint && make check && make batcheck"]
