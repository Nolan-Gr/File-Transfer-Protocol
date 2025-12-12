package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// listeMessage garde un historique simple des échanges pour débogage.
// On stocke des chaînes pour pouvoir afficher plus tard ce qui a été envoyé/reçu.
var listeMessage = []string{"Historique des messages : \n"}

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
	defer conn.Close()
	log.Println("Connecté au serveur:", conn.RemoteAddr().String())

	reader := bufio.NewReader(conn) // lecture depuis la connexion
	writer := bufio.NewWriter(conn) // écriture (nécessaire pour p.Send_message)
	posActuelle := "Docs"           // position locale dans l'arbre de fichiers (répertoire courant)

	// Étape 1 : Attendre le message "hello" du serveur
	msg, err := p.Receive_message(conn, reader)
	if err != nil {
		// Gestion simple des erreurs, on loggue et on quitte la fonction
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de 'hello':", err)
		} else {
			log.Println("Erreur lors de la réception de 'hello' ou déconnexion:", err)
		}
		return
	}
	listeMessage = append(listeMessage, "received message :", msg)

	if strings.TrimSpace(msg) != "hello" {
		// Si le serveur n'a pas envoyé ce qu'on attend, on arrête le protocole
		log.Println("Protocole échoué : Attendu 'hello', reçu:", strings.TrimSpace(msg))
		return
	}

	// Étape 2 : Le client répond "start"
	listeMessage = append(listeMessage, "sent message :", "start \n")
	if err := p.Send_message(conn, writer, "start"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'start':", err)
		} else {
			log.Println("Erreur lors de l'envoi de 'start':", err)
		}
		return
	}

	// Étape 3 : Attendre le message "ok" du serveur
	msg, err = p.Receive_message(conn, reader)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de 'ok':", err)
		} else {
			log.Println("Erreur lors de la réception de 'ok' ou déconnexion:", err)
		}
		return
	}
	listeMessage = append(listeMessage, "received message :", msg)

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

		// Le client se déconnecte localement
		if command == "END" {
			break

		} else if command == "GET" && !isControlPort && len(split) == 2 {
			// GET <filename> : on ajoute la position actuelle au tableau pour que Getclient sache où chercher
			split = append(split, posActuelle)
			if !Getclient(conn, line, split, writer, reader) {
				return // Erreur critique -> fermer la connexion
			}

		} else if command == "LIST" {
			// LIST <dir> : renvoie la liste des fichiers
			split = append(split, posActuelle)
			if !ListClient(conn, split, writer, reader) {
				return
			}

		} else if command == "HELP" {
			listeMessage = append(listeMessage, "sent message :", "Help \n")
			if err := p.Send_message(conn, writer, "Help"); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'help':", err)
				} else {
					log.Println("Erreur lors de l'envoi de 'help':", err)
				}
				return
			}

			msg, err = p.Receive_message(conn, reader)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de la réception de la réponse help:", err)
				} else {
					log.Println("Erreur lors de la réception de la réponse ou déconnexion:", err)
				}
				return
			}
			listeMessage = append(listeMessage, "received message :", msg)
			log.Println(msg)

		} else if command == "TERMINATE" && isControlPort {
			// commande spéciale disponible seulement sur le port de contrôle
			if !TerminateClient(conn, writer, reader) {
				return
			}
			return // Fermer la connexion après terminate

		} else if command == "HIDE" && isControlPort && len(split) == 2 {
			// HIDE <file> (contrôle)
			split = append(split, posActuelle)
			if !HideClient(conn, split, writer, reader) {
				return
			}

		} else if command == "REVEAL" && isControlPort && len(split) == 2 {
			// REVEAL <file> (contrôle)
			split = append(split, posActuelle)
			if !RevealClient(conn, split, writer, reader) {
				return
			}

		} else if command == "MESSAGES" && slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			// Affiche l'historique si on est en mode debug
			fmt.Println(strings.Trim(fmt.Sprint(listeMessage), "[]"))

		} else if command == "TREE" {
			// TREE <dir> : affiche l'arborescence
			split = append(split, posActuelle)
			if !treeClient(conn, split, writer, reader) {
				return
			}

		} else if command == "GOTO" {
			// GOTO <target> : change la position locale en interrogeant le serveur
			split = append(split, posActuelle)
			if split[1] == ".." {
				// remonter d'un niveau
				posActuelle = GOTOClient(conn, posActuelle, split, writer, reader)
			} else {
				// descendre dans un sous-dossier : on ajoute "/<nom>" au chemin local
				posActuelle = posActuelle + "/" + GOTOClient(conn, posActuelle, split, writer, reader)
			}

		} else {
			// Commande inconnue -> informer le serveur et afficher la réponse
			listeMessage = append(listeMessage, "sent message :", "Unknown \n")
			if err := p.Send_message(conn, writer, "Unknown"); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'unknown':", err)
				} else {
					log.Println("Erreur lors de l'envoi de 'unknown':", err)
				}
				return
			}

			msg, err = p.Receive_message(conn, reader)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de la réception de la réponse unknown:", err)
				} else {
					log.Println("Erreur lors de la réception de la réponse ou déconnexion:", err)
				}
				return
			}
			listeMessage = append(listeMessage, "received message :", msg)
			log.Println(msg)
		}
	}

	// Étape 5 : Le client répond "end" pour clore la session proprement
	listeMessage = append(listeMessage, "sent message :", "end \n")
	if err := p.Send_message(conn, writer, "end"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'end':", err)
		} else {
			log.Println("Erreur lors de l'envoi de 'end':", err)
		}
		return
	}

	// Étape 6 : Attendre le message "ok" final du serveur
	msg, err = p.Receive_message(conn, reader)
	if err != nil {
		// La déconnexion immédiate du serveur après l'envoi du "ok" est possible
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de 'ok' final:", err)
		} else if err != net.ErrClosed && err != io.EOF {
			log.Println("Erreur lors de la réception de 'ok' final ou déconnexion:", err)
		} else {
			log.Println("Connexion fermée par le serveur après 'end'.")
		}
		return
	}
	listeMessage = append(listeMessage, "received message :", msg)

	if strings.TrimSpace(msg) != "ok" {
		log.Println("Protocole échoué : Attendu 'ok' final, reçu:", strings.TrimSpace(msg))
		return
	}

	log.Println("Protocole terminé avec succès. Déconnexion du client.")
}

// Getclient gère la commande GET : demander un fichier, recevoir son contenu et sauvegarder localement.
// splitGET : [ "GET", "<filename>", "<position>" ]
func Getclient(conn net.Conn, line string, splitGET []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	listeMessage = append(listeMessage, "sent message :", line, "\n")
	if err := p.Send_message(conn, writer, "GET"+" "+splitGET[1]+" "+splitGET[2]); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande GET:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la commande:", err)
		}
		return false
	}

	// Attend la réponse du serveur
	var response, err = p.Receive_message(conn, reader)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse GET:", err)
		} else {
			log.Println("Erreur lors de la réception de la réponse:", err)
		}
		return false
	}
	listeMessage = append(listeMessage, "received message :", response)
	response = strings.TrimSpace(response)
	log.Println(response)

	// fichier introuvable
	if response == "FileUnknown" {
		log.Println("Fichier introuvable sur le serveur")

		// Envoie "OK" pour confirmer la réception de FileUnknown
		listeMessage = append(listeMessage, "sent message :", "OK \n")
		if err := p.Send_message(conn, writer, "OK"); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'OK':", err)
			} else {
				log.Println("Erreur lors de l'envoi de 'OK':", err)
			}
			return false
		}

	} else if response == "Start" {
		// Le serveur envoie ensuite le contenu du fichier
		data, err := p.Receive_message(conn, reader)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de la lecture du fichier:", err)
			} else {
				log.Println("Erreur lors de la lecture du fichier:", err)
			}
			return false
		}
		listeMessage = append(listeMessage, "received message :", data)

		// Sauvegarde le fichier localement avec le même nom
		err = os.WriteFile(splitGET[1], []byte(data), 0770)
		if err != nil {
			log.Println("Erreur lors de la sauvegarde du fichier:", err)
			return false
		}

		log.Printf("Fichier '%s' reçu et sauvegardé (%d octets)\n", splitGET[1], len(data))
		log.Printf("Contenu du fichier '%s':\n%s\n", splitGET[1], string(data))

		// Envoie "OK" pour confirmer la bonne réception
		listeMessage = append(listeMessage, "sent message :", "OK \n")
		if err := p.Send_message(conn, writer, "OK"); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'OK':", err)
			} else {
				log.Println("Erreur lors de l'envoi de 'OK':", err)
			}
			return false
		}
	} else {
		// Toute autre réponse est imprévue
		log.Println("Réponse inattendue du serveur:", response)
	}

	return true
}

// HideClient demande au serveur de cacher un fichier (commande disponible sur le port de contrôle).
func HideClient(conn net.Conn, split []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	command := "HIDE " + split[1] + " " + split[2]
	listeMessage = append(listeMessage, "sent message :", command, "\n")
	if err := p.Send_message(conn, writer, command); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande HIDE:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la commande:", err)
		}
		return false
	}

	// Attendre la réponse du serveur
	response, err := p.Receive_message(conn, reader)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse HIDE:", err)
		} else {
			log.Println("Erreur lors de la réception de la réponse:", err)
		}
		return false
	}
	listeMessage = append(listeMessage, "received message :", response)
	response = strings.TrimSpace(response)

	if response == "FileUnknown" {
		log.Println("Fichier introuvable sur le serveur")
	} else if response == "OK" {
		log.Printf("Fichier '%s' caché avec succès\n", split[1])
	} else {
		log.Println("Réponse inattendue du serveur:", response)
	}

	return true
}

// RevealClient demande au serveur de révéler un fichier caché (port de contrôle).
func RevealClient(conn net.Conn, split []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	command := "REVEAL " + split[1] + " " + split[2]
	listeMessage = append(listeMessage, "sent message :", command, "\n")
	if err := p.Send_message(conn, writer, command); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande REVEAL:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la commande:", err)
		}
		return false
	}

	// Attendre la réponse du serveur
	response, err := p.Receive_message(conn, reader)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse REVEAL:", err)
		} else {
			log.Println("Erreur lors de la réception de la réponse:", err)
		}
		return false
	}
	listeMessage = append(listeMessage, "received message :", response)
	response = strings.TrimSpace(response)

	if response == "FileUnknown" {
		log.Println("Fichier introuvable (ou pas caché) sur le serveur")
	} else if response == "OK" {
		log.Printf("Fichier '%s' révélé avec succès\n", split[1])
	} else {
		log.Println("Réponse inattendue du serveur:", response)
	}

	return true
}

// ListClient demande la liste des fichiers et l'affiche.
// split : [ "LIST", "<dir>", "<posActuelle>" ]
func ListClient(conn net.Conn, split []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	listeMessage = append(listeMessage, "sent message :", "List \n")
	if err := p.Send_message(conn, writer, "List "+split[1]); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande LIST:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la commande:", err)
		}
		return false
	}

	// Attend la réponse du serveur
	var response, err = p.Receive_message(conn, reader)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse LIST:", err)
		} else {
			log.Println("Erreur lors de la réception de la réponse:", err)
		}
		return false
	}
	listeMessage = append(listeMessage, "received message :", response)
	response = strings.TrimSpace(response)

	if response == "Start" {
		// Le serveur va envoyer la liste ; on confirme par "OK"
		listeMessage = append(listeMessage, "sent message :", "OK \n")
		if err := p.Send_message(conn, writer, "OK"); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'OK':", err)
			} else {
				log.Println("Erreur lors de l'envoi de 'OK':", err)
			}
			return false
		}

		data, err := p.Receive_message(conn, reader)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de la lecture de la liste:", err)
			} else {
				log.Println("Erreur lors de la lecture de la liste:", err)
			}
			return false
		}
		listeMessage = append(listeMessage, "received message :", data)

		// Le serveur renvoie les éléments séparés par "--"
		var datas = strings.Split(data, "--")
		log.Println("\n=== Liste des fichiers disponibles ===")
		for _, item := range datas {
			if strings.TrimSpace(item) != "" {
				log.Println(strings.TrimSpace(item))
			}
		}
		log.Println("=====================================")
	}

	// Fin de l'opération LIST : on envoie "ok" pour clore l'échange
	listeMessage = append(listeMessage, "sent message :", "ok \n")
	if err := p.Send_message(conn, writer, "ok"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'ok' final LIST:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la commande:", err)
		}
		return false
	}

	return true
}

// TerminateClient envoie la commande TERMINATE au serveur de contrôle et attend la progression.
// La boucle lit tous les messages jusqu'à ce que le serveur signale qu'il s'arrête.
func TerminateClient(conn net.Conn, writer *bufio.Writer, reader *bufio.Reader) bool {
	listeMessage = append(listeMessage, "sent message :", "Terminate \n")
	if err := p.Send_message(conn, writer, "Terminate"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande TERMINATE:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la commande:", err)
		}
		return false
	}

	log.Println("Commande TERMINATE envoyée, attente de la réponse du serveur...")

	for {
		rep, err := p.Receive_message(conn, reader)
		if err != nil {
			// La connexion peut être fermée après le message final
			if err == io.EOF {
				log.Println("Serveur déconnecté")
				return true
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de la réception de la réponse TERMINATE:", err)
			} else {
				log.Println("Erreur lors de la réception de la réponse:", err)
			}
			return false
		}

		listeMessage = append(listeMessage, "received message :", rep)
		rep = strings.TrimSpace(rep)

		// On affiche les différents messages d'avancement ou le message final.
		if rep == "Terminaison finie, le serveur s'éteint" {
			log.Println(rep)
			log.Println("Le serveur s'est arrêté avec succès")
			return true
		} else if strings.Contains(rep, "opération") {
			// message d'état intermédiaire
			log.Println(rep)
		} else {
			log.Println(rep)
		}
	}
}

// treeClient demande l'arbre (liste) d'un dossier et l'affiche.
// Similaire à ListClient mais affiche aussi le contexte ("vous êtes à la racine", etc.).
func treeClient(conn net.Conn, split []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	listeMessage = append(listeMessage, "sent message :", "tree \n")
	if err := p.Send_message(conn, writer, "tree "+split[1]); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande TREE:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la commande:", err)
		}
		return false
	}

	// Attend la réponse du serveur
	var response, err = p.Receive_message(conn, reader)
	listeMessage = append(listeMessage, "received message :", response)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse TREE:", err)
		} else {
			log.Println("Erreur lors de la réception de la réponse:", err)
		}
		return false
	}
	listeMessage = append(listeMessage, "received message :", response)
	response = strings.TrimSpace(response)

	if response == "Start" {
		log.Println("entered start")
		// Le serveur va envoyer la liste ; on confirme par "OK"
		listeMessage = append(listeMessage, "sent message :", "OK \n")
		if err := p.Send_message(conn, writer, "OK"); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'OK':", err)
			} else {
				log.Println("Erreur lors de l'envoi de 'OK':", err)
			}
			return false
		}

		data, err := p.Receive_message(conn, reader)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de la lecture de la liste TREE:", err)
			} else {
				log.Println("Erreur lors de la lecture de la liste:", err)
			}
			return false
		}
		listeMessage = append(listeMessage, "received message :", data)
		log.Println(data)

		var datas = strings.Split(data, "--")
		if split[1] == "Docs" {
			// racine
			log.Println("vous êtes à la racine")
		} else {
			log.Println("vous êtes dans", split[1])
		}
		log.Println("\n=== Liste des fichiers disponibles ===")
		for _, item := range datas {
			if strings.TrimSpace(item) != "" {
				log.Println(strings.TrimSpace(item))
			}
		}
		log.Println("=====================================")
	}

	// Fin de l'opération TREE : envoi de l'acquittement final "ok"
	listeMessage = append(listeMessage, "sent message :", "ok \n")
	if err := p.Send_message(conn, writer, "ok"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'ok' final TREE:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la commande:", err)
		}
		return false
	}

	return true
}

// GOTOClient demande au serveur de changer de dossier et met à jour la position locale.
// split : [ "GOTO", "<target>", "<posActuelle>" ]
func GOTOClient(conn net.Conn, posActuelle string, split []string, writer *bufio.Writer, reader *bufio.Reader) string {
	listeMessage = append(listeMessage, "sent message :", "GOTO \n")
	if err := p.Send_message(conn, writer, "GOTO "+split[1]+" "+split[2]); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return ""
	}

	var response, err = p.Receive_message(conn, reader)
	listeMessage = append(listeMessage, "received message :", response)
	if err != nil {
		log.Println("Erreur lors de la réception de la réponse:", err)
		return ""
	}
	response = strings.TrimSpace(response)

	// Interprétation des réponses serveur :
	// "Start" -> on peut aller dans le dossier demandé
	// "NO!" -> on est déjà dans ce dossier
	// "back" -> il faut remonter d'un niveau ; on calcule la nouvelle position à partir du chemin
	if response == "Start" {
		posActuelle = split[1]
	} else if response == "NO!" {
		log.Println("vous êtes déjà dans ce fichier")
	} else if response == "back" {
		// on cherche la dernière barre '/' dans le chemin et on garde la partie précédente
		var index = ParcourPath(split)
		// split[2][0:index] garde la partie du chemin avant la dernière barre
		posActuelle = split[2][0:index]
	}
	return posActuelle
}

// ParcourPath retourne l'index de la dernière barre '/' dans split[2].
// Utilisé pour remonter d'un niveau dans le chemin local.
// Remarque : si il n'y a pas de '/', la fonction plantera (index out of range).
func ParcourPath(split []string) int {
	var posTab []int
	for i, pos := range split[2] {
		if pos == '/' {
			// on enregistre les positions où il y a une barre
			posTab = append(posTab, i)
		}
	}
	// On renvoie la position de la dernière barre trouvée
	return posTab[len(posTab)-1]
}
