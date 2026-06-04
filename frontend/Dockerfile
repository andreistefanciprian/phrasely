FROM nginxinc/nginx-unprivileged:alpine

# Copy only web assets — not Dockerfile, templates, or other build artefacts
COPY *.html /usr/share/nginx/html/
COPY *.js   /usr/share/nginx/html/
COPY *.css  /usr/share/nginx/html/
COPY assets/ /usr/share/nginx/html/assets/

# Templates are processed with envsubst at container startup.
# API_HOST must be set in the environment (docker-compose or Railway).
COPY templates/default.conf.template /etc/nginx/templates/default.conf.template

EXPOSE 8080
