// Package svc — служба SafeBridge: ядро с правами администратора в отдельном процессе
// (на macOS — LaunchDaemon), окно приложения работает от имени пользователя и говорит
// со службой через unix-сокет. По сокету идут те же сообщения, что помощник получал от
// расширения ({id, cmd, args} и ответы), по одному JSON в строке. Служебные сообщения
// отличаются полем "svc".
package svc

import (
	"bufio"
	"encoding/json"
	"errors"
	"log"
	"net"
	"sync"
	"time"
)

// Core — ядро, которое служба обслуживает (host.Host).
type Core interface {
	HandleJSON([]byte)
	Shutdown()
}

type svcMsg struct {
	Svc     string `json:"svc"`
	Version string `json:"version,omitempty"`
}

const maxLine = 16 << 20

// ---------- служба ----------

// Server держит одно ядро и одного клиента: новое окно приложения сменяет прежнее.
// Когда клиент отключился (приложение закрыли или оно упало), ядро останавливается —
// TUN-адаптер и маршруты убираются, как при выходе из приложения.
type Server struct {
	Version string
	Allow   func(net.Conn) bool // проверка, кто подключился (пользователь, установивший службу)
	Core    Core

	mu  sync.Mutex
	cur net.Conn
}

// Emit отправляет сообщение ядра текущему клиенту.
func (s *Server) Emit(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return
	}
	s.cur.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := s.cur.Write(append(append([]byte{}, b...), '\n')); err != nil {
		s.cur.Close()
	}
}

func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		if s.Allow != nil && !s.Allow(c) {
			log.Println("svc: отклонено подключение чужого пользователя")
			c.Close()
			continue
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	s.mu.Lock()
	old := s.cur
	s.cur = c
	s.mu.Unlock()
	if old != nil {
		old.Close()
		s.Core.Shutdown() // новое окно начинает с чистого ядра
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		line := sc.Bytes()
		var m svcMsg
		if json.Unmarshal(line, &m) == nil && m.Svc != "" {
			if m.Svc == "hello" {
				b, _ := json.Marshal(svcMsg{Svc: "hello", Version: s.Version})
				s.mu.Lock()
				if s.cur == c {
					c.SetWriteDeadline(time.Now().Add(10 * time.Second))
					c.Write(append(b, '\n'))
				}
				s.mu.Unlock()
			}
			continue
		}
		s.Core.HandleJSON(append([]byte{}, line...))
	}
	s.mu.Lock()
	mine := s.cur == c
	if mine {
		s.cur = nil
	}
	s.mu.Unlock()
	c.Close()
	if mine {
		s.Core.Shutdown()
	}
}

// ---------- клиент (окно приложения) ----------

// ErrNoService — службы нет или она не отвечает.
var ErrNoService = errors.New("служба SafeBridge не запущена")

// Client — ядро для окна приложения, которое на самом деле работает в службе.
type Client struct {
	Path string
	// Emit — событие в интерфейс: "native" (сообщение ядра) или "native-down" (связь со службой пропала)
	Emit func(name, data string)

	mu      sync.Mutex
	conn    net.Conn
	version string
}

// Hello подключается к службе (если ещё нет) и возвращает её версию.
func (c *Client) Hello() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.version, nil // уже подключены: не рвём связь, иначе служба остановит ядро
	}
	return c.dialLocked()
}

func (c *Client) dialLocked() (string, error) {
	conn, err := net.DialTimeout("unix", c.Path, 3*time.Second)
	if err != nil {
		return "", ErrNoService
	}
	b, _ := json.Marshal(svcMsg{Svc: "hello"})
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(append(b, '\n')); err != nil {
		conn.Close()
		return "", ErrNoService
	}
	r := bufio.NewReaderSize(conn, 64<<10)
	line, err := r.ReadBytes('\n')
	var m svcMsg
	if err != nil || json.Unmarshal(line, &m) != nil || m.Svc != "hello" {
		conn.Close()
		return "", ErrNoService
	}
	conn.SetDeadline(time.Time{})
	c.conn = conn
	c.version = m.Version
	go c.read(conn, r)
	return m.Version, nil
}

func (c *Client) read(conn net.Conn, r *bufio.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		line := sc.Bytes()
		var m svcMsg
		if json.Unmarshal(line, &m) == nil && m.Svc != "" {
			continue
		}
		c.Emit("native", string(line))
	}
	c.mu.Lock()
	mine := c.conn == conn
	if mine {
		c.conn = nil
	}
	c.mu.Unlock()
	conn.Close()
	if mine {
		c.Emit("native-down", "Служба SafeBridge отключилась")
	}
}

// HandleJSON передаёт команду ядру в службе; если службы нет — интерфейс узнает об этом
// так же, как расширение узнаёт, что помощник завершил работу.
func (c *Client) HandleJSON(b []byte) {
	c.mu.Lock()
	if c.conn == nil {
		if _, err := c.dialLocked(); err != nil {
			c.mu.Unlock()
			c.Emit("native-down", err.Error())
			return
		}
	}
	conn := c.conn
	c.mu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(append(append([]byte{}, b...), '\n')); err != nil {
		conn.Close() // read() увидит обрыв и сообщит интерфейсу
	}
}

// Shutdown отключается от службы — служба сама остановит ядро.
func (c *Client) Shutdown() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
}
