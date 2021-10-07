FROM golang AS build

COPY . /src
RUN cd /src/ingress && go build

FROM openresty/openresty:bionic

MAINTAINER opcycle <oss@opcycle.net>

ENV DOCKER_HOST "unix:///var/run/docker.sock"
ENV UPDATE_INTERVAL "1"
ENV OUTPUT_FILE "/etc/nginx/conf.d/proxy.conf"
ENV TEMPLATE_FILE "/etc/ingress/ingress.tpl"

RUN /usr/local/openresty/luajit/bin/luarocks install lua-resty-auto-ssl
RUN /usr/local/openresty/luajit/bin/luarocks install lua-resty-http

RUN openssl req -new -newkey rsa:2048 -days 3650 -nodes -x509 -subj '/CN=sni-support-required-for-valid-ssl' \
	-keyout /etc/ssl/resty-auto-ssl-fallback.key \
	-out /etc/ssl/resty-auto-ssl-fallback.crt

COPY --from=build /src/ingress/ingress /usr/bin/ingress

RUN mkdir -p /etc/ingress
RUN rm -f /etc/nginx/conf.d/default.conf
ADD ingress/ingress.tpl /etc/ingress
#ADD nginx.conf /usr/local/openresty/nginx/conf/nginx.conf

HEALTHCHECK --interval=3s --timeout=3s \
	CMD curl -f http://localhost/ || exit 1

ENTRYPOINT ["/usr/bin/ingress"]
