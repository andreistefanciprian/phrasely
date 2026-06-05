FROM nginxinc/nginx-unprivileged:alpine

# Copy only web assets — not Dockerfile, templates, or other build artefacts
COPY *.html      /usr/share/nginx/html/
COPY *.js        /usr/share/nginx/html/
COPY *.css       /usr/share/nginx/html/
COPY llms.txt    /usr/share/nginx/html/llms.txt
COPY robots.txt  /usr/share/nginx/html/robots.txt
COPY assets/     /usr/share/nginx/html/assets/

# Templates are processed with envsubst at container startup.
# API_HOST must be set in the environment (docker-compose or Railway).
COPY templates/default.conf.template /etc/nginx/templates/default.conf.template

# Sourced before envsubst runs — exports RESOLVER from /etc/resolv.conf
# --chmod=755 sets executable bit at copy time (avoids chmod as non-root user)
COPY --chmod=755 docker-entrypoint.d/16-set-resolver.envsh /docker-entrypoint.d/16-set-resolver.envsh

EXPOSE 8080
