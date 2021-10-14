FROM golang AS build

COPY . /src
RUN cd /src/ingress && go build

FROM fedora

LABEL maintainer="OpCycle <oss@opcycle.net>"
LABEL repository="https://github.com/opcycle/docker-swarm-ingress"

ARG RESTY_YUM_REPO="https://openresty.org/package/fedora/openresty.repo"
ARG RESTY_LUAROCKS_VERSION="3.7.0"

RUN dnf install -y dnf-plugins-core \
    && dnf config-manager --add-repo ${RESTY_YUM_REPO} \
    && dnf install -y \
        gettext \
        make \
        openresty \
        openresty-opm \
        openresty-resty \
		openssl \
        tar \
        unzip \
    && cd /tmp \
    && curl -fSL https://luarocks.github.io/luarocks/releases/luarocks-${RESTY_LUAROCKS_VERSION}.tar.gz -o luarocks-${RESTY_LUAROCKS_VERSION}.tar.gz \
    && tar xzf luarocks-${RESTY_LUAROCKS_VERSION}.tar.gz \
    && cd luarocks-${RESTY_LUAROCKS_VERSION} \
    && ./configure \
        --prefix=/usr/local/openresty/luajit \
        --with-lua=/usr/local/openresty/luajit \
        --lua-suffix=jit-2.1.0-beta3 \
        --with-lua-include=/usr/local/openresty/luajit/include/luajit-2.1 \
    && make build \
    && make install \
    && cd /tmp \
    && rm -rf luarocks-${RESTY_LUAROCKS_VERSION} luarocks-${RESTY_LUAROCKS_VERSION}.tar.gz \
    && dnf clean all \
    && mkdir -p /var/run/openresty \
    && ln -sf /dev/stdout /usr/local/openresty/nginx/logs/access.log \
    && ln -sf /dev/stderr /usr/local/openresty/nginx/logs/error.log

ARG RESTY_J="1"

ENV PATH=$PATH:/usr/local/openresty/luajit/bin:/usr/local/openresty/nginx/sbin:/usr/local/openresty/bin
ENV LUA_PATH="/usr/local/openresty/site/lualib/?.ljbc;/usr/local/openresty/site/lualib/?/init.ljbc;/usr/local/openresty/lualib/?.ljbc;/usr/local/openresty/lualib/?/init.ljbc;/usr/local/openresty/site/lualib/?.lua;/usr/local/openresty/site/lualib/?/init.lua;/usr/local/openresty/lualib/?.lua;/usr/local/openresty/lualib/?/init.lua;./?.lua;/usr/local/openresty/luajit/share/luajit-2.1.0-beta3/?.lua;/usr/local/share/lua/5.1/?.lua;/usr/local/share/lua/5.1/?/init.lua;/usr/local/openresty/luajit/share/lua/5.1/?.lua;/usr/local/openresty/luajit/share/lua/5.1/?/init.lua"
ENV LUA_CPATH="/usr/local/openresty/site/lualib/?.so;/usr/local/openresty/lualib/?.so;./?.so;/usr/local/lib/lua/5.1/?.so;/usr/local/openresty/luajit/lib/lua/5.1/?.so;/usr/local/lib/lua/5.1/loadall.so;/usr/local/openresty/luajit/lib/lua/5.1/?.so"
ENV DOCKER_HOST "unix:///var/run/docker.sock"
ENV UPDATE_INTERVAL "1"
ENV OUTPUT_FILE "/etc/nginx/conf.d/proxy.conf"
ENV TEMPLATE_FILE "/etc/ingress/ingress.tpl"
ENV OPENRESTY_USER="openresty" \
    OPENRESTY_UID="8983" \
    OPENRESTY_GROUP="openresty" \
    OPENRESTY_GID="8983"

RUN groupadd -r --gid $OPENRESTY_GID $OPENRESTY_GROUP
RUN useradd -r --uid $OPENRESTY_UID --gid $OPENRESTY_GID $OPENRESTY_USER

RUN /usr/local/openresty/luajit/bin/luarocks install lua-resty-auto-ssl
RUN /usr/local/openresty/luajit/bin/luarocks install lua-resty-http

RUN openssl req -new -newkey rsa:2048 -days 3650 -nodes -x509 -subj '/CN=sni-support-required-for-valid-ssl' \
	-keyout /etc/ssl/resty-auto-ssl-fallback.key \
	-out /etc/ssl/resty-auto-ssl-fallback.crt

COPY --from=build /src/ingress/ingress /usr/bin/ingress

RUN mkdir /etc/resty-auto-ssl
RUN chown openresty /etc/resty-auto-ssl
RUN mkdir -p /etc/ingress
RUN rm -f /etc/nginx/conf.d/default.conf
ADD ingress/ingress.tpl /etc/ingress
ADD nginx.conf /usr/local/openresty/nginx/conf/nginx.conf

HEALTHCHECK --interval=3s --timeout=3s \
	CMD curl -f http://localhost/health || exit 1

ENTRYPOINT ["/usr/bin/ingress"]
