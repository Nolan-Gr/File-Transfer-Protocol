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
// - compteurClient : nombre de connexions clientes actives.
// - compteurOperations : nombre d'opérations (LIST/GET/HIDE/REVEAL et autres) en cours.
var (
	compteurClient     int
	compteurOperations int
	compteurMutex      sync.Mutex
)

// Canal de signal de terminaison : on le ferme pour indiquer à toutes les goroutines de bloquer de nouvelles connexions.
// shutdownOnce évite de fermer deux fois.
var shutdownChan = make(chan struct{})
var shutdownOnce sync.Once

// Booléen et mutex pour indiquer si le serveur est en train de se terminer.
var terminaisonDuServeur = false
var terminaisonMutex sync.RWMutex

// ClientIO : stocke reader/writer du client de contrôle qui a demandé TERMINATE.
// On garde ces informations pour pouvoir envoyer des messages d'état pendant la terminaison.
type ClientIO struct {
	writer *bufio.Writer
	reader *bufio.Reader
}

var (
	clientTerminant      ClientIO
	clientTerminantMutex sync.Mutex
)

// fonctions pour manipuler les compteurs.
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

// Accès au flag de terminaison du serveur.
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

// TerminateServer : procédure de terminaison du serveur
func TerminateServer(conn net.Conn) {
	log.Println("Initiation de la terminaison du serveur...")
	setServerShuttingDown() // Indique que le serveur s'arrête

	clientTerminantMutex.Lock()
	writer := clientTerminant.writer
	clientTerminantMutex.Unlock()

	// Boucle d'attente : on surveille opérations et clients.
	for {
		ops := getCompteurOperations()
		clients := getCompteurClient()

		// Soustraire le client de contrôle qui a initié la commande
		clientsApresControle := clients - 1

		// Condition de sortie : aucune opération en cours et aucun client (hors control).
		if ops == 0 && clientsApresControle == 0 {
			break
		}

		msg := fmt.Sprintf("Opérations en cours : %d, Clients actifs (hors contrôle) : %d. Attente...", ops, clientsApresControle)
		log.Println(msg)
		if err := p.Send_message(conn, writer, msg); err != nil {
			log.Println("Erreur lors de l'envoi du message d'attente de terminaison:", err)
		}

		time.Sleep(1 * time.Second)
	}

	finalMsg := "Terminaison finie, le serveur s'éteint"
	log.Println(finalMsg)

	if err := p.Send_message(conn, writer, finalMsg); err != nil {
		log.Println("Erreur lors de l'envoi du message final de terminaison:", err)
	}

	// Fermer shutdownChan pour signaler aux goroutines d'arrêter.
	shutdownOnce.Do(func() {
		close(shutdownChan)
	})

	// ClientLogOut sera appelé par le defer de HandleControlClient après le retour.
	log.Println("Terminaison complète, fermeture du serveur...")

	// On force une sortie après un court délai pour s'assurer de la fermeture.
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}
