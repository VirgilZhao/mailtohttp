package email

import (
	"fmt"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap/client"
	"log"
)

type EmailApp struct {
	imapAddress string
	imapPort    int
	email       string
	password    string
	folder      string
	client      *client.Client
	UpdateChan  chan string
	inbox       *imap.MailboxStatus
	msgChan     *chan string
}

func NewEmailApp(imapAddress string, imapPort int, email string, password string, folder string, msgChan *chan string) *EmailApp {
	return &EmailApp{
		imapAddress: imapAddress,
		imapPort:    imapPort,
		email:       email,
		password:    password,
		folder:      folder,
		UpdateChan:  make(chan string, 10),
		msgChan:     msgChan,
	}
}

func (ea *EmailApp) login() error {
	log.Printf("Connecting to server ...")
	c, err := client.DialTLS(fmt.Sprintf("%s:%v", ea.imapAddress, ea.imapPort), nil)
	if err != nil {
		log.Println(err)
		return err
	}
	ea.client = c
	log.Println("Connected")

	if err := c.Login(ea.email, ea.password); err != nil {
		log.Println(err)
		return err
	}
	log.Println("Logged in")

	mbox, err := ea.client.Select("INBOX", false)
	if err != nil {
		log.Println(err)
		return err
	}
	ea.inbox = mbox
	return nil
}

func (ea *EmailApp) StopEmailReceive() {

}

func (ea *EmailApp) StopIdle() {

}

func (ea *EmailApp) StartIdle(updateChan chan string) {
	if err := ea.login(); err != nil {
		return
	}
	defer ea.client.Logout()
	idleClient := idle.NewClient(ea.client)
	updates := make(chan client.Update)
	ea.client.Updates = updates
	done := make(chan error, 1)
	go func() {
		done <- idleClient.IdleWithFallback(nil, 0)
	}()
	log.Println("listen updates")
	for {
		select {
		case update := <-updates:
			log.Println("New update:", update)
			switch update.(type) {
			case *client.MailboxUpdate:
				updateChan <- "update"
				break
			}
			break
		case err := <-done:
			if err != nil {
				log.Println(err)
				return
			}
			log.Println("Not idling anymore")
			return
		}
	}
}

func (ea *EmailApp) StartEmailReceive() {
	if err := ea.login(); err != nil {
		return
	}
	log.Println("listen messages")
	for {
		select {
		case <-ea.UpdateChan:
			ea.getLatestMessages()
		default:
			break
		}
	}
}

func (ea *EmailApp) getLatestMessages() {
	if ea.client.Check() != nil {
		if err := ea.login(); err != nil {
			return
		}
	}
	from := uint32(1)
	to := ea.inbox.Messages
	if ea.inbox.Messages > 5 {
		from = ea.inbox.Messages - 5
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	messages := make(chan *imap.Message, 10)
	err := ea.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	log.Println(err)
	log.Println("Last 5 messages")
	for msg := range messages {
		log.Println("* " + msg.Envelope.Subject)
	}
	log.Println("done")
}
