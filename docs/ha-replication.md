# Репликация и отказоустойчивость

Документ описывает план репликации БД и S3, а также архитектуру без единой центральной точки отказа при сохранении доменного доступа.

---

## 1. Цели

- **Отказоустойчивость**: при падении одного сервера остальные продолжают обслуживать домен
- **Репликация данных**: БД и S3 реплицируются между узлами
- **Децентрализация**: нет одного «центрального» сервера; каждый домен — независимый кластер
- **Доступ по домену**: `@user:example.org` по-прежнему резолвится через DNS на доступный кластер

---

## 2. Модель: домен = кластер

```
                    DNS: example.org → LB (VIP или Round-robin)
                                    │
         ┌──────────────────────────┼──────────────────────────┐
         │                          │                          │
         ▼                          ▼                          ▼
   ┌───────────┐             ┌───────────┐             ┌───────────┐
   │  Server 1 │             │  Server 2 │             │  Server N │
   │  (app)    │             │  (app)    │             │  (app)    │
   └─────┬─────┘             └─────┬─────┘             └─────┬─────┘
         │                         │                         │
         └─────────────────────────┼─────────────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
              ▼                    ▼                    ▼
       ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
       │ PostgreSQL  │     │ MinIO/S3    │     │   Redis     │
       │ (primary +  │     │ (distributed│     │  (cluster)  │
       │  replicas)  │     │  или CRR)   │     │             │
       └─────────────┘     └─────────────┘     └─────────────┘
```

**Ключевое**: каждый домен (`example.org`, `friend.net`) — отдельный кластер. Между доменами — федерация (S2S). Центрального «главного» сервера нет: любой домен может работать автономно.

---

## 3. Репликация PostgreSQL

### Варианты

| Вариант | Описание | Плюсы | Минусы |
|---------|----------|-------|--------|
| **Streaming Replication** | Primary + 1+ Standby, async/sync | Просто, штатные средства PG | Ручной failover |
| **Patroni + etcd** | Автоматический failover | HA из коробки | Сложнее, нужен etcd |
| **Managed (RDS, Cloud SQL)** | Облачный PostgreSQL | Автоматический failover, бэкапы | Vendor lock-in, стоимость |

### Рекомендация для self-hosted

**Streaming Replication** для MVP:

- Primary: запись
- Standby: чтение (можно направить sync-запросы на read replica)
- Failover: ручной или через внешний скрипт/healthcheck

**Patroni** для полноценного HA:

- Автовыбор Primary при падении текущего
- etcd/Consul как distributed lock

### Конфиг (пример для streaming replication)

```yaml
# postgres-primary: postgres с wal_level=replica
# postgres-replica: recovery.conf / standby.signal, подключение к primary
```

---

## 4. Репликация S3 / MinIO

### Варианты

| Вариант | Описание | Плюсы | Минусы |
|---------|----------|-------|--------|
| **MinIO Distributed** | 4+ нод MinIO в Erasure Coding | Встроенная репликация, отказоустойчивость | Минимум 4 ноды |
| **MinIO + Site Replication** | Несколько MinIO-кластеров, replication между ними | Multi-site | Сложнее настройка |
| **S3 Cross-Region Replication** | AWS S3 CRR | Managed, надёжно | Только AWS или S3-совместимые |
| **Два MinIO + sync скрипт** | rclone / mc mirror | Просто | Не real-time, возможны расхождения |

### Рекомендация

**MinIO Distributed Mode** (4 ноды):

```bash
# Запуск 4 нод MinIO как один кластер
minio server http://minio{1...4}/data{1...2}
```

Каждый объект — Erasure Coded, выдерживает потерю нескольких нод.

**Альтернатива (2 ноды)**: два отдельных MinIO + асинхронный sync через `mc mirror` или rclone (для MVP).

---

## 5. Доступ по домену без единого центра

### Текущая модель

- `@user:example.org` — пользователь зарегистрирован на homeserver домена `example.org`
- Discovery: `https://example.org/.well-known/federation`
- S2S: `https://example.org/federation/v1/...`
- C2S: `https://example.org/api/v1/...`

### В HA-кластере

1. **DNS**: `example.org` → A/AAAA или CNAME на Load Balancer
2. **Load Balancer**: Nginx, Traefik, HAProxy или облачный LB (AWS ALB, Cloud LB)
3. **LB** балансирует на N экземпляров приложения (server)
4. Все экземпляры читают/пишут в одну и ту же БД и S3

**«Центра» нет**: каждый домен — свой кластер. Federation строится между доменами, а не вокруг одного центрального сервера.

### Геораспределённость (опционально)

Если нужно несколько регионов для одного домена:

- **Вариант A**: один домен → один LB → несколько датацентров (global LB, GeoDNS)
- **Вариант B**: поддомены по регионам (`eu.example.org`, `us.example.org`) — тогда это разные «домены» в смысле федерации

Для федерации достаточно одного домена и одного публичного endpoint.

---

## 6. Redis (если используется)

- **Redis Sentinel**: автоматический failover
- **Redis Cluster**: шардирование
- Для MVP: один Redis с persistence (RDB/AOF) — восстановление после рестарта

---

## 7. Stateless приложение

Текущий server — по сути stateless:

- Сессия в JWT
- Данные в PostgreSQL
- Медиа в S3
- WebSocket: нужна либо sticky session на LB, либо общий pub/sub (Redis) для уведомлений между нодами

**WebSocket в кластере**:

- Sticky session (ip_hash / cookie) на LB, или
- Redis Pub/Sub: каждая нода подписана на канал, при новом сообщении — рассылка по локальным WS-клиентам

---

## 8. План внедрения (поэтапно)

### Фаза 1: Подготовка
- [ ] Stateless server: убедиться, что нет in-memory state (кроме stream Hub)
- [ ] WebSocket: переход на Redis Pub/Sub для multi-node

### Фаза 2: PostgreSQL
- [ ] Настроить streaming replication (primary + standby)
- [ ] Опционально: Patroni для автofailover

### Фаза 3: MinIO/S3
- [ ] Либо MinIO Distributed (4 ноды)
- [ ] Либо два MinIO + sync-скрипт (проще для MVP)

### Фаза 4: Load Balancer + несколько app-реплик
- [ ] Добавить LB (Traefik/Nginx) перед server
- [ ] Запустить 2+ экземпляров server
- [ ] DNS: домен → LB

### Фаза 5: Мониторинг и тесты
- [ ] Health checks, алерты
- [ ] Тесты failover (остановка нод, переключение Primary)

---

## 9. Реализация (deploy/main)

Использование:

```bash
# С репликой PostgreSQL и Traefik (профиль ha)
docker compose -p main -f deploy/main/docker-compose.yml --env-file deploy/main/.env --profile ha up -d
```

Компоненты:

| Сервис | Описание |
|--------|----------|
| `postgres` | Primary с replication (wal_level=replica) |
| `postgres-replica` | Standby (профиль `ha`), pg_basebackup от primary |
| `server2` | Второй экземпляр приложения (round-robin за Traefik) |
| `traefik` | LB, порты 80/443, Let's Encrypt |

Подробно: [SETUP-MAIN.md](SETUP-MAIN.md)

**Первичный запуск с replication**: для создания пользователя `replicator` на primary нужна чистая БД. Если `postgres_data` уже существует, выполните на primary:
```sql
CREATE USER replicator WITH REPLICATION PASSWORD 'repl_password';
```
И добавьте в pg_hba.conf: `host replication replicator 0.0.0.0/0 scram-sha-256`

---

## 10. Резюме

| Компонент | Решение | Результат |
|-----------|---------|-----------|
| App | Несколько реплик за LB | Нет SPOF на уровне приложения |
| PostgreSQL | Streaming replication / Patroni | Отказоустойчивость БД |
| S3/MinIO | Distributed MinIO или sync | Отказоустойчивость медиа |
| Домен | DNS → LB → реплики | Сохранён доменный доступ |
| Федерация | Без изменений | Каждый домен — независимый кластер |

Единого центрального сервера нет: каждый домен обслуживается своим кластером, федерация — peer-to-peer между доменами.
