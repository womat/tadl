# tadl – Deutsche Kurzfassung

🇬🇧 [Full documentation in English](README.md)

<p align="center">
  <img src="docs/screenshots/web-ui.png" width="640" alt="Weboberfläche von tadl mit vier Temperaturen, den Ausgängen und dem Zustand des DL-Bus">
</p>

**tadl liest den DL-Bus eines Heizungsreglers UVR42 oder UVR31 von Technische Alternative auf einem
Raspberry Pi und zeigt die Temperaturen live an.**

Die Solar- und Heizungsregler UVR42 und UVR31 von [Technische Alternative](https://www.ta.co.at/)
senden ihre Temperaturen und Ausgangszustände über den **DL-Bus**, die Datenleitung für die
Datenlogger von TA. Über einen Optokoppler an einen GPIO-Pin eines Raspberry Pi angeschlossen,
macht tadl daraus Messwerte:

- die **Temperaturen** aller Fühler und den **Zustand der Ausgänge** (Pumpen, Ventile),
- per **MQTT** bei jeder deutlichen Änderung und in einem festen Intervall veröffentlicht, z. B. für
  Node-RED, Home Assistant oder ioBroker,
- live auf einer **eingebauten Webseite**, mit eigenen Fühlernamen, einem Balken pro Temperatur und
  dem Verlauf der letzten 15 Minuten,
- über eine **HTTPS-REST-API** mit API-Key für Skripte und Monitoring.

Bitrate und Polarität der Leitung erkennt tadl selbst; ein invertierender Optokoppler braucht keine
Einstellung. Keine Cloud, keine Datenbank: ein einzelnes Programm und eine YAML-Datei.

## In fünf Schritten

1. **Herunterladen:** Das Archiv für deinen Pi gibt es unter
   [Releases](https://github.com/womat/tadl/releases/latest): `armv6` für Pi 1 und Zero,
   `armv7` für 32-Bit-Systeme, `arm64` für 64-Bit-Systeme.
2. **Installieren:** System-User `tadl` anlegen, Programm und `config.yaml` nach `/opt/tadl`
   kopieren, Zertifikat erzeugen.
3. **Konfigurieren:** `env: prod`, API-Key, Reglertyp (`uvr42` oder `uvr31`), GPIO-Pin und
   MQTT-Broker; optional Namen und Bereiche der Fühler für die Webseite (`datalogger.sensors`).
4. **Anschließen:** Der DL-Bus führt **12 V** – nie direkt an einen GPIO-Pin, die GPIOs vertragen
   höchstens 3,3 V. Die Datenleitung über einen Vorwiderstand an die LED eines **Optokopplers**
   (z. B. PC817), dessen Transistor den GPIO-Pin gegen GND zieht, mit Pull-up auf 3,3 V; siehe
   [Wiring](README.md#wiring):

   <img src="docs/wiring-optocoupler.svg" width="640" alt="Schaltbild: DL-Leitung über D1 (1N4148) und R1 (2,2 kΩ) an die LED des PC817, dessen Transistor GPIO4 gegen GND zieht, Pull-up 10 kΩ auf 3V3">
5. **Starten:** als systemd-Dienst, dann `https://<dein-pi>:8443/` im Browser öffnen.

Die genauen Befehle stehen im [Quick start](README.md#quick-start), alle Einstellungen unter
[Configuration](README.md#configuration). Ohne Regler lässt sich tadl mit dem Emulator `dlbussim`
testen, der in jedem Release-Archiv mitkommt, siehe [Testing without a controller](README.md#testing-without-a-controller).

## Lizenz

MIT, siehe [`LICENSE`](LICENSE).
