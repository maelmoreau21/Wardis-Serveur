# Wardis Server 🚀

Wardis est un système de gestion vidéo (VMS) et de contrôle d'accès de niveau industriel, conçu pour offrir une alternative moderne, performante et hautement disponible aux solutions propriétaires majeures comme Genetec.

Ce dépôt contient la partie **Serveur** de l'application, développée en Go.

## 🛠️ Architecture & Technologies
* **Langage :** Go (Golang) pour des performances backend optimales et une faible empreinte mémoire.
* **Bus d'événements :** NATS, assurant une communication en temps réel ultra-rapide entre les microservices.
* **Gestion Flux Vidéo :** MediaMTX (RTSP, WebRTC, WHEP) pour le routage vidéo direct basse latence.
* **Stockage :** Stockage hybride local et Object Storage (MinIO / S3) pour la rétention à long terme.
* **Base de données :** PostgreSQL (piloté avec support des migrations).

## ✨ Fonctionnalités implémentées
* **Dual-Streaming :** Gestion d'un flux haute résolution pour l'enregistrement continu et d'un flux basse résolution (WebRTC) pour l'affichage en direct sans latence.
* **Intégration ONVIF :** Découverte automatique des caméras sur le réseau local via WS-Discovery et contrôle PTZ.
* **Automated Video Tiering :** Worker asynchrone chargé de migrer les archives vidéo locales vers le stockage MinIO selon des règles de rétention définies.
* **RBAC Avancé :** Système de droits ultra-granulaire permettant de restreindre les accès (Direct, Archives, Contrôle) par caméra, porte ou zone géographique.
* **Preuve Juridique :** Endpoint d'exportation vidéo sécurisé générant un manifeste signé numériquement avec empreinte SHA-256.

## 📦 Déploiement & CI/CD
Le serveur est conçu pour être cloud-native et redondant :
* **Docker & Docker Compose :** Pour les environnements de développement et les déploiements légers.
* **Kubernetes (K8s) :** Manifestes inclus pour orchestrer la haute disponibilité et la redondance des services.
* **GitHub Actions :** Build et publication automatiques de l'image Docker sur le GitHub Container Registry (GHCR) à chaque push sur `main`.

---

## ⚖️ Licence & Propriété Intellectuelle

**PROPRIETARY & SOURCE-AVAILABLE LICENSE**

Copyright (c) 2026 Maël Moreau. Tous droits réservés.

Le code source présent dans ce dépôt est public uniquement à des fins de consultation, de démonstration technique et d'audit. 
* **Exploitation commerciale :** Strictement interdite. Seul l'auteur original (Maël Moreau) détient le droit exclusif d'exploiter ce logiciel à des fins commerciales, lucratives ou professionnelles.
* **Forks et Modifications :** En cas de fork ou de dérivation autorisée par la plateforme GitHub, le crédit complet à l'auteur original doit être maintenu de manière visible dans l'ensemble du code et des documentations.
* **Utilisation tierce :** Aucune licence d'utilisation, de distribution ou de modification gratuite n'est accordée aux tiers sans l'accord écrit explicite de l'auteur.