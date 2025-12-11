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
		if Remote == "3334" {
			log.Println("Le port 3334 est déjà occupé")
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

		// Le client se déconnecte
		if strings.ToUpper(split[0]) == "END" {
			break
			// Le client envoie une commande connue
		} else if strings.ToUpper(split[0]) == "GET" && Remote == "3333" {
			Getclient(line, split, conn, writer, reader)
		} else if strings.ToUpper(split[0]) == "LIST" {
			ListClient(writer, reader)
		} else if strings.ToUpper(split[0]) == "TERMINATE" && Remote == "3334" {
			TerminateClient(writer, reader)
		} else if strings.ToUpper(split[0]) == "HIDE" && Remote == "3334" {
			HideClient(split, writer, reader)
		} else if strings.ToUpper(split[0]) == "MESSAGES" && slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			fmt.Println(strings.Trim(fmt.Sprint(listeMessage), "[]"))
			if err := p.Send_message(writer, "messages"); err != nil {
				log.Println("Erreur lors de l'envoi de 'end':", err)
				continue
			}
		} else {
			listeMessage = append(listeMessage, "sent message :", "Unknown \n")
			if err := p.Send_message(writer, "Unknown"); err != nil {
				log.Println("Erreur lors de l'envoi de 'unknown':", err)
				continue
			}
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
	// Le defer conn.Close() s'occupera de la fermeture
}

func Getclient(line string, splitGET []string, conn net.Conn, writer *bufio.Writer, reader *bufio.Reader) {
	listeMessage = append(listeMessage, "sent message :", line, "\n")
	if err := p.Send_message(writer, line); err != nil {
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
		err = os.WriteFile(splitGET[1], []byte(data), 770)
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
	context.TODO()
}

func ListClient(writer *bufio.Writer, reader *bufio.Reader) {
	listeMessage = append(listeMessage, "sent message :", "List \n")
	if err := p.Send_message(writer, "List"); err != nil {
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
		// Le serveur va envoyer le fichier
		// Lire tout le contenu
		listeMessage = append(listeMessage, "sent message :", "OK \n")
		if err := p.Send_message(writer, "OK"); err != nil {
			log.Println("Erreur lors de l'envoi de 'OK':", err)
			return
		}
		data, err := p.Receive_message(reader)
		listeMessage = append(listeMessage, "received message :", data)
		if err != nil {
			log.Println("Erreur lors de la lecture du fichier:", err)
			return
		}
		var datas = strings.Split(data, "--")
		for newdata := range datas {
			log.Println(datas[newdata])
		}
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
	for {
		rep, err := p.Receive_message(reader)
		listeMessage = append(listeMessage, "received message :", rep)
		if err != nil {
			log.Println("Erreur lors de la réception de la réponse:", err)
			return
		}
		rep = strings.TrimSpace(rep)
		if rep == "Terminaison finie, le serveur s'éteint" {
			log.Println(rep)
			return
		} else {
			log.Println(rep)
		}
	}
}
