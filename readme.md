# Exporter для мониторинга памяти сервисов docker compose

## Пример
```yml
  services-metrics:
    image: porebric/services-metrics:latest
    container_name: services-metrics
    environment:
      ALL_SERVICES: 0
      SERVICE_1: container_name_1 # значение, указанное в container_name
      SERVICE_2: container_name_2
    ports:
      - 9339:9338
    depends_on:
      - prometheus
      - redis2
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```