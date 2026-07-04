Tu travailles sur Wardis, un logiciel de vidéosurveillance et contrôle d'accès (VMS)
composé de deux dépôts :
- Client bureau : Tauri v2 + React + TypeScript + Tailwind CSS + Zustand
- Serveur : Go, migrations SQL, déploiement Docker

Objectif produit : rapprocher Wardis, visuellement et fonctionnellement, des standards
du marché VMS (Genetec Security Center, Hikvision iVMS-4200), en t'inspirant de leurs
patterns d'interface et de leurs fonctionnalités courantes — sans copier leur code,
leurs assets graphiques ni leur marque.

Utilisateur cible : opérateur de salle de contrôle, qui surveille plusieurs écrans en
simultané, a besoin de rapidité de réaction sur alarme et de clarté visuelle en continu
(souvent de nuit, dans une salle sombre).

Contraintes transverses à respecter dans TOUS les chantiers suivants :
1. Thème sombre par défaut, contraste élevé, faible fatigue visuelle.
2. Architecture modulaire : chaque grand écran ("tâche" façon Genetec : Surveillance,
   Alarmes, Contrôle d'accès, Cartographie, Rapports, Système) doit être un module
   chargé indépendamment, pas un gros fichier monolithique.
3. L'app doit rester utilisable avec de nombreux flux vidéo simultanés (perf, lazy
   loading, pause du décodage sur les tuiles hors écran).
4. Le client et le serveur doivent rester synchronisés au niveau des contrats d'API
   (types partagés / schéma clair), pas d'improvisation de champs des deux côtés.
5. Avant toute modification structurante (routing, store global, schéma DB), résume
   ton plan en 5 lignes avant d'écrire du code.
6. Commits petits et fréquents, un chantier = une série de commits testables.
7. N'invente pas de dépendance à un service cloud propriétaire non mentionné ici.

Confirme que tu as bien comrpis ce contexte avant de commencer le premier chantier.