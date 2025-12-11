package server

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

var connectiontime time.Time

// Canal qui sert de compteur pour savoir combien de clients sont connectés
var compteurClient = make(chan int, 1)

// Canal qui sert de compteur pour savoir combien d'operations sont en cours
var compteurOperations = make(chan int, 1)

// Boolean pour la terminaison du serveur
var terminaisonDuServeur bool = false

type ClientIO struct {
	writer *bufio.Writer
	reader *bufio.Reader
}

var clientTerminant ClientIO

var listeMessage = []string{"Historique des messages : \n"}

var listeValues = []string{}

// Lance le serveur sur le port donné
func RunServer(port *string, controlPort *string) {
	compteurClient <- 0
	compteurOperations <- 0
	listeValues = append(listeValues, *port, *controlPort)

	go runNormalServer(port) // Lancer le serveur de base en goroutine

	runControlServer(controlPort) // Lance le serveur contrôle pour avoir qu'une seule connexion à la fois
}

func runNormalServer(port *string) {
	// Ouvre le serveur sur le port
	l, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	connectiontime = time.Now()

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
		if terminaisonDuServeur {
			slog.Info("Server is terminating, no longer accepting new connections.")
			break
		}
		// Attend qu'un client se connecte
		c, err := l.Accept()
		if err != nil {
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String() + " on port " + *port)

		// Lance la gestion du client en parallèle pour pas bloquer
		go HandleClient(c)
	}
	TerminateServerP2(clientTerminant.writer, clientTerminant.reader)
	return
}

func runControlServer(controlPort *string) {
	// Ouvre le serveur sur le port
	l, err := net.Listen("tcp", ":"+*controlPort)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	connectiontime = time.Now()

	// Ferme le serveur proprement quand la fonction se termine
	defer func() {
		err := l.Close()
		if err != nil {
			return
		}
		slog.Debug("Stopped listening on port " + *controlPort)
	}()
	slog.Debug("Now listening on port " + *controlPort)

	// Boucle infinie qui attend les connexions
	for {
		if terminaisonDuServeur {
			slog.Info("Server is terminating, no longer accepting new connections.")
			break
		}
		// Attend qu'un client se connecte
		c, err := l.Accept()
		if err != nil {
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String())

		// Lance la gestion du client en parallèle pour pas bloquer
		HandleControlClient(c)
	}
	TerminateServerP2(clientTerminant.writer, clientTerminant.reader)
	return
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
	log.Println("adresse IP du nouveau client :", conn.RemoteAddr().String(), " connecté le : ", time.Now(), " connecté sur le port ", "3333")

	// Prépare la lecture et l'écriture des messages
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Envoie "hello" au client pour commencer
	listeMessage = append(listeMessage, "sent message :", "hello \n")
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

		// Cas ou la requet est en plusieurs parties
		var commGet = strings.Split(cleanedMsg, " ")

		// le message start (global)
		if !terminaisonDuServeur {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				listeMessage = append(listeMessage, "sent message :", "ok \n")
				if err := p.Send_message(writer, "ok"); err != nil {
					log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
					return
				}

				// LIST
			} else if cleanedMsg == "List" {
				incrementeCompteurOperations()
				log.Println("Commande LIST reçue")
				ListServer(writer, reader)
				var fichier, err = os.ReadDir("Docs")
				log.Println("fichier :", fichier)
				if err != nil {
					log.Println(err)
				}
				var liste = ParcourFolder(fichier, " ", 0)
				log.Println(liste)
				decrementeCompteurOperations()

				// GET
			} else if len(commGet) == 2 && commGet[0] == "GET" {
				incrementeCompteurOperations()
				log.Println("Commande GET reçue pour:", commGet[1])
				Getserver(commGet, writer, reader)
				decrementeCompteurOperations()

				// TERMINATE
			} else if cleanedMsg == "Terminate" {
				log.Println("Commande TERMINATE reçue")
				TerminateServerP1(writer, reader)

				// UNKNOWN
			} else if cleanedMsg == "Unknown" {
				log.Println("Commande inconnue. Veuillez entrer GET, LIST, TERMINATE ou END.")
				continue

				//END ou message inattendu
			} else if cleanedMsg == "end" {
				listeMessage = append(listeMessage, "sent message :", "ok \n")
				if err := p.Send_message(writer, "ok"); err != nil {
					log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
				}
				return
			} else if cleanedMsg == "messages" {
				fmt.Println(strings.Trim(fmt.Sprint(listeMessage), "[]"))
			} else {
				// Message inconnu : log, informer le client et continuer la connexion
				log.Println("Message inattendu du client:", cleanedMsg)
				continue
			}
		} else {
			listeMessage = append(listeMessage, "sent message :", "ok \n")
			// Si le serveur est en terminaison, on informe le client et on ferme la connexion
			if err := p.Send_message(writer, "Server terminating, connection closing."); err != nil {
				log.Println("Erreur lors de l'envoi du message de terminaison:", err)
			}
			continue
		}

		//msg, err = p.Receive_message(reader)
		log.Println("message1")
		//if err != nil {
		//	log.Println("Erreur lors de la réception de la réponse:", err)
		//	return
		//}
		// pour le mode debug
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			log.Println("debug ")
			DebugServer(writer, reader)
		}
	}
}

func HandleControlClient(conn net.Conn) {
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
	listeMessage = append(listeMessage, "sent message :", "hello \n")
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

		// Cas ou la requet est en plusieurs parties
		var commGet = strings.Split(cleanedMsg, " ")

		// le message start (global)
		if !terminaisonDuServeur {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				listeMessage = append(listeMessage, "sent message :", "ok \n")
				if err := p.Send_message(writer, "ok"); err != nil {
					log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
					return
				}

				// LIST
			} else if cleanedMsg == "List" {
				incrementeCompteurOperations()
				log.Println("Commande LIST reçue")
				ListServer(writer, reader)
				decrementeCompteurOperations()

				// GET
			} else if len(commGet) == 2 && commGet[0] == "GET" {
				incrementeCompteurOperations()
				log.Println("Commande GET reçue pour:", commGet[1])
				Getserver(commGet, writer, reader)
				decrementeCompteurOperations()

				// TERMINATE
			} else if cleanedMsg == "Terminate" {
				log.Println("Commande TERMINATE reçue")
				TerminateServerP1(writer, reader)

				// UNKNOWN
			} else if cleanedMsg == "Unknown" {
				log.Println("Commande inconnue. Veuillez entrer GET, LIST, TERMINATE ou END.")
				continue

				//END ou message inattendu
			} else if cleanedMsg == "end" {
				listeMessage = append(listeMessage, "sent message :", "ok \n")
				if err := p.Send_message(writer, "ok"); err != nil {
					log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
				}
				return
			} else if cleanedMsg == "messages" {
				log.Println(listeMessage)
			} else {
				// Message inconnu : log, informer le client et continuer la connexion
				log.Println("Message inattendu du client:", cleanedMsg)
				continue
			}
		} else {
			listeMessage = append(listeMessage, "sent message :", "ok \n")
			// Si le serveur est en terminaison, on informe le client et on ferme la connexion
			if err := p.Send_message(writer, "Server terminating, connection closing."); err != nil {
				log.Println("Erreur lors de l'envoi du message de terminaison:", err)
			}
			continue
		}

		//msg, err = p.Receive_message(reader)
		log.Println("message1")
		//if err != nil {
		//	log.Println("Erreur lors de la réception de la réponse:", err)
		//	return
		//}
		// pour le mode debug
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			log.Println("debug ")
			DebugServer(writer, reader)
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

func Getserver(commGet []string, writer *bufio.Writer, reader *bufio.Reader) {
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
			listeMessage = append(listeMessage, "sent message :", "Start \n")
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
			listeMessage = append(listeMessage, "sent message :", string(data), "\n")
			err = p.Send_message(writer, string(data))
			if err != nil {
				log.Println("N'a pas pû transférer le fichier :", err)
			}
		}
	}
	// gestion du FileUnknown
	if !found {
		log.Println("Fichier non trouvé:", commGet[1]) // ← Log
		listeMessage = append(listeMessage, "sent message :", "FileUnknown \n")
		if err := p.Send_message(writer, "FileUnknown"); err != nil {
			log.Println("Erreur lors de l'envoi de 'FileUnknown':", err)
			return
		}
	}
	// ok du client
	var response, _ = p.Receive_message(reader)
	log.Println("Réponse du client:", response)
}

func ListServer(writer *bufio.Writer, reader *bufio.Reader) {
	var fichiers, err = os.ReadDir("Docs")
	var list = ""
	var size = 0
	listeMessage = append(listeMessage, "sent message :", "Start \n")
	if err := p.Send_message(writer, "Start"); err != nil {
		log.Println("Erreur lors de l'envoi de 'Start':", err)
		return
	}
	log.Println(fichiers)
	data, err := p.Receive_message(reader)
	log.Println("data:", data)
	if err != nil {
		log.Println("Erreur lors de la lecture du fichier:", err)
		return
	} else if strings.TrimSpace(data) == "OK" {
		for _, fichier := range fichiers {
			fileInfo, err := fichier.Info()
			if err != nil {
				log.Println("Erreur lors de la lecture du fichier:", err)
				return
			}
			list = list + " --" + fichier.Name() + " " + strconv.FormatInt(int64(fileInfo.Size()), 10)
			size = size + 1
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	var newlist = "FileCnt : " + strconv.Itoa(size) + list
	log.Println(newlist)
	listeMessage = append(listeMessage, "sent message :", newlist, "\n")
	p.Send_message(writer, newlist)

}

func ParcourFolder(fichiers []os.DirEntry, list string, size int) string {
	for _, fichier := range fichiers {
		fileInfo, err := fichier.Info()
		if err != nil {
			log.Println("Erreur lors de la lecture du fichier:", err)
			return err.Error()
		}
		size = size + 1

		if fichier.IsDir() {
			log.Println(filepath.Join("Docs/", fichier.Name()))
			var newfichiers, err = os.ReadDir(filepath.Join("Docs/", fichier.Name()))
			if err != nil {
				log.Fatal(err)
			}
			var liste = ParcourFolder(newfichiers, list, size)
			list = list + " --" + fichier.Name() + " " + strconv.FormatInt(int64(fileInfo.Size()), 10) + " -- sous-dossier: " + " [" + liste + "]"
		} else {
			list = list + " --" + fichier.Name() + " " + strconv.FormatInt(int64(fileInfo.Size()), 10)
		}
	}
	return list
}

func DebugServer(writer *bufio.Writer, reader *bufio.Reader) {
	nbOperation := <-compteurOperations
	nbClient := <-compteurClient
	var list = []string{"Debug :", "Serveur créé à", slog.AnyValue(connectiontime).String(), "nombre d'opérations en cours :", strconv.FormatInt(int64(nbOperation), 10), "nombre de clients connectés :", strconv.FormatInt(int64(nbClient), 10)}
	slog.Debug(list[0])
	slog.Debug(list[1] + " " + list[2])
	slog.Debug(list[3] + " " + list[4])
	slog.Debug(list[5] + " " + list[6])
	compteurClient <- nbClient
	compteurOperations <- nbOperation
	listeMessage = append(listeMessage, "sent message :", " debug \n")
	//if err := p.Send_message(writer, "debug"); err != nil {
	//	log.Println("Erreur lors de l'envoi de la commande:", err)
	//	return
	//}
	//var msg, err = p.Receive_message(reader)
	//log.Println(msg)
	//if err != nil {
	//	log.Println("Erreur lors de la réception de la réponse:", err)
	//	return
	//}
	var newlist = strings.Join(list, "--")
	log.Println(newlist)
	//if msg == "ok" {
	//	p.Send_message(writer, newlist)
	//}
}

func incrementeCompteurOperations() {
	nbOperation := <-compteurOperations // Récupère le nombre actuel
	nbOperation++                       // Ajoute 1
	log.Println("nombre d'operations en cours': ", nbOperation)
	compteurOperations <- nbOperation // Remet le nouveau nombre
}

func decrementeCompteurOperations() {
	nbOperation := <-compteurOperations // Récupère le nombre actuel
	nbOperation--                       // Enlève 1
	log.Println("nombre d'operations en cours': ", nbOperation)
	compteurOperations <- nbOperation // Remet le nouveau nombre
}

func getnbcompteur(compteur chan int) int {
	nbOperation := <-compteur // Récupère le nombre actuel
	compteur <- nbOperation   // Remet le nouveau nombre
	return nbOperation
}

func TerminateServerP1(writer *bufio.Writer, reader *bufio.Reader) {
	// Met le boolean de terminaison à true
	terminaisonDuServeur = true
	// Pour ajouter un client :
	//clientTerminant = append(clientTerminant, ClientIO{
	//	reader,writer})
	clientTerminant.reader = reader
	clientTerminant.writer = writer
}

func TerminateServerP2(writer *bufio.Writer, reader *bufio.Reader) {

	// Informe le client que la terminaison est en cours
	nbOperation := getnbcompteur(compteurOperations)
	for nbOperation > 0 {
		time.Sleep(1 * time.Second)
		nbOperation = getnbcompteur(compteurOperations)
		message := "il reste " + strconv.Itoa(nbOperation) + " opérations en cours, attente de la fin..."
		listeMessage = append(listeMessage, "sent message :", message, "\n")
		if err := p.Send_message(writer, message); err != nil {
			log.Println("Erreur lors de l'envoi de la commande:", err)
			return
		}
	}
	listeMessage = append(listeMessage, "sent message :", "Terminaison finie, le serveur s'éteint \n")
	if err := p.Send_message(writer, "Terminaison finie, le serveur s'éteint"); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return
	}
}

func tree(writer *bufio.Writer, reader *bufio.Reader) {
	var fichiers, err = os.ReadDir("Docs")
	var list = ""
	var size = 0
	listeMessage = append(listeMessage, "sent message :", "Start \n")
	if err := p.Send_message(writer, "Start"); err != nil {
		log.Println("Erreur lors de l'envoi de 'Start':", err)
		return
	}
	log.Println(fichiers)
	data, err := p.Receive_message(reader)
	log.Println("data:", data)
	if err != nil {
		log.Println("Erreur lors de la lecture du fichier:", err)
		return
	} else if strings.TrimSpace(data) == "OK" {
		//list, size = ParcourFolder(fichiers, list, size)
		if err != nil {
			log.Fatal(err)
		}
	}
	var newlist = "FileCnt : " + strconv.Itoa(size) + list
	log.Println(newlist)
	listeMessage = append(listeMessage, "sent message :", newlist, "\n")
	p.Send_message(writer, newlist)
}
