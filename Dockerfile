# This docker image is for integration testing only.

FROM golang:1.12-buster

ARG DEBIAN_FRONTEND=noninteractive

ENV TEST_BINARY=/test/trellis-cli
ENV TEST_DUMMY=/test/dummy

WORKDIR /app

# CircleCI
# https://circleci.com/docs/2.0/custom-images/
RUN apt-get -q update && \
    apt-get install -q -y --no-install-recommends git openssh-client openssh-server tar gzip && \
    apt-get clean && apt-get -y autoremove && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Ansible
# https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html#latest-releases-via-apt-debian
RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 93C4A3FD7BB9C367 && \
    echo "deb http://ppa.launchpad.net/ansible/ansible-2.7/ubuntu trusty main" | tee -a /etc/apt/sources.list && \
    apt-get -q update && \
    apt-get install -q -y --no-install-recommends ansible && \
    apt-get clean && apt-get -y autoremove && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Trellis
RUN git clone https://github.com/roots/trellis.git "${TEST_DUMMY}/trellis" && \
    cd "${TEST_DUMMY}/trellis" && \
    ansible-galaxy install -r requirements.yml

# Bedrock
RUN mkdir -p "${TEST_DUMMY}/site" && \
    touch "${TEST_DUMMY}/site/.keep"

RUN go version && \
    ansible --version && \
    echo "Trellis commit: " && cd "${TEST_DUMMY}/trellis" && git log -1 --format="%h %s %aD"
