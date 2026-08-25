# Корпоративный DNS через туннель (mode-following)

Решение позволяет клиентскому OpenWrt-роутеру (sing-box + dnsmasq) резолвить
корпоративные домены через корпоративный DNS-сервер, при этом **путь запроса
следует за активным режимом селектора** — это не ломает ни awg-туннель, ни
proxy-режим olcrtc.

## Как это работает

```
dnsmasq (LAN-клиенты)
   └─ корп-домены → server=/CORP-DOMAIN/127.0.0.1#5353
        └─ sing-box dns-in (127.0.0.1:5353) → hijack-dns
             └─ corp-dns server (TCP CORP_DNS_SERVER:53, detour: route-select)
                  ├─ режим awg:  TCP-DNS уходит в туннель → серверный dnsmasq →
                  │              sing-box сервера → fakeip (клиент не знает корп-DNS)
                  └─ режим proxy: TCP-DNS идёт через SOCKS → корп-DNS отвечает сам
```

Ключевые элементы:

- **`corp-dns`** — DNS-сервер sing-box типа `tcp` на `CORP_DNS_SERVER:53`
  с **`detour: route-select`**, а НЕ с туннельным outbound'ом напрямую.
- **`route-select` default = туннельный outbound** — после рестарта sing-box
  селектор не «уезжает» в `direct` (это ломало корп-доступ).
- **dnsmasq** форвардит только корп-домены в `127.0.0.1#5353` (dns-in),
  остальное резолвит как обычно; для корп-доменов задан `rebind_domain`.

### Почему detour: route-select, а не туннель напрямую

Если привязать `corp-dns` к туннельному outbound'у напрямую (`detour: awg`),
то в proxy-режиме olcrtc DNS-запросы пойдут в обход SOCKS-туннеля и корп-ресурсы
из proxy-режима перестанут резолвиться. `route-select` держит DNS в одном
потоке с данными — в каждом режиме запрос идёт правильным путём.

## Клиент: установка

Требования: sing-box (tproxy), шаблон `deploy/openwrt/sing-box-config.json`
(содержит `dns-in` 127.0.0.1:5353 и правило `hijack-dns`), dnsmasq, UCI.

```sh
CORP_DNS_SERVER=10.0.0.5 CORP_DOMAINS="corp.example intranet.example" sh deploy/openwrt/setup-tunnel-corp-dns.sh
```

Что делает скрипт (с бэкапами и автооткатом):

1. Бэкапит `/etc/sing-box/config.json` и `/etc/config/dhcp`, пишет скрипт
   отката в `/tmp/restore-corp-dns-<TS>.sh`.
2. Добавляет в sing-box: сервер `corp-dns` (TCP, `detour: route-select`),
   правило резолва корп-доменов, меняет default селектора на туннель.
3. Прописывает в dnsmasq `server=/DOMAIN/127.0.0.1#5353` и `rebind_domain`.
4. Рестартует сервисы.
5. Watchdog (по умолчанию 45 с): проверяет, что корп-домен резолвится и
   публичный DNS отвечает; иначе — автооткат (`ROLLBACK_DONE`).

Переменные окружения — в шапке скрипта (`CORP_DNS_SERVER`, `CORP_DOMAINS`,
`TUNNEL_IFACE`, `SELECTOR_DEFAULT`, `WATCHDOG_SEC`, `FORCE` и др.).
Для фонового запуска: `setsid sh setup-tunnel-corp-dns.sh >/tmp/setup-corp-dns.out 2>&1 </dev/null &`
и следить за маркерами `STAGE_OK` / `ROLLBACK_DONE` в логе.

## Сервер: авторизация туннельных peer'ов

Корп-трафик из туннеля разрешён только для списка peer'ов. При добавлении
нового клиента добавьте его туннельный IP в оба ACL (fw4 подхватит после
`fw4 reload`):

`/etc/nftables.d/10-vpn-forward.nft` (forward из туннеля в корп. интерфейс):

```
iifname "TUNNEL_IF" oifname "CORP_IF" ip saddr != { PEER_1, PEER_2, NEW_PEER } drop
```

`/etc/nftables.d/15-corp-acl.nft` (prerouting, доступ к корп. подсети):

```
ip daddr CORP_SUBNET ip saddr != { PEER_1, PEER_2, NEW_PEER } drop
```

Плюс на сервере желателен перехват DNS из туннеля (DNAT 53 → серверный dnsmasq →
sing-box fakeip): тогда клиент резолвит корп-имена, вообще не зная корп-DNS.

## Проверка end-to-end

- `nslookup CORP-DOMAIN 127.0.0.1` → ответ (в туннельном режиме — fakeip 198.18.x.x)
- `curl -I https://CORP-DOMAIN/` → 200
- `curl -I https://ya.ru/` → доступен (публичный интернет жив)
- на сервере: `conntrack -L | grep CORP_DNS_SERVER` — запросы из туннеля
  перехватываются серверным dnsmasq

## Watchdog и откат

Паттерн «примени → проверь в окне → откат»: скрипт сам восстанавливает бэкапы
и рестартует сервисы при неудаче. Ручной откат: `sh /tmp/restore-corp-dns-*.sh`.
Бэкапы: `config.json.bak-*`, `dhcp.bak-*`.

## Конфиденциальность

Все реальные значения (IP корп-DNS, домены, подсети) выносятся в переменные
окружения и плейсхолдеры — в репозиторий они не попадают. Не коммитьте файлы
с реальными данными: черновики и диагностика закрыты в `.gitignore`
(`/wd_*.sh`, `/diag_*.sh`, `/fix_*.sh`, `/plan-*.md`, `/singbox-*.json`,
`*.router.conf`). Если реальные значения уже попали в историю — перепишите её
до пуша в публичный репозиторий:

- `scripts/scrub-history.sh` — одноразовый скраб истории (`git filter-repo`,
  правила замен в локальном `.scrub-rules.txt`): mirror-клон + scrub + проверка.
  Запуск — вручную, перед merge develop→main; после — force-push из mirror.
- `.githooks/pre-push` — автоматический гард на каждый push: сверяет пушимые
  коммиты с `.scrub-rules.txt` и блокирует push при утечке паттерна.

## Troubleshooting

| Симптом | Причина | Решение |
|---|---|---|
| корп-хост: RST через ~5 с | ACL сервера не включает peer | добавить IP в 10-vpn-forward.nft / 15-corp-acl.nft, fw4 reload |
| корп-домен: DNS timeout | DNS-путь сломан | проверить dns-in/hijack-dns, nslookup 127.0.0.1 |
| после рестарта нет корп-доступа | селектор сброшен в default | default = туннельный outbound (делает скрипт) |
| UDP-DNS не работает в proxy | WebRTC-туннель не тянет UDP | corp-dns только по TCP |
| корп-домен резолвится в публичный IP | dnsmasq не форвардит в dns-in | server=/DOMAIN/127.0.0.1#5353 + rebind_domain |
