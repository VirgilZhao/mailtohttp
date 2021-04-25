package emailv2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/VirgilZhao/mailtohttp/model"
	"github.com/emersion/go-imap"
	idle "github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"time"
)

type EmailApp struct {
	config     *model.ServiceConfig
	msgChan    chan string
	client     *client.Client
	idleClient *idle.IdleClient
	inbox      *imap.MailboxStatus
	updateChan chan client.Update
	stopChan   chan string
}

func NewEmailApp(config *model.ServiceConfig, msgChan chan string) *EmailApp {
	return &EmailApp{
		config:     config,
		msgChan:    msgChan,
		stopChan:   make(chan string, 1),
		updateChan: make(chan client.Update, 1),
	}
}

func (ea *EmailApp) sendMessage(msgType string, text string) {
	log.Println(text)
	data := model.SocketMessage{
		MsgType: "message",
		Data:    msgType + ":" + text,
	}
	bytes, err := json.Marshal(&data)
	if err != nil {
		return
	}
	ea.msgChan <- string(bytes)
}

func (ea *EmailApp) login() error {
	ea.sendMessage("login", "Connecting to server ...")
	if ea.client != nil {
		ea.client.Close()
	}
	c, err := client.DialTLS(fmt.Sprintf("%s:%v", ea.config.EmailSettings.ImapAddress, ea.config.EmailSettings.ImapPort), nil)
	if err != nil {
		ea.sendMessage("login", err.Error())
		return err
	}
	ea.client = c
	ea.sendMessage("login", "Connected")

	if err := c.Login(ea.config.EmailSettings.Email, ea.config.EmailSettings.Password); err != nil {
		ea.sendMessage("login", err.Error())
		return err
	}
	ea.sendMessage("login", "Logged in")

	mbox, err := ea.client.Select(ea.config.EmailSettings.Folder, false)
	if err != nil {
		ea.sendMessage("login", err.Error())
		return err
	}
	ea.inbox = mbox
	ea.client.Updates = ea.updateChan
	ea.idleClient = idle.NewClient(ea.client)
	if support, err := ea.idleClient.SupportIdle(); err != nil || !support {
		ea.sendMessage("login", "server not support IMAP IDLE")
	}
	return nil
}

func (ea *EmailApp) getLatestMessages(count uint32) {
	if ea.client.Check() != nil {
		if err := ea.login(); err != nil {
			ea.sendMessage("getLatestMessages", err.Error())
			return
		}
	}
	from := uint32(1)
	to := ea.inbox.Messages
	if ea.inbox.Messages > count {
		from = ea.inbox.Messages - count
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	// Get the whole message body
	section := &imap.BodySectionName{}

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- ea.client.Fetch(seqset, []imap.FetchItem{section.FetchItem()}, messages)
	}()
	for {
		select {
		case msg := <-messages:
			if msg == nil {
				ea.sendMessage("getLatestMessages", "done")
				return
			}
			r := msg.GetBody(section)
			if r == nil {
				ea.sendMessage("getLatestMessages", "Server didn't returned message body")
				continue
			}
			// Create a new mail reader
			mr, err := mail.CreateReader(r)
			if err != nil {
				ea.sendMessage("getLatestMessages", err.Error())
				continue
			}
			// Process each message's part
			for {
				p, err := mr.NextPart()
				if err == io.EOF {
					break
				} else if err != nil {
					ea.sendMessage("getLatestMessages", err.Error())
					continue
				}

				success := false
				switch h := p.Header.(type) {
				case *mail.InlineHeader:
					// This is the message's text (can be plain-text or HTML)
					b, _ := ioutil.ReadAll(p.Body)
					// log.Println("Got text: %v", string(b))
					if ea.decodeEmail(string(b)) {
						success = true
					}
				case *mail.AttachmentHeader:
					// This is an attachment
					filename, _ := h.Filename()
					ea.sendMessage("getLatestMessages", fmt.Sprintf("Got attachment: %v", filename))
				default:
					break
				}
				if success {
					break
				}
			}
		case err := <-done:
			if err != nil {
				ea.sendMessage("getLatestMessages", err.Error())
			}
		}
	}
}

func (ea *EmailApp) decodeEmail(message string) bool {
	params := make([]model.Param, 0)
	for _, content := range ea.config.ContentPatterns {
		valReg, err := regexp.Compile(content.Regex)
		if err != nil {
			ea.sendMessage("decodeEmail", err.Error())
		}
		matches := valReg.FindAllString(message, -1)
		if content.Require && len(matches) == 0 {
			return true
		}
		vals := make([]string, 0)
		for _, m := range matches {
			vals = append(vals, m)
		}
		params = append(params, model.Param{
			Name:  content.Param,
			Value: vals,
		})
	}
	ea.sendHttp(params)
	// ea.sendMessage("decodeEmail", fmt.Sprintf("%v", params))
	return true
}

func (ea *EmailApp) sendHttp(params []model.Param) error {
	body := model.HttpBody{Params: params}
	jsonData, err := json.Marshal(body)
	ea.sendMessage("sendHttp: body ", string(jsonData))
	if err != nil {
		ea.sendMessage("sendHttp", err.Error())
		return err
	}
	req, err := http.NewRequest("POST", ea.config.CallbackUrl, bytes.NewReader(jsonData))
	if err != nil {
		ea.sendMessage("sendHttp", err.Error())
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ea.sendMessage("sendHttp", err.Error())
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		ea.sendMessage("sendHttp", "http status err")
		return errors.New("http status err")
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	ea.sendMessage("sendHttp: response ", string(respBody))
	return nil
}

func isMailBoxUpdate(update client.Update) *client.MailboxUpdate {
	if val, ok := update.(*client.MailboxUpdate); ok {
		return val
	}
	return nil
}

func (ea *EmailApp) waitMailUpdate() (*client.MailboxUpdate, error) {
	done := make(chan error, 1)
	stop := make(chan struct{})
	go func() {
		done <- ea.idleClient.IdleWithFallback(stop, 5*time.Minute)
	}()
	var mailBoxUpdate *client.MailboxUpdate
forLoop:
	for {
		select {
		case update := <-ea.updateChan:
			if mailBoxUpdate = isMailBoxUpdate(update); mailBoxUpdate != nil {
				ea.sendMessage("waitMailUpdate", "update received")
				break forLoop
			}
		case err := <-done:
			ea.sendMessage("waitMailUpdate: not idling any more ", err.Error())
			return nil, errors.New("idle is done")
		case <-ea.stopChan:
			ea.sendMessage("waitMailUpdate:", "stop signal")
			return nil, nil
		}
	}
	close(stop)
	<-done
	return mailBoxUpdate, nil
}

func (ea *EmailApp) getEmails(mailbox *imap.MailboxStatus) {
	ea.sendMessage("getEmails", mailbox.Name)
	ea.getLatestMessages(uint32(5))
}

func (ea *EmailApp) Start() error {
	if err := ea.login(); err != nil {
		ea.sendMessage("Start", "quit for login failed")
		return nil
	}
	defer ea.client.Logout()
	for {
		ea.sendMessage("start", "idling now")
		update, err := ea.waitMailUpdate()
		if err != nil {
			if ea.client.Check() != nil {
				ea.login()
			}
			continue
		}
		if update == nil {
			ea.sendMessage("start", "stop idle")
			break
		}
		ea.getEmails(update.Mailbox)
	}
	return nil
}

func (ea *EmailApp) Stop() {
	ea.stopChan <- ""
}
