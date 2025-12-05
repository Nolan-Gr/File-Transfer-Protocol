package client

import (
	"bufio"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"net"
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

	// Étape 4 & Répétition: Tirer un nombre aléatoire N et envoyer N messages "data",
	// en attendant "ok" à chaque fois.
	n := randomInt(1, 5)
	log.Printf("Le client va envoyer %d messages 'data'.", n)

	for i := 0; i < n; i++ {
		// Le client envoie "data"
		if err := p.Send_message(writer, "data"); err != nil {
			log.Println("Erreur lors de l'envoi de 'data':", err)
			return
		}

		// Attendre la réponse "ok" du serveur
		msg, err = p.Receive_message(reader)
		if err != nil {
			log.Println("Erreur lors de la réception de 'ok' (après data) ou déconnexion:", err)
			return
		}
		if strings.TrimSpace(msg) != "ok" {
			log.Println("Protocole échoué : Attendu 'ok' (après data), reçu:", strings.TrimSpace(msg))
			return
		}
	}

	// Étape 6 : Le client répond "end"
	if err := p.Send_message(writer, "end"); err != nil {
		log.Println("Erreur lors de l'envoi de 'end':", err)
		return
	}

	// Étape 7 : Attendre le message "ok" final du serveur
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

func randomInt(i int, i2 int) int {
	return rand.Intn(i2-i+1) + i
}
