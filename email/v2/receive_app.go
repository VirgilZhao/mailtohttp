package v2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/VirgilZhao/mailtohttp/model"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-message/mail"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
)

type ReceiveApp struct {
	App
	stopChan chan string
}

func NewReceiveApp(config *model.ServiceConfig, msgChan chan string) *ReceiveApp {
	return &ReceiveApp{
		App: App{
			Name:    "ReceiveApp",
			config:  config,
			msgChan: msgChan,
		},
		stopChan: make(chan string),
	}
}

func (ea *ReceiveApp) Start(updateMsgChan chan string) {
	for {
		ea.sendMessage("Start", "wait new email")
		select {
		case <-updateMsgChan:
			ea.getLatestMessages(5)
			break
		case <-ea.stopChan:
			ea.sendMessage("Start", "stop by signal")
			return
		}
	}
}

func (ea *ReceiveApp) Stop() {
	ea.stopChan <- ""
}

func (ea *ReceiveApp) getLatestMessages(count uint32) error {
	if err := ea.login(); err != nil {
		return err
	}
	defer ea.client.Logout()
	mbox, err := ea.client.Select(ea.config.EmailSettings.Folder, false)
	if err != nil {
		ea.sendMessage("GetLatestMessages", err.Error())
		return err
	}
	from := uint32(1)
	to := mbox.Messages
	if mbox.Messages > count {
		from = mbox.Messages - count
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)
	// Get the whole message body
	section := &imap.BodySectionName{}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- ea.client.Fetch(seqset, []imap.FetchItem{section.FetchItem()}, messages)
	}()
	for msg := range messages {
		r := msg.GetBody(section)
		if r == nil {
			ea.sendMessage("GetLatestMessages", "Server didn't returned message body")
			continue
		}
		// Create a new mail reader
		mr, err := mail.CreateReader(r)
		if err != nil {
			ea.sendMessage("GetLatestMessages", err.Error())
			continue
		}
		// Process each message's part
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				ea.sendMessage("GetLatestMessages", err.Error())
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
				ea.sendMessage("GetLatestMessages", fmt.Sprintf("Got attachment: %v", filename))
			default:
				break
			}
			if success {
				break
			}
		}
	}
	if err := <-done; err != nil {
		ea.sendMessage("GetLatestMessages", err.Error())
	}
	ea.sendMessage("GetLatestMessages", "done")
	return nil
}

func (ea *ReceiveApp) decodeEmail(message string) bool {
	params := make([]model.Param, 0)
	for _, content := range ea.config.ContentPatterns {
		valReg, err := regexp.Compile(content.Regex)
		if err != nil {
			ea.sendMessage("GetLatestMessages", err.Error())
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
	ea.sendMessage("decodeEmail", fmt.Sprintf("%v", params))
	ea.sendHttp(params)
	return true
}

func (ea *ReceiveApp) sendHttp(params []model.Param) error {
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
