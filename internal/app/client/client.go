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

var listeMessage = []string{"Historique des messages : \n"}

var Remote string

func Run(remote string) {
	log.Println(remote)
	Remote = remote

	c, err := net.Dial("tcp", remote)
	if err != nil {
		if strings.Contains(remote, "3334") {
			log.Println("Le port 3334 est déjà occupé ou le serveur de contrôle n'est pas accessible")
			return
		}
		slog.Error(err.Error())
		return
	}
	slog.Info("Connected to " + c.RemoteAddr().String())
	// Delegue
	RunClient(c)

	slog.Debug("Connection closed")
}

func RunClient(conn net.Conn) {
	defer conn.Close()
	slog.Info("Connecté au serveur:", conn.RemoteAddr().String())

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	posActuelle := "Docs"

	// Étape 1 : Attendre le message "hello" du serveur
	msg, err := p.Receive_message(reader)
	listeMessage = append(listeMessage, "received message :", msg)
	if err != nil {
		log.Println("Erreur lors de la réception de 'hello' ou déconnexion:", err)
		return
	}
	if strings.TrimSpace(msg) != "hello" {
		log.Println("Protocole échoué : Attendu 'hello', reçu:", strings.TrimSpace(msg))
		return
	}

	// Étape 2 : Le client répond "start"
	listeMessage = append(listeMessage, "sent message :", "start \n")
	if err := p.Send_message(writer, "start"); err != nil {
		log.Println("Erreur lors de l'envoi de 'start':", err)
		return
	}

	// Étape 3 : Attendre le message "ok" du serveur
	msg, err = p.Receive_message(reader)
	listeMessage = append(listeMessage, "received message :", msg)
	if err != nil {
		log.Println("Erreur lors de la réception de 'ok' ou déconnexion:", err)
		return
	}
	if strings.TrimSpace(msg) != "ok" {
		log.Println("Protocole échoué : Attendu 'ok' (après start), reçu:", strings.TrimSpace(msg))
		return
	}
	reader2 := bufio.NewReader(os.Stdin)

	// Étape 4: le client entre ce qu'il souhaite dans le terminal
	for {
		fmt.Print("\nEntrez une commande à envoyer au serveur (ou 'end' pour terminer) : ")
		line, _ := reader2.ReadString('\n')
		line = strings.TrimSpace(line)

		var split = strings.Split(line, " ")
		command := strings.ToUpper(split[0])

		// Déterminer si c'est le port de contrôle
		isControlPort := strings.Contains(Remote, "3334")

		// Le client se déconnecte
		if command == "END" {
			break

			// Commandes disponibles sur le port normal (3333)
		} else if command == "GET" && !isControlPort && len(split) == 2 {
			split = append(split, posActuelle)
			Getclient(line, split, writer, reader)

		} else if command == "LIST" {
			split = append(split, posActuelle)
			ListClient(split, writer, reader)

		} else if command == "HELP" {
			listeMessage = append(listeMessage, "sent message :", "Help \n")
			if err := p.Send_message(writer, "Help"); err != nil {
				log.Println("Erreur lors de l'envoi de 'help':", err)
				return
			}

			msg, err = p.Receive_message(reader)
			listeMessage = append(listeMessage, "received message :", msg)
			if err != nil {
				log.Println("Erreur lors de la réception de la réponse ou déconnexion:", err)
				return
			}
			log.Println(msg)

			// Commandes disponibles uniquement sur le port de contrôle (3334)
		} else if command == "TERMINATE" && isControlPort {
			TerminateClient(writer, reader)
			return // Fermer la connexion après terminate

		} else if command == "HIDE" && isControlPort && len(split) == 2 {
			split = append(split, posActuelle)
			HideClient(split, writer, reader)

		} else if command == "REVEAL" && isControlPort && len(split) == 2 {
			split = append(split, posActuelle)
			RevealClient(split, writer, reader)

		} else if command == "MESSAGES" && slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			fmt.Println(strings.Trim(fmt.Sprint(listeMessage), "[]"))
			// Ne pas envoyer de message au serveur pour cette commande locale

		} else {
			// Commande inconnue ou invalide
			listeMessage = append(listeMessage, "sent message :", "Unknown \n")
			if err := p.Send_message(writer, "Unknown"); err != nil {
				log.Println("Erreur lors de l'envoi de 'unknown':", err)
				return
			}

			msg, err = p.Receive_message(reader)
			listeMessage = append(listeMessage, "received message :", msg)
			if err != nil {
				log.Println("Erreur lors de la réception de la réponse ou déconnexion:", err)
				return
			}
			log.Println(msg)
		}
	}

	// Étape 5 : Le client répond "end"
	listeMessage = append(listeMessage, "sent message :", "end \n")
	if err := p.Send_message(writer, "end"); err != nil {
		log.Println("Erreur lors de l'envoi de 'end':", err)
		return
	}

	// Étape 6 : Attendre le message "ok" final du serveur
	msg, err = p.Receive_message(reader)
	listeMessage = append(listeMessage, "received message :", msg)
	if err != nil {
		// La déconnexion immédiate du serveur après l'envoi du "ok" est possible
		if err != net.ErrClosed && err != io.EOF {
			log.Println("Erreur lors de la réception de 'ok' final ou déconnexion:", err)
		} else {
			log.Println("Connexion fermée par le serveur après 'end'.")
		}
		return
	}
	if strings.TrimSpace(msg) != "ok" {
		log.Println("Protocole échoué : Attendu 'ok' final, reçu:", strings.TrimSpace(msg))
		return
	}

	log.Println("Protocole terminé avec succès. Déconnexion du client.")
}

func Getclient(line string, splitGET []string, writer *bufio.Writer, reader *bufio.Reader) {
	listeMessage = append(listeMessage, "sent message :", line, "\n")
	if err := p.Send_message(writer, splitGET[0]+" "+splitGET[1]+" "+splitGET[2]); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}

	// Attend la réponse du serveur
	var response, err = p.Receive_message(reader)
	listeMessage = append(listeMessage, "received message :", response)
	if err != nil {
		log.Println("Erreur lors de la réception de la réponse:", err)
		return
	}
	response = strings.TrimSpace(response)
	log.Println(response)

	// fichier introuvable
	if response == "FileUnknown" {
		log.Println("Fichier introuvable sur le serveur")

		// Envoie "OK" pour confirmer la réception de FileUnknown
		listeMessage = append(listeMessage, "sent message :", "OK \n")
		if err := p.Send_message(writer, "OK"); err != nil {
			log.Println("Erreur lors de l'envoi de 'OK':", err)
			return
		}

	} else if response == "Start" {
		// Le serveur va envoyer le fichier
		// Lire tout le contenu
		data, err := p.Receive_message(reader)
		listeMessage = append(listeMessage, "received message :", data)
		if err != nil {
			log.Println("Erreur lors de la lecture du fichier:", err)
			return
		}

		// Sauvegarde le fichier localement avec le même nom
		err = os.WriteFile(splitGET[1], []byte(data), 0770)
		if err != nil {
			log.Println("Erreur lors de la sauvegarde du fichier:", err)
			return
		}

		log.Printf("Fichier '%s' reçu et sauvegardé (%d octets)\n", splitGET[1], len(data))
		log.Printf("Contenu du fichier '%s':\n%s\n", splitGET[1], string(data))

		// Envoie "OK" pour confirmer la bonne réception
		listeMessage = append(listeMessage, "sent message :", "OK \n")
		if err := p.Send_message(writer, "OK"); err != nil {
			log.Println("Erreur lors de l'envoi de 'OK':", err)
			return
		}
	} else {
		log.Println("Réponse inattendue du serveur:", response)
	}
}

func HideClient(split []string, writer *bufio.Writer, reader *bufio.Reader) {
	command := "HIDE " + split[1] + " " + split[2]
	listeMessage = append(listeMessage, "sent message :", command, "\n")
	if err := p.Send_message(writer, command); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}

	// Attendre la réponse du serveur
	response, err := p.Receive_message(reader)
	listeMessage = append(listeMessage, "received message :", response)
	if err != nil {
		log.Println("Erreur lors de la réception de la réponse:", err)
		return
	}
	response = strings.TrimSpace(response)

	if response == "FileUnknown" {
		log.Println("Fichier introuvable sur le serveur")
	} else if response == "OK" {
		log.Printf("Fichier '%s' caché avec succès\n", split[1])
	} else {
		log.Println("Réponse inattendue du serveur:", response)
	}
}

func RevealClient(split []string, writer *bufio.Writer, reader *bufio.Reader) {
	command := "REVEAL " + split[1] + " " + split[2]
	listeMessage = append(listeMessage, "sent message :", command, "\n")
	if err := p.Send_message(writer, command); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}

	// Attendre la réponse du serveur
	response, err := p.Receive_message(reader)
	listeMessage = append(listeMessage, "received message :", response)
	if err != nil {
		log.Println("Erreur lors de la réception de la réponse:", err)
		return
	}
	response = strings.TrimSpace(response)

	if response == "FileUnknown" {
		log.Println("Fichier introuvable (ou pas caché) sur le serveur")
	} else if response == "OK" {
		log.Printf("Fichier '%s' révélé avec succès\n", split[1])
	} else {
		log.Println("Réponse inattendue du serveur:", response)
	}
}

func ListClient(split []string, writer *bufio.Writer, reader *bufio.Reader) {
	listeMessage = append(listeMessage, "sent message :", "List \n")
	if err := p.Send_message(writer, "List "+split[1]); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}
	// Attend la réponse du serveur
	var response, err = p.Receive_message(reader)
	listeMessage = append(listeMessage, "received message :", response)
	if err != nil {
		log.Println("Erreur lors de la réception de la réponse:", err)
		return
	}
	response = strings.TrimSpace(response)

	if response == "Start" {
		// Le serveur va envoyer la liste
		listeMessage = append(listeMessage, "sent message :", "OK \n")
		if err := p.Send_message(writer, "OK"); err != nil {
			log.Println("Erreur lors de l'envoi de 'OK':", err)
			return
		}
		data, err := p.Receive_message(reader)
		listeMessage = append(listeMessage, "received message :", data)
		if err != nil {
			log.Println("Erreur lors de la lecture de la liste:", err)
			return
		}
		var datas = strings.Split(data, "--")
		log.Println("\n=== Liste des fichiers disponibles ===")
		for _, item := range datas {
			if strings.TrimSpace(item) != "" {
				log.Println(strings.TrimSpace(item))
			}
		}
		log.Println("=====================================")
	}

	listeMessage = append(listeMessage, "sent message :", "ok \n")
	if err := p.Send_message(writer, "ok"); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}
}

func TerminateClient(writer *bufio.Writer, reader *bufio.Reader) {
	listeMessage = append(listeMessage, "sent message :", "Terminate \n")
	if err := p.Send_message(writer, "Terminate"); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}

	log.Println("Commande TERMINATE envoyée, attente de la réponse du serveur...")

	for {
		rep, err := p.Receive_message(reader)
		if err != nil {
			// La connexion peut être fermée après le message final
			if err == io.EOF {
				log.Println("Serveur déconnecté")
				return
			}
			log.Println("Erreur lors de la réception de la réponse:", err)
			return
		}

		listeMessage = append(listeMessage, "received message :", rep)
		rep = strings.TrimSpace(rep)

		if rep == "Terminaison finie, le serveur s'éteint" {
			log.Println(rep)
			log.Println("Le serveur s'est arrêté avec succès")
			return
		} else if strings.Contains(rep, "opération") {
			log.Println(rep)
		} else {
			log.Println(rep)
		}
	}
}
