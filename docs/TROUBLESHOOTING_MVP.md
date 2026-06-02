# Troubleshooting MVP

## Positionierung

Mimir soll im MVP **kein allgemeiner Infra-Autopilot** sein.

Mimir soll im MVP ein:

**Secure AI Troubleshooting Copilot for Infrastructure**

sein.

Fokus:

- SSH
- Docker / Docker Compose
- Kubernetes
- Networking
- API Health / Debugging
- erklärbare, auditierbare Diagnose

Nicht Fokus:

- Provisioning
- Terraform-/OpenTofu-Codegenerierung
- Ansible-Automation
- mutierende Remediation

## Kernproblem

Infra-Troubleshooting ist oft:

- verteilt über viele Systeme
- log- und kontextlastig
- zeitkritisch
- schwer reproduzierbar
- riskant, wenn AI zu viel autonom darf

Mimir soll genau dort helfen:

- Discovery
- Diagnose
- Erklärung
- nächster sicherer Schritt
- Audit

## MVP-Claim

**The safest way to use AI for infrastructure troubleshooting.**

## Zielgruppe

- DevOps Engineers
- Platform Engineers
- SREs
- Sysadmins
- Infra-nahe Engineers mit SSH-, Network-, Container- und API-Fokus

## Must-Have Features

### 1. Discovery-First Troubleshooting

Die App muss echte Zielobjekte read-only discovern können.

MVP-relevant:

- Kubernetes Namespaces
- Kubernetes Pods
- Kubernetes Ressourcen
- Docker Container
- Docker Compose Services

### 2. Guardrailed AI

Die AI darf:

- Output erklären
- den nächsten sicheren Schritt vorschlagen
- read-only Tools wählen

Die AI darf im MVP nicht:

- mutieren
- remediaten
- deployen
- löschen

### 3. SSH / Terminal Troubleshooting

Die App muss für echte Terminal-/SSH-Sessions nutzbar sein.

MVP-relevant:

- Output einordnen
- sichere Folge-Schritte vorschlagen
- Discovery-Ergebnisse mit Troubleshooting kombinieren

### 4. Troubleshooting Workflows

Die App braucht wiederverwendbare Diagnoseabläufe.

MVP-relevant:

- Host Basic Triage
- Docker Compose Debug
- Kubernetes Pod Triage
- Network/API Health Check

### 5. Auditability

Die App muss nachvollziehbar machen:

- welcher Kontext an AI ging
- was redigiert wurde
- welche Discovery-Ergebnisse verwendet wurden
- welches Tool gewählt wurde
- warum etwas erlaubt oder blockiert wurde

## Workflows vs Playbooks

Die Begriffe sind nah verwandt, aber nicht identisch.

### Workflow

Ein Workflow ist die technische Ablaufdefinition.

Beispiele:

- Step 1: Discovery
- Step 2: Logs
- Step 3: Explain
- Step 4: Suggest next step

### Playbook

Ein Playbook ist ein benannter, kuratierter, wiederverwendbarer Troubleshooting-Ablauf für einen konkreten Fall.

Beispiele:

- `Docker Compose Debug`
- `K8s Pod Triage`
- `Host Incident Triage`

Praktisch im MVP:

- **Playbooks sind gespeicherte, produktisierte Workflows**

Also:

- Workflow = Engine-/Ablaufmodell
- Playbook = nutzerseitig benannter Standardablauf

## Was Nicht Zum MVP Gehört

- autonome Fixes
- AI-Remediation
- mutierende Infrastrukturaktionen
- breite Provisioning-Automation
- zu viel allgemeine IDE-/Coding-Assistance
- “wir bauen Claude/Warp nach”

## Launch-Demo

### Demo 1: Docker Compose Incident

1. Compose-Services discovern
2. Service auswählen
3. Logs ziehen
4. AI erklärt die Lage
5. AI schlägt den nächsten read-only Schritt vor
6. alles ist geloggt

### Demo 2: Kubernetes Pod Triage

1. Namespaces discovern
2. Pods discovern
3. Pod-Logs oder Describe
4. AI fasst Problem und Hypothesen zusammen
5. nächster sicherer Schritt

### Demo 3: Remote Host Triage

1. SSH-Session öffnen
2. Services / Ports / Ressourcen prüfen
3. AI erklärt das Bild
4. AI schlägt sicheren nächsten Schritt vor

## MVP Success Criteria

Der MVP ist gut genug, wenn:

1. Discovery sichtbar und nutzbar ist
2. read-only Troubleshooting-Flows wirklich Zeit sparen
3. AI-Aktionen auditierbar und erklärbar sind
4. Guardrails als echter Differenziator spürbar sind
5. die App in Incidents vertrauenswürdiger wirkt als generische Chat-Tools
