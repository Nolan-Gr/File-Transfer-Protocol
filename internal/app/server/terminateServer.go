package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

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
var terminaisonDuServeur = false
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

// TerminateServer : procédure de terminaison propre.
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

		// On essaye d'envoyer le message d'état au client initiateur ;
		// en cas d'erreur d'envoi on continue (ne bloque pas la terminaison).
		if err := p.Send_message(conn, writer, msg); err != nil {
			log.Println("Erreur lors de l'envoi du message d'attente de terminaison:", err)
		}

		// Attente active courte (polling simple).
		time.Sleep(1 * time.Second)
	}

	finalMsg := "Terminaison finie, le serveur s'éteint"
	log.Println(finalMsg)

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
