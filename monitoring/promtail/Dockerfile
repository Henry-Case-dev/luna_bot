FROM alpine:3.19

RUN apk add --no-cache curl unzip libc6-compat

# Скачиваем Promtail
RUN curl -fsSL "https://github.com/grafana/loki/releases/download/v2.9.0/promtail-linux-amd64.zip" -o /tmp/promtail.zip && \
    unzip /tmp/promtail.zip -d /tmp && \
    mv /tmp/promtail-linux-amd64 /usr/local/bin/promtail && \
    chmod +x /usr/local/bin/promtail && \
    rm /tmp/promtail.zip

# Создаем директории
RUN mkdir -p /etc/promtail /tmp/promtail

# Копируем конфигурацию
COPY promtail-config.yml /etc/promtail/config.yml

CMD ["/usr/local/bin/promtail", "-config.file=/etc/promtail/config.yml"]
