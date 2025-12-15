package client

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// Remote conserve l'adresse du serveur utilisé (utile pour détecter le "port de contrôle").
var Remote string

// Run tente de se connecter au serveur distant et lance la boucle cliente.
// remote doit être de la forme "host:port".
func Run(remote string) {
	log.Println(remote)
	Remote = remote

	c, err := net.Dial("tcp", remote)
	if err != nil {
		// message spécifique pour le port de contrôle (3334)
		if strings.Contains(remote, "3334") {
			log.Println("Le port 3334 est déjà occupé ou le serveur de contrôle n'est pas accessible")
			return
		}
		slog.Error(err.Error())
		return
	}
	slog.Info("Connected to " + c.RemoteAddr().String())
	// Délègue la suite au gestionnaire de connexion
	RunClient(c)

	slog.Debug("Connection closed")
}

// RunClient gère la session avec le serveur.
// Le protocole ici est séquentiel : on attend "hello", on envoie "start", on attend "ok", puis boucle de commande.
func RunClient(conn net.Conn) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			slog.Error(err.Error())
		}
	}(conn)
	log.Println("Connecté au serveur:", conn.RemoteAddr().String())

	reader := bufio.NewReader(conn) // lecture depuis la connexion
	writer := bufio.NewWriter(conn) // écriture (nécessaire pour p.Send_message)
	posActuelle := "Docs"           // position locale dans l'arbre de fichiers (répertoire courant)

	// Étape 1 : Attendre le message "hello" du serveur
	msg, err := p.Receive_message(conn, reader)
	if err != nil {
		// Gestion simple des erreurs, on loggue et on quitte la fonction
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de 'hello':", err)
		}
		return
	}

	if strings.TrimSpace(msg) != "hello" {
		// Si le serveur n'a pas envoyé ce qu'on attend, on arrête le protocole
		log.Println("Protocole échoué : Attendu 'hello', reçu:", strings.TrimSpace(msg))
		return
	}

	// Étape 2 : Le client répond "start"
	if err := p.Send_message(conn, writer, "start"); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'start':", err)
		}
		return
	}

	// Étape 3 : Attendre le message "ok" du serveur
	msg, err = p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de 'ok':", err)
		}
		return
	}

	if strings.TrimSpace(msg) != "ok" {
		log.Println("Protocole échoué : Attendu 'ok' (après start), reçu:", strings.TrimSpace(msg))
		return
	}

	reader2 := bufio.NewReader(os.Stdin) // lecture des commandes utilisateur

	// Étape 4: boucle de commandes utilisateur
	for {
		fmt.Print("\nVous êtes dans ", posActuelle, "\nEntrez une commande à envoyer au serveur (ou 'end' pour terminer) : ")
		line, err := reader2.ReadString('\n')
		if err != nil {
			log.Println("Erreur lecture stdin:", err)
			break
		}
		line = strings.TrimSpace(line)

		var split = strings.Split(line, " ")
		command := strings.ToUpper(split[0])
		log.Println(command)
		// Déterminer si c'est le port de contrôle (port spécial pour certaines commandes)
		isControlPort := strings.Contains(Remote, "3334")

		switch {
		// Le client se déconnecte localement
		case command == "END":
			break

			// GET <filename> : on ajoute la position actuelle au tableau pour que Getclient sache où chercher
		case command == "GET" && !isControlPort && len(split) == 2:
			split = append(split, posActuelle)
			if !Getclient(conn, split, writer, reader) {
				return
			}

			// LIST <dir> : renvoie la liste des fichiers
		case command == "LIST":
			split = append(split, posActuelle)
			if !ListClient(conn, split, writer, reader) {
				return
			}

			// HELP : le client reçoit la liste des commandes qu'il peut effectuer
		case command == "HELP":
			if err := p.Send_message(conn, writer, "Help"); err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'help':", err)
				}
				return
			}

			msg, err = p.Receive_message(conn, reader)
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de la réception de la réponse help:", err)
				}
				return
			}
			log.Println(msg)

			// commande spéciale disponible seulement sur le port de contrôle
			// TERMINATE : permet d'éteindre le serveur et de déconnecter les autres clients
			// une fois leurs requêtes terminées
		case command == "TERMINATE" && isControlPort:
			if !TerminateClient(conn, writer, reader) {
				return
			}
			return // Fermer la connexion après terminate

			// HIDE <file> : permet de cacher un fichier visible
		case command == "HIDE" && isControlPort && len(split) == 2:
			split = append(split, posActuelle)
			if !HideClient(conn, split, writer, reader) {
				return
			}

			// REVEAL <file> : permet de révéler un fichier caché
		case command == "REVEAL" && isControlPort && len(split) == 2:
			split = append(split, posActuelle)
			if !RevealClient(conn, split, writer, reader) {
				return
			}

			// TREE : affiche l'arborescence
		case command == "TREE":
			split = append(split, posActuelle)
			if !treeClient(conn, split, writer, reader) {
				return
			}

			// GOTO <target> : change la position locale en interrogeant le serveur
		case command == "GOTO":
			split = append(split, posActuelle)
			var index = p.ParcourPath(split[2]) + 1
			if index == -1 {
				index = 0
			}
			log.Println(split[2][index:len(split[2])], split)
			if split[1] == ".." {
				// remonter d'un niveau
				posActuelle = GOTOClient(conn, posActuelle, split, writer, reader)
			} else if split[2][index:len(split[2])] == split[1] {
				log.Println("vous êtes déjà dans ce fichier")
			} else {
				log.Println("position", posActuelle)
				// descendre dans un sous-dossier : on ajoute "/<nom>" au chemin local
				posActuelle = posActuelle + "/" + GOTOClient(conn, posActuelle, split, writer, reader)
			}
			// Commande inconnue : informer le serveur et afficher la réponse
		default:
			if err := p.Send_message(conn, writer, "Unknown"); err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'unknown':", err)
				}
				return
			}

			msg, err = p.Receive_message(conn, reader)
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de la réception de la réponse unknown:", err)
				}
				return
			}
			log.Println(msg)
		}
	}

	// Étape 5 : Le client répond "end" pour clore la session proprement
	if err := p.Send_message(conn, writer, "end"); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'end':", err)
		}
		return
	}

	// Étape 6 : Attendre le message "ok" final du serveur
	msg, err = p.Receive_message(conn, reader)
	if err != nil {
		// La déconnexion immédiate du serveur après l'envoi du "ok" est possible
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de 'ok' final:", err)
		}
		return
	}

	if strings.TrimSpace(msg) != "ok" {
		log.Println("Protocole échoué : Attendu 'ok' final, reçu:", strings.TrimSpace(msg))
		return
	}

	log.Println("Protocole terminé avec succès. Déconnexion du client.")
}
