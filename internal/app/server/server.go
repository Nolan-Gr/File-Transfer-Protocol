package server

import (
	"bufio"
	"log"
	"log/slog"
	"net"
	"strings"
	"time"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// Canal qui sert de compteur pour savoir combien de clients sont connectés
var compteurClient = make(chan int, 1)

// Lance le serveur sur le port donné
func RunServer(port *string) {
	compteurClient <- 0

	// Ouvre le serveur sur le port
	l, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	// Ferme le serveur proprement quand la fonction se termine
	defer func() {
		err := l.Close()
		if err != nil {
			return
		}
		slog.Debug("Stopped listening on port " + *port)
	}()
	slog.Debug("Now listening on port " + *port)

	// Boucle infinie qui attend les connexions
	for {
		// Attend qu'un client se connecte
		c, err := l.Accept()
		if err != nil {
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String())

		// Lance la gestion du client en parallèle pour pas bloquer
		go HandleClient(c)
	}
}

// Gère un client
func HandleClient(conn net.Conn) {
	// À la fin de la fonction, on déconnecte le client
	defer ClientLogOut(conn)

	taille := <-compteurClient // Récupère le nombre actuel
	taille++                   // Ajoute 1
	log.Println("nombre de client : ", taille)
	compteurClient <- taille // Remet le nouveau nombre

	// Affiche qui s'est connecté et quand
	log.Println("adresse IP du nouveau client :", conn.RemoteAddr().String(), " connecté le : ", time.Now())

	// Prépare la lecture et l'écriture des messages
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Envoie "hello" au client pour commencer
	if err := p.Send_message(writer, "hello"); err != nil {
		log.Println("Erreur lors de l'envoi de 'hello':", err)
		return
	}

	// Boucle qui attend les messages du client
	for {
		// Lit le message du client
		msg, err := p.Receive_message(reader)
		if err != nil {
			// Si erreur = client déco
			log.Println("Client déconnecté ou erreur de lecture:", err)
			return
		}

		// Enlève les espaces dans le message
		cleanedMsg := strings.TrimSpace(msg)

		// Regarde ce que le client a envoyé
		switch cleanedMsg {
		case "start":
			// Client dit "start" -> on répond "ok"
			if err := p.Send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
				return
			}

		case "data":
			// Client envoie des données -> on répond "ok"
			if err := p.Send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'data':", err)
				return
			}

		case "end":
			// Client dit "end" -> on répond "ok" et on arrête
			if err := p.Send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
			}
			return

		default:
			// Le client a envoyé un truc bizarre -> on arrête
			log.Println("Message inattendu du client:", cleanedMsg)
			return
		}
	}
}

// Déconnecte le client
func ClientLogOut(conn net.Conn) {
	taille := <-compteurClient // Récupère le nombre actuel
	taille--                   // Enlève 1
	log.Println("nombre de client : ", taille)
	compteurClient <- taille // Remet le nouveau nombre

	// Affiche qui s'est déconnecté et quand
	log.Println("adresse IP du client : ", conn.RemoteAddr().String(), " déconnecté le : ", time.Now())

	// Ferme la connexion
	err := conn.Close()
	if err != nil {
		return
	}
}
