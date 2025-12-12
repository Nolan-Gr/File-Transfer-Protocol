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
		go HandleControlClient(c)
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
	if err := p.Send_message(conn, writer, "hello"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'hello':", err)
		} else {
			log.Println("Erreur lors de l'envoi de 'hello':", err)
		}
		return
	}

	for {
		msg, err := p.Receive_message(conn, reader)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de la réception d'un message client:", err)
			} else {
				log.Println("Client déconnecté ou erreur de lecture:", err)
			}
			return
		}

		cleanedMsg := strings.TrimSpace(msg)
		log.Println(cleanedMsg)

		var commGet = strings.Split(cleanedMsg, " ")

		if !isServerShuttingDown() {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				addToListeMessage("sent message :", "ok \n")
				if err := p.Send_message(conn, writer, "ok"); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'ok' après 'start':", err)
					} else {
						log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
					}
					return
				}

			} else if len(commGet) == 2 && commGet[0] == "List" {
				nbOp := incrementerOperations()
				log.Println("Commande LIST reçue, opérations en cours:", nbOp)
				if !ListServer(conn, commGet, writer, reader) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande LIST terminée, opérations restantes:", nbOp)

			} else if len(commGet) == 3 && commGet[0] == "GET" {
				nbOp := incrementerOperations()
				log.Println("Commande GET reçue pour:", commGet[1], ", opérations en cours:", nbOp)
				if !Getserver(conn, commGet, writer, reader) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande GET terminée, opérations restantes:", nbOp)

			} else if cleanedMsg == "Unknown" {
				addToListeMessage("sent message :", "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande. \n")
				if err := p.Send_message(conn, writer, "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande."); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi du message unknown:", err)
					} else {
						log.Println("Erreur lors de l'envoi du message de terminaison:", err)
					}
					return
				}
				log.Println("Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande.")

			} else if cleanedMsg == "Help" {
				helpMessage := "Commandes disponibles : LIST, GET <filename>, HELP, END"
				addToListeMessage("sent message :", helpMessage+" \n")
				if err := p.Send_message(conn, writer, helpMessage); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'help':", err)
					} else {
						log.Println("Erreur lors de l'envoi de 'help':", err)
					}
					return
				}

			} else if commGet[0] == "tree" {
				if err != nil {
					log.Println(err)
					continue
				}
				if !tree(conn, writer, reader) {
					return
				}
			} else if commGet[0] == "GOTO" {
				if err != nil {
					log.Println(err)
				}

				if commGet[1] == ".." {
					addToListeMessage("sent message :", "back", " \n")
					if err := p.Send_message(conn, writer, "back"); err != nil {
						log.Println("Erreur lors de l'envoi de 'Start':", err)
						return
					}
				} else if commGet[2] != commGet[1] {
					var fichiers, err = os.ReadDir(commGet[2])
					if err != nil {
						log.Println(err)
					}
					for _, fichier := range fichiers {
						if fichier.Name() == commGet[1] && fichier.IsDir() {
							addToListeMessage("sent message :", "Start \n")
							if err := p.Send_message(conn, writer, "Start"); err != nil {
								log.Println("Erreur lors de l'envoi de 'Start':", err)
								return
							}
						}
					}
				} else {
					addToListeMessage("sent message :", "NO! \n")
					if err := p.Send_message(conn, writer, "NO!"); err != nil {
						log.Println("Erreur lors de l'envoi de 'Start':", err)
						return
					}
				}
			} else if cleanedMsg == "end" {
				addToListeMessage("sent message :", "ok \n")
				if err := p.Send_message(conn, writer, "ok"); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'ok' après 'end':", err)
					} else {
						log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
					}
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
			addToListeMessage("sent message :", "Server terminating, connection closing. \n")
			if err := p.Send_message(conn, writer, "Server terminating, connection closing."); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi du message de terminaison:", err)
				} else {
					log.Println("Erreur lors de l'envoi du message de terminaison:", err)
				}
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
	if err := p.Send_message(conn, writer, "hello"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'hello':", err)
		} else {
			log.Println("Erreur lors de l'envoi de 'hello':", err)
		}
		return
	}

	for {
		msg, err := p.Receive_message(conn, reader)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de la réception d'un message client control:", err)
			} else {
				log.Println("Client déconnecté ou erreur de lecture:", err)
			}
			return
		}

		cleanedMsg := strings.TrimSpace(msg)
		log.Println(cleanedMsg)

		var commHideReveal = strings.Split(cleanedMsg, " ")

		if !isServerShuttingDown() {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				addToListeMessage("sent message :", "ok \n")
				if err := p.Send_message(conn, writer, "ok"); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'ok' après 'start':", err)
					} else {
						log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
					}
					return
				}

			} else if len(commHideReveal) == 2 && commHideReveal[0] == "List" {
				nbOp := incrementerOperations()
				log.Println("Commande LIST reçue, opérations en cours:", nbOp)
				if !ListServer(conn, commHideReveal, writer, reader) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande LIST terminée, opérations restantes:", nbOp)

			} else if cleanedMsg == "Terminate" {
				log.Println("Commande TERMINATE reçue")
				clientTerminantMutex.Lock()
				clientTerminant.reader = reader
				clientTerminant.writer = writer
				clientTerminantMutex.Unlock()
				// Lancer la terminaison dans une goroutine
				go TerminateServer(conn)
				// Attendre que le serveur se termine
				<-shutdownChan
				return

			} else if cleanedMsg == "Unknown" {
				addToListeMessage("sent message :", "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande. \n")
				if err := p.Send_message(conn, writer, "Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande."); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi du message unknown:", err)
					} else {
						log.Println("Erreur lors de l'envoi du message de terminaison:", err)
					}
					return
				}
				log.Println("Commande inconnue. Veuillez entrer HELP pour avoir la liste de commande.")

			} else if cleanedMsg == "Help" {
				helpMessage := "Commandes disponibles : LIST, HIDE, REVEAL, HELP, END et TERMINATE	"
				addToListeMessage("sent message :", helpMessage+" \n")
				if err := p.Send_message(conn, writer, helpMessage); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'help':", err)
					} else {
						log.Println("Erreur lors de l'envoi de 'help':", err)
					}
					return
				}

			} else if len(commHideReveal) == 3 && commHideReveal[0] == "HIDE" {
				nbOp := incrementerOperations()
				log.Println("Commande HIDE reçue, opérations en cours:", nbOp)
				if !HIDE(conn, commHideReveal, writer, reader) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande HIDE terminée, opérations restantes:", nbOp)

			} else if len(commHideReveal) == 3 && commHideReveal[0] == "REVEAL" {
				nbOp := incrementerOperations()
				log.Println("Commande REVEAL reçue, opérations en cours:", nbOp)
				if !REVEAL(conn, commHideReveal, writer, reader) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande REVEAL terminée, opérations restantes:", nbOp)

			} else if cleanedMsg == "end" {
				addToListeMessage("sent message :", "ok \n")
				if err := p.Send_message(conn, writer, "ok"); err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Timeout lors de l'envoi de 'ok' après 'end':", err)
					} else {
						log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
					}
				}
				return

			} else if cleanedMsg == "messages" {
				listeMessageMutex.Lock()
				log.Println(listeMessage)
				listeMessageMutex.Unlock()

			} else if commHideReveal[0] == "tree" {
				if err != nil {
					log.Println(err)
				}
				tree(conn, writer, reader)
			} else if commHideReveal[0] == "GOTO" {
				if err != nil {
					log.Println(err)
				}

				if commHideReveal[1] == ".." {
					addToListeMessage("sent message :", "back", " \n")
					if err := p.Send_message(conn, writer, "back"); err != nil {
						log.Println("Erreur lors de l'envoi de 'Start':", err)
						return
					}
				} else if commHideReveal[2] != commHideReveal[1] {
					var fichiers, err = os.ReadDir(commHideReveal[2])
					if err != nil {
						log.Println(err)
					}
					for _, fichier := range fichiers {
						if fichier.Name() == commHideReveal[1] && fichier.IsDir() {
							addToListeMessage("sent message :", "Start \n")
							if err := p.Send_message(conn, writer, "Start"); err != nil {
								log.Println("Erreur lors de l'envoi de 'Start':", err)
								return
							}
						}
					}
				} else {
					addToListeMessage("sent message :", "NO! \n")
					if err := p.Send_message(conn, writer, "NO!"); err != nil {
						log.Println("Erreur lors de l'envoi de 'Start':", err)
						return
					}
				}
			} else {
				log.Println("Message inattendu du client:", cleanedMsg)
				continue
			}
		} else {
			addToListeMessage("sent message :", "Server terminating, connection closing. \n")
			if err := p.Send_message(conn, writer, "Server terminating, connection closing."); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi du message de terminaison:", err)
				} else {
					log.Println("Erreur lors de l'envoi du message de terminaison:", err)
				}
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

func Getserver(conn net.Conn, commGet []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	var fichiers, err = os.ReadDir(commGet[2])
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return false
	}
	var found = false

	for _, fichier := range fichiers {
		if commGet[1] == fichier.Name() {
			found = true
			log.Println("Fichier trouvé:", fichier.Name())
			var path = filepath.Join(commGet[2], fichier.Name())

			addToListeMessage("sent message :", "Start \n")
			if err := p.Send_message(conn, writer, "Start"); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'Start':", err)
				} else {
					log.Println("Erreur lors de l'envoi de 'Start':", err)
				}
				return false
			}

			var data, err = os.ReadFile(path)
			if err != nil {
				log.Println("Ne peut pas lire le contenu du fichier :", err)
				return false
			}

			addToListeMessage("sent message :", string(data), "\n")
			err = p.Send_message(conn, writer, string(data))
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors du transfert du fichier:", err)
				} else {
					log.Println("N'a pas pû transférer le fichier :", err)
				}
				return false
			}
			break
		}
	}

	if !found {
		log.Println("Fichier non trouvé:", commGet[1])
		addToListeMessage("sent message :", "FileUnknown \n")
		if err := p.Send_message(conn, writer, "FileUnknown"); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'FileUnknown':", err)
			} else {
				log.Println("Erreur lors de l'envoi de 'FileUnknown':", err)
			}
			return false
		}
	}

	var response, err2 = p.Receive_message(conn, reader)
	if err2 != nil {
		if netErr, ok := err2.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la confirmation GET:", err2)
		} else {
			log.Println("Erreur lors de la réception de la confirmation:", err2)
		}
		return false
	}
	log.Println("Réponse du client:", response)
	return true
}

func HIDE(conn net.Conn, commHideReveal []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	var fichiers, err = os.ReadDir(commHideReveal[2])
	if err != nil {
		log.Println("Erreur lecture dossier:", err)
		return false
	}
	var found = false

	for _, fichier := range fichiers {
		if commHideReveal[1] == fichier.Name() {
			found = true
			log.Println("Fichier trouvé:", fichier.Name())
			var oldPath = filepath.Join(commHideReveal[2], fichier.Name())
			var newPath = filepath.Join(commHideReveal[2], "."+fichier.Name())

			err := os.Rename(oldPath, newPath)
			if err != nil {
				log.Println("Ne peut pas rename le fichier :", err)
				return false
			}
			log.Println("Le fichier a bien été HIDE")

			addToListeMessage("sent message :", "OK \n")
			if err := p.Send_message(conn, writer, "OK"); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'OK' HIDE:", err)
				} else {
					log.Println("Erreur lors de l'envoi de 'OK':", err)
				}
				return false
			}
			break
		}
	}

	if !found {
		log.Println("Fichier non trouvé:", commHideReveal[1])
		addToListeMessage("sent message :", "FileUnknown \n")
		if err := p.Send_message(conn, writer, "FileUnknown"); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'FileUnknown' HIDE:", err)
			} else {
				log.Println("Erreur lors de l'envoi de 'FileUnknown':", err)
			}
			return false
		}
	}

	return true
}

func REVEAL(conn net.Conn, commHideReveal []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	var fichiers, err = os.ReadDir(commHideReveal[2])
	if err != nil {
		log.Println("Erreur lecture dossier :", err)
		return false
	}
	var found = false

	for _, fichier := range fichiers {
		if commHideReveal[1] == fichier.Name() {
			found = true
			log.Println("Fichier trouvé:", fichier.Name())
			var oldPath = filepath.Join(commHideReveal[2], fichier.Name())
			var newPath = filepath.Join(commHideReveal[2], strings.TrimPrefix(fichier.Name(), "."))

			err := os.Rename(oldPath, newPath)
			if err != nil {
				log.Println("Ne peut pas rename le fichier :", err)
				return false
			}
			log.Println("Le fichier a bien été REVEAL")

			addToListeMessage("sent message :", "OK \n")
			if err := p.Send_message(conn, writer, "OK"); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'OK' REVEAL:", err)
				} else {
					log.Println("Erreur lors de l'envoi de 'OK':", err)
				}
				return false
			}
			break
		}
	}

	if !found {
		log.Println("Fichier non trouvé:", commHideReveal[1])
		addToListeMessage("sent message :", "FileUnknown \n")
		if err := p.Send_message(conn, writer, "FileUnknown"); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'FileUnknown' REVEAL:", err)
			} else {
				log.Println("Erreur lors de l'envoi de 'FileUnknown':", err)
			}
			return false
		}
	}

	return true
}

func ListServer(conn net.Conn, commHideReveal []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	var fichiers, err = os.ReadDir(commHideReveal[1])
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return false
	}

	var list = ""
	var size = 0

	addToListeMessage("sent message :", "Start \n")
	if err := p.Send_message(conn, writer, "Start"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'Start' LIST:", err)
		} else {
			log.Println("Erreur lors de l'envoi de 'Start':", err)
		}
		return false
	}

	log.Println(fichiers)
	data, err := p.Receive_message(conn, reader)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la confirmation LIST:", err)
		} else {
			log.Println("Erreur lors de la lecture du fichier:", err)
		}
		return false
	}
	log.Println("data:", data)

	if strings.TrimSpace(data) == "OK" {
		for _, fichier := range fichiers {
			if fichier.Name()[0] != '.' {
				fileInfo, err := fichier.Info()
				if err != nil {
					log.Println("Erreur lors de la lecture du fichier:", err)
					return false
				}
				list = list + " --" + fichier.Name() + " " + strconv.FormatInt(int64(fileInfo.Size()), 10)
				size = size + 1
			}
		}
	}

	var newlist = "FileCnt : " + strconv.Itoa(size) + list
	log.Println(newlist)
	addToListeMessage("sent message :", newlist, "\n")
	if err := p.Send_message(conn, writer, newlist); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la liste:", err)
		} else {
			log.Println("Erreur lors de l'envoi de la liste:", err)
		}
		return false
	}

	return true
}

func ParcourFolder(fichiers []os.DirEntry, list string, size int) (string, int) {
	for _, fichier := range fichiers {
		fileInfo, err := fichier.Info()
		if err != nil {
			log.Println("Erreur lors de la lecture du fichier:", err)
			return err.Error(), 0
		}
		size = size + 1
		if fichier.Name()[0] != '.' {
			if fichier.IsDir() {
				log.Println(filepath.Join("Docs/", fichier.Name()))
				var newfichiers, err = os.ReadDir(filepath.Join("Docs/", fichier.Name()))
				if err != nil {
					log.Println("Erreur lecture sous-dossier:", err)
					continue
				}
				var liste, newsize = ParcourFolder(newfichiers, list, size)
				size = size + newsize
				list = list + " --" + fichier.Name() + " " + strconv.FormatInt(int64(fileInfo.Size()), 10) + " -- sous-dossier: " + " [" + liste + "]"
			} else {
				list = list + " --" + fichier.Name() + " " + strconv.FormatInt(int64(fileInfo.Size()), 10)
			}
		}
	}
	return list, size
}

// Envoie des informations de debug au client (si le niveau de log est debug)
func DebugServer(writer *bufio.Writer, reader *bufio.Reader) bool {
	msg := fmt.Sprintf("DebugInfo: clients=%d, operations=%d, uptime=%s",
		getCompteurClient(),
		getCompteurOperations(),
		time.Since(connectiontime).Truncate(time.Second).String())

	addToListeMessage("sent message :", msg, "\n")
	if err := p.Send_message(nil, writer, msg); err != nil {
		log.Println("Erreur lors de l'envoi du message de debug:", err)
		return false
	}
	return true
}

// Gère la terminaison propre du serveur
func TerminateServer(conn net.Conn) {
	log.Println("Initiation de la terminaison du serveur...")
	setServerShuttingDown() // Indiquer que le serveur s'arrête

	clientTerminantMutex.Lock()
	writer := clientTerminant.writer
	clientTerminantMutex.Unlock()

	// Boucle d'attente pour les opérations en cours
	for {
		ops := getCompteurOperations()
		clients := getCompteurClient()
		// Soustraire le client de contrôle qui a initié la commande
		clientsApresControle := clients - 1

		if ops == 0 && clientsApresControle == 0 {
			break
		}

		msg := fmt.Sprintf("Opérations en cours : %d, Clients actifs (hors contrôle) : %d. Attente...", ops, clientsApresControle)
		log.Println(msg)

		addToListeMessage("sent message :", msg, "\n")
		if err := p.Send_message(conn, writer, msg); err != nil {
			log.Println("Erreur lors de l'envoi du message d'attente de terminaison:", err)
			// Continuer même en cas d'erreur d'envoi
		}

		time.Sleep(1 * time.Second)
	}

	finalMsg := "Terminaison finie, le serveur s'éteint"
	log.Println(finalMsg)
	addToListeMessage("sent message :", finalMsg, "\n")

	if err := p.Send_message(conn, writer, finalMsg); err != nil {
		log.Println("Erreur lors de l'envoi du message final de terminaison:", err)
	}

	// Fermer le canal de shutdown pour signaler aux listeners de se fermer
	shutdownOnce.Do(func() {
		close(shutdownChan)
	})

	// ClientLogOut sera appelé par le defer de HandleControlClient
	// après le retour de TerminateServer (suite à la réception du signal)
	log.Println("Terminaison complète, fermeture du serveur...")

	// Forcer la sortie du programme après un court délai
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}

func tree(conn net.Conn, writer *bufio.Writer, reader *bufio.Reader) bool {
	log.Println("tree func")
	var fichiers, err = os.ReadDir("Docs")
	var list = ""
	var size = 0
	listeMessage = append(listeMessage, "sent message :", "Start \n")
	if err := p.Send_message(conn, writer, "Start"); err != nil {
		log.Println("Erreur lors de l'envoi de 'Start':", err)
		return false
	}
	log.Println(fichiers)
	data, err := p.Receive_message(conn, reader)
	log.Println("data:", data)
	if err != nil {
		log.Println("Erreur lors de la lecture du fichier:", err)
		return false
	} else if strings.TrimSpace(data) == "OK" {
		var templist, tempsize = ParcourFolder(fichiers, list, size)
		log.Println("list : ", size, templist)
		list = list + templist
		size = size + tempsize
		if err != nil {
			log.Fatal(err)
		}
	}
	var newlist = "FileCnt : " + strconv.Itoa(size) + list
	log.Println(newlist)
	listeMessage = append(listeMessage, "sent message :", newlist, "\n")
	p.Send_message(conn, writer, newlist)
	return true
}
