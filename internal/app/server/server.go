package server

import (
	"bufio"
	"log"
	"log/slog"
	"net"
	"os"
	"path/filepath"
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
		log.Println(cleanedMsg)

		// Client dit "GET ..."
		var commGet = strings.Split(cleanedMsg, " ")
		if len(commGet) == 2 && commGet[0] == "GET" {
			log.Println("Commande GET reçue pour:", commGet[1])

			var fichiers, err = os.ReadDir("Docs")
			if err != nil {
				log.Fatal(err)
			}
			var found = false

			for _, fichier := range fichiers {
				if commGet[1] == fichier.Name() {
					found = true
					log.Println("Fichier trouvé:", fichier.Name())
					var path = filepath.Join("Docs", fichier.Name())

					// envoie du start
					if err := p.Send_message(writer, "Start"); err != nil {
						log.Println("Erreur lors de l'envoi de 'Start':", err)
						return
					}
					// lecture du contenu
					var data, err = os.ReadFile(path)
					if err != nil {
						log.Println("Ne peut pas lire le contenu du fichier :", err)
						return
					}
					// transfert du fichier
					_, err = conn.Write(data)
					if err != nil {
						log.Println("N'a pas pû transférer le fichier :", err)
					}
				}
			}
			// gestion du FileUnknown
			if !found {
				log.Println("Fichier non trouvé:", commGet[1]) // ← Log
				if err := p.Send_message(writer, "FileUnknown"); err != nil {
					log.Println("Erreur lors de l'envoi de 'FileUnknown':", err)
					return
				}
			}
			// ok du client
			var response, _ = p.Receive_message(reader)
			log.Println("Réponse du client:", response)
			continue

		} else if cleanedMsg == "start" { // ← Remplace le switch par des else if
			if err := p.Send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
				return
			}
		} else if cleanedMsg == "data" {
			if err := p.Send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'data':", err)
				return
			}
		} else if cleanedMsg == "end" {
			if err := p.Send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
			}
			return
		} else {
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
