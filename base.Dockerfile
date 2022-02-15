FROM library/ubuntu:20.04

RUN sed -i -e "s/archive.ubuntu.com/mirrors.aliyun.com/g" /etc/apt/sources.list && \
    sed -i -e "s/security.ubuntu.com/mirrors.aliyun.com/g" /etc/apt/sources.list && \
    rm -rf /var/lib/apt/lists/* \
    apt-get clean && apt-get update -qq > /dev/null

RUN apt-get install -qq --assume-yes --fix-missing \
    build-essential gcc g++ zlib1g-dev python python3 wget  && \
    rm -rf /var/lib/apt/lists/*