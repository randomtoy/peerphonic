FROM nginx:1.29-alpine

COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY web/index.html web/styles.css web/app.js /usr/share/nginx/html/

RUN cp /etc/nginx/conf.d/default.conf /tmp/default.conf && \
    sed -i 's#http://peerphonic:8080#http://127.0.0.1:8080#g' /etc/nginx/conf.d/default.conf && \
    nginx -t && \
    mv /tmp/default.conf /etc/nginx/conf.d/default.conf && \
    touch /var/run/nginx.pid && \
    chown -R nginx:nginx /var/run/nginx.pid /var/cache/nginx /etc/nginx/conf.d

USER nginx

EXPOSE 8080
