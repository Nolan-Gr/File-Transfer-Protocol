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
	"sync"
	"time"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

var connectiontime time.Time

// Utilisation de sync.Mutex pour les compteurs au lieu de canaux
var (
	compteurClient     int
	compteurOperations int
	compteurMutex      sync.Mutex
)

// Canal pour signaler la terminaison (utilisé avec sync.Once pour éviter double close)
var shutdownChan = make(chan struct{})
var shutdownOnce sync.Once

// Boolean pour la terminaison du serveur
var terminaisonDuServeur bool = false
var terminaisonMutex sync.RWMutex

type ClientIO struct {
	writer *bufio.Writer
	reader *bufio.Reader
}

var (
	clientTerminant      ClientIO
	clientTerminantMutex sync.Mutex
)

var (
	listeMessage      = []string{"Historique des messages : \n"}
	listeMessageMutex sync.Mutex
)

var listeValues = []string{}

// WaitGroup pour attendre la fermeture des serveurs
var serverWg sync.WaitGroup

// Fonctions helper pour manipuler les compteurs de façon thread-safe
func incrementerClient() int {
	compteurMutex.Lock()
	defer compteurMutex.Unlock()
	compteurClient++
	return compteurClient
}

func decrementerClient() int {
	compteurMutex.Lock()
	defer compteurMutex.Unlock()
	compteurClient--
	return compteurClient
}

func incrementerOperations() int {
	compteurMutex.Lock()
	defer compteurMutex.Unlock()
	compteurOperations++
	return compteurOperations
}

func decrementerOperations() int {
	compteurMutex.Lock()
	defer compteurMutex.Unlock()
	compteurOperations--
	return compteurOperations
}

func getCompteurOperations() int {
	compteurMutex.Lock()
	defer compteurMutex.Unlock()
	return compteurOperations
}

func getCompteurClient() int {
	compteurMutex.Lock()
	defer compteurMutex.Unlock()
	return compteurClient
}

func isServerShuttingDown() bool {
	terminaisonMutex.RLock()
	defer terminaisonMutex.RUnlock()
	return terminaisonDuServeur
}

func setServerShuttingDown() {
	terminaisonMutex.Lock()
	defer terminaisonMutex.Unlock()
	terminaisonDuServeur = true
}

func addToListeMessage(messages ...string) {
	listeMessageMutex.Lock()
	defer listeMessageMutex.Unlock()
	listeMessage = append(listeMessage, messages...)
}

// Lance le serveur sur le port donné
func RunServer(port *string, controlPort *string) {
	listeValues = append(listeValues, *port, *controlPort)

	serverWg.Add(2)
	go runNormalServer(port)
	go runControlServer(controlPort)

	// Attendre que les deux serveurs se terminent
	serverWg.Wait()
	log.Println("Tous les serveurs sont arrêtés")
}

func runNormalServer(port *string) {
	defer serverWg.Done()

	l, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	connectiontime = time.Now()

	defer func() {
		err := l.Close()
		if err != nil {
			return
		}
		slog.Debug("Stopped listening on port " + *port)
	}()
	slog.Debug("Now listening on port " + *port)

	// Goroutine pour fermer le listener quand shutdownChan est fermé
	go func() {
		<-shutdownChan
		l.Close()
	}()

	for {
		c, err := l.Accept()
		if err != nil {
			// Vérifier si c'est une erreur due à la fermeture du listener
			if isServerShuttingDown() {
				slog.Info("Server normal terminé, arrêt des nouvelles connexions")
				return
			}
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String() + " on port " + *port)
		go HandleClient(c)
	}
}

func runControlServer(controlPort *string) {
	defer serverWg.Done()

	l, err := net.Listen("tcp", ":"+*controlPort)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	defer func() {
		err := l.Close()
		if err != nil {
			return
		}
		slog.Debug("Stopped listening on port " + *controlPort)
	}()
	slog.Debug("Now listening on port " + *controlPort)

	// Goroutine pour fermer le listener quand shutdownChan est fermé
	go func() {
		<-shutdownChan
		l.Close()
	}()

	for {
		c, err := l.Accept()
		if err != nil {
			if isServerShuttingDown() {
				slog.Info("Server de contrôle terminé, arrêt des nouvelles connexions")
				return
			}
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String())
		HandleControlClient(c)
	}
}

// Gère un client
func HandleClient(conn net.Conn) {
	defer ClientLogOut(conn)

	taille := incrementerClient()
	log.Println("nombre de client : ", taille)

	log.Println("adresse IP du nouveau client :", conn.RemoteAddr().String(), " connecté le : ", time.Now(), " connecté sur le port ", "3333")

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	addToListeMessage("sent message :", "hello \n")
	if err := p.Send_message(writer, "hello"); err != nil {
		log.Println("Erreur lors de l'envoi de 'hello':", err)
		return
	}

	for {
		msg, err := p.Receive_message(reader)
		if err != nil {
			log.Println("Client déconnecté ou erreur de lecture:", err)
			return
		}

		cleanedMsg := strings.TrimSpace(msg)
		log.Println(cleanedMsg)

		var commGet = strings.Split(cleanedMsg, " ")

		if !isServerShuttingDown() {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				addToListeMessage("sent message :", "ok \n")
				if err := p.Send_message(writer, "ok"); err != nil {
					log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
					return
				}

			} else if cleanedMsg == "List" {
				nbOp := incrementerOperations()
				log.Println("Commande LIST reçue, opérations en cours:", nbOp)
				ListServer(writer, reader)
				nbOp = decrementerOperations()
				log.Println("Commande LIST terminée, opérations restantes:", nbOp)

			} else if len(commGet) == 2 && commGet[0] == "GET" {
				nbOp := incrementerOperations()
				log.Println("Commande GET reçue pour:", commGet[1], ", opérations en cours:", nbOp)
				Getserver(commGet, writer, reader)
				nbOp = decrementerOperations()
				log.Println("Commande GET terminée, opérations restantes:", nbOp)

			} else if cleanedMsg == "Unknown" {
				addToListeMessage("sent message :", "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande. \n")
				if err := p.Send_message(writer, "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande."); err != nil {
					log.Println("Erreur lors de l'envoi du message de terminaison:", err)
				}
				log.Println("Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande.")

			} else if cleanedMsg == "Help" {
				helpMessage := "Commandes disponibles : LIST, GET <filename>, HELP, END"
				addToListeMessage("sent message :", helpMessage+" \n")
				if err := p.Send_message(writer, helpMessage); err != nil {
					log.Println("Erreur lors de l'envoi de 'help':", err)
					return
				}

			} else if cleanedMsg == "end" {
				addToListeMessage("sent message :", "ok \n")
				if err := p.Send_message(writer, "ok"); err != nil {
					log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
				}
				return
			} else if cleanedMsg == "messages" {
				listeMessageMutex.Lock()
				fmt.Println(strings.Trim(fmt.Sprint(listeMessage), "[]"))
				listeMessageMutex.Unlock()
			} else {
				log.Println("Message inattendu du client:", cleanedMsg)
				continue
			}
		} else {
			addToListeMessage("sent message :", "ok \n")
			if err := p.Send_message(writer, "Server terminating, connection closing."); err != nil {
				log.Println("Erreur lors de l'envoi du message de terminaison:", err)
			}
			return
		}

		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			log.Println("debug ")
			DebugServer(writer, reader)
		}
	}
}

func HandleControlClient(conn net.Conn) {
	defer ClientLogOut(conn)

	taille := incrementerClient()
	log.Println("nombre de client : ", taille)

	log.Println("adresse IP du nouveau client :", conn.RemoteAddr().String(), " connecté le : ", time.Now())

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	addToListeMessage("sent message :", "hello \n")
	if err := p.Send_message(writer, "hello"); err != nil {
		log.Println("Erreur lors de l'envoi de 'hello':", err)
		return
	}

	for {
		msg, err := p.Receive_message(reader)
		if err != nil {
			log.Println("Client déconnecté ou erreur de lecture:", err)
			return
		}

		cleanedMsg := strings.TrimSpace(msg)
		log.Println(cleanedMsg)

		var commHideReveal = strings.Split(cleanedMsg, " ")

		if !isServerShuttingDown() {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				addToListeMessage("sent message :", "ok \n")
				if err := p.Send_message(writer, "ok"); err != nil {
					log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
					return
				}

			} else if cleanedMsg == "List" {
				nbOp := incrementerOperations()
				log.Println("Commande LIST reçue, opérations en cours:", nbOp)
				ListServer(writer, reader)
				nbOp = decrementerOperations()
				log.Println("Commande LIST terminée, opérations restantes:", nbOp)

			} else if cleanedMsg == "Terminate" {
				log.Println("Commande TERMINATE reçue")
				clientTerminantMutex.Lock()
				clientTerminant.reader = reader
				clientTerminant.writer = writer
				clientTerminantMutex.Unlock()
				// Lancer la terminaison dans une goroutine
				go TerminateServer()
				// Attendre que le serveur se termine
				<-shutdownChan
				return

			} else if cleanedMsg == "Unknown" {
				addToListeMessage("sent message :", "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande. \n")
				if err := p.Send_message(writer, "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande."); err != nil {
					log.Println("Erreur lors de l'envoi du message de terminaison:", err)
				}
				log.Println("Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande.")

			} else if cleanedMsg == "Help" {
				helpMessage := "Commandes disponibles : LIST, HIDE, REVEAL, HELP, END et TERMINATE	"
				addToListeMessage("sent message :", helpMessage+" \n")
				if err := p.Send_message(writer, helpMessage); err != nil {
					log.Println("Erreur lors de l'envoi de 'help':", err)
					return
				}

			} else if len(commHideReveal) == 2 && commHideReveal[0] == "HIDE" {
				nbOp := incrementerOperations()
				log.Println("Commande HIDE reçue, opérations en cours:", nbOp)
				HIDE(commHideReveal, writer, reader)
				nbOp = decrementerOperations()
				log.Println("Commande HIDE terminée, opérations restantes:", nbOp)

			} else if cleanedMsg == "end" {
				addToListeMessage("sent message :", "ok \n")
				if err := p.Send_message(writer, "ok"); err != nil {
					log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
				}
				return

			} else if cleanedMsg == "messages" {
				listeMessageMutex.Lock()
				log.Println(listeMessage)
				listeMessageMutex.Unlock()

			} else {
				log.Println("Message inattendu du client:", cleanedMsg)
				continue
			}
		} else {
			addToListeMessage("sent message :", "ok \n")
			if err := p.Send_message(writer, "Server terminating, connection closing."); err != nil {
				log.Println("Erreur lors de l'envoi du message de terminaison:", err)
			}
			return
		}

		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			log.Println("debug ")
			DebugServer(writer, reader)
		}
	}
}

// Déconnecte le client
func ClientLogOut(conn net.Conn) {
	taille := decrementerClient()
	log.Println("nombre de client : ", taille)

	log.Println("adresse IP du client : ", conn.RemoteAddr().String(), " déconnecté le : ", time.Now())

	err := conn.Close()
	if err != nil {
		return
	}
}

func Getserver(commGet []string, writer *bufio.Writer, reader *bufio.Reader) {
	var fichiers, err = os.ReadDir("Docs")
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return
	}
	var found = false

	for _, fichier := range fichiers {
		if commGet[1] == fichier.Name() {
			found = true
			log.Println("Fichier trouvé:", fichier.Name())
			var path = filepath.Join("Docs", fichier.Name())

			addToListeMessage("sent message :", "Start \n")
			if err := p.Send_message(writer, "Start"); err != nil {
				log.Println("Erreur lors de l'envoi de 'Start':", err)
				return
			}
			var data, err = os.ReadFile(path)
			if err != nil {
				log.Println("Ne peut pas lire le contenu du fichier :", err)
				return
			}
			addToListeMessage("sent message :", string(data), "\n")
			err = p.Send_message(writer, string(data))
			if err != nil {
				log.Println("N'a pas pû transférer le fichier :", err)
			}
			break
		}
	}
	if !found {
		log.Println("Fichier non trouvé:", commGet[1])
		addToListeMessage("sent message :", "FileUnknown \n")
		if err := p.Send_message(writer, "FileUnknown"); err != nil {
			log.Println("Erreur lors de l'envoi de 'FileUnknown':", err)
			return
		}
	}
	var response, _ = p.Receive_message(reader)
	log.Println("Réponse du client:", response)
}

func HIDE(commHideReveal []string, writer *bufio.Writer, reader *bufio.Reader) {
	var fichiers, err = os.ReadDir("Docs")
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return
	}
	var found = false

	for _, fichier := range fichiers {
		if commHideReveal[1] == fichier.Name() {
			found = true
			log.Println("Fichier trouvé:", fichier.Name())
			var oldPath = filepath.Join("Docs", fichier.Name())
			var newPath = filepath.Join("Docs", "."+fichier.Name())

			err := os.Rename(oldPath, newPath)
			if err != nil {
				log.Println("Ne peut pas rename le fichier :", err)
				return
			}
			log.Println("Le fichier a bien été HIDE")

			addToListeMessage("sent message :", "OK \n")
			if err := p.Send_message(writer, "OK"); err != nil {
				log.Println("Erreur lors de l'envoi de 'OK':", err)
				return
			}
			break
		}
	}
	if !found {
		log.Println("Fichier non trouvé:", commHideReveal[1])
		addToListeMessage("sent message :", "FileUnknown \n")
		if err := p.Send_message(writer, "FileUnknown"); err != nil {
			log.Println("Erreur lors de l'envoi de 'FileUnknown':", err)
			return
		}
	}
}

func ListServer(writer *bufio.Writer, reader *bufio.Reader) {
	var fichiers, err = os.ReadDir("Docs")
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return
	}
	var list = ""
	var size = 0
	addToListeMessage("sent message :", "Start \n")
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
	}
	var newlist = "FileCnt : " + strconv.Itoa(size) + list
	log.Println(newlist)
	addToListeMessage("sent message :", newlist, "\n")
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
				log.Println("Erreur lecture sous-dossier:", err)
				continue
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
	nbOperation := getCompteurOperations()
	nbClient := getCompteurClient()
	var list = []string{"Debug :", "Serveur créé à", slog.AnyValue(connectiontime).String(), "nombre d'opérations en cours :", strconv.FormatInt(int64(nbOperation), 10), "nombre de clients connectés :", strconv.FormatInt(int64(nbClient), 10)}
	slog.Debug(list[0])
	slog.Debug(list[1] + " " + list[2])
	slog.Debug(list[3] + " " + list[4])
	slog.Debug(list[5] + " " + list[6])
	addToListeMessage("sent message :", " debug \n")

	var newlist = strings.Join(list, "--")
	log.Println(newlist)
}

// Fonction de terminaison unifiée
func TerminateServer() {
	log.Println("Démarrage de la procédure de terminaison...")

	// Marquer le serveur comme en cours de terminaison
	setServerShuttingDown()

	// Fermer le canal pour arrêter l'acceptation de nouvelles connexions (une seule fois)
	shutdownOnce.Do(func() {
		close(shutdownChan)
	})

	// Récupérer le client de contrôle de façon thread-safe
	clientTerminantMutex.Lock()
	writer := clientTerminant.writer
	clientTerminantMutex.Unlock()

	// Attendre que toutes les opérations en cours se terminent
	for {
		nbOperation := getCompteurOperations()
		if nbOperation <= 0 {
			break
		}

		message := fmt.Sprintf("Il reste %d opération(s) en cours, attente de la fin...", nbOperation)
		log.Println(message)
		addToListeMessage("sent message :", message, "\n")
		if err := p.Send_message(writer, message); err != nil {
			log.Println("Erreur lors de l'envoi du message d'attente:", err)
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Envoyer le message final au client de contrôle
	addToListeMessage("sent message :", "Terminaison finie, le serveur s'éteint \n")
	if err := p.Send_message(writer, "Terminaison finie, le serveur s'éteint"); err != nil {
		log.Println("Erreur lors de l'envoi du message de terminaison:", err)
	}

	log.Println("Terminaison complète, fermeture du serveur...")

	// Forcer la sortie du programme après un court délai
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}
