package irc

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type HandlerFunc func(*Message)

//go:generate goast write impl Queue.go
type MChannel chan *Message

type Server struct {
	Name     string
	Users    map[string]*User
	Channels map[string]*Channel
	Msgs     MChannel
	inMsgs   MChannel //Used internally
	ip       string
	conn     net.Conn
	Prof     *Profile
	Me       *User
	handlers map[string]HandlerFunc
	m1, m2   sync.Mutex
	count    int
	pause    chan bool
	c        <-chan time.Time
}

func NewServer(name, ip string, prof *Profile) *Server {
	s := new(Server)
	s.Name = name
	s.Users = make(map[string]*User)
	s.Channels = make(map[string]*Channel)
	s.handlers = make(map[string]HandlerFunc)
	s.inMsgs = make(chan *Message)
	s.Msgs = s.inMsgs.Queue()
	s.ip = ip
	s.conn = nil //Explicit
	s.Prof = prof
	s.Me = s.NewUser("~me", "~me")
	s.pause = make(chan bool, 1)
	s.c = time.After(100 * time.Millisecond)
	return s
}

func (s *Server) Start() {
	var err error
	s.conn, err = net.Dial("tcp", s.ip)
	if err != nil {
		log.Fatal(err)
	}
	/*
		go func() {
			for {
				if s.count == 50 {
					s.m1.Lock()
					s.count = 0
					s.m1.Unlock()
					s.pause <- true
				}
				<-time.Tick(25 * time.Millisecond)
			}
		}()
	*/
	go func() {
		<-time.After(1 * time.Second)
		s.Nick(s.Prof.nname)
		s.User(s.Prof.uname, s.Prof.rname)
		<-time.After(2 * time.Second)
		s.Join(s.Prof.autos...)
	}()
	go func() {
		var i int
		for {
			<-time.After(10 * time.Second)
			s.conn.Write([]byte(fmt.Sprintf("PING %d\r\n", i)))
			i++
			if i == 2000000000 {
				i = 0
			}
		}
	}()
	s.AddHandler("pings", func(m *Message) {
		if m.Type == PING {
			s.Say("PONG " + m.Text)
		}
	})
	s.AddHandler("chans", func(m *Message) {
		if c, ok := m.Destination.(*Channel); ok {
			if c == nil {
				fmt.Println("***NIL CHANNEL")
				return
			}
			c.inMsgs <- m
		}
		if stub, ok := m.Destination.(*Stub); ok {
			s.Channels[stub.Ident()].inMsgs <- m
		}
	})
	s.AddHandler("server", func(m *Message) {
		if m.Source == s {
			s.inMsgs <- m
		}
	})
	s.AddHandler("users", func(m *Message) {
		u, ok := m.Source.(*User)
		if ok && m.Destination == s.Me { //Iff the message is going to us
			u.inMsgs <- m
		}
	})
	s.AddHandler("nicks", func(m *Message) {
		if m.Type == NICK {
			u := m.Source.(*User)
			u.Name = m.Text
		}
	})
	s.AddHandler("quits", func(m *Message) {
		if m.Type == QUIT {
			for _, c := range s.Channels {
				t := new(Message)
				*t = *m
				t.Type = PART
				t.Destination = c
				c.inMsgs <- t
			}
			delete(s.Users, m.Source.Ident())
		}
	})
	buf := make([]byte, 512)
	go func() {
		for {
			if s.conn == nil {
				return
			}
			n, err := s.conn.Read(buf)
			if err == io.EOF {
				defer Restart(&s)
				return
			}
			if err != nil {
				log.Fatal(err)
			}
			strs := strings.Split(string(buf[:n]), "\r\n")
			strs = strs[:len(strs)-1]
			for _, str := range strs {
				m := s.NewMessage(str)
				if m != nil {
					for _, f := range s.handlers {
						f(m)
					}
				}
			}
		}
	}()
}

func (s *Server) Nick(str string) {
	s.Say("NICK " + str)
}

func (s *Server) User(uname, rname string) {
	s.Say(fmt.Sprintf("USER %s 0 * :%s", uname, rname))
}

func (s *Server) Join(chans ...string) []*Channel {
	wg := &sync.WaitGroup{}
	if len(chans) < 1 {
		return nil
	}
	channels := make([]*Channel, len(chans))
	for _, str := range chans {
		s.Say("JOIN " + str)
		c := s.NewChannel(str)
		c.AddHandler("wait", func(m *Message) {
			if m.Type == RPL_NAMREPLY {
				wg.Done()
				c.RemHandler("wait")
			}
		})
		channels = append(channels, c)
		wg.Add(1)
	}
	wg.Wait()
	return channels
}

func (s *Server) Write(p []byte) (int, error) {
	if len(p) > 510 {
		p = p[:511]
	}
	<-s.c
	//fmt.Println(">>" + string(p))
	s.c = time.After(100 * time.Millisecond)
	return s.conn.Write(append(p, "\r\n"...))
}

func (s *Server) Say(str string) {
	s.Write([]byte(str))
}

func (s *Server) AddHandler(name string, f HandlerFunc) string {
	if name == "" {
		name = uuid.New()
	}
	s.handlers[name] = f
	return name
}

func (s *Server) RemHandler(name string) {
	delete(s.handlers, name)
}

func (s *Server) PrivMsg(t Target, str string) {
	s.Say(fmt.Sprintf("PRIVMSG %s :%s", t.Ident(), str))
}

func (s *Server) Notice(t Target, str string) {
	s.Say(fmt.Sprintf("NOTICE %s :%s", t.Ident(), str))
}

func (s *Server) Topic(c *Channel, str string) {
	s.Say(fmt.Sprintf("TOPIC %s :%s", c.Name, str))
}

func (s *Server) String() string {
	if s == nil {
		return ""
	}
	return s.Name
}

func (s *Server) Ident() string {
	if s == nil {
		return ""
	}
	return s.Name
}

func Restart(s **Server) {
	(*s).conn.Close()
	h := (*s).handlers
	*s = NewServer((*s).Name, (*s).ip, (*s).Prof)
	(*s).handlers = h
	(*s).Start()
}
