FROM nginxinc/nginx-unprivileged:alpine
COPY . /usr/share/nginx/html
# Templates are processed with envsubst at container startup.
# API_HOST must be set in the environment (docker-compose or Railway).
COPY templates/default.conf.template /etc/nginx/templates/default.conf.template
EXPOSE 8080
