FROM nginxinc/nginx-unprivileged:alpine

# Copy only web assets — not Dockerfile, templates, or other build artefacts
COPY *.html /usr/share/nginx/html/
COPY *.js   /usr/share/nginx/html/
COPY *.css  /usr/share/nginx/html/
COPY assets/ /usr/share/nginx/html/assets/

# Templates are processed with envsubst at container startup.
# API_HOST must be set in the environment (docker-compose or Railway).
COPY templates/default.conf.template /etc/nginx/templates/default.conf.template

# Sourced before envsubst runs — exports RESOLVER from /etc/resolv.conf
# Must be executable so the nginx entrypoint doesn't ignore it
COPY docker-entrypoint.d/16-set-resolver.envsh /docker-entrypoint.d/16-set-resolver.envsh
RUN chmod +x /docker-entrypoint.d/16-set-resolver.envsh

EXPOSE 8080
