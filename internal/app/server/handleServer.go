package server

import (
	"bufio"
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"strings"
	"time"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// HandleClient : logique pour un client "normal"
func HandleClient(conn net.Conn) {
	defer ClientLogOut(conn)

	taille := incrementerClient()
	log.Println("nombre de client : ", taille)

	log.Println("adresse IP du nouveau client :", conn.RemoteAddr().String(), " connecté le : ", time.Now(), " connecté sur le port ", "3333")

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Envoyer greeting initial via protocole (Send_message gère le flush/format)
	if err := p.Send_message(conn, writer, "hello"); err != nil {
		// Sensible aux erreurs réseau (timeouts etc.)
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'hello':", err)
		}
		return
	}

	for {
		// Boucle de réception de commandes
		msg, err := p.Receive_message(conn, reader)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de la réception d'un message client:", err)
			}
			return
		}

		cleanedMsg := strings.TrimSpace(msg)
		log.Println(cleanedMsg)

		var commGet = strings.Split(cleanedMsg, " ")

		// Si le serveur n'est pas en train de se terminer, traiter les commandes
		if !isServerShuttingDown() {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				// Répondre OK pour démarrer la session.
				if err := p.Send_message(conn, writer, "ok"); err != nil {
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'ok' après 'start':", err)
					}
					return
				}

				// LIST : envoie la liste des commandes que le client peut utiliser
			} else if len(commGet) == 2 && commGet[0] == "List" {
				nbOp := incrementerOperations()
				log.Println("Commande LIST reçue, opérations en cours:", nbOp)
				if !ListServer(conn, commGet, writer, reader) {
					// En cas d'erreur, décrémenter et quitter.
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande LIST terminée, opérations restantes:", nbOp)

				// GET : transfert d'un fichier
			} else if len(commGet) == 3 && commGet[0] == "GET" {
				nbOp := incrementerOperations()
				log.Println("Commande GET reçue pour:", commGet[1], ", opérations en cours:", nbOp)
				if !Getserver(conn, commGet, writer, reader) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande GET terminée, opérations restantes:", nbOp)

				// UNKNOWN : envoie le message d'aide si la commande envoyee n'est pas reconnue
			} else if cleanedMsg == "Unknown" {
				// Commande inconnue : renvoyer message d'aide.
				if err := p.Send_message(conn, writer, "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande."); err != nil {
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi du message unknown:", err)
					}
					return
				}
				log.Println("Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande.")

				// HELP : le client reçoit la liste des commandes qu'il peut effectuer
			} else if commGet[0] == "Help" {
				helpMessage := "Commandes disponibles : LIST, GET <filename>, GOTO <target>, TREE, HELP, END"
				if commGet[1] == "true" {
					helpMessage += ", MESSAGES"
				}
				if err := p.Send_message(conn, writer, helpMessage); err != nil {
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'help':", err)
					}
					return
				}

			} else if commGet[0] == "tree" {
				// Envoie l'arbre récursif du dossier "Docs"
				if !tree(conn, writer, reader) {
					return
				}

			} else if commGet[0] == "GOTO" {
				if !GOTO(commGet, conn, writer) { // Appel avec vérification de l'échec réseau
					return // Fermer la connexion en cas d'erreur Send_message/Timeout dans GOTO
				}

			} else if cleanedMsg == "end" {
				// Fin de la session cliente.
				if err := p.Send_message(conn, writer, "ok"); err != nil {
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'ok' après 'end':", err)
					}
				}
				return

			} else {
				// Message inattendu : on l'ignore (mais on log)
				log.Println("Message inattendu du client:", cleanedMsg)
				continue
			}

		} else {
			// Si le serveur est en cours d'arrêt : informer le client et couper la connexion
			if err := p.Send_message(conn, writer, "Server terminating, connection closing."); err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi du message de terminaison:", err)
				}
			}
			return
		}

		// Si le logger est en mode debug, on renvoie des infos de debug au client
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			log.Println("debug ")
			DebugServer(conn, writer)
		}
	}
}

// HandleControlClient : logique pour le client de contrôle (suppression de l'ancienne logique d'historique)
func HandleControlClient(conn net.Conn) {
	defer ClientLogOut(conn)

	taille := incrementerClient()
	log.Println("nombre de client : ", taille)

	log.Println("adresse IP du nouveau client :", conn.RemoteAddr().String(), " connecté le : ", time.Now())

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	if err := p.Send_message(conn, writer, "hello"); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'hello':", err)
		}
		return
	}

	for {
		msg, err := p.Receive_message(conn, reader)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de la réception d'un message client control:", err)
			}
			return
		}

		cleanedMsg := strings.TrimSpace(msg)
		log.Println(cleanedMsg)

		var commHideReveal = strings.Split(cleanedMsg, " ")

		if !isServerShuttingDown() {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				if err := p.Send_message(conn, writer, "ok"); err != nil {
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'ok' après 'start':", err)
					}
					return
				}

				// LIST : envoie la liste des commandes que le client peut utiliser
			} else if len(commHideReveal) == 2 && commHideReveal[0] == "List" {
				nbOp := incrementerOperations()
				log.Println("Commande LIST reçue, opérations en cours:", nbOp)
				if !ListServer(conn, commHideReveal, writer, reader) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande LIST terminée, opérations restantes:", nbOp)

				// TERMINATE : permet d'éteindre le serveur et de déconnecter les autres clients
			} else if cleanedMsg == "Terminate" {
				// Stocker les flux du client initiant la terminaison pour pouvoir
				// lui envoyer des messages d'état durant l'arrêt.
				log.Println("Commande TERMINATE reçue")
				clientTerminantMutex.Lock()
				clientTerminant.reader = reader
				clientTerminant.writer = writer
				clientTerminantMutex.Unlock()
				// Lancer la procédure de terminaison dans une goroutine séparée
				go TerminateServer(conn)
				// Attendre que shutdownChan soit fermé (TerminateServer le ferme).
				<-shutdownChan
				return

				// UNKNOWN : envoie le message d'aide si la commande envoyee n'est pas reconnue
			} else if cleanedMsg == "Unknown" {
				if err := p.Send_message(conn, writer, "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande."); err != nil {
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi du message unknown:", err)
					}
					return
				}
				log.Println("Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande.")

				// HELP : le client reçoit la liste des commandes qu'il peut effectuer
			} else if commHideReveal[0] == "Help" {
				helpMessage := "Commandes disponibles : LIST, HIDE <filename>, REVEAL <filename>, GOTO <target>, TREE, HELP, END, TERMINATE"
				if commHideReveal[1] == "true" {
					helpMessage += ", MESSAGES"
				}
				if err := p.Send_message(conn, writer, helpMessage); err != nil {
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'help':", err)
					}
					return
				}

				// HIDE <file> : permet de cacher un fichier visible
			} else if len(commHideReveal) == 3 && commHideReveal[0] == "HIDE" {
				nbOp := incrementerOperations()
				log.Println("Commande HIDE reçue, opérations en cours:", nbOp)
				if !HIDE(conn, commHideReveal, writer) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande HIDE terminée, opérations restantes:", nbOp)

				// REVEAL <file> : permet de révéler un fichier caché
			} else if len(commHideReveal) == 3 && commHideReveal[0] == "REVEAL" {
				nbOp := incrementerOperations()
				log.Println("Commande REVEAL reçue, opérations en cours:", nbOp)
				if !REVEAL(conn, commHideReveal, writer) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande REVEAL terminée, opérations restantes:", nbOp)

				// END : pour la deconnexion du client
			} else if cleanedMsg == "end" {
				if err := p.Send_message(conn, writer, "ok"); err != nil {
					var netErr net.Error
					if errors.As(err, &netErr) && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'ok' après 'end':", err)
					}
				}
				return

			} else if commHideReveal[0] == "tree" {
				tree(conn, writer, reader)

			} else if commHideReveal[0] == "GOTO" {
				if !GOTO(commHideReveal, conn, writer) {
					return
				}

			} else {
				log.Println("Message inattendu du client:", cleanedMsg)
				continue
			}

		} else {
			// Si le serveur est en cours d'arrêt : informer le client et couper la connexion.
			if err := p.Send_message(conn, writer, "Server terminating, connection closing."); err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi du message de terminaison:", err)
				}
			}
			return
		}

		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			log.Println("debug ")
			DebugServer(conn, writer)
		}
	}
}
