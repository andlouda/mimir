# ADR-0012: Portabler Export/Import des Credential-Vaults

## Status

**Vorgeschlagen / AUFGESCHOBEN (spaeteres Feature)** — bewusst noch nicht
implementiert. Dieses ADR haelt die Designentscheidung fest, damit die Migration
zwischen Geraeten spaeter kontrolliert umgesetzt werden kann, statt dass Nutzer
die geraetegebundene Datei kopieren (was nicht funktioniert, siehe unten).

## Kontext

Credentials (SSH-Passwoerter, AI-API-Key) liegen entweder im OS-Keyring oder im
verschluesselten Datei-Fallback (`ssh_secrets.enc`). Beide sind **bewusst nicht
portabel**:

- **Keyring:** Schluessel ist ans OS-Login/Konto gebunden, kein exfiltrierbarer
  Blob — per Design nicht uebertragbar.
- **Datei-Fallback:** Der KEK mischt ein **Maschinen-Secret** ein
  (`deriveKEK = Argon2id(Master-Passwort ‖ Maschinen-Secret, salt)`). Eine auf
  ein anderes Geraet kopierte `ssh_secrets.enc` laesst sich daher **selbst mit
  korrektem Master-Passwort nicht entschluesseln**. Das ist die zweite
  Schutzschicht (gestohlene/gesyncte/gebackupte Datei ist nutzlos) — kollidiert
  aber mit dem berechtigten Wunsch, Credentials auf einen anderen Server/Rechner
  zu migrieren.

Heutiger Migrationspfad: auf dem Zielgeraet **neu eingeben** (Profile selbst
liegen separat und sind ohnehin portabel). Fuer mehr als eine Handvoll Profile
ist das mühsam.

## Entscheidung (vorgeschlagen, spaeter umzusetzen)

Ein **expliziter, opt-in Export/Import** eines portablen Bundles — nicht das
Kopieren der geraetegebundenen Datei.

- `ExportPortableVault(masterPassword) -> blob` : entschluesselt den lokalen
  Vault (Store muss unlocked sein) und re-verschluesselt alle Eintraege in ein
  **portables Bundle**, dessen KEK **nur** aus dem (ggf. neu/staerker
  abgefragten) Passwort abgeleitet wird — **ohne Maschinen-Secret**.
- `ImportPortableVault(blob, masterPassword)` : entschluesselt das Bundle
  passwort-only und schreibt die Eintraege in den **lokalen** Store des
  Zielgeraets — d. h. sie werden dort wieder ans lokale Backend gebunden:
  - Keyring-Backend vorhanden → Eintraege landen im OS-Keyring.
  - Datei-Backend → Eintraege werden unter dem **lokalen** Maschinen-Secret neu
    gewrappt.

So bleibt die Alltagsspeicherung geraetegebunden (sicher), und Migration ist
trotzdem moeglich — bewusst, kontrolliert, mit Warnhinweis.

### Format-Skizze

Das v2-Format und das Keyslot-Modell tragen das bereits: ein portables Bundle
ist ein v2-aehnliches Objekt mit einem Passwort-Slot, dessen KDF als
**`portable: true`** markiert ist. `deriveKEK` ueberspringt dann das
Maschinen-Secret:

```go
func deriveKEK(masterPassword string, kdf kdfParams) []byte {
    input := []byte(masterPassword)
    if !kdf.Portable {           // <-- neues Feld, default false
        input = append(append(input, 0x00), machineSecret()...)
    }
    return argon2.IDKey(input, kdf.Salt, kdf.Time, kdf.Memory, kdf.Threads, kdf.KeyLen)
}
```

Das Bundle traegt eine eigene Kennung (`"type":"mimir-portable-vault"`,
Versionsfeld) und einen frischen, hoeher parametrisierten Argon2id-Salt
(siehe Sicherheit).

## Sicherheits-Ueberlegungen

- **Offline-Brute-Force:** Ein portables Bundle ist genau das, was wir bei der
  Alltags-Datei vermeiden — ein mitnehmbarer Blob. Seine Sicherheit haengt
  **allein** am Passwort. Daher:
  - **Starkes** Export-Passwort erzwingen (deutlich strenger als das normale
    Master-Passwort-Minimum), Passphrase-Empfehlung.
  - **Haertere KDF-Parameter** fuer das Bundle (mehr Argon2-Zeit/Speicher) als
    fuer die geraetegebundene Datei.
  - Klarer UI-Warnhinweis beim Export: „Diese Datei ist ohne Geraetebindung —
    schuetze sie wie ein Passwort, uebertrage sie sicher, loesche sie danach."
- **Ephemerer Umgang:** Export idealerweise direkt in einen vom Nutzer
  gewaehlten Pfad (SaveFileDialog), nicht in den Config-Ordner; keine
  Zwischenkopien; nach Import zum Loeschen raten.
- **Kein Downgrade der Alltagsspeicherung:** Export/Import veraendert die lokale
  Speicherung nicht — die bleibt keyring- bzw. maschinengebunden.
- **Integritaet/AEAD:** wie gehabt AES-256-GCM mit AAD-Bindung pro Eintrag.

## Konsequenzen

### Positiv
- Sauberer, dokumentierter Migrationspfad statt „Datei kopieren" (das ohnehin
  nicht funktioniert).
- Alltags-Datei bleibt geraetegebunden → starke At-Rest-Eigenschaften bleiben
  erhalten.
- Import respektiert das Ziel-Backend (landet z. B. direkt im OS-Keyring).
- Baut vollstaendig auf vorhandenem v2-/Keyslot-Code auf (nur `Portable`-Flag +
  zwei Methoden + Bindings + UI).

### Negativ
- Erzeugt bewusst einen passwort-only Blob → Offline-Angriffsflaeche, muss durch
  starke KDF + Nutzerfuehrung abgefedert werden.
- Zusaetzliche UI (Export-/Import-Dialog, Passwortabfrage, Warnungen).
- Versionierung/Kompatibilitaet des Bundle-Formats ist zu pflegen.

## Alternativen

| Alternative | Bewertung |
|-------------|-----------|
| **Datei direkt kopieren** | Funktioniert nicht (Maschinen-Secret); kein gangbarer Weg. |
| **Neu eingeben auf Zielgeraet** | Heutiger Default; fuer wenige Profile ok, skaliert schlecht. Bleibt als Fallback. |
| **Maschinen-Secret generell abschaltbar (Portable-Modus dauerhaft)** | Schwaecht die Alltags-Datei dauerhaft; abgelehnt — Portabilitaet soll opt-in pro Export sein, nicht globaler Sicherheits-Downgrade. |
| **Sync ueber Cloud/Server** | Eigenes, groesseres Vertrauens-/Threat-Model; ausserhalb des local-first-Prinzips. Nicht jetzt. |

## Offene Fragen (vor Umsetzung)
1. Mindest-Passwortstaerke und Argon2-Parameter speziell fuer das Bundle.
2. Import-Konfliktverhalten: vorhandene Eintraege ueberschreiben, ueberspringen
   oder mergen (pro Eintrags-ID)?
3. Soll der Export auch SSH-Profil-Metadaten buendeln (Komfort) oder strikt nur
   Geheimnisse (Profile sind bereits portabel)?
4. Format-Versionierung und Vorwaertskompatibilitaet.

## Bezug
- Setzt auf das v2-/Keyslot-Format und `deriveKEK` aus dem Credential-Refactor
  auf. Verwandt: ADR-0011 (FIDO2-Unlock) — beide erweitern denselben
  Vault-Kern.
