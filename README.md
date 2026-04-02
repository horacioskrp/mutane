<div align="center">

<img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white"/>
<img src="https://img.shields.io/badge/PostgreSQL-16+-4169E1?style=flat-square&logo=postgresql&logoColor=white"/>
<img src="https://img.shields.io/badge/License-MIT-f6245b?style=flat-square"/>
<img src="https://img.shields.io/badge/Status-Alpha-orange?style=flat-square"/>

# Mutane CMS

**Headless CMS open source — léger, rapide, sans dépendances lourdes.**

Modèlez votre contenu, gérez vos médias, consommez votre API REST depuis n'importe quelle application.

</div>

---

## Aperçu

Mutane est un CMS headless écrit entièrement en Go. Il expose une **API REST publique** sécurisée par clé API et une **interface d'administration** web complète, le tout dans un seul binaire autonome.

- **Zéro framework** — stdlib Go + `net/http`
- **Interface admin** intégrée — HTML/CSS/JS vanilla, aucun build frontend requis
- **Base de données** — PostgreSQL, migrations automatiques au démarrage
- **Binaire unique** — déployez un seul fichier, aucune dépendance runtime

---

## Fonctionnalités

### Content Management
- ✅ **Types de contenu** dynamiques avec builder visuel de champs
- ✅ **13 types de champs** : texte, nombre, email, date, booléen, rich text, JSON, média, énumération, relation, UID, password, blocks
- ✅ **Réorganisation des champs** par glisser-déposer
- ✅ **Entrées** avec statut brouillon / publié
- ✅ **Pagination, recherche et filtres** sur tous les tableaux

### Media
- ✅ Upload de fichiers (32 MB max par fichier)
- ✅ Galerie avec prévisualisation
- ✅ Stockage local avec URL publique configurable

### Sécurité & Auth
- ✅ Authentification JWT (Bearer token + cookie de session)
- ✅ 2FA TOTP (Google Authenticator, Authy…)
- ✅ Clés API **publiques** (`mut_pub_…`) et **privées** (`mut_prv_…`)
- ✅ Rotation de clé atomique (révocation + régénération en une transaction)
- ✅ Expiration de clé configurable (30j / 90j / 6 mois / 1 an / jamais)
- ✅ Hash SHA-256 des clés (O(1), sans bcrypt pour les tokens haute entropie)

### Developer Experience
- ✅ **Hot-reload** en mode dev (air + SSE browser refresh)
- ✅ API publique versionnée `/v1/` avec pagination et enveloppe JSON standardisée
- ✅ CORS configuré
- ✅ Migrations SQL auto-appliquées au démarrage

---

## Démarrage rapide

### Prérequis

| Outil | Version minimale |
|-------|-----------------|
| Go | 1.22 |
| PostgreSQL | 14 |
| Node.js *(optionnel, pour rebuilder le CSS)* | 18 |

### 1 — Cloner

```bash
git clone https://github.com/votre-org/mutane.git
cd mutane
```

### 2 — Configurer l'environnement

```bash
cp .env.example .env
```

Éditez `.env` :

```dotenv
PORT=8080

APP_ENV=development        # active le hot-reload

DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=votre_mot_de_passe
DB_NAME=mutane
DB_SSLMODE=disable

JWT_SECRET=changez-moi-avec-une-chaine-aleatoire-min-32-chars

UPLOAD_DIR=uploads
UPLOAD_BASE_URL=http://localhost:8080/uploads
```

### 3 — Créer la base de données

```bash
psql -U postgres -c "CREATE DATABASE mutane;"
```

Les migrations s'appliquent automatiquement au premier démarrage.

### 4 — Lancer

**Mode développement (hot-reload) :**

```bash
# Installer air (une seule fois)
go install github.com/air-verse/air@latest

# Lancer
air
```

**Mode standard :**

```bash
go run ./cmd/server/
```

**Build production :**

```bash
go build -o mutane ./cmd/server/
./mutane
```

### 5 — Premier accès

Ouvrez [http://localhost:8080](http://localhost:8080) — vous serez redirigé vers la page de configuration initiale pour créer votre compte administrateur.

---

## Structure du projet

```
mutane/
├── cmd/
│   └── server/
│       └── main.go              # Point d'entrée
│
├── internal/
│   ├── api/
│   │   ├── handler.go           # Agrégation des handlers
│   │   ├── middleware.go        # Auth (Session, Bearer, APIKey), CORS, Logger
│   │   └── routes.go            # Table de routage complète
│   │
│   ├── auth/
│   │   ├── handler.go           # Login, Register, Logout, 2FA
│   │   ├── jwt.go               # Génération / validation JWT
│   │   └── totp.go              # Secret TOTP, validation
│   │
│   ├── content/
│   │   ├── model.go             # ContentType, Field, Entry + FieldType constants
│   │   ├── repository.go        # CRUD SQL (LEFT JOIN fields, ReorderFields…)
│   │   └── handler.go           # Handlers HTTP + pages admin
│   │
│   ├── apikey/
│   │   ├── model.go             # APIKey, KeyType (public/private)
│   │   ├── repository.go        # Create, Rotate, Revoke, Validate (SHA-256)
│   │   └── handler.go           # Endpoints CRUD + Rotate
│   │
│   ├── media/
│   │   ├── model.go             # Media struct
│   │   ├── repository.go        # CRUD table media
│   │   ├── storage.go           # LocalStorage (save, delete, URL)
│   │   └── handler.go           # Upload, List, Delete
│   │
│   ├── admin/
│   │   ├── handler.go           # Me, SettingsPage
│   │   └── stats.go             # Dashboard stats
│   │
│   ├── public/
│   │   └── handler.go           # GET /v1/{slug}, /v1/{slug}/{id}, /v1/media
│   │
│   ├── setup/
│   │   └── handler.go           # Onboarding initial
│   │
│   ├── devreload/
│   │   └── reload.go            # SSE hub + polling watcher (dev only)
│   │
│   ├── ctxkey/
│   │   └── ctxkey.go            # Clé de contexte partagée (userID)
│   │
│   └── database/
│       ├── connect.go           # Pool de connexion PostgreSQL
│       └── migrate.go           # Runner de migrations SQL
│
├── migrations/
│   ├── 001_users.up.sql
│   ├── 002_content_types.up.sql
│   ├── 003_entries.up.sql
│   ├── 004_media.up.sql
│   ├── 005_api_keys.up.sql
│   ├── 006_api_keys_v2.up.sql
│   └── 007_api_keys_fix.up.sql
│
├── web/
│   └── static/
│       ├── admin.html           # Interface admin (SPA vanilla JS)
│       └── login.html           # Page de connexion
│
├── uploads/                     # Fichiers uploadés (gitignored)
├── .air.toml                    # Config hot-reload (air)
├── .env.example
├── go.mod
└── go.sum
```

---

## API Reference

### Authentification

Toutes les routes `/api/*` requièrent un header `Authorization: Bearer <token>`.
Les routes `/v1/*` requièrent un header `X-API-Key: mut_pub_xxxx` ou `X-API-Key: mut_prv_xxxx`.

### Auth

| Méthode | Endpoint | Description |
|---------|----------|-------------|
| `POST` | `/api/auth/register` | Créer un compte |
| `POST` | `/api/auth/login` | Connexion → retourne `token` |
| `POST` | `/api/auth/logout` | Déconnexion (clear cookie) |
| `POST` | `/api/auth/2fa/enable` | Activer la 2FA (Bearer) |
| `POST` | `/api/auth/2fa/verify` | Vérifier le code TOTP (Bearer) |

### Content Types

| Méthode | Endpoint | Description |
|---------|----------|-------------|
| `GET` | `/api/content-types` | Liste tous les types |
| `POST` | `/api/content-types` | Créer un type |
| `GET` | `/api/content-types/{id}` | Détail + champs |
| `PUT` | `/api/content-types/{id}` | Modifier |
| `DELETE` | `/api/content-types/{id}` | Supprimer |
| `POST` | `/api/content-types/{id}/fields` | Ajouter un champ |
| `DELETE` | `/api/content-types/{id}/fields/{fid}` | Supprimer un champ |
| `PUT` | `/api/content-types/{id}/fields/reorder` | Réordonner les champs |

### Entrées

| Méthode | Endpoint | Description |
|---------|----------|-------------|
| `GET` | `/api/content-types/{typeId}/entries` | Liste les entrées |
| `POST` | `/api/content-types/{typeId}/entries` | Créer une entrée |
| `GET` | `/api/content-types/{typeId}/entries/{id}` | Détail |
| `PUT` | `/api/content-types/{typeId}/entries/{id}` | Modifier |
| `DELETE` | `/api/content-types/{typeId}/entries/{id}` | Supprimer |

### Médias

| Méthode | Endpoint | Description |
|---------|----------|-------------|
| `GET` | `/api/media` | Liste les médias |
| `POST` | `/api/media/upload` | Uploader un fichier (multipart) |
| `DELETE` | `/api/media/{id}` | Supprimer |

### Clés API

| Méthode | Endpoint | Description |
|---------|----------|-------------|
| `GET` | `/api/keys` | Liste toutes les clés (actives + révoquées) |
| `POST` | `/api/keys` | Créer une clé |
| `POST` | `/api/keys/{id}/rotate` | Rotation (révoque + régénère) |
| `DELETE` | `/api/keys/{id}` | Révoquer |

### API Publique `/v1/` — `X-API-Key` requis

| Méthode | Endpoint | Query params | Description |
|---------|----------|--------------|-------------|
| `GET` | `/v1/{slug}` | `page`, `limit` | Liste les entrées publiées |
| `GET` | `/v1/{slug}/{id}` | — | Détail d'une entrée publiée |
| `GET` | `/v1/media` | `page`, `limit` | Liste les médias |

**Exemple de réponse paginée :**

```json
{
  "data": [
    { "id": 1, "title": "Hello World", "published_at": "2025-01-15T10:00:00Z" }
  ],
  "meta": {
    "total": 42,
    "page": 1,
    "limit": 20
  }
}
```

**Exemple d'appel curl :**

```bash
curl https://votre-domaine.com/v1/articles \
  -H "X-API-Key: mut_pub_a1b2c3d4e5f6" \
  | jq .
```

---

## Clés API — Types et sécurité

| Type | Préfixe | Usage recommandé |
|------|---------|-----------------|
| **Publique** | `mut_pub_…` | Applications front-end, navigateur, mobile |
| **Privée** | `mut_prv_…` | Server-to-server, scripts back-end, CI/CD |

> ⚠️ **Ne jamais exposer une clé privée côté client.** Elle doit rester dans les variables d'environnement de votre serveur.

Les clés sont stockées sous forme de hash **SHA-256** (jamais en clair). La valeur complète n'est affichée **qu'une seule fois** à la création ou après une rotation.

---

## Hot-reload (développement)

Avec `APP_ENV=development` et `air` :

```
Sauvegarde d'un .go    → air recompile → serveur redémarre → navigateur recharge
Sauvegarde admin.html  → watcher SSE détecte → navigateur recharge (~350ms)
```

**En production** (`APP_ENV` absent ou différent de `development`) : aucun watcher, aucune route `/dev/reload`, aucun overhead.

---

## Variables d'environnement

| Variable | Défaut | Description |
|----------|--------|-------------|
| `PORT` | `8080` | Port d'écoute du serveur |
| `APP_ENV` | — | `development` pour activer le hot-reload |
| `DB_HOST` | `localhost` | Hôte PostgreSQL |
| `DB_PORT` | `5432` | Port PostgreSQL |
| `DB_USER` | — | Utilisateur PostgreSQL |
| `DB_PASSWORD` | — | Mot de passe PostgreSQL |
| `DB_NAME` | — | Nom de la base de données |
| `DB_SSLMODE` | `disable` | Mode SSL (`disable`, `require`, `verify-full`) |
| `DATABASE_URL` | — | DSN complète (prioritaire sur les variables ci-dessus) |
| `JWT_SECRET` | `change-me` | Secret de signature JWT (min. 32 caractères) |
| `UPLOAD_DIR` | `uploads` | Répertoire de stockage des fichiers |
| `UPLOAD_BASE_URL` | — | URL de base pour les médias servis |

---

## Déploiement

### Docker

```bash
docker build -t mutane .
docker run -p 8080:8080 --env-file .env.prod mutane
```

### Docker Compose

```bash
docker compose up -d
```

### Binaire natif

```bash
# Build
go build -ldflags="-s -w" -o mutane ./cmd/server/

# Copier le binaire + dossiers requis sur le serveur
scp mutane user@server:/opt/mutane/
scp -r migrations web user@server:/opt/mutane/

# Lancer (avec systemd, supervisor, etc.)
APP_ENV=production ./mutane
```

---

## Développement

### Lancer les tests

```bash
go test ./...
```

### Rebuilder le CSS (si vous modifiez les classes Tailwind)

```bash
npm install
npm run css:watch
```

### Ajouter une migration

Créez un fichier `migrations/00X_description.up.sql` — il sera appliqué automatiquement au prochain démarrage.

---

## Roadmap

- [ ] Webhooks sur événements (création/modification/suppression d'entrées)
- [ ] Internationalisation (i18n) des entrées
- [ ] Rôles et permissions granulaires
- [ ] Plugins / champs personnalisés
- [ ] Interface de gestion des utilisateurs
- [ ] Export/import de contenu (JSON, CSV)
- [ ] Templates Templ pour le rendu server-side

---

## Licence

MIT — voir [LICENSE](LICENSE)
