package client

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

func Run(remote string) {

	c, err := net.Dial("tcp", remote)
	if err != nil {
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
	if err != nil {
		log.Println("Erreur lors de la réception de 'hello' ou déconnexion:", err)
		return
	}
	if strings.TrimSpace(msg) != "hello" {
		log.Println("Protocole échoué : Attendu 'hello', reçu:", strings.TrimSpace(msg))
		return
	}

	// Étape 2 : Le client répond "start"
	if err := p.Send_message(writer, "start"); err != nil {
		log.Println("Erreur lors de l'envoi de 'start':", err)
		return
	}

	// Étape 3 : Attendre le message "ok" du serveur
	msg, err = p.Receive_message(reader)
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

		var splitGET = strings.Split(line, " ")

		// Le client se déconnecte
		if strings.ToUpper(splitGET[0]) == "END" {
			break
			// Le client envoie une commande GET
		} else if strings.ToUpper(splitGET[0]) == "GET" {
			Getclient(line, splitGET, conn, writer, reader)
		} else if strings.ToUpper(splitGET[0]) == "LIST" {
			ListClient(line, splitGET, conn, writer, reader)
		}
	}

	// Étape 5 : Le client répond "end"
	if err := p.Send_message(writer, "end"); err != nil {
		log.Println("Erreur lors de l'envoi de 'end':", err)
		return
	}

	// Étape 6 : Attendre le message "ok" final du serveur
	msg, err = p.Receive_message(reader)
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
	if err := p.Send_message(writer, line); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}
	// Attend la réponse du serveur
	var response, err = p.Receive_message(reader)
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
		if err := p.Send_message(writer, "OK"); err != nil {
			log.Println("Erreur lors de l'envoi de 'OK':", err)
			return
		}

	} else if response == "Start" {
		// Le serveur va envoyer le fichier
		// Lire tout le contenu
		data, err := p.Receive_message(reader)
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
		if err := p.Send_message(writer, "OK"); err != nil {
			log.Println("Erreur lors de l'envoi de 'OK':", err)
			return
		}
	} else {
		log.Println("Réponse inattendue du serveur:", response)
	}
}

func ListClient(line string, splitGET []string, conn net.Conn, writer *bufio.Writer, reader *bufio.Reader) {
	if err := p.Send_message(writer, line); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}
	// Attend la réponse du serveur
	var response, err = p.Receive_message(reader)
	if err != nil {
		log.Println("Erreur lors de la réception de la réponse:", err)
		return
	}
	response = strings.TrimSpace(response)
	log.Println(response)

	if response == "Start" {
		// Le serveur va envoyer le fichier
		// Lire tout le contenu
		if err := p.Send_message(writer, "OK"); err != nil {
			log.Println("Erreur lors de l'envoi de 'OK':", err)
			return
		}
		data, err := p.Receive_message(reader)
		if err != nil {
			log.Println("Erreur lors de la lecture du fichier:", err)
			return
		}
		var datas = strings.Split(data, "--")
		for newdata := range datas {
			log.Println(datas[newdata])
		}
	}
}
