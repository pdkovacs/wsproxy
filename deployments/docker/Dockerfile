FROM ubuntu:22.04

LABEL Name="wsproxy"
LABEL Version="1.0.0"

RUN useradd -m --shell /bin/false wsproxy

RUN apt-get update && apt-get install -y git curl && apt-get clean
RUN mkdir -p /opt/wsproxy
WORKDIR /opt/wsproxy
COPY wsproxy ./
RUN chmod +x wsproxy
RUN ls -la /opt/
EXPOSE 8080

USER wsproxy

CMD [ "/opt/wsproxy/wsproxy" ]
