package ngap

import (
	"log"

	"github.com/free5gc/ngap/ngapType"
)

func HandleNGAPMessage(ngapMsg *ngapType.NGAPPDU) {
	switch ngapMsg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		handleInitiatingMessage(ngapMsg.InitiatingMessage)
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		handleSuccessfulOutcome(ngapMsg.SuccessfulOutcome)
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		handleUnsuccessfulOutcome(ngapMsg.UnsuccessfulOutcome)
	default:
		log.Println("[ERROR] Unknown NGAP message type")
	}
}

func handleInitiatingMessage(initMsg *ngapType.InitiatingMessage) {
	// Implement handling of initiating messages
}

func handleSuccessfulOutcome(successMsg *ngapType.SuccessfulOutcome) {
	// Implement handling of successful outcomes
}

func handleUnsuccessfulOutcome(unsuccessMsg *ngapType.UnsuccessfulOutcome) {
	// Implement handling of unsuccessful outcomes
}