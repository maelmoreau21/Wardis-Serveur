# Wardis Project Context & Memory

Ce fichier fournit le contexte essentiel de l'application Wardis pour guider les futurs agents IA intervenant sur le dépôt.

## 🚀 Présentation de Wardis
Wardis est une console de supervision et de contrôle d'accès de niveau industriel (VMS / Contrôle d'accès) construite sur une architecture découplée :
- **Frontend** : Dépôt `Wardis-Client`, application React (TypeScript) embarquée avec **Tauri v2** et stylisée avec **Tailwind CSS v4** (dans `App.css` importé par `main.tsx`).
- **Backend** : Dépôt `Wardis-Serveur` développé en Go, utilisant PostgreSQL, NATS (bus de message), MediaMTX (WebRTC/WHEP/HLS) et MinIO (Object Storage).

---

## 🔒 Authentification & Identifiants
- **Saisie de l'adresse du serveur** : L'écran de connexion permet d'entrer l'adresse URL du serveur de destination (sauvegardée dans le `localStorage` sous la clé `wardis-server-url`).
- **Identifiants** : La validation stricte du format e-mail a été désactivée. Le système accepte les noms d'utilisateurs simples (LDAP / textuels).
- **Compte Administrateur par défaut** (généré au premier lancement du serveur) :
  - **Identifiant** : `root`
  - **Mot de passe** : `root`
- **Commande CLI pour créer de nouveaux comptes** :
  ```bash
  docker exec wardis-server-dev ./server --create-user --username NOM_D_UTILISATEUR --password MOT_DE_PASSE --role admin
  ```

---

## 📡 Résolution des Flux & Permissions
- **HTTP / HTTPS Permissifs dans Tauri** : Afin d'autoriser la console à se connecter à n'importe quelle adresse IP ou domaine de serveur de sécurité, les autorisations de fetch de Tauri dans `src-tauri/capabilities/default.json` sont configurées sur l'identifiant `"http:default"` avec le scope global `http://*` et `https://*`.
- **Intégration SafeFetch** : Pour éviter les pannes de requête lors du test ou de l'exécution hors Tauri (ex: dans un simple navigateur web) ou en cas de problème de chargement de l'IPC Tauri, toutes les requêtes d'API s'effectuent via le helper `safeFetch` défini dans `src/store/config.ts`.
- **Ports Vidéo** :
  - API Rest principale : Port `8080` (dynamique)
  - Flux vidéo HLS : Port `8888` (généré dynamiquement par `getHlsBaseUrl`)
  - Flux vidéo WebRTC WHEP : Port `8889` (généré dynamiquement par `getWhepBaseUrl`)

---

## 🎨 Identité Visuelle (UI/UX)
- Le design est épuré, sobre, professionnel avec des angles arrondis, du flat design moderne et des thèmes clair et sombre ajustés dans `App.css`.
- Pas de scanlines CRT rétro, pas de bordures de style cyberpunk brackets. L'en-tête de connexion inclut un badge dynamique interrogeant l'endpoint public `/health` toutes les 8 secondes pour afficher en temps réel si le serveur ciblé est **Actif**, **Inactif** ou **En cours de vérification**.
