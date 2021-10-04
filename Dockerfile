FROM openresty/openresty:bionic

ENV DOCKER_HOST "unix:///var/run/docker.sock"
ENV UPDATE_INTERVAL "1"
ENV DEBUG "false"

RUN /usr/local/openresty/luajit/bin/luarocks install lua-resty-auto-ssl
RUN /usr/local/openresty/luajit/bin/luarocks install lua-resty-http
RUN apt-get update
RUN apt-get install -y dnsutils python3 python3-venv python3-wheel

RUN python3 -m venv /opt/ingress
RUN /opt/ingress/bin/pip install --upgrade pip
RUN /opt/ingress/bin/pip install docker jinja2 setproctitle

RUN openssl req -new -newkey rsa:2048 -days 3650 -nodes -x509 -subj '/CN=sni-support-required-for-valid-ssl' \
	-keyout /etc/ssl/resty-auto-ssl-fallback.key \
	-out /etc/ssl/resty-auto-ssl-fallback.crt

ADD ingress/ingressd.j2 /opt/ingress
ADD ingress/ingressd /opt/ingress/bin/ingressd
RUN chmod +x /opt/ingress/bin/ingressd

HEALTHCHECK --interval=30s --timeout=3s \
	CMD curl -f http://localhost/ || exit 1

ENTRYPOINT ["/opt/ingress/bin/ingressd"]
