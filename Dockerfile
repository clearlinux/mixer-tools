FROM clearlinux/mixer-ci:latest
ENV LC_ALL="en_US.UTF-8"
WORKDIR /home/clr/mixer-tools
COPY --chown=clr:clr . .
ENTRYPOINT ["/home/clr/mixer-tools/entrypoint.sh"]
