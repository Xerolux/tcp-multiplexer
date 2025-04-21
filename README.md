# tcp-multiplexer

TCP-Multiplexer ist ein leistungsstarkes Tool, das mehrere Client-Verbindungen über eine einzelne TCP-Verbindung zu einem Zielserver bündelt. Es ist ideal für Szenarien, in denen der Zielserver nur eine begrenzte Anzahl gleichzeitiger TCP-Verbindungen unterstützt.

Ein typischer Anwendungsfall ist die Verbindung mehrerer Modbus/TCP-Clients mit Solarwechselrichtern, die oft nur eine einzelne TCP-Verbindung unterstützen.

## Inhaltsverzeichnis

- [Architektur](#architektur)
- [Funktionsweise](#funktionsweise)
- [Unterstützte Protokolle](#unterstützte-anwendungsprotokolle)
- [Installation](#installation)
- [Konfiguration](#konfiguration)
- [Systemd-Service](#systemd-service)
- [Verwendungsbeispiele](#verwendungsbeispiele)
- [Performance-Optimierung](#performance-optimierung)
- [Fehlerbehebung](#fehlerbehebung)
- [Mitwirken](#mitwirken)
- [Lizenz](#lizenz)

## Architektur

```
┌──────────┐
│ ┌────────┴──┐      ┌─────────────────┐      ┌───────────────┐
│ │           ├─────►│                 │      │               │
│ │ client(s) │      │ tcp-multiplexer ├─────►│ target server │
└─┤           ├─────►│                 │      │               │
  └───────────┘      └─────────────────┘      └───────────────┘


─────► TCP connection

drawn by https://asciiflow.com/
```

Im Gegensatz zu einem Reverse-Proxy wird die TCP-Verbindung zwischen `tcp-multiplexer` und dem Zielserver für alle Client-Verbindungen wiederverwendet. Die neue Implementierung unterstützt jetzt auch einen Connection-Pool für höhere Durchsatzraten.

## Funktionsweise

Der TCP-Multiplexer verwendet einen Anfrage-Antwort-Muster, um die Kommunikation zu koordinieren:

1. Client stellt eine TCP-Verbindung zum Multiplexer her
2. Multiplexer empfängt Nachrichten vom Client
3. Multiplexer leitet die Anfrage an den Zielserver weiter
4. Multiplexer empfängt die Antwort vom Zielserver
5. Multiplexer leitet die Antwort zurück an den Client

Die verbesserte Version bietet:
- Connection-Pooling für höheren Durchsatz
- Automatische Wiederherstellung fehlgeschlagener Verbindungen
- Gesundheitsüberwachung für Verbindungen
- Konfigurierbare Timeouts und Puffergrößen

## Unterstützte Anwendungsprotokolle

TCP-Multiplexer unterstützt verschiedene Anwendungsprotokolle:

1. **echo**: Zeilenumbruch-terminierte Nachrichten (`\n`)
2. **http**: HTTP/1.1 ohne HTTPS oder WebSocket
3. **iso8583**: ISO-8583-Nachrichten mit 2-Byte-Header für die Längenangabe
4. **modbus**: Modbus/TCP-Protokoll
5. **mpu**: MPU Switch Format für ISO-8583

```bash
$ ./tcp-multiplexer list
* iso8583
* echo
* http
* modbus
* mpu

usage for example: ./tcp-multiplexer server -p echo
```

## Installation

### Option 1: Vorkompilierte Binärdateien

Lade die neueste Version von der [Releases-Seite](https://github.com/Xerolux/tcp-multiplexer/releases) für dein Betriebssystem und deine Architektur herunter:

```bash
# Beispiel für Linux amd64
wget https://github.com/Xerolux/tcp-multiplexer/releases/latest/download/tcp-multiplexer_linux_amd64.tar.gz
tar -xzf tcp-multiplexer_linux_amd64.tar.gz
chmod +x tcp-multiplexer
sudo mv tcp-multiplexer /usr/local/bin/
```

### Option 2: Mit Go installieren

```bash
go install github.com/Xerolux/tcp-multiplexer@latest
```

Oder aus dem Quellcode kompilieren:

```bash
git clone https://github.com/Xerolux/tcp-multiplexer.git
cd tcp-multiplexer
make build
```

### Option 3: Docker-Container

```bash
# Mit Docker Hub
docker pull xerolux/tcp-multiplexer:latest

# Mit GitHub Container Registry
docker pull ghcr.io/xerolux/tcp-multiplexer:latest
```

## Konfiguration

### Grundlegende Konfiguration

```bash
tcp-multiplexer server -p <protokoll> -t <ziel-server> -l <port>
```

| Parameter | Beschreibung | Standardwert |
|-----------|--------------|--------------|
| `-p, --applicationProtocol` | Zu verwendendes Protokoll (echo/http/iso8583/modbus/mpu) | echo |
| `-t, --targetServer` | Adresse des Zielservers im Format host:port | 127.0.0.1:1234 |
| `-l, --listen` | Lokaler Port, auf dem der Multiplexer lauscht | 8000 |
| `--timeout` | Timeout für Verbindungen in Sekunden | 60 |
| `--delay` | Verzögerung nach dem Verbindungsaufbau | 0 |
| `-v, --verbose` | Ausführliche Protokollierung aktivieren | false |
| `-d, --debug` | Debug-Protokollierung aktivieren | false |

### Erweiterte Konfiguration

Die verbesserte Version unterstützt zusätzliche Parameter:

| Parameter | Beschreibung | Standardwert |
|-----------|--------------|--------------|
| `--max-connections` | Maximale Anzahl gleichzeitiger Verbindungen zum Zielserver | 1 |
| `--reconnect-backoff` | Anfängliche Wartezeit zwischen Verbindungsversuchen | 1s |
| `--health-check-interval` | Intervall für Verbindungs-Gesundheitsüberprüfungen | 30s |
| `--queue-size` | Größe der Anfrage-Warteschlange | 32 |

## Systemd-Service

Für einen automatischen Start des TCP-Multiplexers beim Systemstart auf Linux-Systemen mit systemd kannst du einen systemd-Service einrichten.

### Service-Datei erstellen

Erstelle die Datei `/etc/systemd/system/tcp-multiplexer.service`:

```bash
sudo nano /etc/systemd/system/tcp-multiplexer.service
```

Füge den folgenden Inhalt ein (passe die Parameter an deine Umgebung an):

```ini
[Unit]
Description=TCP Multiplexer Service
Documentation=https://github.com/Xerolux/tcp-multiplexer
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=tcp-multiplexer
Group=tcp-multiplexer
ExecStart=/usr/local/bin/tcp-multiplexer server -p modbus -t 192.168.1.22:1502 -l 5020 --max-connections 2
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tcp-multiplexer
# Hardening options
ProtectSystem=full
PrivateTmp=true
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

### Benutzer für den Service erstellen

```bash
sudo useradd -r -s /bin/false tcp-multiplexer
```

### Service aktivieren und starten

```bash
# Lade die systemd-Konfiguration neu
sudo systemctl daemon-reload

# Aktiviere den Service für automatischen Start beim Booten
sudo systemctl enable tcp-multiplexer.service

# Starte den Service
sudo systemctl start tcp-multiplexer.service

# Überprüfe den Status
sudo systemctl status tcp-multiplexer.service
```

### Service verwalten

```bash
# Service stoppen
sudo systemctl stop tcp-multiplexer.service

# Service neustarten
sudo systemctl restart tcp-multiplexer.service

# Logs anzeigen
sudo journalctl -u tcp-multiplexer.service

# Logs fortlaufend anzeigen
sudo journalctl -u tcp-multiplexer.service -f
```

### Konfiguration anpassen

Wenn du die Konfiguration des Services ändern möchtest, bearbeite die Service-Datei und starte den Service neu:

```bash
sudo nano /etc/systemd/system/tcp-multiplexer.service
sudo systemctl daemon-reload
sudo systemctl restart tcp-multiplexer.service
```

## Verwendungsbeispiele

### Beispiel: Modbus-Proxy für Solarwechselrichter

```bash
# Über Kommandozeile
tcp-multiplexer server -p modbus -t 192.168.1.22:1502 -l 5020 -v --max-connections 2

# Mit Docker
docker run -p 5020:5020 ghcr.io/xerolux/tcp-multiplexer server -t 192.168.1.22:1502 -l 5020 -p modbus -v
```

### Beispiel: HTTP-Proxy mit Connection-Pool

```bash
tcp-multiplexer server -p http -t backend.example.com:80 -l 8080 --max-connections 5 --health-check-interval 60s
```

### Docker Compose

Verwende die enthaltene `compose.yml`-Datei als Vorlage:

```yaml
services:
  modbus-proxy:
    image: ghcr.io/xerolux/tcp-multiplexer
    container_name: modbus_proxy
    ports:
      - "5020:5020"
    command: [ "server", "-t", "192.168.1.22:1502", "-l", "5020", "-p", "modbus", "-v", "--max-connections", "2" ]
    restart: unless-stopped
```

## Tests durchführen

### Echo-Server-Test

1. Start eines Echo-Servers (lauscht auf Port 1234)

```bash
$ go run example/echo-server/main.go
1: 127.0.0.1:1234 <-> 127.0.0.1:58088
```

2. Start des TCP-Multiplexers (lauscht auf Port 8000)

```bash
$ ./tcp-multiplexer server -p echo -t 127.0.0.1:1234 -l 8000
INFO[2021-05-09T02:06:40+08:00] creating target connection
INFO[2021-05-09T02:06:40+08:00] new target connection: 127.0.0.1:58088 <-> 127.0.0.1:1234
INFO[2021-05-09T02:07:57+08:00] #1: 127.0.0.1:58342 <-> 127.0.0.1:8000
```

3. Client-Test

```bash
$ nc 127.0.0.1 8000
kkk
kkk
^C
$ nc 127.0.0.1 8000
mmm
mmm
```

## Performance-Optimierung

Um die bestmögliche Leistung zu erzielen:

1. **Connection-Pool anpassen**: Erhöhe `--max-connections` für höheren Durchsatz, insbesondere bei vielen gleichzeitigen Clients.

2. **Gesundheitsüberwachung konfigurieren**: Passe `--health-check-interval` an die Stabilität deines Netzwerks an.

3. **Warteschlangengrößen optimieren**: Erhöhe bei Bedarf `--queue-size` für Anwendungen mit Burst-Traffic.

4. **Verbindungs-Timeouts**: Stelle sicher, dass `--timeout` ausreichend lang ist, um legitime Operationen abzuschließen, aber kurz genug, um hängende Verbindungen zu erkennen.

5. **Container-Limits**: Bei Verwendung mit Docker solltest du angemessene CPU- und Speicherbeschränkungen setzen.

## Fehlerbehebung

### Häufige Probleme

1. **Verbindungsfehler zum Zielserver**:
   - Überprüfe die Netzwerkverbindung zum Zielserver
   - Vergewissere dich, dass der Zielserver auf dem angegebenen Port lauscht
   - Erhöhe `--verbose` für detailliertere Logs

2. **Timeouts während der Kommunikation**:
   - Erhöhe den Timeout-Wert mit `--timeout`
   - Überprüfe die Netzwerklatenz zum Zielserver

3. **Hohe CPU- oder Speicherauslastung**:
   - Reduziere `--max-connections` oder `--queue-size`
   - Überprüfe, ob der Zielserver mit dem Verkehrsvolumen umgehen kann

4. **Inkonsistente Antworten**:
   - Stelle sicher, dass das richtige Protokoll mit `-p` ausgewählt wurde
   - Überprüfe, ob der Zielserver das erwartete Protokoll unterstützt

### Logging und Debugging

Aktiviere ausführliche Protokollierung für eine bessere Fehlerbehebung:

```bash
tcp-multiplexer server -p modbus -t 192.168.1.22:1502 -l 5020 -v -d
```

## Mitwirken

Beiträge sind willkommen! Bitte öffne Issues oder Pull Requests für Verbesserungen oder Fehlerbehebungen.

## Lizenz

Dieses Projekt steht unter der MIT-Lizenz - siehe die [LICENSE](LICENSE) Datei für Details.
