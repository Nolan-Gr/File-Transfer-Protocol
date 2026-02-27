# Protocole FTP

**Équipe :** JOLY-Alexis, GRILLET-Nolan, COZIC-Martin

---

## Contexte 

Voici un projet fait dans le cadre de la matière "Programmation Système", vous trouverez plus de détails sur ce que l'on devait développer dans le markdown "consignes".

Petites précisions sur l'organisation du code : 

- vous trouverez les main.go du client et du serveur dans le dossier "cmd"

- vous trouverez les différentes fonctions gérant leurs comportements dans le dossier "internal". Dans "internal/pkg/proto" se trouvent les fonctions partagées entre serveur et client 
tandis que dans "internal/app/client" se trouvent les fonctionnalités propre au client et dans "internal/app/serveur" se trouvent les fonctionnalités propre au serveur