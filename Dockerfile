FROM clearlinux:latest

ENV GOPATH /home/gopath
ENV PATH="/home/gopath/bin:${PATH}"
COPY . / /home/gopath/src/github.com/clearlinux/mixer-tools/
RUN swupd bundle-add mixer go-basic c-basic os-core-update-dev && \
    git config --global user.email "travis@example.com" && \
    git config --global user.name "Travis CI" && \
    clrtrust generate && \
    go get -u gopkg.in/alecthomas/gometalinter.v2 && \
    gometalinter.v2 --install

ENTRYPOINT ["/bin/sh", "-c", "cd /home/gopath/src/github.com/clearlinux/mixer-tools && make && make install && make lint; make check"]

