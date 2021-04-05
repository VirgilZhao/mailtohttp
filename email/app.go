package email

import (
	"fmt"
	"github.com/VirgilZhao/mailtohttp/model"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"io"
	"io/ioutil"
	"log"
	"regexp"
)

type EmailApp struct {
	imapAddress string
	imapPort    int
	email       string
	password    string
	folder      string
	client      *client.Client
	UpdateChan  chan string
	stopChan    chan string
	inbox       *imap.MailboxStatus
	config      *model.ServiceConfig
}

func NewEmailApp(config *model.ServiceConfig) *EmailApp {
	return &EmailApp{
		imapAddress: config.EmailSettings.ImapAddress,
		imapPort:    config.EmailSettings.ImapPort,
		email:       config.EmailSettings.Email,
		password:    config.EmailSettings.Password,
		folder:      config.EmailSettings.Folder,
		UpdateChan:  make(chan string, 10),
		stopChan:    make(chan string, 1),
		config:      config,
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

	mbox, err := ea.client.Select(ea.folder, false)
	if err != nil {
		log.Println(err)
		return err
	}
	ea.inbox = mbox
	return nil
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
		case <-ea.stopChan:
			log.Println("loop idle quit")
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
		case <-ea.stopChan:
			log.Println("loop receive quit")
			return
		}
	}
}

func (ea *EmailApp) StopLoop() {
	ea.stopChan <- ""
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

	// Get the whole message body
	section := &imap.BodySectionName{}

	messages := make(chan *imap.Message, 10)
	err := ea.client.Fetch(seqset, []imap.FetchItem{section.FetchItem()}, messages)
	log.Println(err)
	for msg := range messages {
		r := msg.GetBody(section)
		if r == nil {
			log.Println("Server didn't returned message body")
			continue
		}

		// Create a new mail reader
		mr, err := mail.CreateReader(r)
		if err != nil {
			log.Println(err)
			continue
		}
		// Process each message's part
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				log.Println(err)
				continue
			}

			success := false
			switch h := p.Header.(type) {
			case *mail.InlineHeader:
				// This is the message's text (can be plain-text or HTML)
				b, _ := ioutil.ReadAll(p.Body)
				//log.Println("Got text: %v", string(b))
				if ea.decodeEmail(string(b)) {
					success = true
				}
			case *mail.AttachmentHeader:
				// This is an attachment
				filename, _ := h.Filename()
				log.Println("Got attachment: %v", filename)
			default:
				break
			}
			if success {
				break
			}
		}
	}
	log.Println("done")
}

func (ea *EmailApp) decodeEmail(message string) bool {
	params := make([]model.Param, 0)
	for _, content := range ea.config.ContentPatterns {
		valReg := regexp.MustCompile(content.Regex)
		matches := valReg.FindAllStringSubmatch(message, -1)
		vals := make([]string, 0)
		for _, m := range matches {
			for _, item := range m {
				vals = append(vals, item)
			}
		}
		params = append(params, model.Param{
			Name:  content.Param,
			Value: vals,
		})
	}
	log.Println(params)
	return true
}
