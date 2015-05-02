package irc

import (
	"fmt"
	"github.com/Didact/my"
	"io"
	"strconv"
	"strings"
	"time"
)

//go:generate stringer -type=MsgType
type MsgType int

//Overwrites the first few RPLs. Sue me.
const (
	PRIVMSG MsgType = iota + 1
	NOTICE
	NICK
	JOIN
	PART
	QUIT
	TOPIC
	KICK
	MODE
	PING
	PONG
	RPL_NOTOPIC  MsgType = 331
	RPL_TOPIC    MsgType = 332
	RPL_NAMREPLY MsgType = 353
)

var Types = map[string]MsgType{
	"PRIVMSG": PRIVMSG,
	"PONG":    PONG,
	"NOTICE":  NOTICE,
	"NICK":    NICK,
	"JOIN":    JOIN,
	"PART":    PART,
	"QUIT":    QUIT,
	"TOPIC":   TOPIC,
	"KICK":    KICK,
	"MODE":    MODE,
	"332":     RPL_NOTOPIC,
	"333":     RPL_TOPIC,
	"353":     RPL_NAMREPLY,
}

type Target interface {
	io.Writer
	Say(string)
	Ident() string //because Name is already used
	fmt.Stringer
	Old() bool
}

type Message struct {
	Server      *Server
	Time        time.Time
	Text        string
	Type        MsgType
	Source      Target
	Destination Target
	Raw         string
	Extra       []byte
}

//FORAMAT: <SOURCE> <COMMAND> <DESTINATION> <TEXT>
//SERVER NUM NICK DEST TEXT
func (s *Server) NewMessage(str string) *Message {
	this := new(Message)
	this.Server = s
	this.Time = time.Now()
	this.Raw = str
	this.Extra = []byte{0}
	fields := strings.SplitN(str, " ", 5)
	for i, str2 := range fields {
		fields[i] = strings.TrimLeft(str2, ":")
	}
	if fields[0] == "PING" {
		this.Type = PING
		this.Text = str[5:]
		this.Source = s
		this.Destination = s.Me
		return this
	}
	if len(fields) < 3 {
		return nil
	}
	this.Type = Types[fields[1]]

	var src Target
	user := my.SplitAll(fields[0], "!@")
	switch {
	case len(user) == 1: //Comes from the server
		src = s
	default: //Some form of user
		if user[0] == s.Prof.nname { //Message came from us
			src = s.Me
			break
		}
		u, ok := s.Users[user[0]]
		if !ok || u.old {
			u = s.NewUser(user[0], user[1])
		}
		src = u
	}

	if _, err := strconv.Atoi(fields[1]); err == nil { //Temp hack
		fields = strings.Fields(str)
		switch this.Type {
		case RPL_NAMREPLY:
			this.Source = s
			this.Destination = s.Channels[fields[4]]
			for i, s2 := range fields[5:] {
				fields[i+5] = strings.TrimLeft(s2, ":+~@+&")
			}
			this.Text = strings.Join(fields[5:], " ")
			return this
		}
		this.Text = strings.Join(fields[2:], " ")
		this.Source = s
		this.Destination = s.Me
		return this
	}

	var dst Target
	user = my.SplitAll(fields[2], "!@")
	switch {
	case strings.HasPrefix(fields[2], "#"): //To a channel
		c, ok := s.Channels[fields[2]]
		if !ok || c.old {
			c = s.NewChannel(fields[2])
		}
		dst = c
	case user[0] == s.Prof.nname: //To you
		dst = s.Me
	default:
		dst = s.Me
	}

	this.Source = src
	this.Destination = dst

	switch this.Type {
	case NICK:
		this.Text = fields[2]
	case JOIN:
	case MODE, KICK, NOTICE, TOPIC, PRIVMSG:
		this.Text = strings.Join(fields[3:], " ")
	case QUIT:
		if len(fields) > 2 { //Reason for quit given
			this.Text = fields[2]
		}
	case PART:
		if len(fields) > 3 {
			this.Text = fields[3]
		}
	}

	return this
}

/*
func ReadMessages(s *Server) <-chan *Message {
	msgs := make(chan *Message)
	go func() {
		c := s.conn
		for c.Err() == nil {
			<-c.Ready
			strs := strings.Split(string(c.Read()), "\r\n")
			strs = strs[:len(strs)-1]
			for _, str := range strs {
				m := s.NewMessage(str)
				if m != nil {
					msgs <- m
				}
			}
		}
	}()
	return msgs
}
*/

func (m *Message) String() string {
	return fmt.Sprintf("[%4d-%02d-%02d %02d:%02d:%02d]%s/%s %s->%s:%s",
		m.Time.Year(),
		m.Time.Month(),
		m.Time.Day(),
		m.Time.Hour(),
		m.Time.Minute(),
		m.Time.Second(),
		m.Server.Ident(),
		m.Source.Ident(),
		m.Type,
		m.Destination.Ident(),
		m.Text)
}

/*func (m MsgType) String() string {
	var s string
	switch m {
	case JOIN:
		s = "JOIN"
	case KICK:
		s = "KICK"
	case MODE:
		s = "MODE"
	case NOTICE:
		s = "NOTICE"
	case PART:
		s = "PART"
	case PING:
		s = "PING"
	case PRIVMSG:
		s = "PRIVMSG"
	case QUIT:
		s = "QUIT"
	case TOPIC:
		s = "TOPIC"
	}
	return s
}*/

type Stub struct {
	name string
}

func (s *Stub) Ident() string {
	return s.name
}
func (s *Stub) String() string {
	return s.name
}
func (s *Stub) Write(p []byte) (int, error) {
	return 0, nil
}
func (s *Stub) Say(str string) {}
