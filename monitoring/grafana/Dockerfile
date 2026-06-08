FROM alpine:3.19

RUN apk add --no-cache curl tar

# Скачиваем Grafana
RUN curl -fsSL "https://dl.grafana.com/oss/release/grafana-10.1.0.linux-amd64.tar.gz" -o /tmp/grafana.tar.gz && \
    tar -xzf /tmp/grafana.tar.gz -C /tmp && \
    mv /tmp/grafana-10.1.0 /opt/grafana && \
    rm /tmp/grafana.tar.gz

# Создаем директории
RUN mkdir -p /var/lib/grafana /etc/grafana/provisioning/datasources
RUN chmod -R 755 /var/lib/grafana

# Копируем конфигурации
COPY provisioning/ /etc/grafana/provisioning/
COPY grafana.ini /etc/grafana/grafana.ini

EXPOSE 3000

CMD ["/opt/grafana/bin/grafana-server", "--homepath=/opt/grafana", "--config=/etc/grafana/grafana.ini"]
