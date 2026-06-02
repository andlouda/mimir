# ADR-0011: FIDO2-/Hardware-Key-Unlock fuer den Credential-Store

## Status

**Vorgeschlagen / OFFEN** — Krypto und Slot-Verwaltung sind implementiert und
getestet; die physische Authenticator-Anbindung (Ceremony) ist noch nicht
entschieden. Diese Entscheidung blockiert die Wahl des Tech-Stacks (insb. die
Einfuehrung von CGO) und ist daher bewusst als offenes ADR dokumentiert.

## Kontext

Der Credential-Store (`mimir/ssh/secrets.go`) speichert SSH-Passwoerter und den
AI-API-Key. Wenn kein OS-Keyring verfuegbar ist (haeufig auf headless-Linux /
ohne D-Bus), faellt er auf eine verschluesselte Datei zurueck. Diese nutzt seit
ADR-relevantem Security-Fix **Envelope-Encryption** mit **Key-Slots** (LUKS-artig):

- Ein zufaelliger 256-bit **DEK** verschluesselt alle Eintraege (AES-256-GCM,
  AAD = Eintrags-ID).
- Der DEK wird pro Slot mit einem **KEK** gewrappt. Heute existiert der
  Passwort-Slot: `KEK = Argon2id(Master-Passwort ‖ Maschinen-Secret, salt)`.
- Beliebig viele weitere Slots koennen denselben DEK entsperren.

Der Nutzer moechte einen **FIDO2-Hardware-Key (YubiKey)** als zusaetzliche
Entsperrmethode: steckt der Key, soll er genutzt werden, sonst das Passwort.

### Anforderungen

1. **Modus: Key ODER Passwort** (kein erzwungenes 2FA). Das Passwort bleibt als
   Recovery-Pfad, falls der Hardware-Key verloren geht.
2. **Linux und macOS muessen zuverlaessig funktionieren** (explizite Vorgabe),
   Windows ebenfalls.
3. Der Default-Build soll **pure-Go** bleiben (das Projekt nutzt bewusst
   `modernc.org/sqlite` statt CGO-SQLite, siehe Build-Philosophie).
4. Keine Schwaechung des bestehenden Modells: ein FIDO-Slot darf den DEK nur
   zusaetzlich wrappen, nie das Passwort-Recovery aushebeln.

### Was bereits implementiert und getestet ist

Die Crypto-Grenze ist absichtlich so geschnitten, dass die Ceremony austauschbar
bleibt:

- `AddFIDOSlot(credentialID, rpID, challengeSalt, hmacSecret, label)` — wrappt
  den DEK unter `KEK = HKDF-SHA256(hmacSecret)` und haengt einen `fido2`-Slot an.
- `FIDOChallenges() []FIDOChallenge` — liefert `credentialID`, `rpId`,
  `challengeSalt` der enrollten Slots fuer die Assertion.
- `UnlockFIDO(hmacSecret)` — leitet denselben KEK ab und unwrappt den DEK.

Damit ist der einzige offene Teil: **Wie kommt `hmacSecret` physisch vom Key?**
Das `hmacSecret` ist das Ergebnis der FIDO2-`hmac-secret`-Extension (CTAP2) bzw.
der WebAuthn-`prf`-Extension — ein deterministisches, geraetegebundenes,
per-Credential-Geheimnis, das nur nach User-Presence (Touch) und optional
User-Verification (PIN) herausgegeben wird. Exakt dieses Primitiv nutzen auch
`systemd-cryptenroll`, LUKS-FIDO2 und `age-plugin-fido2-hmac`.

## Entscheidung (zu treffen)

Welcher Mechanismus holt das `hmac-secret`/PRF-Geheimnis vom Authenticator?

Vorgeschlagen wird ein **Provider-Interface**, damit die Entscheidung isoliert
und spaeter erweiterbar bleibt:

```go
// FIDOAuthenticator abstrahiert den physischen Authenticator-Zugriff.
type FIDOAuthenticator interface {
    // Enroll erzeugt ein Credential mit hmac-secret/PRF und liefert
    // credentialID, rpID, challengeSalt und das initiale hmacSecret.
    Enroll(ctx context.Context, label string) (FIDOEnrollment, error)
    // Assert fuehrt eine getAssertion fuer ein bekanntes Credential aus und
    // liefert das hmacSecret zum gespeicherten challengeSalt.
    Assert(ctx context.Context, ch FIDOChallenge) (hmacSecret []byte, err error)
}
```

Die `ssh.SecretStore`-API bleibt unveraendert; nur ein Provider wird vorgeschaltet.

## Optionen

### Option A — WebAuthn-PRF im WebView (frontend-getrieben, pure-Go)

Die Wails-WebView fuehrt `navigator.credentials.create/get` mit der `prf`-
Extension aus; das OS regelt Touch/PIN. Das Frontend reicht das base64-codierte
PRF-Ergebnis an die bereits existierenden Bindings (`EnrollFIDO` /
`UnlockSecretsFIDO`) weiter.

- **Pro:** keine nativen Abhaengigkeiten, kein CGO; nutzt den OS-eigenen
  FIDO-Stack; auf Windows (WebView2/Chromium) ausgereift.
- **Contra:** WebView-abhaengig. **WebKitGTK (Linux) unterstuetzt PRF praktisch
  nicht** → verletzt Anforderung 2. macOS (WKWebView) erst mit neueren
  Versionen. Braucht eine stabile, sichere `rpId`/Origin in der WebView
  (Wails-Origins wie `wails://`/`https://wails.localhost` sind hier heikel).

### Option B — Native libfido2 (CGO)

`github.com/keys-pub/go-libfido2` spricht ueber libfido2/hidapi direkt per
USB-HID mit dem Authenticator.

- **Pro:** konsistent ueber Linux/macOS, headless-faehig, voller Zugriff auf die
  hmac-secret-Extension, unabhaengig vom WebView.
- **Contra:** **fuehrt CGO ein** (verletzt Anforderung 3 fuer den Default-Build);
  libfido2 + Header muessen pro Plattform vorhanden/gebuendelt sein
  (Build-/Release-Komplexitaet). Auf **Windows 10+** ist direkter FIDO-HID-Zugriff
  fuer nicht-elevierte Prozesse gesperrt — dort muss ohnehin die OS-WebAuthn-API
  genutzt werden, libfido2 hilft auf Windows also nur eingeschraenkt.

### Option C — Hybrid: Provider-Interface, WebAuthn primaer + libfido2 hinter Build-Tag (EMPFEHLUNG)

- **WebAuthn-PRF-Provider** (pure-Go, Default-Build) fuer Windows und modernes
  macOS.
- **libfido2-Provider** hinter Build-Tag `fido2` (CGO, opt-in) fuer Linux und
  macOS, ohne den Default-Build zu brechen.
- Release-Builds fuer Linux/macOS werden mit `-tags fido2` erstellt; Windows
  bleibt pure-Go.

- **Pro:** erfuellt Anforderung 2 (Linux/Mac) **und** haelt den Default-Build
  pure-Go (Anforderung 3). CGO-Risiko ist auf getaggte Builds isoliert und
  reversibel.
- **Contra:** zwei Code-Pfade + Wartung; Release-Pipeline muss den getaggten
  Build pro Plattform erzeugen; libfido2 muss fuer Linux/macOS-Release verfuegbar
  sein.

### Option D — OS-native APIs direkt aus Go (ohne libfido2)

Windows `webauthn.dll`, macOS `AuthenticationServices`, Linux libfido2 — jeweils
direkt angebunden.

- **Pro:** maximal nativ, keine WebView-Origin-Probleme.
- **Contra:** drei plattformspezifische, schwer testbare Implementierungen;
  macOS-Anbindung braucht ObjC/CGO; hoechster Aufwand. Fuer ein pre-MVP
  unverhaeltnismaessig.

## Plattform-Eignung (PRF/hmac-secret)

| Plattform | WebAuthn-PRF im WebView | libfido2 (CGO) |
|-----------|-------------------------|----------------|
| Windows   | gut (WebView2/Chromium) | eingeschr. (HID gesperrt, braucht WebAuthn-API/Elevation) |
| macOS     | nur neuere WKWebView    | gut |
| Linux     | praktisch nicht (WebKitGTK) | gut |

→ Kein einzelner Mechanismus deckt alle drei sauber ab. Das ist der Kern, warum
Option C empfohlen wird.

## Sicherheits-Ueberlegungen

- **User Verification:** Enrollment sollte UV (PIN/Biometrie) verlangen, damit
  ein gestohlener, gesteckter Key allein nicht entsperrt. Touch (User Presence)
  ist Minimum.
- **Recovery:** Der Passwort-Slot bleibt immer erhalten (Modus „Key ODER
  Passwort"). Verlust des Keys ⇒ Entsperren via Passwort. Ein FIDO-Slot darf
  niemals der einzige Slot sein.
- **Gespeicherte Metadaten:** `credentialID`, `rpId`, `challengeSalt` liegen
  unverschluesselt in `ssh_secrets.enc`. Das ist akzeptabel — ohne den
  physischen Key liefern sie kein Geheimnis. Der `challengeSalt` ist der
  hmac-secret-Input und muss stabil gespeichert werden.
- **rpId-Bindung:** Die `rpId` muss fest und app-spezifisch sein, damit
  Assertions nicht von einer fremden Origin angefordert werden koennen
  (relevant v.a. bei der WebAuthn-Variante).
- **Resident vs. non-resident:** Non-resident (server-side) Credentials
  vermeiden Slot-Verbrauch auf dem Key; die `credentialID` muss dann gespeichert
  werden (tun wir bereits).
- **KEK-Ableitung:** `hmacSecret` ist bereits HMAC-SHA256-Output (hochentrop);
  HKDF-SHA256 mit Domain-Separation genuegt (implementiert in `deriveFIDOKEK`).

## Konsequenzen

### Positiv (bei Option C)

- Hardware-Key-Unlock auf allen drei Zielplattformen, ohne den pure-Go-Default
  aufzugeben.
- Bestehendes Keyslot-Modell wird wiederverwendet; keine weitere Formataenderung
  noetig (das v2-Format kennt `fido2`-Slots bereits).
- CGO bleibt isoliert und optional.

### Negativ

- Doppelter Ceremony-Code (WebAuthn + libfido2) und entsprechende Wartung.
- Release-Pipeline muss `-tags fido2` fuer Linux/macOS einbauen und libfido2
  bereitstellen/buendeln.
- WebAuthn-Pfad erfordert eine geklaerte, stabile `rpId`/Origin in der WebView.

## Offene Fragen (vor Annahme zu klaeren)

1. **CGO akzeptiert?** Ohne CGO ist zuverlaessiges Linux-FIDO nicht realistisch.
   Ist der getaggte CGO-Build (Option C) ok, oder soll Linux vorerst nur den
   Passwort-Fallback haben?
2. **macOS-Pfad:** WebAuthn (neuere WKWebView) oder ebenfalls libfido2?
3. **rpId/Origin** der Wails-WebView fuer den WebAuthn-Pfad final festlegen.
4. **UV-Policy:** PIN erzwingen oder Touch-only?
5. **Release-Tooling:** libfido2-Bereitstellung in CI fuer Linux/macOS.

## Aktueller Implementierungsstand

- Erledigt & getestet: Keyslot-Format, `AddFIDOSlot`, `FIDOChallenges`,
  `UnlockFIDO`, `deriveFIDOKEK`, App-Bindings `EnrollFIDO`/`ListFIDOChallenges`/
  `UnlockSecretsFIDO`, Frontend-Button (ruht bis Provider existiert).
- Offen: `FIDOAuthenticator`-Provider (Option C), Frontend-Ceremony (WebAuthn),
  Release-Tooling fuer den `fido2`-Build.

## Alternativen-Zusammenfassung

| Alternative | Grund fuer (vorlaeufige) Ablehnung |
|-------------|-------------------------------------|
| Nur Option A (WebAuthn) | Faellt auf Linux (WebKitGTK kein PRF) aus → verletzt Kernanforderung. |
| Nur Option B (libfido2) | CGO im Default-Build; auf Windows ohnehin eingeschraenkt. |
| Option D (OS-APIs direkt) | Drei schwer testbare native Pfade; unverhaeltnismaessig fuer pre-MVP. |
