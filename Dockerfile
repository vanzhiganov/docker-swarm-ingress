FROM golang AS build

RUN ls -la
RUN cd ingressd && go build

FROM openresty/openresty:bionic

MAINTAINER opcycle <oss@opcycle.net>

ENV DOCKER_HOST "unix:///var/run/docker.sock"
ENV UPDATE_INTERVAL "1"
ENV OUTPUT_FILE "/etc/nginx/conf.d/proxy.conf"
ENV TEMPLATE_FILE "/etc/ingressd/ingressd.tpl"

RUN /usr/local/openresty/luajit/bin/luarocks install lua-resty-auto-ssl
RUN /usr/local/openresty/luajit/bin/luarocks install lua-resty-http

RUN openssl req -new -newkey rsa:2048 -days 3650 -nodes -x509 -subj '/CN=sni-support-required-for-valid-ssl' \
	-keyout /etc/ssl/resty-auto-ssl-fallback.key \
	-out /etc/ssl/resty-auto-ssl-fallback.crt

COPY --from=build ingressd/ingressd /usr/bin/ingressd

RUN mkdir -p /etc/ingressd
ADD ingressd/ingressd.tpl /etc/ingressd

HEALTHCHECK --interval=30s --timeout=3s \
	CMD curl -f http://localhost/ || exit 1

ENTRYPOINT ["/usr/bin/ingressd"]
