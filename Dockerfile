# This docker image is for integration testing only.

FROM golang:1.18-bullseye

ARG DEBIAN_FRONTEND=noninteractive

ENV TEST_BINARY=/test/trellis-cli
ENV TEST_DUMMY=/test/dummy

WORKDIR /app

# Trellis
RUN git clone https://github.com/roots/trellis.git "${TEST_DUMMY}/trellis"

# Ansible
RUN apt-get -q update && \
    apt-get install -q -y --no-install-recommends python3-pip && \
    apt-get clean && apt-get -y autoremove && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* && \
    cd "${TEST_DUMMY}/trellis" && pip install -r requirements.txt

# Ansible galaxy
RUN cd "${TEST_DUMMY}/trellis" && ansible-galaxy install -r galaxy.yml

# Bedrock
RUN mkdir -p "${TEST_DUMMY}/site" && \
    touch "${TEST_DUMMY}/site/.keep"

RUN go version && \
    ansible --version && \
    echo "Trellis commit: " && cd "${TEST_DUMMY}/trellis" && git log -1 --format="%h %s %aD"
