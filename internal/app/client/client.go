package client

import (
	"log/slog"
	"net"
	"bufio"
	"log"
	"os"
	"strings"
	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

func Run(remote string) {

	c, e := net.Dial("tcp", remote)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	defer func() {
		c.Close()
		slog.Debug("Connection closed")
	}()
	slog.Info("Connected to " + c.RemoteAddr().String())

	return
}


func runClient(conn net.Conn) {
	defer conn.Close()
	log.Println("Connecté au serveur:", conn.RemoteAddr().String())

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	
	// Étape 1 : Attendre le message "hello" du serveur
	msg, err := receive_message(reader)
	if err != nil {
		log.Println("Erreur lors de la réception de 'hello' ou déconnexion:", err)
		return
	}
	if strings.TrimSpace(msg) != "hello" {
		log.Println("Protocole échoué : Attendu 'hello', reçu:", strings.TrimSpace(msg))
		return
	}

	// Étape 2 : Le client répond "start"
	if err := send_message(writer, "start"); err != nil {
		log.Println("Erreur lors de l'envoi de 'start':", err)
		return
	}
	
	// Étape 3 : Attendre le message "ok" du serveur
	msg, err = receive_message(reader)
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
		if err := send_message(writer, "data"); err != nil {
			log.Println("Erreur lors de l'envoi de 'data':", err)
			return
		}

		// Attendre la réponse "ok" du serveur
		msg, err = receive_message(reader)
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
	if err := send_message(writer, "end"); err != nil {
		log.Println("Erreur lors de l'envoi de 'end':", err)
		return
	}

	// Étape 7 : Attendre le message "ok" final du serveur
	msg, err = receive_message(reader)
	if err != nil {
		// La déconnexion immédiate du serveur après l'envoi du "ok" est possible
		if err != net.ErrClosed && err != os.EOF {
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

func main() {
	// Tenter de se connecter au serveur
	conn, err := net.Dial("tcp", SERVER_ADDR)
	if err != nil {
		log.Fatal("Erreur de connexion au serveur:", err)
	}
	
	runClient(conn)
}