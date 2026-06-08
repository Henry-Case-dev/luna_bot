FROM alpine:3.19

RUN apk add --no-cache curl unzip

# Скачиваем Loki
RUN curl -fsSL "https://github.com/grafana/loki/releases/download/v2.9.0/loki-linux-amd64.zip" -o /tmp/loki.zip && \
    unzip /tmp/loki.zip -d /tmp && \
    mv /tmp/loki-linux-amd64 /usr/local/bin/loki && \
    chmod +x /usr/local/bin/loki && \
    rm /tmp/loki.zip

# Создаем директории
RUN mkdir -p /loki /etc/loki
RUN chmod -R 755 /loki

# Копируем конфигурацию
COPY loki-config.yml /etc/loki/config.yml

EXPOSE 3100

CMD ["/usr/local/bin/loki", "-config.file=/etc/loki/config.yml"]
