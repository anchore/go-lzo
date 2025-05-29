FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y \
        python3 \
        python3-pip \
        build-essential \
        g++ \
        liblzo2-dev \
    && pip3 install python-lzo \
    && rm -rf /var/lib/apt/lists/*


COPY testdata/lzo-tool.cpp .
RUN g++ -o /usr/local/bin/lzo-tool lzo-tool.cpp -llzo2

COPY testdata/lzo-tool.py /usr/local/bin/lzo-tool.py

WORKDIR /workspace

CMD ["bash"]