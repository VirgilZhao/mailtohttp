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
