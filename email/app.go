package email

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/VirgilZhao/mailtohttp/model"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap-idle"
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
	name             string
	imapAddress      string
	imapPort         int
	email            string
	password         string
	folder           string
	client           *client.Client
	UpdateChan       chan string
	stopChan         chan string
	httpStopChan     chan string
	inbox            *imap.MailboxStatus
	config           *model.ServiceConfig
	msgChan          chan string
	httpDataChan     chan model.HttpSender
	retryDataMap     map[int]model.HttpSender
	ticker           *time.Ticker
	currentTickerPos int
}

func NewEmailApp(name string, config *model.ServiceConfig, msgChan chan string) *EmailApp {
	return &EmailApp{
		name:             name,
		imapAddress:      config.EmailSettings.ImapAddress,
		imapPort:         config.EmailSettings.ImapPort,
		email:            config.EmailSettings.Email,
		password:         config.EmailSettings.Password,
		folder:           config.EmailSettings.Folder,
		UpdateChan:       make(chan string, 10),
		stopChan:         make(chan string, 1),
		httpStopChan:     make(chan string, 1),
		config:           config,
		msgChan:          msgChan,
		httpDataChan:     make(chan model.HttpSender, 100),
		retryDataMap:     make(map[int]model.HttpSender),
		currentTickerPos: 0,
	}
}

func (ea *EmailApp) sendMessage(text string) {
	log.Println(ea.name + ":" + text)
	data := model.SocketMessage{
		MsgType: "message",
		Data:    ea.name + ":" + text,
	}
	bytes, err := json.Marshal(&data)
	if err != nil {
		return
	}
	ea.msgChan <- string(bytes)
}

func (ea *EmailApp) login() error {
	ea.sendMessage("Connecting to server ...")
	c, err := client.DialTLS(fmt.Sprintf("%s:%v", ea.imapAddress, ea.imapPort), nil)
	if err != nil {
		ea.sendMessage(err.Error())
		return err
	}
	ea.client = c
	ea.sendMessage("Connected")

	if err := c.Login(ea.email, ea.password); err != nil {
		ea.sendMessage(err.Error())
		return err
	}
	ea.sendMessage("Logged in")

	mbox, err := ea.client.Select(ea.folder, false)
	if err != nil {
		ea.sendMessage(err.Error())
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
	for {
		ea.sendMessage("listen updates")
		select {
		case update := <-updates:
			ea.sendMessage("New update arrived")
			switch update.(type) {
			case *client.MailboxUpdate:
				updateChan <- "update"
				break
			}
			break
		case err := <-done:
			if err != nil {
				ea.sendMessage(err.Error())
				return
			}
			ea.sendMessage("Not idling anymore")
			return
		case <-ea.stopChan:
			ea.client.Logout()
			ea.sendMessage("loop idle quit")
			return
		}
	}
}

func (ea *EmailApp) StartEmailReceive() {
	if err := ea.login(); err != nil {
		return
	}
	for {
		ea.sendMessage("listen messages")
		select {
		case <-ea.UpdateChan:
			ea.getLatestMessages()
		case <-ea.stopChan:
			ea.client.Logout()
			ea.sendMessage("loop receive quit")
			return
		}
	}
}

func (ea *EmailApp) StopLoop() {
	ea.stopChan <- ""
	ea.httpStopChan <- ""
}

func (ea *EmailApp) getLatestMessages() {
	if ea.client.Check() != nil {
		if err := ea.login(); err != nil {
			return
		}
	}
	from := uint32(1)
	to := ea.inbox.Messages
	if ea.inbox.Messages > 10 {
		from = ea.inbox.Messages - 10
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	// Get the whole message body
	section := &imap.BodySectionName{}

	messages := make(chan *imap.Message, 10)
	err := ea.client.Fetch(seqset, []imap.FetchItem{section.FetchItem()}, messages)
	if err != nil {
		ea.sendMessage(err.Error())
	}
	for msg := range messages {
		r := msg.GetBody(section)
		if r == nil {
			ea.sendMessage("Server didn't returned message body")
			continue
		}

		// Create a new mail reader
		mr, err := mail.CreateReader(r)
		if err != nil {
			ea.sendMessage(err.Error())
			continue
		}
		// Process each message's part
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				ea.sendMessage(err.Error())
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
				ea.sendMessage(fmt.Sprintf("Got attachment: %v", filename))
			default:
				break
			}
			if success {
				break
			}
		}
	}
	ea.sendMessage("done")
}

func (ea *EmailApp) decodeEmail(message string) bool {
	params := make([]model.Param, 0)
	for _, content := range ea.config.ContentPatterns {
		valReg, err := regexp.Compile(content.Regex)
		if err != nil {
			ea.sendMessage(err.Error())
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
	ea.sendMessage(fmt.Sprintf("%v", params))
	ea.httpDataChan <- model.HttpSender{
		Params:    params,
		Timestamp: time.Now().Unix(),
		NextRun:   0,
	}
	return true
}

func (ea *EmailApp) StartHttpLoop() {
	for {
		ea.sendMessage("http listening")
		select {
		case sender := <-ea.httpDataChan:
			err := ea.sendHttp(sender.Params)
			if err != nil {
				ea.sendMessage(err.Error())
			}
			break
		case <-ea.httpStopChan:
			ea.sendMessage("http loop stop")
			return
		}
	}
}

func (ea *EmailApp) sendHttp(params []model.Param) error {
	body := model.HttpBody{Params: params}
	jsonData, err := json.Marshal(body)
	if err != nil {
		ea.sendMessage(err.Error())
		return err
	}
	req, err := http.NewRequest("POST", ea.config.CallbackUrl, bytes.NewReader(jsonData))
	if err != nil {
		ea.sendMessage(err.Error())
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ea.sendMessage(err.Error())
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		ea.sendMessage("http status err")
		return errors.New("http status err")
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	ea.sendMessage(string(respBody))
	return nil
}
