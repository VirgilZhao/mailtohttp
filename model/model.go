package model

type EmailSettings struct {
	ImapAddress string `json:"imapAddress"`
	ImapPort    int    `json:"imapPort"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Folder      string `json:"folder"`
}

type ServiceContentPattern struct {
	Param string `json:"param"`
	Regex string `json:"regex"`
	Require bool `json:"require"`
}

type ServiceConfig struct {
	EmailSettings   EmailSettings           `json:"emailSettings"`
	ContentPatterns []ServiceContentPattern `json:"contentPatterns"`
	CallbackUrl     string                  `json:"callbackUrl"`
}

type LoginResponse struct {
	Login  string        `json:"login"`
	Config ServiceConfig `json:"config"`
}

type SocketMessage struct {
	MsgType string `json:"msg_type"`
	Data    string `json:"data"`
}

type Param struct {
	Name  string   `json:"name"`
	Value []string `json:"value"`
}

type HttpSender struct {
	Params    []Param `json:"params"`
	Timestamp int64   `json:"timestamp"`
	NextRun   int64   `json:"next_run"`
}

type HttpBody struct {
	Params []Param `json:"params"`
}

type EmailPwdBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
