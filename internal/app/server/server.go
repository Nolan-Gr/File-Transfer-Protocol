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

// connectiontime : moment où le serveur normal a commencé à écouter.
// Utilisé pour calculer l'uptime dans DebugServer.
var connectiontime time.Time

// Compteurs globaux protégés par un mutex.
// - compteurClient : nombre de connexions clientes actives (inclut control + normal).
// - compteurOperations : nombre d'opérations (LIST/GET/HIDE/REVEAL) en cours.
var (
	compteurClient     int
	compteurOperations int
	compteurMutex      sync.Mutex
)

// Canal de signal de terminaison : on le ferme pour indiquer à toutes
// les goroutines d'arrêter d'accepter/invoquer de nouvelles connexions.
// shutdownOnce évite un double close (panique).
var shutdownChan = make(chan struct{})
var shutdownOnce sync.Once

// Booléen et mutex pour indiquer si le serveur est en train de se terminer.
// L'accès se fait via setServerShuttingDown/isServerShuttingDown pour être thread-safe.
var terminaisonDuServeur bool = false
var terminaisonMutex sync.RWMutex

// ClientIO : stocke reader/writer du client de contrôle qui a demandé TERMINATE.
// On garde ces streams pour pouvoir envoyer des messages d'état pendant la terminaison.
type ClientIO struct {
	writer *bufio.Writer
	reader *bufio.Reader
}

var (
	clientTerminant      ClientIO
	clientTerminantMutex sync.Mutex
)

// Historique de messages envoyés (usage pour debug/monitoring).
// Accès protégé par listeMessageMutex.
var (
	listeMessage      = []string{"Historique des messages : \n"}
	listeMessageMutex sync.Mutex
)

var listeValues = []string{}

// WaitGroup pour attendre la fin des deux serveurs (normal + contrôle)
// lors d'un RunServer.
var serverWg sync.WaitGroup

// Helpers thread-safe pour manipuler les compteurs.
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

// Accès thread-safe au drapeau de terminaison du serveur.
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

// Ajoute un ou plusieurs messages à l'historique de façon thread-safe.
func addToListeMessage(messages ...string) {
	listeMessageMutex.Lock()
	defer listeMessageMutex.Unlock()
	listeMessage = append(listeMessage, messages...)
}

// RunServer lance deux listeners concurrents : un server "normal" et un server "control".
// On utilise un WaitGroup pour attendre leur terminaison.
func RunServer(port *string, controlPort *string) {
	listeValues = append(listeValues, *port, *controlPort)

	serverWg.Add(2)
	go runNormalServer(port)
	go runControlServer(controlPort)

	// Attendre que les deux serveurs se terminent (appelé après shutdown).
	serverWg.Wait()
	log.Println("Tous les serveurs sont arrêtés")
}

// Listener principal pour les clients "normaux".
func runNormalServer(port *string) {
	defer serverWg.Done()

	l, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	// enregistrer le temps de début pour l'uptime
	connectiontime = time.Now()

	// On ferme le listener à la sortie de la fonction.
	defer func() {
		err := l.Close()
		if err != nil {
			return
		}
		slog.Debug("Stopped listening on port " + *port)
	}()
	slog.Debug("Now listening on port " + *port)

	// Goroutine qui ferme le listener lorsque shutdownChan est fermé.
	// Cela permet à l'Accept() bloquant de sortir avec une erreur contrôlable.
	go func() {
		<-shutdownChan
		l.Close()
	}()

	for {
		c, err := l.Accept()
		if err != nil {
			// Si on est en cours d'arrêt, on termine proprement la boucle.
			if isServerShuttingDown() {
				slog.Info("Server normal terminé, arrêt des nouvelles connexions")
				return
			}
			// Erreur non liée au shutdown : on log et on continue.
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String() + " on port " + *port)
		go HandleClient(c)
	}
}

// Listener pour le port de contrôle (commande TERMINATE, HIDE/REVEAL, etc.).
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

	// Même mécanisme de fermeture via shutdownChan.
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

// HandleClient : logique pour un client "normal".
// - Envoie "hello" au début.
// - Attend des commandes (start, LIST, GET, tree, GOTO, end, messages, Help).
// - Protège l'accès aux compteurs et à l'historique.
func HandleClient(conn net.Conn) {
	defer ClientLogOut(conn)

	taille := incrementerClient()
	log.Println("nombre de client : ", taille)

	log.Println("adresse IP du nouveau client :", conn.RemoteAddr().String(), " connecté le : ", time.Now(), " connecté sur le port ", "3333")

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Envoyer greeting initial via protocole (Send_message gère le flush/format).
	addToListeMessage("sent message :", "hello \n")
	if err := p.Send_message(conn, writer, "hello"); err != nil {
		// Sensible aux erreurs réseau (timeouts etc.)
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'hello':", err)
		} else {
			log.Println("Erreur lors de l'envoi de 'hello':", err)
		}
		return
	}

	for {
		// Boucle de réception de commandes.
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

		// Si le serveur n'est pas en train de se terminer, traiter les commandes.
		if !isServerShuttingDown() {
			log.Println("cleanedMessage :", cleanedMsg)
			if cleanedMsg == "start" {
				// Répondre OK pour démarrer la session.
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
				// LIST : incrémenter compteur d'opération, exécuter ListServer.
				nbOp := incrementerOperations()
				log.Println("Commande LIST reçue, opérations en cours:", nbOp)
				if !ListServer(conn, commGet, writer, reader) {
					// En cas d'erreur, décrémenter et quitter.
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande LIST terminée, opérations restantes:", nbOp)

			} else if len(commGet) == 3 && commGet[0] == "GET" {
				// GET : transfert d'un fichier
				nbOp := incrementerOperations()
				log.Println("Commande GET reçue pour:", commGet[1], ", opérations en cours:", nbOp)
				if !Getserver(conn, commGet, writer, reader) {
					decrementerOperations()
					return
				}
				nbOp = decrementerOperations()
				log.Println("Commande GET terminée, opérations restantes:", nbOp)

			} else if cleanedMsg == "Unknown" {
				// Commande inconnue : renvoyer message d'aide.
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
				// Envoie l'arbre récursif du dossier "Docs"
				if !tree(conn, writer, reader) {
					return
				}
			} else if commGet[0] == "GOTO" {
				// GOTO : navigue vers un dossier donné (proto simple ici)
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
				// Fin de la session cliente.
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
				// Affiche l'historique côté serveur (debug).
				listeMessageMutex.Lock()
				fmt.Println(strings.Trim(fmt.Sprint(listeMessage), "[]"))
				listeMessageMutex.Unlock()

			} else {
				// Message inattendu : on l'ignore (mais on log).
				log.Println("Message inattendu du client:", cleanedMsg)
				continue
			}
		} else {
			// Si le serveur est en cours d'arrêt : informer le client et couper la connexion.
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

		// Si le logger est en mode debug, on renvoie des infos de debug au client.
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			log.Println("debug ")
			DebugServer(writer, reader)
		}
	}
}

// HandleControlClient : similaire à HandleClient mais ajoute des commandes de contrôle
// (HIDE, REVEAL, TERMINATE). Lorsque TERMINATE est reçu, on stocke le writer/reader de
// ce client pour l'utiliser pendant la séquence de terminaison.
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
				// Stocker les flux du client initiant la terminaison pour pouvoir
				// lui envoyer des messages d'état durant l'arrêt.
				log.Println("Commande TERMINATE reçue")
				clientTerminantMutex.Lock()
				clientTerminant.reader = reader
				clientTerminant.writer = writer
				clientTerminantMutex.Unlock()
				// Lancer la procédure de terminaison dans une goroutine séparée
				// pour ne pas bloquer la boucle de réception du client de contrôle.
				go TerminateServer(conn)
				// Attendre que shutdownChan soit fermé (TerminateServer le ferme).
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
				tree(conn, writer, reader)
			} else if commHideReveal[0] == "GOTO" {
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

// ClientLogOut : décrémente le compteur client et ferme la connexion.
// Appelée en defer à la sortie des handlers.
func ClientLogOut(conn net.Conn) {
	taille := decrementerClient()
	log.Println("nombre de client : ", taille)

	log.Println("adresse IP du client : ", conn.RemoteAddr().String(), " déconnecté le : ", time.Now())

	err := conn.Close()
	if err != nil {
		return
	}
}

// Getserver : implémentation de GET.
// - Parcourt le dossier fourni (commGet[2]) pour trouver le fichier commGet[1].
// - Envoie "Start", envoie le contenu si trouvé, puis attend la confirmation client.
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

	// Attendre la confirmation du client après le transfert/erreur.
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

// HIDE : renomme le fichier en le préfixant par '.' pour le cacher.
// Envoie OK si succès, FileUnknown si fichier non trouvé.
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

// REVEAL : retire le préfixe '.' pour rendre visible le fichier.
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

// ListServer : envoie la liste des fichiers non cachés dans le dossier demandé.
// Protocole : envoie "Start", attend "OK" du client, puis envoie "FileCnt : N --name size ..."
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
			// Ignorer les fichiers cachés (commençant par '.')
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

// ParcourFolder : fonction récursive utilisée par tree pour construire l'arborescence.
// Retourne la liste sous forme de chaîne et le nombre total d'éléments trouvés.
// Remarque : gère les erreurs en les loggant, et continue sur sous-dossiers problématiques.
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

// DebugServer : envoie des informations de debug au client.
// Utilise getCompteurClient/getCompteurOperations et connectiontime pour l'uptime.
// Ici p.Send_message est appelé avec nil pour le net.Conn car seule la writer est utilisée.
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

// TerminateServer : procédure de terminaison propre.
// - marque le serveur comme en arrêt (setServerShuttingDown)
// - attend que toutes les opérations en cours soient terminées et que les autres clients se déconnectent
// - envoie des messages de statut au client de contrôle qui a demandé la terminaison
// - ferme shutdownChan (une seule fois) pour déclencher la fermeture des listeners
// - appelle os.Exit(0) après un court délai pour s'assurer d'une sortie complète.
func TerminateServer(conn net.Conn) {
	log.Println("Initiation de la terminaison du serveur...")
	setServerShuttingDown() // Indiquer que le serveur s'arrête

	clientTerminantMutex.Lock()
	writer := clientTerminant.writer
	clientTerminantMutex.Unlock()

	// Boucle d'attente : on surveille ops et clients.
	for {
		ops := getCompteurOperations()
		clients := getCompteurClient()
		// Soustraire le client de contrôle qui a initié la commande,
		// car il est toujours connecté pendant la procédure.
		clientsApresControle := clients - 1

		// Condition de sortie : aucune opération en cours et aucun client (hors control).
		if ops == 0 && clientsApresControle == 0 {
			break
		}

		msg := fmt.Sprintf("Opérations en cours : %d, Clients actifs (hors contrôle) : %d. Attente...", ops, clientsApresControle)
		log.Println(msg)

		addToListeMessage("sent message :", msg, "\n")
		// On essaye d'envoyer le message d'état au client initiateur ;
		// en cas d'erreur d'envoi on continue (ne bloque pas la terminaison).
		if err := p.Send_message(conn, writer, msg); err != nil {
			log.Println("Erreur lors de l'envoi du message d'attente de terminaison:", err)
		}

		// Attente active courte (polling simple). On pourrait améliorer avec condition variables.
		time.Sleep(1 * time.Second)
	}

	finalMsg := "Terminaison finie, le serveur s'éteint"
	log.Println(finalMsg)
	addToListeMessage("sent message :", finalMsg, "\n")

	if err := p.Send_message(conn, writer, finalMsg); err != nil {
		log.Println("Erreur lors de l'envoi du message final de terminaison:", err)
	}

	// Fermer shutdownChan une seule fois pour signaler aux goroutines d'arrêter.
	shutdownOnce.Do(func() {
		close(shutdownChan)
	})

	// ClientLogOut sera appelé par le defer de HandleControlClient après le retour.
	log.Println("Terminaison complète, fermeture du serveur...")

	// On force une sortie du process après un court délai pour s'assurer de la fermeture.
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}

// tree : construit et envoie l'arbre complet du dossier "Docs".
// Protocole similaire à LIST : Start -> attendre OK -> envoyer la liste complète.
func tree(conn net.Conn, writer *bufio.Writer, reader *bufio.Reader) bool {
	log.Println("tree func")

	var fichiers, err = os.ReadDir("Docs")
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return false
	}

	var list = ""
	var size = 0

	addToListeMessage("sent message :", "Start \n")
	if err := p.Send_message(conn, writer, "Start"); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'Start' (tree):", err)
		} else {
			log.Println("Erreur lors de l'envoi de 'Start':", err)
		}
		return false
	}

	log.Println(fichiers)
	data, err := p.Receive_message(conn, reader)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la confirmation 'OK' (tree):", err)
		} else {
			log.Println("Erreur lors de la lecture du fichier (attendu OK):", err)
		}
		return false
	}
	log.Println("data:", data)

	if strings.TrimSpace(data) == "OK" {
		var templist, tempsize = ParcourFolder(fichiers, list, size)
		log.Println("list : ", tempsize, templist)
		list = list + templist
		size = tempsize
		var newlist = "FileCnt : " + strconv.Itoa(size) + list
		log.Println(newlist)

		addToListeMessage("sent message :", newlist, "\n")
		if err := p.Send_message(conn, writer, newlist); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de la liste finale (tree):", err)
			} else {
				log.Println("Erreur lors de l'envoi de la liste finale:", err)
			}
			return false
		}
	} else {
		log.Println("Protocole tree échoué : Attendu 'OK', reçu:", strings.TrimSpace(data))
		return false
	}

	return true
}
